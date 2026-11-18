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
