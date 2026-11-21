package billing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// stripeGolden is what the Stripe adapter did for every PaymentIntent
// status before it was switched onto ClassifyPaymentIntent/ChargeOutcome,
// written out by hand (not derived from the classifier), so the switch is
// checked against an independent reference.
var stripeGolden = []struct {
	status, reason string
	// lookup (FindCharge / stripeIntent.result)
	lookup       ChargeStatus
	lookupReason string
	// synchronous Charge: "paid", "pending", "canceled" or "decline"
	charge       string
	chargeReason string
}{
	{"succeeded", "", ChargeSucceeded, "", "paid", ""},
	{"succeeded", "Your card has insufficient funds.", ChargeSucceeded, "", "paid", ""},
	{"requires_payment_method", "", ChargeFailed, "Your card was declined.", "decline", "Your card was declined."},
	{"requires_payment_method", "Your card has insufficient funds.", ChargeFailed, "Your card has insufficient funds.", "decline", "Your card has insufficient funds."},
	{"canceled", "", ChargeFailed, "", "canceled", ""},
	{"canceled", "Your card was declined by the bank.", ChargeFailed, "Your card was declined by the bank.", "decline", "Your card was declined by the bank."},
	{"processing", "", ChargePending, "", "pending", ""},
	{"processing", "x", ChargePending, "", "pending", ""},
	{"requires_action", "", ChargePending, "", "pending", ""},
	{"requires_capture", "", ChargePending, "", "pending", ""},
	{"requires_confirmation", "", ChargePending, "", "pending", ""},
	{"", "", ChargePending, "", "pending", ""},
	{"something_new", "x", ChargePending, "", "pending", ""},
}

func TestStripeIntentMappingMatchesGolden(t *testing.T) {
	status, reason := "", ""
	s := pass2Stripe(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"id":"pi_g","status":%q,"last_payment_error":{"message":%q}}`, status, reason)
	})
	for _, g := range stripeGolden {
		var pi stripeIntent
		pi.ID, pi.Status, pi.LastPaymentError.Message = "pi_g", g.status, g.reason
		if res := pi.result(); res.Status != g.lookup || res.Reason != g.lookupReason || res.Ref != "pi_g" {
			t.Errorf("lookup %q/%q = %+v, want %s %q", g.status, g.reason, res, g.lookup, g.lookupReason)
		}
		status, reason = g.status, g.reason
		ref, err := s.Charge(context.Background(), "cus_1", "pm_1", 500, "Scry top-up", "topup-1-golden")
		if ref != "pi_g" {
			t.Errorf("charge %q/%q: ref %q", g.status, g.reason, ref)
		}
		var d *DeclineError
		var c *CanceledError
		var p *PendingError
		got, gotReason := "", ""
		switch {
		case err == nil:
			got = "paid"
		case errors.As(err, &d):
			got, gotReason = "decline", d.Reason
			if d.Ref != "pi_g" {
				t.Errorf("decline names %q", d.Ref)
			}
		case errors.As(err, &c):
			got = "canceled"
			if c.Ref != "pi_g" {
				t.Errorf("cancel names %q", c.Ref)
			}
		case errors.As(err, &p):
			got = "pending"
			if p.Ref != "pi_g" {
				t.Errorf("pending names %q", p.Ref)
			}
		default:
			got = "error: " + err.Error()
		}
		if got != g.charge || gotReason != g.chargeReason {
			t.Errorf("charge %q/%q = %s %q, want %s %q", g.status, g.reason, got, gotReason, g.charge, g.chargeReason)
		}
	}
}

// A failed renewal invoice carries the period it was for, so a stale
// failure (for a period the plan has already moved past) can be told from
// the open one by the app.
func TestStripeFailedInvoiceCarriesItsPeriod(t *testing.T) {
	s := fuzzStripe(t)
	sign := func(body []byte) string { return "t=1800000000,v1=" + fuzzSign("1800000000", body) }
	body := []byte(`{"id":"evt_f","type":"invoice.payment_failed","created":1800000000,"livemode":false,"data":{"object":{"id":"in_f","subscription":"sub_1","amount_due":500,"lines":{"data":[{"period":{"start":1800000000,"end":1802592000}}]}}}}`)
	evs, err := s.ParseWebhook(context.Background(), body, sign(body), fuzzNow)
	if err != nil || len(evs) != 1 {
		t.Fatalf("%v %v", evs, err)
	}
	ev := evs[0]
	if ev.Kind != InvoiceFailed || ev.SubscriptionRef != "sub_1" || ev.AmountCents != 500 || ev.Reason != DeclinedSentence {
		t.Fatalf("failed invoice: %+v", ev)
	}
	if !ev.PeriodStart.Equal(time.Unix(1800000000, 0)) || !ev.PeriodEnd.Equal(time.Unix(1802592000, 0)) {
		t.Fatalf("failed invoice period %v to %v", ev.PeriodStart, ev.PeriodEnd)
	}
	// Without lines the period stays zero: the app then cannot order it and
	// applies it as before.
	body = []byte(`{"id":"evt_g","type":"invoice.payment_failed","created":1800000000,"livemode":false,"data":{"object":{"id":"in_g","subscription":"sub_1","amount_due":500}}}`)
	evs, err = s.ParseWebhook(context.Background(), body, sign(body), fuzzNow)
	if err != nil || len(evs) != 1 || !evs[0].PeriodStart.IsZero() || !evs[0].PeriodEnd.IsZero() {
		t.Fatalf("failed invoice without lines: %+v %v", evs, err)
	}
}
