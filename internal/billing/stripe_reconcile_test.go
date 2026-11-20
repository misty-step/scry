package billing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Reconciliation looks PaymentIntents up (never creates or confirms one):
// the top-up key travels as metadata and as the Idempotency-Key, and the
// lookup lists the customer's intents since the attempt began.
func TestF2_StripeChargeKeyAndFindCharge(t *testing.T) {
	var charged, listed string
	status := "succeeded"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/v1/payment_intents":
			_ = r.ParseForm()
			charged = r.Header.Get("Idempotency-Key") + "|" + r.PostForm.Get("metadata[scry_key]")
			w.Write([]byte(`{"id":"pi_1","status":"succeeded"}`))
		case r.Method == "GET" && r.URL.Path == "/v1/payment_intents":
			listed = r.URL.RawQuery
			w.Write([]byte(`{"has_more":false,"data":[{"id":"pi_other","status":"succeeded","metadata":{"scry_key":"topup-1-other"}},
			  {"id":"pi_1","status":"` + status + `","metadata":{"scry_key":"topup-1-abc"},"last_payment_error":{"message":"Your card has insufficient funds."}}]}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "no", 400)
		}
	}))
	defer srv.Close()
	s, err := NewStripe("sk_test_x", "pk_test_x", "price_x", "whsec_x")
	if err != nil {
		t.Fatal(err)
	}
	s.BaseURL = srv.URL
	if ref, err := s.Charge(context.Background(), "cus_1", "pm_1", 500, "Scry top-up", "topup-1-abc"); err != nil || ref != "pi_1" {
		t.Fatal(ref, err)
	}
	if charged != "topup-1-abc|topup-1-abc" {
		t.Fatalf("key on the charge: %q", charged)
	}
	since := time.Unix(1_800_000_000, 0)
	for st, want := range map[string]ChargeStatus{"succeeded": ChargeSucceeded, "processing": ChargePending, "canceled": ChargeFailed, "requires_payment_method": ChargeFailed} {
		status = st
		res, err := s.FindCharge(context.Background(), "cus_1", "pm_1", "topup-1-abc", since)
		if err != nil || res.Status != want || res.Ref != "pi_1" {
			t.Errorf("%s: %+v %v", st, res, err)
		}
		if want == ChargeFailed && res.Reason != "Your card has insufficient funds." {
			t.Errorf("%s reason: %q", st, res.Reason)
		}
	}
	if !strings.Contains(listed, "customer=cus_1") || !strings.Contains(listed, "created%5Bgte%5D=1799999400") {
		t.Fatalf("list query: %s", listed)
	}
	res, err := s.FindCharge(context.Background(), "cus_1", "pm_1", "topup-1-missing", since)
	if err != nil || res.Status != ChargeNotFound {
		t.Fatalf("missing: %+v %v", res, err)
	}
}

// The test processor is idempotent in the key, like Stripe, and can be slow.
func TestF2_FakeChargeIsIdempotentAndCanBeSlow(t *testing.T) {
	f := Fake{}
	tok := "tok_test_ok_Visa_4242"
	a, _ := f.Charge(context.Background(), "", tok, 500, "", "k1")
	b, _ := f.Charge(context.Background(), "", tok, 500, "", "k1")
	c, _ := f.Charge(context.Background(), "", tok, 500, "", "k2")
	found, _ := f.FindCharge(context.Background(), "", tok, "k1", time.Now())
	if a != b || a == c || found.Ref != a || found.Status != ChargeSucceeded {
		t.Fatal(a, b, c, found)
	}
	slow := Fake{ChargeDelay: 200 * time.Millisecond}
	start := time.Now()
	if _, err := slow.Charge(context.Background(), "", tok, 500, "", "k3"); err != nil || time.Since(start) < 200*time.Millisecond {
		t.Fatal(err, time.Since(start))
	}
}
