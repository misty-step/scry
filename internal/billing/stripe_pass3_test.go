package billing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// F2 (Pass 3): requires_action and requires_confirmation, like processing
// and requires_capture, are not final. Charge returns the id with a
// *PendingError, never a decline.
func TestF2P3_StripeNonFinalChargeIsPending(t *testing.T) {
	status := ""
	s := pass2Stripe(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"id":"pi_3","status":%q,"last_payment_error":{"message":"Authentication required."}}`, status)
	})
	for _, st := range []string{"processing", "requires_capture", "requires_action", "requires_confirmation"} {
		status = st
		ref, err := s.Charge(context.Background(), "cus_1", "pm_1", 500, "Scry top-up", "topup-1-abc")
		var p *PendingError
		var d *DeclineError
		if !errors.As(err, &p) || errors.As(err, &d) || p.Ref != "pi_3" || ref != "pi_3" {
			t.Fatalf("%s: %q %v, want pending pi_3", st, ref, err)
		}
	}
	for st, reason := range map[string]string{"requires_payment_method": "Authentication required.", "canceled": "Authentication required."} {
		status = st
		var d *DeclineError
		if _, err := s.Charge(context.Background(), "cus_1", "pm_1", 500, "Scry top-up", "topup-1-abc"); !errors.As(err, &d) || d.Reason != reason || d.Ref != "pi_3" {
			t.Fatalf("%s: %v, want a decline naming pi_3", st, err)
		}
	}
}

// F2 (Pass 3): lookups (by id and by list) report requires_action and
// requires_confirmation as ChargePending with the intent's id.
func TestF2P3_StripeFindChargeNonFinalIsPending(t *testing.T) {
	status := ""
	s := pass2Stripe(t, func(w http.ResponseWriter, r *http.Request) {
		pi := fmt.Sprintf(`{"id":"pi_4","status":%q,"customer":"cus_1","metadata":{"scry_key":"topup-1-abc"}}`, status)
		if r.URL.Path == "/v1/payment_intents" {
			fmt.Fprintf(w, `{"has_more":false,"data":[%s]}`, pi)
			return
		}
		fmt.Fprint(w, pi)
	})
	byID := ChargeQuery{CustomerRef: "cus_1", ChargeRef: "pi_4", Key: "topup-1-abc", Since: time.Now()}
	byList := ChargeQuery{CustomerRef: "cus_1", Key: "topup-1-abc", Since: time.Now()}
	want := map[string]ChargeStatus{"requires_action": ChargePending, "requires_confirmation": ChargePending, "processing": ChargePending,
		"requires_capture": ChargePending, "succeeded": ChargeSucceeded, "canceled": ChargeFailed, "requires_payment_method": ChargeFailed}
	for st, w := range want {
		status = st
		for name, q := range map[string]ChargeQuery{"id": byID, "list": byList} {
			if res, err := s.FindCharge(context.Background(), q); err != nil || res.Status != w || res.Ref != "pi_4" {
				t.Errorf("%s by %s: %+v %v, want %s with pi_4", st, name, res, err, w)
			}
		}
	}
	status = "requires_payment_method"
	if res, _ := s.FindCharge(context.Background(), byID); res.Reason != "Your card was declined." {
		t.Errorf("requires_payment_method with no sentence: %q", res.Reason)
	}
}

// F2 (Pass 3): an error answer naming a PaymentIntent surfaces its id: on
// a card_error decline, and on any other (indeterminate) error.
func TestF2P3_StripeErrorNamesIntent(t *testing.T) {
	code, body := 0, ""
	s := pass2Stripe(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
		fmt.Fprint(w, body)
	})
	code, body = 402, `{"error":{"type":"card_error","code":"card_declined","message":"Your card was declined.","payment_intent":{"id":"pi_5","status":"requires_payment_method"}}}`
	ref, err := s.Charge(context.Background(), "cus_1", "pm_1", 500, "Scry top-up", "topup-1-abc")
	var d *DeclineError
	if !errors.As(err, &d) || d.Ref != "pi_5" || d.Reason != "Your card was declined." {
		t.Fatalf("402: %q %v, want a decline naming pi_5", ref, err)
	}
	code, body = 402, `{"error":{"type":"card_error","code":"card_declined","message":"Your card was declined."}}`
	if _, err := s.Charge(context.Background(), "cus_1", "pm_1", 500, "Scry top-up", "topup-1-abc"); !errors.As(err, &d) || d.Ref != "" {
		t.Fatalf("402 without intent: %v", err)
	}
	code, body = 500, `{"error":{"type":"api_error","code":"","message":"Something went wrong.","payment_intent":{"id":"pi_6"}}}`
	_, err = s.Charge(context.Background(), "cus_1", "pm_1", 500, "Scry top-up", "topup-1-abc")
	var pe *ProcessorError
	if !errors.As(err, &pe) || errors.As(err, &d) || pe.Ref != "pi_6" || pe.Status != 500 {
		t.Fatalf("500: %v, want an indeterminate error naming pi_6", err)
	}
}
