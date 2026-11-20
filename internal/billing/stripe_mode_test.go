package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Slice 12: Stripe is pinned to one explicit mode. Keys and webhooks from
// the other mode are refused loudly, never silently accepted.
func TestStripeModeMatchedKeys(t *testing.T) {
	for _, tc := range []struct {
		name        string
		live        bool
		secret, pub string
		ok          bool
		errHas      string
	}{
		{"live accepts sk_live", true, "sk_live_x", "pk_live_x", true, ""},
		{"live accepts rk_live", true, "rk_live_x", "pk_live_x", true, ""},
		{"live rejects sk_test", true, "sk_test_x", "pk_live_x", false, "test-mode key"},
		{"live rejects rk_test", true, "rk_test_x", "pk_live_x", false, "test-mode key"},
		{"live rejects pk_test", true, "sk_live_x", "pk_test_x", false, "STRIPE_PUBLISHABLE_KEY is a test-mode key"},
		{"live rejects junk secret", true, "nonsense", "pk_live_x", false, "must be a live-mode key"},
		{"live rejects empty publishable", true, "sk_live_x", "", false, "must be a live-mode key"},
		{"test accepts sk_test", false, "sk_test_x", "pk_test_x", true, ""},
		{"test accepts rk_test", false, "rk_test_x", "pk_test_x", true, ""},
		{"test rejects sk_live", false, "sk_live_x", "pk_test_x", false, "live-mode key"},
		{"test rejects rk_live", false, "rk_live_x", "pk_test_x", false, "live-mode key"},
		{"test rejects pk_live", false, "sk_test_x", "pk_live_x", false, "STRIPE_PUBLISHABLE_KEY is a live-mode key"},
		{"test rejects junk secret", false, "whatever", "pk_test_x", false, "must be a test-mode key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := NewStripeWithMode(tc.live, tc.secret, tc.pub, "price_x", "whsec_x")
			if tc.ok {
				if err != nil {
					t.Fatal(err)
				}
				want := "stripe-test"
				if tc.live {
					want = "stripe-live"
				}
				if s.Name() != want || s.Live() != tc.live {
					t.Fatalf("Name() = %q Live() = %v", s.Name(), s.Live())
				}
				return
			}
			if err == nil || s != nil {
				t.Fatal("wrong-mode key accepted")
			}
			if !strings.Contains(err.Error(), tc.errHas) {
				t.Fatalf("error %q does not say %q", err, tc.errHas)
			}
			// The refusal never echoes the key itself.
			if strings.Contains(err.Error(), tc.secret) && len(tc.secret) > 8 {
				t.Fatalf("error leaks the key: %v", err)
			}
		})
	}
	for _, live := range []bool{true, false} {
		mode := "test"
		if live {
			mode = "live"
		}
		if _, err := NewStripeWithMode(live, "sk_"+mode+"_x", "pk_"+mode+"_x", "", "whsec_x"); err == nil {
			t.Fatal("missing price accepted")
		}
		if _, err := NewStripeWithMode(live, "sk_"+mode+"_x", "pk_"+mode+"_x", "price_x", ""); err == nil {
			t.Fatal("missing webhook secret accepted")
		}
	}
	// NewStripe stays the test-mode constructor.
	if s, err := NewStripe("sk_test_x", "pk_test_x", "price_x", "whsec_x"); err != nil || s.Name() != "stripe-test" {
		t.Fatal(s, err)
	}
	if _, err := NewStripe("sk_live_x", "pk_live_x", "price_x", "whsec_x"); err == nil {
		t.Fatal("NewStripe accepted live keys")
	}
}

func TestStripeWebhookLivemodeMatchesAdapter(t *testing.T) {
	now := time.Unix(1800000000, 0)
	sign := func(body []byte) string {
		stamp := strconv.FormatInt(now.Unix(), 10)
		m := hmac.New(sha256.New, []byte("whsec_mode"))
		m.Write([]byte(stamp + "."))
		m.Write(body)
		return "t=" + stamp + ",v1=" + hex.EncodeToString(m.Sum(nil))
	}
	event := func(livemode bool) []byte {
		lm := "false"
		if livemode {
			lm = "true"
		}
		return []byte(`{"id":"evt_mode","type":"invoice.paid","created":1800000000,"livemode":` + lm + `,"data":{"object":{"subscription":"sub_mode","amount_paid":500}}}`)
	}
	live, err := NewStripeWithMode(true, "sk_live_mode", "pk_live_mode", "price_mode", "whsec_mode")
	if err != nil {
		t.Fatal(err)
	}
	test, err := NewStripeWithMode(false, "sk_test_mode", "pk_test_mode", "price_mode", "whsec_mode")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		s        *Stripe
		livemode bool
		ok       bool
	}{
		{"live adapter accepts livemode true", live, true, true},
		{"live adapter rejects livemode false", live, false, false},
		{"test adapter accepts livemode false", test, false, true},
		{"test adapter rejects livemode true", test, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := event(tc.livemode)
			ev, err := tc.s.ParseWebhook(context.Background(), body, sign(body), now)
			if tc.ok {
				if err != nil || len(ev) != 1 || ev[0].Kind != InvoicePaid || ev[0].AmountCents != 500 {
					t.Fatalf("ev %v err %v", ev, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "webhook refused") {
				t.Fatalf("wrong-mode webhook not refused loudly: %v %v", ev, err)
			}
		})
	}
	// A webhook with livemode omitted is a test-mode event to Stripe's
	// decoder (false), so a live adapter refuses it too.
	body := []byte(`{"id":"evt_nolm","type":"invoice.paid","created":1800000000,"data":{"object":{"subscription":"sub_mode","amount_paid":500}}}`)
	if _, err := live.ParseWebhook(context.Background(), body, sign(body), now); err == nil {
		t.Fatal("live adapter accepted an event without livemode")
	}
	// Signature is still checked before mode.
	body = event(true)
	if _, err := live.ParseWebhook(context.Background(), body, "t=1800000000,v1=00", now); err == nil || strings.Contains(err.Error(), "webhook refused") {
		t.Fatalf("bad signature not refused first: %v", err)
	}
}
