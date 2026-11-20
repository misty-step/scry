package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"testing"
	"time"
)

func pass4bSign(now time.Time, body []byte) string {
	stamp := strconv.FormatInt(now.Unix(), 10)
	m := hmac.New(sha256.New, []byte("whsec_pass4b"))
	m.Write([]byte(stamp + "."))
	m.Write(body)
	return "t=" + stamp + ",v1=" + hex.EncodeToString(m.Sum(nil))
}

// Pass 4b: a payment_intent.canceled webhook keeps the processor's
// last_payment_error.message as the reason (ID-only and keyed), like a
// direct canceled charge; with no message it has no reason.
func TestP4b_StripeCanceledWebhookKeepsSuppliedReason(t *testing.T) {
	s, _ := NewStripe("sk_test_pass4b", "pk_test_pass4b", "price_pass4b", "whsec_pass4b")
	now := time.Unix(1_800_000_000, 0)
	for _, tc := range []struct{ meta, lpe, want string }{
		{`{}`, `{"message":"The processor rejected this card."}`, "The processor rejected this card."},
		{`{"scry_key":"topup-1-abc"}`, `{"message":"The processor rejected this card."}`, "The processor rejected this card."},
		{`{}`, `{"message":""}`, ""},
		{`{}`, `null`, ""},
		{`{"scry_key":"topup-1-abc"}`, `{}`, ""},
	} {
		body := []byte(fmt.Sprintf(`{"id":"evt_p4b","type":"payment_intent.canceled","created":1800000000,"livemode":false,"data":{"object":{"id":"pi_p4b","object":"payment_intent","customer":"cus_p4b","metadata":%s,"last_payment_error":%s}}}`, tc.meta, tc.lpe))
		evs, err := s.ParseWebhook(context.Background(), body, pass4bSign(now, body), now)
		if err != nil || len(evs) != 1 {
			t.Fatalf("%s %s: %v %v", tc.meta, tc.lpe, evs, err)
		}
		if ev := evs[0]; ev.Kind != TopupFailed || ev.ChargeRef != "pi_p4b" || ev.Reason != tc.want {
			t.Fatalf("%s %s: %+v; want reason %q", tc.meta, tc.lpe, ev, tc.want)
		}
	}
}

// Pass 4b: a PaymentIntent event whose object has an empty id is
// malformed and ignored, even when it carries metadata.scry_key.
func TestP4b_StripeIntentWebhookWithoutIDIsIgnored(t *testing.T) {
	s, _ := NewStripe("sk_test_pass4b", "pk_test_pass4b", "price_pass4b", "whsec_pass4b")
	now := time.Unix(1_800_000_000, 0)
	for _, typ := range []string{"payment_intent.succeeded", "payment_intent.payment_failed", "payment_intent.canceled"} {
		for _, id := range []string{`"id":"",`, ``} {
			body := []byte(fmt.Sprintf(`{"id":"evt_p4b_noid","type":%q,"created":1800000000,"livemode":false,"data":{"object":{%s"object":"payment_intent","customer":"cus_p4b","metadata":{"scry_key":"topup-1-abc"}}}}`, typ, id))
			if evs, err := s.ParseWebhook(context.Background(), body, pass4bSign(now, body), now); err != nil || len(evs) != 0 {
				t.Fatalf("%s %q: %v %v; want ignored", typ, id, evs, err)
			}
		}
	}
}
