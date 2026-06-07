package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GuildSubscription is a guild's Stripe subscription state.
type GuildSubscription struct {
	GuildID          int64
	CustomerID       string
	SubscriptionID   string
	Status           string
	CurrentPeriodEnd *time.Time
	UpdatedAt        time.Time
}

// Active reports whether the subscription currently entitles premium: an active
// or trialing status whose period hasn't lapsed.
func (s GuildSubscription) Active(now time.Time) bool {
	if s.Status != "active" && s.Status != "trialing" {
		return false
	}
	if s.CurrentPeriodEnd != nil && now.After(*s.CurrentPeriodEnd) {
		return false
	}
	return true
}

// SubscriptionRepo manages guild_subscriptions.
type SubscriptionRepo struct{ pool *pgxpool.Pool }

// Get returns a guild's subscription, found=false when there is none.
func (r *SubscriptionRepo) Get(ctx context.Context, guildID int64) (GuildSubscription, bool, error) {
	s := GuildSubscription{GuildID: guildID}
	err := r.pool.QueryRow(ctx, `
		SELECT stripe_customer_id, stripe_subscription_id, status, current_period_end, updated_at
		FROM guild_subscriptions WHERE guild_id = $1`, guildID).
		Scan(&s.CustomerID, &s.SubscriptionID, &s.Status, &s.CurrentPeriodEnd, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return GuildSubscription{}, false, nil
	}
	if err != nil {
		return GuildSubscription{}, false, err
	}
	return s, true, nil
}

// GuildBySubscription resolves the guild owning a Stripe subscription id.
func (r *SubscriptionRepo) GuildBySubscription(ctx context.Context, subID string) (int64, bool, error) {
	var gid int64
	err := r.pool.QueryRow(ctx,
		`SELECT guild_id FROM guild_subscriptions WHERE stripe_subscription_id = $1`, subID).Scan(&gid)
	if err == pgx.ErrNoRows {
		return 0, false, nil
	}
	return gid, err == nil, err
}

// Upsert writes a guild's subscription state (keyed by guild_id). current_period_end
// is COALESCEd so an event that lacks it (or out-of-order delivery) can't wipe a
// known period.
func (r *SubscriptionRepo) Upsert(ctx context.Context, s GuildSubscription) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO guild_subscriptions
			(guild_id, stripe_customer_id, stripe_subscription_id, status, current_period_end, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (guild_id) DO UPDATE SET
			stripe_customer_id     = EXCLUDED.stripe_customer_id,
			stripe_subscription_id = EXCLUDED.stripe_subscription_id,
			status                 = EXCLUDED.status,
			current_period_end     = COALESCE(EXCLUDED.current_period_end, guild_subscriptions.current_period_end),
			updated_at             = now()`,
		s.GuildID, s.CustomerID, s.SubscriptionID, s.Status, s.CurrentPeriodEnd)
	if err != nil {
		return fmt.Errorf("upsert subscription: %w", err)
	}
	return nil
}

// LinkStripe records the customer/subscription ids for a guild WITHOUT changing
// entitlement (status/period). Used on checkout.session.completed; the real
// status arrives via customer.subscription.* events. A brand-new row starts
// 'incomplete' (not premium).
func (r *SubscriptionRepo) LinkStripe(ctx context.Context, guildID int64, customerID, subscriptionID string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO guild_subscriptions (guild_id, stripe_customer_id, stripe_subscription_id, status)
		VALUES ($1, $2, $3, 'incomplete')
		ON CONFLICT (guild_id) DO UPDATE SET
			stripe_customer_id     = EXCLUDED.stripe_customer_id,
			stripe_subscription_id = EXCLUDED.stripe_subscription_id,
			updated_at             = now()`,
		guildID, customerID, subscriptionID)
	if err != nil {
		return fmt.Errorf("link stripe: %w", err)
	}
	return nil
}

// SetStatus updates just the status/period for a subscription id (webhook path).
func (r *SubscriptionRepo) SetStatus(ctx context.Context, subID, status string, periodEnd *time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE guild_subscriptions
		SET status = $2, current_period_end = $3, updated_at = now()
		WHERE stripe_subscription_id = $1`, subID, status, periodEnd)
	return err
}
