package web_test

import (
	"context"
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
