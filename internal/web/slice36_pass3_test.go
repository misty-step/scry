package web_test

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"scry/internal/billing"
	"scry/internal/store"
)

// heldPay is the test processor with its Subscribe held open.
type heldPay struct {
	billing.Fake
	mu      sync.Mutex
	calls   int
	entered chan struct{}
	release chan struct{}
}

func (p *heldPay) Subscribe(ctx context.Context, sp billing.SubscribeParams) (billing.Subscribed, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	p.entered <- struct{}{}
	<-p.release
	return p.Fake.Subscribe(ctx, sp)
}

// Slice 36 pass 3: a confirmed upgrade submitted while the learner's first
// one is still with the processor, in the next minute, is answered 409
// with a calm sentence and no form to submit again. Nothing is sent for
// it; the first is charged once. Both card steps answer alike.
func TestSlice36_SecondUpgradeWhileOneIsInFlightIsACalmConflict(t *testing.T) {
	for _, path := range []string{"/signup/card", "/billing/card"} {
		t.Run(path, func(t *testing.T) {
			w := newWorld(t)
			pay := &heldPay{entered: make(chan struct{}, 8), release: make(chan struct{})}
			w.app.Pay = pay
			release := sync.OnceFunc(func() { close(pay.release) })
			t.Cleanup(release)
			b := w.browser()
			b.signUpNoCard("twice@example.com")
			lib := w.lib("twice@example.com")
			before, _ := lib.Subscription(ctx)
			w.advance(before.PeriodEnd.Sub(w.now()) - 30*time.Minute - time.Second) // second 59
			b.mustGet(path)
			if b.terms != "immediate" {
				t.Fatalf("card step carries %q", b.terms)
			}
			second := &browser{t: t, w: w, c: b.c, csrf: b.csrf, terms: b.terms} // the same learner's other tab
			first := make(chan int, 1)
			go func() {
				status, _ := b.post(path, "number", goodCard, "expiry", "12/30", "cvc", "123")
				first <- status
			}()
			<-pay.entered
			w.advance(2 * time.Second)
			status, page := second.post(path, "number", goodCard, "expiry", "12/30", "cvc", "123")
			if status != http.StatusConflict {
				t.Fatalf("second upgrade gave %d", status)
			}
			has(t, page, "Your card is already being processed. Refresh Billing in a moment — nothing extra was charged.", `href="/billing"`)
			hasNot(t, page, "data-card-form", `name="number"`)
			if got, _ := lib.Subscription(ctx); got != before {
				t.Fatalf("the refused request changed the plan: %+v", got)
			}
			release()
			if status := <-first; status != http.StatusOK {
				t.Fatalf("first upgrade gave %d", status)
			}
			sub, _ := lib.Subscription(ctx)
			led, _ := lib.Ledger(ctx)
			if pay.calls != 1 || sub.Status != store.SubActive || len(led) != 2 || led[0].Kind != "charge" || led[0].AmountCents != 500 {
				t.Fatalf("%d calls, plan %+v, ledger %+v", pay.calls, sub, led)
			}
		})
	}
}

// hookPay is the test processor running a hook once, inside Tokenize: after
// the card handler has read the plan, before it acts on it.
type hookPay struct {
	billing.Fake
	mu         sync.Mutex
	subscribes int
	hook       func()
}

func (p *hookPay) Tokenize(number, expiry, cvc string, now time.Time) (string, error) {
	p.mu.Lock()
	hook := p.hook
	p.hook = nil
	p.mu.Unlock()
	if hook != nil {
		hook()
	}
	return p.Fake.Tokenize(number, expiry, cvc, now)
}

func (p *hookPay) Subscribe(ctx context.Context, sp billing.SubscribeParams) (billing.Subscribed, error) {
	p.mu.Lock()
	p.subscribes++
	p.mu.Unlock()
	return p.Fake.Subscribe(ctx, sp)
}

// Slice 36 pass 3: a lapsed card-backed plan restarted by two requests at
// once: one has read the lapsed plan when the other restarts it and is
// charged. The first must not write the lapsed plan back over the paid one
// and then charge again.
func TestSlice36_RestartOfALapsedPlanNeverOverwritesThePaidOne(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("lapsed@example.com")
	lib := w.lib("lapsed@example.com")
	if _, err := lib.ChangeSubscription(ctx, func(s *store.Subscription) {
		s.Status, s.PastDueSince, s.PeriodEnd = store.SubPastDue, w.now().AddDate(0, 0, -8), w.now().AddDate(0, 1, 0)
	}); err != nil {
		t.Fatal(err)
	}
	pay := &hookPay{}
	w.app.Pay = pay
	b.mustGet("/billing/card")
	other := &browser{t: t, w: w, c: b.c, csrf: b.csrf, terms: b.terms}
	pay.hook = func() {
		if status, _ := other.post("/billing/card", "number", goodCard, "expiry", "12/30", "cvc", "123"); status != http.StatusOK {
			t.Errorf("the other restart gave %d", status)
		}
		if sub, _ := lib.Subscription(ctx); sub.Status != store.SubActive {
			t.Errorf("the other restart left %+v", sub)
		}
	}
	if status, _ := b.post("/billing/card", "number", goodCard, "expiry", "12/30", "cvc", "123"); status != http.StatusOK {
		t.Fatalf("restart gave %d", status)
	}
	if sub, _ := lib.Subscription(ctx); pay.subscribes != 1 || sub.Status != store.SubActive {
		t.Fatalf("%d subscriptions, plan %+v", pay.subscribes, sub)
	}
}
