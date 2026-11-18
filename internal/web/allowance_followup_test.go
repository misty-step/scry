package web_test

import (
	"strings"
	"testing"
)

func TestF1_TrialDraftRefusalAgreesWithBilling(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("draftcap@example.com")
	for i := 0; i < 40; i++ {
		b.get("/learn")
		b.post("/learn", "q", "Meditations")
		path := b.path
		w.app.WaitDrafts()
		body := b.mustGet(path)
		if strings.Contains(body, "can't draft") || strings.Contains(body, "Not enough allowance") {
			has(t, body, "Not enough allowance left for a draft")
			has(t, b.mustGet("/learn"), "Not enough allowance left for a draft")
			has(t, b.mustGet("/billing"), "Not enough allowance left for a draft", "0 drafts")
			return
		}
	}
	t.Fatal("never refused")
}

func TestF2_RepeatedTopupIsTruthful(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("topuprepeat@example.com")
	b.get("/billing")
	_, body := b.post("/billing/topup")
	has(t, body, "Topped up.")
	before, _ := w.lib("topuprepeat@example.com").Subscription(ctx)
	_, body = b.post("/billing/topup")
	has(t, body, "already processed", "No additional credit")
	hasNot(t, body, "Topped up.")
	after, _ := w.lib("topuprepeat@example.com").Subscription(ctx)
	if before.TopupMicros != after.TopupMicros {
		t.Fatal("duplicate credit")
	}
}

func TestF2_ConcurrentTopupsReportOneCredit(t *testing.T) {
	w := newWorld(t)
	w.browser().signUp("concurrentcredit@example.com")
	u := w.user("concurrentcredit@example.com")
	results := make(chan bool, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() { added, err := w.app.TopUpResult(ctx, u.ID); results <- added; errs <- err }()
	}
	count := 0
	for i := 0; i < 8; i++ {
		if <-results {
			count++
		}
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
	if count != 1 {
		t.Fatalf("reported %d credits, want 1", count)
	}
}
