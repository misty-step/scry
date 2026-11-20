package web_test

import (
	"net/http"
	"testing"
	"time"

	"scry/internal/app"
	"scry/internal/store"
)

// pendingHoldsSlot checks the attempt is pending with the intent's id
// kept, holding one cap slot, with no credit yet.
func pendingHoldsSlot(t *testing.T, w *world, uid int64, piID string) store.TopupAttempt {
	t.Helper()
	us := w.store.For(uid)
	att := onlyAttempt(t, w, uid)
	sub, _ := us.Subscription(ctx)
	n, _ := us.MonthTopups(ctx, w.now())
	if att.Status != store.TopupPending || att.ChargeRef != piID || n != 1 || sub.TopupMicros != 0 || paidTopups(t, us) != 0 {
		t.Fatalf("attempt status=%s charge=%q slots=%d credit=%d; want pending with %s holding one slot", att.Status, att.ChargeRef, n, sub.TopupMicros, piID)
	}
	return att
}

// failedWithRef checks the attempt failed, kept the intent's id, released
// its cap slot and granted nothing.
func failedWithRef(t *testing.T, w *world, uid int64, piID string) store.TopupAttempt {
	t.Helper()
	us := w.store.For(uid)
	att := onlyAttempt(t, w, uid)
	sub, _ := us.Subscription(ctx)
	n, _ := us.MonthTopups(ctx, w.now())
	if att.Status != store.TopupFailed || att.ChargeRef != piID || n != 0 || sub.TopupMicros != 0 || paidTopups(t, us) != 0 {
		t.Fatalf("attempt status=%s charge=%q slots=%d credit=%d; want failed with %s, slot released", att.Status, att.ChargeRef, n, sub.TopupMicros, piID)
	}
	return att
}

// F2 (Pass 3): a 200 PaymentIntent in requires_action or
// requires_confirmation is not final. It is never a decline: the intent id
// is kept, the attempt stays pending holding its slot, and it settles later
// by a lookup by that id (no webhook needed) or by webhook, crediting
// exactly once; or it fails cleanly, keeping the id.
func TestF2P3_NonFinalIntentStaysPendingThenSettles(t *testing.T) {
	for _, status := range []string{"requires_action", "requires_confirmation"} {
		for _, end := range []string{"succeeded-lookup", "succeeded-webhook", "canceled-lookup", "requires_payment_method-lookup", "canceled-webhook"} {
			t.Run(status+"/"+end, func(t *testing.T) {
				w, b, st, uid := stripeWorld(t, status+"-"+end+"@example.com")
				st.set(func() { st.answer = status })
				if loc := b.postTopup(); loc != "/billing?ok=pending" {
					t.Fatalf("%s redirected to %s, want ok=pending", status, loc)
				}
				pi := st.only()
				page := b.mustGet("/billing?ok=pending")
				has(t, page, "being confirmed with the card processor")
				hasNot(t, page, "declined", "confirm this payment")
				pendingHoldsSlot(t, w, uid, pi.ID)
				// Still in that state at the processor: a reload and the sweep
				// both leave it pending.
				b.mustGet("/billing")
				w.advance(2 * time.Minute)
				if err := w.app.ReconcileAllTopUps(ctx); err != nil {
					t.Fatal(err)
				}
				pendingHoldsSlot(t, w, uid, pi.ID)
				switch end {
				case "succeeded-lookup":
					st.set(func() { pi.Status = "succeeded" })
					has(t, b.mustGet("/billing"), "Topped up.", "You also have $4 of top-up credit")
					exactlyOneCredit(t, w, st, uid)
					st.mu.Lock()
					gets, lists := len(st.gets), len(st.lists)
					st.mu.Unlock()
					if gets == 0 || lists != 0 {
						t.Fatalf("lookups by id=%d, lists=%d; want retrieval by the kept id", gets, lists)
					}
					// A webhook arriving after the lookup credited never credits again.
					if code := w.webhook("evt_p3_after", "payment_intent.succeeded", pi); code != http.StatusNoContent {
						t.Fatalf("webhook: %d", code)
					}
					w.webhook("evt_p3_after_2", "payment_intent.succeeded", pi)
					b.mustGet("/billing")
					exactlyOneCredit(t, w, st, uid)
					if att := onlyAttempt(t, w, uid); att.ChargeRef != pi.ID {
						t.Fatalf("charge_ref %q", att.ChargeRef)
					}
				case "succeeded-webhook":
					st.set(func() { pi.Status = "succeeded" })
					w.webhook("evt_p3_ok", "payment_intent.succeeded", pi)
					w.webhook("evt_p3_ok", "payment_intent.succeeded", pi)
					w.webhook("evt_p3_ok_2", "payment_intent.succeeded", pi)
					b.mustGet("/billing") // the lookup after the webhook never credits again
					exactlyOneCredit(t, w, st, uid)
				case "canceled-lookup", "requires_payment_method-lookup":
					st.set(func() { pi.Status = end[:len(end)-len("-lookup")] })
					b.mustGet("/billing")
					failedWithRef(t, w, uid, pi.ID)
				case "canceled-webhook":
					st.set(func() { pi.Status = "canceled" })
					w.webhook("evt_p3_cancel", "payment_intent.canceled", pi)
					failedWithRef(t, w, uid, pi.ID)
				}
				if st.postCount() != 1 {
					t.Fatalf("%d charges, want 1", st.postCount())
				}
			})
		}
	}
}

// F2 (Pass 3): a 402 card_error naming the refused PaymentIntent fails the
// attempt with that intent's id kept; a late success for it still credits
// once, found by that id.
func TestF2P3_DeclineWithIntentKeepsChargeRef(t *testing.T) {
	w, b, st, uid := stripeWorld(t, "decline402@example.com")
	st.set(func() { st.decline = "Your card has insufficient funds." })
	loc := b.postTopup()
	if loc != "/billing?declined=Your+card+has+insufficient+funds." {
		t.Fatalf("402 redirected to %s", loc)
	}
	pi := st.only()
	att := failedWithRef(t, w, uid, pi.ID)
	if att.Reason != "Your card has insufficient funds." {
		t.Fatalf("reason %q", att.Reason)
	}
	// Settled by a webhook matched on the kept id, once.
	st.set(func() { pi.Status = "succeeded" })
	w.webhook("evt_402_late", "payment_intent.succeeded", pi)
	w.webhook("evt_402_late_2", "payment_intent.succeeded", pi)
	exactlyOneCredit(t, w, st, uid)
}

// F2 (Pass 3): the reply was lost, and the list lookup then finds the
// intent canceled or requiring a new payment method. The attempt fails
// with the discovered intent's id kept.
func TestF2P3_ListDiscoveredFailureKeepsChargeRef(t *testing.T) {
	for _, status := range []string{"canceled", "requires_payment_method"} {
		t.Run(status, func(t *testing.T) {
			w, b, st, uid := stripeWorld(t, "lost-"+status+"@example.com")
			st.set(func() { st.lose, st.answer = true, status })
			if loc := b.postTopup(); loc != "/billing?ok=pending" {
				t.Fatalf("lost reply redirected to %s", loc)
			}
			pi := st.only()
			pendingHoldsSlot(t, w, uid, "") // the id was never seen
			b.mustGet("/billing")
			failedWithRef(t, w, uid, pi.ID)
			st.mu.Lock()
			lists := len(st.lists)
			st.mu.Unlock()
			if lists == 0 {
				t.Fatal("the failure was not discovered by a list lookup")
			}
		})
	}
}

// Copy (Pass 3): a top-up that still cannot be confirmed never claims that
// nothing was charged; it says it could not be confirmed yet, that credit
// is added automatically if it went through, and that it holds a slot.
func TestF2P3_UnconfirmedCopyNeverClaimsNothingCharged(t *testing.T) {
	w, b, st, uid := stripeWorld(t, "unconfirmed@example.com")
	st.set(func() { st.lose, st.hidden = true, true })
	form := b.topup
	if loc := b.postTopup(); loc != "/billing?ok=pending" {
		t.Fatalf("lost reply redirected to %s", loc)
	}
	w.advance(app.TopupSettleWindow + time.Second)
	b.topup = form // the same form, resubmitted
	loc := b.postTopup()
	if loc != "/billing?ok=unconfirmed" {
		t.Fatalf("resubmitted unconfirmed form redirected to %s", loc)
	}
	page := b.mustGet(loc)
	has(t, page, "could not confirm that top-up with the card processor yet", "If the charge went through, its credit is added automatically",
		"never charged twice", "counts toward this month")
	hasNot(t, page, "nothing has been charged", "nothing was charged", "no record of that top-up")
	if att := onlyAttempt(t, w, uid); att.Status != store.TopupPending || st.postCount() != 1 {
		t.Fatalf("unconfirmed: %s posts=%d", att.Status, st.postCount())
	}
}

// F2 (Pass 3): an indeterminate error answer that names the intent keeps
// the attempt pending with that id, so the lookup retrieves it directly.
func TestF2P3_IndeterminateErrorNamingIntentIsLookedUpByID(t *testing.T) {
	w, b, st, uid := stripeWorld(t, "apierr@example.com")
	st.set(func() { st.apiErr, st.answer = true, "succeeded" })
	if loc := b.postTopup(); loc != "/billing?ok=pending" {
		t.Fatalf("500 redirected to %s", loc)
	}
	pi := st.only()
	pendingHoldsSlot(t, w, uid, pi.ID)
	has(t, b.mustGet("/billing"), "You also have $4 of top-up credit")
	exactlyOneCredit(t, w, st, uid)
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.gets) == 0 || len(st.lists) != 0 {
		t.Fatalf("gets=%d lists=%d, want retrieval by the named id", len(st.gets), len(st.lists))
	}
}

// F2 (Pass 3): failing an attempt never clears a charge id it already has.
func TestF2P3_FailTopupKeepsExistingChargeRef(t *testing.T) {
	w, b, st, uid := stripeWorld(t, "keepref@example.com")
	st.set(func() { st.answer = "processing" })
	b.postTopup()
	pi := st.only()
	us := w.store.For(uid)
	att := pendingHoldsSlot(t, w, uid, pi.ID)
	if err := us.FailTopup(ctx, att.Token, "", "", w.now()); err != nil {
		t.Fatal(err)
	}
	failedWithRef(t, w, uid, pi.ID)
}
