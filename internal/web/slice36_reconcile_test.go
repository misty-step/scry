package web_test

import (
	"encoding/json"
	"testing"
	"time"

	"scry/internal/ai"
	"scry/internal/auth"
	"scry/internal/srs"
	"scry/internal/store"
)

// Story (trial) reconciliation: the Clerk sign-up page, the one production
// serves, says the trial needs no card, and the sign-in page does not.
func TestSlice36Reconcile_ClerkSignUpSaysNoCardNeeded(t *testing.T) {
	w := newWorld(t)
	w.app.Auth = &auth.Clerk{PublishableKey: "pk_test_ZXhhbXBsZS5jbGVyay5hY2NvdW50cy5kZXYk", SecretKey: "stand-in-only"}
	b := w.browser()
	has(t, b.mustGet("/signup"), "7-day trial · no card needed")
	hasNot(t, b.mustGet("/signin"), "no card needed")
	w.app.Auth = &auth.Clerk{}
	hasNot(t, b.mustGet("/signup"), "no card needed")
}

// Rule (trial) reconciliation: both card steps say DESIGN.md's sentence,
// inside the trial and after it.
func TestSlice36Reconcile_BothCardStepsSayTheSameTerms(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUpNoCard("terms@example.com")
	sub, _ := w.lib("terms@example.com").Subscription(ctx)
	end := sub.PeriodEnd.Format("January 2")
	for _, path := range []string{"/signup/card", "/billing/card"} {
		has(t, b.mustGet(path), "Nothing is charged today: your trial still ends on "+end+", when your card is first charged $5. Cancel any time before then in Billing.", "Add card · nothing charged today")
	}
	has(t, b.mustGet("/signup/card"), "Not now, back to studying")
	w.advance(sub.PeriodEnd.Sub(w.now()) + time.Minute)
	for _, path := range []string{"/signup/card", "/billing/card"} {
		page := b.mustGet(path)
		has(t, page, "Your 7-day trial ended on "+end+". Add a card to keep studying: $5 today, and then monthly.", "Start · $5 a month")
		// Studying is over until a card is added.
		hasNot(t, page, "back to studying")
	}
}

// Rule (trial) reconciliation: Billing never promises "nothing charged
// today" when adding a card would charge today (the trial's last hour).
func TestSlice36Reconcile_BillingInTheLastHourSaysACardChargesToday(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUpNoCard("hour@example.com")
	sub, _ := w.lib("hour@example.com").Subscription(ctx)
	w.advance(sub.PeriodEnd.Sub(w.now()) - time.Hour)
	has(t, b.mustGet("/billing"), "Add a card · nothing charged today", "first charged when the trial ends")
	w.advance(time.Second)
	page := b.mustGet("/billing")
	has(t, page, "ends within the hour", "$5 a month, charged today", "Add a card · $5 a month", "Trial · last day")
	hasNot(t, page, "nothing charged today", "first charged when the trial ends", "Cancel plan")
}

// Story (trial) reconciliation: an explanation the trial's allowance cannot
// cover says so and where to add a card; a draft that does not fit never
// says the allowance is used up when it is not.
func TestSlice36Reconcile_OutOfAllowanceSaysWhereToAddACard(t *testing.T) {
	w, b, rv, _ := explainWorld(t, "card-backed@example.com", srs.Missed)
	spend := func(email string, left int64) {
		t.Helper()
		if _, err := w.lib(email).ChangeSubscription(ctx, func(s *store.Subscription) { s.UsedMicros, s.TopupMicros = s.AllowanceMicros-left, 0 }); err != nil {
			t.Fatal(err)
		}
	}
	explained := func(id int64) (allowance bool) {
		t.Helper()
		status, body := b.explain(id, true)
		var out struct {
			Text                string
			Fallback, Allowance bool
		}
		if err := json.Unmarshal([]byte(body), &out); err != nil || status != 200 || !out.Fallback || out.Text != ai.CriterionSentence(srs.Missed) {
			t.Fatalf("explain: %d %s %v", status, body, err)
		}
		return out.Allowance
	}
	// A card-backed plan out of allowance keeps the plain fallback.
	spend("card-backed@example.com", 0)
	if explained(rv.ID) {
		t.Fatal("a card-backed plan was told about the trial's allowance")
	}

	// On a no-card trial the same refusal names the allowance and the way on.
	w = newWorld(t)
	w.app.Model = &ai.Fake{Verdicts: []srs.Verdict{srs.Missed}}
	w.app.Explainer = &ai.FakeExplainer{}
	b = w.browser()
	b.signUpNoCard("why@example.com")
	b.addSet("kitchen-numbers")
	item, ok := b.current(b.mustGet("/study"))
	if !ok {
		t.Fatal("nothing to study")
	}
	b.answer(item, "wrong")
	var id int64
	if sess, err := w.lib("why@example.com").ActiveSession(ctx); err != nil || sess == nil || sess.PendingReviewID == 0 {
		t.Fatalf("no judged answer: %v %v", sess, err)
	} else {
		id = sess.PendingReviewID
	}
	spend("why@example.com", 0)
	if !explained(id) {
		t.Fatal("the refusal did not say the trial's allowance ran out")
	}
	_, page := b.explain(id, false)
	has(t, page, `data-explain-allowance>There isn't enough`, `href="/signup/card">Add a card to keep going</a>`, "data-explain-note hidden>")

	has(t, b.mustGet("/billing"), "judging, speaking, explanations and drafting are paused. Add a card to keep going.", "100% of the trial's allowance used")
	hasNot(t, b.mustGet("/billing"), "Smaller operations may still fit")

	spend("why@example.com", 100_000) // 80% used: no room for a draft
	b.mustGet("/learn")
	_, page = b.post("/learn", "q", "volcanoes")
	has(t, page, "enough of the trial", `href="/signup/card">Add a card to keep going</a>`)
	hasNot(t, page, "used up")
}
