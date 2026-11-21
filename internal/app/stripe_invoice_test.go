package app_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"

	"scry/internal/billing"
	"scry/internal/store"
)

type offlineStripe struct{}

func (offlineStripe) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("offline")
}

// End to end through the Stripe adapter: a failed invoice's period is
// parsed from its lines, so a failure for a period that has since been
// paid (a late or reordered invoice.payment_failed) is ignored and never
// puts a paid plan into past_due, while a failure for the next, open
// period still does.
func TestStripeStaleInvoiceFailureIsIgnored(t *testing.T) {
	w := newBillingWorld(t, filepath.Join(t.TempDir(), "stripe-failed.db"))
	const secret = "whsec_e2e"
	s, err := billing.NewStripe("sk_test_e2e", "pk_test_e2e", "price_e2e", secret)
	if err != nil {
		t.Fatal(err)
	}
	s.BaseURL, s.HTTP = "http://stripe.invalid", &http.Client{Transport: offlineStripe{}}
	sub, _ := w.us.Subscription(w.ctx)
	end0 := sub.PeriodEnd
	deliver := func(id, typ string, k int) {
		t.Helper()
		start, end := end0.AddDate(0, k-1, 0), end0.AddDate(0, k, 0)
		amount := `"amount_due":500`
		if typ == "invoice.paid" {
			amount = `"amount_paid":500`
		}
		body := []byte(fmt.Sprintf(`{"id":%q,"type":%q,"created":%d,"livemode":false,"data":{"object":{"id":"in_%s","subscription":%q,%s,"lines":{"data":[{"period":{"start":%d,"end":%d}}]}}}}`,
			id, typ, w.now().Unix(), id, sub.SubscriptionRef, amount, start.Unix(), end.Unix()))
		stamp := strconv.FormatInt(w.now().Unix(), 10)
		m := hmac.New(sha256.New, []byte(secret))
		m.Write([]byte(stamp + "."))
		m.Write(body)
		evs, err := s.ParseWebhook(w.ctx, body, "t="+stamp+",v1="+hex.EncodeToString(m.Sum(nil)), w.now())
		if err != nil || len(evs) != 1 {
			t.Fatalf("%s: %v %v", id, evs, err)
		}
		if !evs[0].PeriodEnd.Equal(end) {
			t.Fatalf("%s: period end %v, want %v", id, evs[0].PeriodEnd, end)
		}
		w.deliver(t, evs[0])
	}
	status := func() store.Subscription {
		t.Helper()
		got, err := w.us.Subscription(w.ctx)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	deliver("evt_fail_1", "invoice.payment_failed", 1) // the trial's first invoice fails
	if got := status(); got.Status != store.SubPastDue || got.PastDueSince.IsZero() {
		t.Fatalf("open failure: %+v", got)
	}
	deliver("evt_paid_1", "invoice.paid", 1) // retried and paid
	if got := status(); got.Status != store.SubActive || !got.PeriodEnd.Equal(end0.AddDate(0, 1, 0)) {
		t.Fatalf("paid: %+v", got)
	}
	failedLines := func() int {
		led, _ := w.us.Ledger(w.ctx)
		n := 0
		for _, e := range led {
			if e.Status == "failed" {
				n++
			}
		}
		return n
	}
	before := failedLines()
	deliver("evt_fail_1_late", "invoice.payment_failed", 1) // an earlier attempt's failure, delivered late
	if got := status(); got.Status != store.SubActive || !got.PastDueSince.IsZero() || got.CardError != "" {
		t.Fatalf("stale failure applied: %+v", got)
	}
	if failedLines() != before {
		t.Fatal("stale failure added a failed charge line")
	}
	deliver("evt_fail_2", "invoice.payment_failed", 2) // the next period's invoice fails
	if got := status(); got.Status != store.SubPastDue {
		t.Fatalf("next period's failure: %+v", got)
	}
}
