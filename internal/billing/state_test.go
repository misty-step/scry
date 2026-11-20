package billing

import (
	"errors"
	"testing"
)

func TestClassifyPaymentIntentTable(t *testing.T) {
	for _, tc := range []struct {
		status     PaymentIntentStatus
		reason     string
		want       ChargeStatus
		wantReason string
	}{
		{PISucceeded, "", ChargeSucceeded, ""},
		{PISucceeded, "ignored", ChargeSucceeded, ""},
		{PIRequiresPaymentMethod, "", ChargeFailed, DeclinedSentence},
		{PIRequiresPaymentMethod, "Your card has insufficient funds.", ChargeFailed, "Your card has insufficient funds."},
		{PICanceled, "", ChargeFailed, ""},
		{PICanceled, "Your card was declined.", ChargeFailed, "Your card was declined."},
		{PIProcessing, "", ChargePending, ""},
		{PIRequiresAction, "", ChargePending, ""},
		{PIRequiresConfirmation, "", ChargePending, ""},
		{PIRequiresCapture, "", ChargePending, ""},
		{"some_future_status", "x", ChargePending, ""},
		{"", "", ChargePending, ""},
	} {
		got, reason := ClassifyPaymentIntent(tc.status, tc.reason)
		if got != tc.want || reason != tc.wantReason {
			t.Errorf("ClassifyPaymentIntent(%q, %q) = %s, %q; want %s, %q", tc.status, tc.reason, got, reason, tc.want, tc.wantReason)
		}
	}
}

// The pure classifier and the Stripe adapter's own mapping
// (stripeIntent.result) agree on every status, with and without a
// processor sentence, so the adapter can be switched onto it safely.
func TestClassifyPaymentIntentMatchesStripeAdapter(t *testing.T) {
	statuses := append([]PaymentIntentStatus{"", "unknown"}, PaymentIntentStatuses...)
	for _, st := range statuses {
		for _, reason := range []string{"", "Your card has insufficient funds."} {
			var pi stripeIntent
			pi.ID, pi.Status, pi.LastPaymentError.Message = "pi_1", string(st), reason
			res := pi.result()
			got, gotReason := ClassifyPaymentIntent(st, reason)
			if res.Status != got || res.Reason != gotReason || res.Ref != "pi_1" {
				t.Errorf("%q/%q: adapter %+v, classifier %s %q", st, reason, res, got, gotReason)
			}
		}
	}
}

func TestChargeOutcomeTypedErrors(t *testing.T) {
	if err := ChargeOutcome(PISucceeded, "pi_1", ""); err != nil {
		t.Fatalf("succeeded: %v", err)
	}
	var p *PendingError
	if err := ChargeOutcome(PIProcessing, "pi_1", ""); !errors.As(err, &p) || p.Ref != "pi_1" {
		t.Fatalf("processing: %v", err)
	}
	var d *DeclineError
	if err := ChargeOutcome(PIRequiresPaymentMethod, "pi_1", ""); !errors.As(err, &d) || d.Reason != DeclinedSentence || d.Ref != "pi_1" {
		t.Fatalf("rpm: %v", err)
	}
	if err := ChargeOutcome(PICanceled, "pi_1", "Do not honor."); !errors.As(err, &d) || d.Reason != "Do not honor." {
		t.Fatalf("canceled with a reason: %v", err)
	}
	var c *CanceledError
	if err := ChargeOutcome(PICanceled, "pi_1", ""); !errors.As(err, &c) || c.Ref != "pi_1" {
		t.Fatalf("canceled: %v", err)
	}
}

func TestPaymentIntentFinalAndEvents(t *testing.T) {
	for _, st := range PaymentIntentStatuses {
		kind, ok := TopupEventFor(st)
		if ok != st.Final() {
			t.Errorf("%s: final=%v but event=%v", st, st.Final(), ok)
		}
		cs, _ := ClassifyPaymentIntent(st, "")
		switch {
		case kind == TopupPaid && cs != ChargeSucceeded,
			kind == TopupFailed && cs != ChargeFailed,
			!ok && cs != ChargePending:
			t.Errorf("%s: event %q disagrees with charge status %s", st, kind, cs)
		}
	}
}
