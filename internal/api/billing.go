package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/billing"
	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/store"
	"github.com/gin-gonic/gin"
)

// isPremium resolves a guild's premium entitlement: the PREMIUM_GUILD_IDS
// allowlist (dev/manual override) OR an active Stripe subscription.
func (s *Server) isPremium(ctx context.Context, guildID string) bool {
	if s.cfg.IsPremiumGuild(guildID) {
		return true
	}
	gid, ok := event.ParseID(guildID)
	if !ok {
		return false
	}
	sub, found, err := s.store.Subscriptions.Get(ctx, gid)
	if err != nil || !found {
		return false
	}
	return sub.Active(time.Now())
}

// handleBillingStatus reports the guild's plan + subscription state.
func (s *Server) handleBillingStatus(c *gin.Context) {
	gid := guildID(c)
	resp := gin.H{
		"premium":         s.isPremium(c.Request.Context(), gid),
		"price":           "$3.99/mo",
		"billing_enabled": s.billing != nil,
	}
	if gidInt, ok := event.ParseID(gid); ok {
		if sub, found, _ := s.store.Subscriptions.Get(c.Request.Context(), gidInt); found {
			resp["status"] = sub.Status
			resp["manage"] = sub.CustomerID != ""
			if sub.CurrentPeriodEnd != nil {
				resp["current_period_end"] = sub.CurrentPeriodEnd.Unix()
			}
		}
	}
	c.JSON(http.StatusOK, resp)
}

// handleCheckout starts a Stripe Checkout Session for the premium plan.
func (s *Server) handleCheckout(c *gin.Context) {
	if s.billing == nil {
		fail(c, http.StatusServiceUnavailable, "billing is not configured")
		return
	}
	gid := guildID(c)
	base := strings.TrimRight(s.cfg.API.WebBaseURL, "/") + "/servers/" + gid + "/billing"
	url, err := s.billing.CreateCheckoutSession(c.Request.Context(), billing.CheckoutParams{
		PriceID:    s.cfg.Billing.PriceID,
		GuildID:    gid,
		SuccessURL: base + "?checkout=success",
		CancelURL:  base + "?checkout=cancel",
	})
	if err != nil {
		s.log.Error("stripe checkout failed", "guild", gid, "err", err)
		fail(c, http.StatusBadGateway, "could not start checkout")
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": url})
}

// handlePortal opens the Stripe Billing Portal so the admin can manage/cancel.
func (s *Server) handlePortal(c *gin.Context) {
	if s.billing == nil {
		fail(c, http.StatusServiceUnavailable, "billing is not configured")
		return
	}
	gid := guildID(c)
	gidInt, _ := event.ParseID(gid)
	sub, found, _ := s.store.Subscriptions.Get(c.Request.Context(), gidInt)
	if !found || sub.CustomerID == "" {
		fail(c, http.StatusBadRequest, "no subscription to manage")
		return
	}
	ret := strings.TrimRight(s.cfg.API.WebBaseURL, "/") + "/servers/" + gid + "/billing"
	url, err := s.billing.CreatePortalSession(c.Request.Context(), sub.CustomerID, ret)
	if err != nil {
		s.log.Error("stripe portal failed", "guild", gid, "err", err)
		fail(c, http.StatusBadGateway, "could not open billing portal")
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": url})
}

// handleStripeWebhook ingests Stripe events (signature-verified) and updates the
// guild's subscription. On cancellation the status flips to canceled, which makes
// isPremium false — existing uploads are kept (never deleted on downgrade); the
// guild just can't add new ones over the free quota.
func (s *Server) handleStripeWebhook(c *gin.Context) {
	if s.billing == nil || s.cfg.Billing.WebhookSecret == "" {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	payload, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	if err := billing.VerifyWebhook(payload, c.GetHeader("Stripe-Signature"), s.cfg.Billing.WebhookSecret, time.Now()); err != nil {
		s.log.Warn("stripe webhook rejected", "err", err)
		c.Status(http.StatusBadRequest)
		return
	}
	var ev billing.Event
	if err := json.Unmarshal(payload, &ev); err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	ctx := c.Request.Context()
	switch ev.Type {
	case "checkout.session.completed":
		// Only record the customer/subscription link here — DON'T grant premium.
		// Checkout can complete before payment confirms, so entitlement is driven
		// solely by the authoritative customer.subscription.* events below.
		sub, _ := billing.ParseSubscription(ev.Data.Object)
		if gid, ok := event.ParseID(sub.GuildID); ok && sub.Customer != "" {
			_ = s.store.Subscriptions.LinkStripe(ctx, gid, sub.Customer, sub.ID)
		}
	case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted":
		sub, _ := billing.ParseSubscription(ev.Data.Object)
		status := sub.Status
		if ev.Type == "customer.subscription.deleted" {
			status = "canceled"
		}
		var periodEnd *time.Time
		if sub.CurrentPeriodEnd > 0 {
			t := time.Unix(sub.CurrentPeriodEnd, 0)
			periodEnd = &t
		}
		if gid, ok := event.ParseID(sub.GuildID); ok {
			_ = s.store.Subscriptions.Upsert(ctx, store.GuildSubscription{
				GuildID: gid, CustomerID: sub.Customer, SubscriptionID: sub.ID,
				Status: status, CurrentPeriodEnd: periodEnd,
			})
		} else {
			_ = s.store.Subscriptions.SetStatus(ctx, sub.ID, status, periodEnd)
		}
	}
	c.Status(http.StatusOK)
}
