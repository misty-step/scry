package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"
)

// F2 (Pass 4): signed PaymentIntent webhooks without metadata.scry_key are
// never dropped by the adapter. Each becomes a top-up event carrying the
// intent's id, its customer and (for a failure) the reason, with an empty
// key, so the app can settle the attempt that recorded that charge_ref.
// A webhook naming neither an intent nor a key is still ignored.
func TestF2P4_StripeIDOnlyIntentWebhooksArePassedOn(t *testing.T) {
	s, _ := NewStripe("sk_test_pass4", "pk_test_pass4", "price_pass4", "whsec_pass4")
	now := time.Unix(1_800_000_000, 0)
	sign := func(body []byte) string {
		stamp := strconv.FormatInt(now.Unix(), 10)
		m := hmac.New(sha256.New, []byte("whsec_pass4"))
		m.Write([]byte(stamp + "."))
		m.Write(body)
		return "t=" + stamp + ",v1=" + hex.EncodeToString(m.Sum(nil))
	}
	for _, tc := range []struct {
		typ, meta, reason string
		kind              EventKind
		want              string
	}{
		{"payment_intent.succeeded", `{}`, "", TopupPaid, ""},
		{"payment_intent.succeeded", `null`, "", TopupPaid, ""},
		{"payment_intent.payment_failed", `{}`, "Your card has insufficient funds.", TopupFailed, "Your card has insufficient funds."},
		{"payment_intent.payment_failed", `{"other":"x"}`, "", TopupFailed, "Your card was declined."},
		{"payment_intent.canceled", `{}`, "", TopupFailed, ""},
	} {
		body := []byte(fmt.Sprintf(`{"id":"evt_p4","type":%q,"created":1800000000,"livemode":false,"data":{"object":{"id":"pi_p4","object":"payment_intent","customer":"cus_p4","metadata":%s,"last_payment_error":{"message":%q}}}}`,
			tc.typ, tc.meta, tc.reason))
		evs, err := s.ParseWebhook(context.Background(), body, sign(body), now)
		if err != nil || len(evs) != 1 {
			t.Fatalf("%s %s: %v %v", tc.typ, tc.meta, evs, err)
		}
		ev := evs[0]
		if ev.Kind != tc.kind || ev.ChargeRef != "pi_p4" || ev.ChargeKey != "" || ev.CustomerRef != "cus_p4" || ev.Reason != tc.want || ev.ID != "evt_p4" {
			t.Fatalf("%s %s: %+v", tc.typ, tc.meta, ev)
		}
	}
	body := []byte(`{"id":"evt_p4_none","type":"payment_intent.succeeded","created":1800000000,"livemode":false,"data":{"object":{"object":"payment_intent","metadata":{}}}}`)
	if evs, err := s.ParseWebhook(context.Background(), body, sign(body), now); err != nil || len(evs) != 0 {
		t.Fatalf("an intent with neither id nor key: %v %v", evs, err)
	}
	// Still signed: an ID-only webhook with a bad signature is refused.
	body = []byte(`{"id":"evt_p4_bad","type":"payment_intent.succeeded","created":1800000000,"livemode":false,"data":{"object":{"id":"pi_p4","metadata":{}}}}`)
	if _, err := s.ParseWebhook(context.Background(), body, "t=1800000000,v1=00", now); err == nil {
		t.Fatal("an unsigned ID-only webhook was accepted")
	}
}

// Minor (Pass 4): a direct HTTP 200 canceled PaymentIntent with no
// last_payment_error is not a card decline. Charge returns its id with a
// *CanceledError and never invents "Your card was declined."; a canceled
// intent that does carry a processor reason, and requires_payment_method,
// are still declines naming the intent.
func TestF2P4_StripeDirectCanceledWithoutReasonIsNotADecline(t *testing.T) {
	status, reason := "", ""
	s := pass2Stripe(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"id":"pi_c","status":%q,"last_payment_error":{"message":%q}}`, status, reason)
	})
	status = "canceled"
	ref, err := s.Charge(context.Background(), "cus_1", "pm_1", 500, "Scry top-up", "topup-1-abc")
	var c *CanceledError
	var d *DeclineError
	if !errors.As(err, &c) || errors.As(err, &d) || c.Ref != "pi_c" || ref != "pi_c" {
		t.Fatalf("canceled without reason: %q %v, want a release naming pi_c", ref, err)
	}
	if err.Error() == declinedSentence {
		t.Fatalf("canceled without reason invented %q", declinedSentence)
	}
	// The lookup of the same intent agrees: failed, no reason.
	if res := (stripeIntent{ID: "pi_c", Status: "canceled"}).result(); res.Status != ChargeFailed || res.Reason != "" || res.Ref != "pi_c" {
		t.Fatalf("lookup of canceled: %+v", res)
	}
	for _, tc := range []struct{ status, reason, want string }{
		{"canceled", "Your card was declined by the bank.", "Your card was declined by the bank."},
		{"requires_payment_method", "", declinedSentence},
		{"requires_payment_method", "Your card has insufficient funds.", "Your card has insufficient funds."},
	} {
		status, reason = tc.status, tc.reason
		ref, err := s.Charge(context.Background(), "cus_1", "pm_1", 500, "Scry top-up", "topup-1-abc")
		if !errors.As(err, &d) || errors.As(err, &c) || d.Reason != tc.want || d.Ref != "pi_c" || ref != "pi_c" {
			t.Fatalf("%s %q: %q %v, want decline %q naming pi_c", tc.status, tc.reason, ref, err, tc.want)
		}
	}
}
