package billing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
)

func sign(ts string, payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write(payload)
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyWebhook(t *testing.T) {
	secret := "whsec_test_secret"
	payload := []byte(`{"type":"checkout.session.completed"}`)
	now := time.Unix(1_700_000_000, 0)
	header := sign("1700000000", payload, secret)

	if err := VerifyWebhook(payload, header, secret, now); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if err := VerifyWebhook([]byte(`{"type":"evil"}`), header, secret, now); err == nil {
		t.Fatal("tampered payload accepted")
	}
	if err := VerifyWebhook(payload, header, "wrong", now); err == nil {
		t.Fatal("wrong secret accepted")
	}
	if err := VerifyWebhook(payload, header, secret, now.Add(10*time.Minute)); err == nil {
		t.Fatal("stale timestamp accepted")
	}
	if err := VerifyWebhook(payload, "garbage", secret, now); err == nil {
		t.Fatal("malformed header accepted")
	}
}

func TestParseSubscription(t *testing.T) {
	// checkout.session: subscription id + client_reference_id
	cs := []byte(`{"id":"cs_1","customer":"cus_1","subscription":"sub_1","client_reference_id":"123","metadata":{}}`)
	s, err := ParseSubscription(cs)
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "sub_1" || s.Customer != "cus_1" || s.GuildID != "123" {
		t.Fatalf("checkout parse: %+v", s)
	}
	// subscription object: id + status + metadata.guild_id + period end
	sub := []byte(`{"id":"sub_2","customer":"cus_2","status":"active","current_period_end":1700000000,"metadata":{"guild_id":"456"}}`)
	s2, err := ParseSubscription(sub)
	if err != nil {
		t.Fatal(err)
	}
	if s2.ID != "sub_2" || s2.Status != "active" || s2.GuildID != "456" || s2.CurrentPeriodEnd != 1700000000 {
		t.Fatalf("subscription parse: %+v", s2)
	}
}
