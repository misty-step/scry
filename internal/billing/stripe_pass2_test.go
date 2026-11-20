package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func pass2Stripe(t *testing.T, h http.HandlerFunc) *Stripe {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	s, err := NewStripe("sk_test_pass2", "pk_test_pass2", "price_pass2", "whsec_pass2")
	if err != nil {
		t.Fatal(err)
	}
	s.BaseURL = srv.URL
	return s
}

// F2 (Pass 2): a PaymentIntent list still truncated (has_more) at the page
// limit is an incomplete lookup, never "not found".
func TestF2_StripeIncompleteScanIsNotAbsence(t *testing.T) {
	pages := 0
	s := pass2Stripe(t, func(w http.ResponseWriter, r *http.Request) {
		pages++
		fmt.Fprintf(w, `{"has_more":true,"data":[{"id":"pi_other_%d","status":"succeeded","metadata":{"scry_key":"topup-1-other"}}]}`, pages)
	})
	res, err := s.FindCharge(context.Background(), ChargeQuery{CustomerRef: "cus_1", Key: "topup-1-abc", Since: time.Unix(1_800_000_000, 0)})
	if !errors.Is(err, ErrLookupIncomplete) || res.Status == ChargeNotFound {
		t.Fatalf("truncated scan: %+v %v, want ErrLookupIncomplete", res, err)
	}
	if pages != findPages {
		t.Fatalf("read %d pages, want %d", pages, findPages)
	}
	// has_more with an empty page cannot be continued: also incomplete.
	s = pass2Stripe(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"has_more":true,"data":[]}`) })
	if _, err := s.FindCharge(context.Background(), ChargeQuery{CustomerRef: "cus_1", Key: "k"}); !errors.Is(err, ErrLookupIncomplete) {
		t.Fatalf("empty has_more page: %v", err)
	}
	// A lookup with no customer recorded is refused, not "not found".
	if _, err := s.FindCharge(context.Background(), ChargeQuery{Key: "k"}); err == nil {
		t.Fatal("lookup without a customer reported a result")
	}
}

// F2 (Pass 2): a 200 PaymentIntent that is still processing is not a
// decline; Charge returns its id and a *PendingError. Final refusals are
// still declines.
func TestF2_StripeProcessingChargeIsPendingNotDeclined(t *testing.T) {
	status := "processing"
	s := pass2Stripe(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"id":"pi_9","status":%q,"last_payment_error":{"message":"Your card has insufficient funds."}}`, status)
	})
	for _, st := range []string{"processing", "requires_capture"} {
		status = st
		ref, err := s.Charge(context.Background(), "cus_1", "pm_1", 500, "Scry top-up", "topup-1-abc")
		var p *PendingError
		var d *DeclineError
		if !errors.As(err, &p) || errors.As(err, &d) || p.Ref != "pi_9" || ref != "pi_9" {
			t.Fatalf("%s: %q %v, want pending pi_9", st, ref, err)
		}
	}
	status = "requires_payment_method"
	var d *DeclineError
	if _, err := s.Charge(context.Background(), "cus_1", "pm_1", 500, "Scry top-up", "topup-1-abc"); !errors.As(err, &d) || d.Reason != "Your card has insufficient funds." {
		t.Fatalf("requires_payment_method: %v", err)
	}
}

// F2 (Pass 2): with the PaymentIntent's id recorded, the lookup retrieves
// that intent directly (no list, no customer filter) and checks it is the
// top-up's own.
func TestF2_StripeFindChargeByIntentID(t *testing.T) {
	var paths []string
	status, key := "succeeded", "topup-1-abc"
	s := pass2Stripe(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if r.URL.Path != "/v1/payment_intents/pi_7" {
			w.WriteHeader(404)
			fmt.Fprint(w, `{"error":{"type":"invalid_request_error","code":"resource_missing"}}`)
			return
		}
		fmt.Fprintf(w, `{"id":"pi_7","status":%q,"customer":"cus_old","metadata":{"scry_key":%q}}`, status, key)
	})
	q := ChargeQuery{CustomerRef: "cus_new", ChargeRef: "pi_7", Key: "topup-1-abc", Since: time.Now()}
	for st, want := range map[string]ChargeStatus{"succeeded": ChargeSucceeded, "processing": ChargePending, "canceled": ChargeFailed} {
		status = st
		if res, err := s.FindCharge(context.Background(), q); err != nil || res.Status != want || res.Ref != "pi_7" {
			t.Errorf("%s: %+v %v", st, res, err)
		}
	}
	for _, p := range paths {
		if p != "GET /v1/payment_intents/pi_7" {
			t.Fatalf("calls: %v, want only the direct retrieve", paths)
		}
	}
	key = "topup-2-someone-else"
	if _, err := s.FindCharge(context.Background(), q); err == nil {
		t.Fatal("an intent with another top-up's key was accepted")
	}
	q.ChargeRef = "pi_missing"
	if res, err := s.FindCharge(context.Background(), q); err == nil || res.Status == ChargeNotFound {
		t.Fatalf("missing intent: %+v %v, want an inconclusive error", res, err)
	}
}

// F2 (Pass 2): PaymentIntent webhooks for Scry top-ups become top-up
// events; intents without Scry's key (a plan's invoices) are ignored.
func TestF2_StripeTopupWebhooks(t *testing.T) {
	s, _ := NewStripe("sk_test_pass2", "pk_test_pass2", "price_pass2", "whsec_pass2")
	now := time.Unix(1_800_000_000, 0)
	sign := func(body []byte) string {
		stamp := strconv.FormatInt(now.Unix(), 10)
		m := hmac.New(sha256.New, []byte("whsec_pass2"))
		m.Write([]byte(stamp + "."))
		m.Write(body)
		return "t=" + stamp + ",v1=" + hex.EncodeToString(m.Sum(nil))
	}
	event := func(typ, meta, reason string) []byte {
		return []byte(fmt.Sprintf(`{"id":"evt_1","type":%q,"created":1800000000,"livemode":false,"data":{"object":{"id":"pi_1","object":"payment_intent","customer":"cus_1","metadata":%s,"last_payment_error":{"message":%q}}}}`, typ, meta, reason))
	}
	for _, tc := range []struct {
		typ, reason string
		kind        EventKind
		want        string
	}{
		{"payment_intent.succeeded", "", TopupPaid, ""},
		{"payment_intent.payment_failed", "Your card has insufficient funds.", TopupFailed, "Your card has insufficient funds."},
		{"payment_intent.payment_failed", "", TopupFailed, "Your card was declined."},
		{"payment_intent.canceled", "", TopupFailed, ""},
	} {
		body := event(tc.typ, `{"scry_key":"topup-1-abc"}`, tc.reason)
		evs, err := s.ParseWebhook(context.Background(), body, sign(body), now)
		if err != nil || len(evs) != 1 {
			t.Fatalf("%s: %v %v", tc.typ, evs, err)
		}
		ev := evs[0]
		if ev.Kind != tc.kind || ev.ChargeRef != "pi_1" || ev.ChargeKey != "topup-1-abc" || ev.CustomerRef != "cus_1" || ev.Reason != tc.want || ev.ID != "evt_1" {
			t.Fatalf("%s: %+v", tc.typ, ev)
		}
	}
	// An intent without Scry's key is still passed on by id (Pass 4): the
	// app settles the attempt that recorded that id, and ignores it when no
	// attempt did (a plan's invoice payment).
	body := event("payment_intent.succeeded", `{}`, "")
	if evs, err := s.ParseWebhook(context.Background(), body, sign(body), now); err != nil || len(evs) != 1 ||
		evs[0].Kind != TopupPaid || evs[0].ChargeRef != "pi_1" || evs[0].ChargeKey != "" || evs[0].CustomerRef != "cus_1" {
		t.Fatalf("an intent without Scry's key: %+v %v", evs, err)
	}
}
