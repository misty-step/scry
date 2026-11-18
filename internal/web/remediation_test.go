package web_test

import (
	"context"
	"scry/internal/ai"
	"scry/internal/billing"
	"sync"
	"testing"
)

type idempotentPay struct {
	billing.Fake
	mu   sync.Mutex
	refs map[string]string
}

func (p *idempotentPay) Charge(_ context.Context, _, _ string, _ int64, _, key string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.refs == nil {
		p.refs = map[string]string{}
	}
	p.refs[key] = "paid-" + key
	return p.refs[key], nil
}
func TestG1_TopUpCreditsOncePerPayment(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("credit@example.com")
	w.app.Pay = &idempotentPay{}
	u := w.user("credit@example.com")
	us := w.store.For(u.ID)
	before, _ := us.Subscription(ctx)
	for i := 0; i < 3; i++ {
		if err := w.app.TopUp(ctx, u.ID); err != nil {
			t.Fatal(err)
		}
	}
	after, _ := us.Subscription(ctx)
	if after.TopupMicros-before.TopupMicros != w.app.Cfg.TopupMicros {
		t.Fatalf("unpaid credit: %d", after.TopupMicros-before.TopupMicros)
	}
}

func TestG2_ConcurrentDraftsCannotSpendLastMicro(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("cap@example.com")
	u := w.user("cap@example.com")
	us := w.store.For(u.ID)
	s, _ := us.Subscription(ctx)
	s.UsedMicros = s.AllowanceMicros - 1
	if err := us.SaveSubscription(ctx, s); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = w.app.Learn(ctx, u, "physics") }()
	}
	wg.Wait()
	w.app.WaitDrafts()
	w.model.mu.Lock()
	defer w.model.mu.Unlock()
	if len(w.model.drafts) != 0 {
		t.Fatalf("unreserved provider calls: %d", len(w.model.drafts))
	}
}

type heldModel struct {
	*spyModel
	started chan struct{}
	release chan struct{}
}

func (m heldModel) Draft(c context.Context, in ai.DraftInput, sink ai.DraftSink) (ai.DraftResult, ai.Usage, error) {
	select {
	case m.started <- struct{}{}:
	default:
	}
	<-m.release
	return m.spyModel.Draft(c, in, sink)
}
func TestG2_ReservationHeldUntilProviderCompletes(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("held@example.com")
	u := w.user("held@example.com")
	started, release := make(chan struct{}, 25), make(chan struct{})
	w.app.Model = heldModel{w.model, started, release}
	_, err := w.app.Learn(ctx, u, "physics")
	if err != nil {
		t.Fatal(err)
	}
	<-started
	for i := 0; i < 24; i++ {
		if _, err := w.app.Learn(ctx, u, "physics"); err != nil {
			t.Fatal(err)
		}
	}
	if len(started) != 0 {
		t.Fatal("concurrent provider entered while budget held")
	}
	close(release)
	w.app.WaitDrafts()
	var held int
	if err := w.store.DB().QueryRow("SELECT count(*) FROM reservations").Scan(&held); err != nil || held != 0 {
		t.Fatalf("unsettled: %d %v", held, err)
	}
	sub, _ := w.store.For(u.ID).Subscription(ctx)
	if sub.UsedMicros <= 0 || sub.UsedMicros >= ai.DraftBudgetMicros {
		t.Fatalf("actual usage not settled: %+v", sub)
	}
}
