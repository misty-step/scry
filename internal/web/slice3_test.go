package web_test

// Regression tests for the Slice #3 Cold Critic defects (D2 voice, F2
// top-up idempotency and reconciliation, the SQLite lock across the
// processor call, Billing/Learn edges) and Phaedrus's on-demand
// explanation ruling.

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"scry/internal/ai"
	"scry/internal/app"
	"scry/internal/billing"
	"scry/internal/srs"
	"scry/internal/store"
)

func (w *world) set(t time.Time) {
	w.mu.Lock()
	w.clock = t
	w.mu.Unlock()
}

// wav builds a mono 16 kHz PCM recording of n samples; tone makes it audible.
func wav(n int, tone bool) []byte {
	b := make([]byte, 44+n*2)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], 16000)
	binary.LittleEndian.PutUint32(b[28:], 32000)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(n*2))
	if tone {
		for i := 0; i < n; i++ {
			binary.LittleEndian.PutUint16(b[44+2*i:], uint16(int16(4000*((i%32)-16)/16)))
		}
	}
	return b
}

func (b *browser) speak(data []byte) (int, map[string]any) {
	b.t.Helper()
	req, _ := http.NewRequest("POST", b.w.srv.URL+"/study/speech", strings.NewReader(string(data)))
	req.Header.Set("Content-Type", "audio/wav")
	req.Header.Set("X-CSRF-Token", b.csrf)
	status, body := b.do(req)
	var out map[string]any
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		b.t.Fatalf("speech reply is not JSON (%d): %s", status, body)
	}
	return status, out
}

// D2: starting and immediately stopping (no samples), a recording too short
// for a word, and a silent one are all "didn't catch that": a 200 with no
// text, no provider call and no metering. Malformed and oversized uploads
// keep their own errors, which the page shows as they are.
func TestD2_UncaughtRecordingsAreDidntCatchThat(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("voice@example.com")
	b.addSet("nato-alphabet")
	b.mustGet("/study")
	for name, rec := range map[string][]byte{
		"stopped at once": wav(0, false),
		"too short":       wav(1600, true),   // 0.1 s
		"silent":          wav(32000, false), // 2 s of digital silence
	} {
		status, out := b.speak(rec)
		if status != http.StatusOK || out["text"] != "" || out["caught"] != false || out["error"] != nil {
			t.Errorf("%s: %d %v, want 200 with no text", name, status, out)
		}
	}
	var metered int
	_ = w.store.DB().QueryRow("SELECT count(*) FROM usage WHERE kind='speech'").Scan(&metered)
	if metered != 0 {
		t.Errorf("%d speech calls metered for recordings with nothing in them", metered)
	}
	// A real (audible) recording is still heard and metered.
	if status, out := b.speak(wav(16000, true)); status != 200 || out["text"] == "" {
		t.Errorf("audible recording: %d %v", status, out)
	}
	if status, out := b.speak([]byte("not a recording at all")); status != http.StatusUnsupportedMediaType || out["error"] == "" {
		t.Errorf("garbage upload: %d %v", status, out)
	}
	if status, out := b.speak(wav(16000*121, true)); status != http.StatusRequestEntityTooLarge || !strings.Contains(fmt.Sprint(out["error"]), "too long") {
		t.Errorf("121 s recording: %d %v", status, out)
	}
	// The page shows DESIGN's exact words for an empty result, and no longer
	// keys them to one status code.
	js := b.mustGet("/static/app.js")
	has(t, js, `else say("Didn't catch that. Try again or type it.");`)
	hasNot(t, js, "res.status === 400")
}

// countingPay is the test processor with a ledger of its own: it counts
// charges per idempotency key and remembers what it charged, as Stripe does.
type countingPay struct {
	billing.Fake
	mu      sync.Mutex
	charges map[string]int
	charged map[string]string // key -> ref, for charges that went through
	lose    bool              // charge, then lose the reply
	entered chan struct{}     // signalled when a charge starts
	hold    chan struct{}     // a charge waits for this
	find    *billing.ChargeResult
}

func (p *countingPay) Charge(ctx context.Context, cus, pm string, cents int64, desc, key string) (string, error) {
	if p.entered != nil {
		p.entered <- struct{}{}
	}
	if p.hold != nil {
		<-p.hold
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.charges == nil {
		p.charges, p.charged = map[string]int{}, map[string]string{}
	}
	p.charges[key]++
	ref := "pi_" + strconv.Itoa(len(p.charged)+1)
	if old, ok := p.charged[key]; ok {
		ref = old // the processor's own idempotency
	}
	p.charged[key] = ref
	if p.lose {
		return "", errors.New("card processor unreachable: connection reset")
	}
	return ref, nil
}

func (p *countingPay) FindCharge(_ context.Context, q billing.ChargeQuery) (billing.ChargeResult, error) {
	key := q.Key
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.find != nil {
		return *p.find, nil
	}
	if ref, ok := p.charged[key]; ok {
		return billing.ChargeResult{Status: billing.ChargeSucceeded, Ref: ref}, nil
	}
	return billing.ChargeResult{Status: billing.ChargeNotFound}, nil
}

func (p *countingPay) total() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, c := range p.charges {
		n += c
	}
	return n
}

func paidTopups(t *testing.T, us *store.UserStore) int {
	t.Helper()
	ledger, err := us.Ledger(ctx)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range ledger {
		if e.Kind == "topup" && e.Status == "paid" {
			n++
		}
	}
	return n
}

// F2: two submissions of the same rendered form 0.6 s apart, straddling a
// UTC minute boundary (:59.7 and :00.3), charge once and credit once.
func TestF2_CrossMinuteDuplicateFormChargesOnce(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("crossminute@example.com")
	pay := &countingPay{}
	w.app.Pay = pay
	us := w.lib("crossminute@example.com")
	before, _ := us.Subscription(ctx)

	w.set(time.Date(2026, 11, 17, 9, 0, 59, 700_000_000, time.UTC))
	b.mustGet("/billing")
	form := b.topup
	_, first := b.post("/billing/topup", "topup_token", form)
	has(t, first, "Topped up.")
	w.set(time.Date(2026, 11, 17, 9, 1, 0, 300_000_000, time.UTC))
	_, second := b.post("/billing/topup", "topup_token", form)
	has(t, second, "already processed", "No additional credit")

	after, _ := us.Subscription(ctx)
	if pay.total() != 1 || after.TopupMicros-before.TopupMicros != w.app.Cfg.TopupMicros || paidTopups(t, us) != 1 {
		t.Fatalf("charges=%d credit=%d paid=%d, want one of each", pay.total(), after.TopupMicros-before.TopupMicros, paidTopups(t, us))
	}

	// The same straddle, concurrently, through the app: still one charge.
	u := w.user("crossminute@example.com")
	token := app.NewTopupToken()
	var wg sync.WaitGroup
	results := make(chan bool, 2)
	for _, at := range []time.Time{
		time.Date(2026, 11, 17, 9, 5, 59, 700_000_000, time.UTC),
		time.Date(2026, 11, 17, 9, 6, 0, 300_000_000, time.UTC),
	} {
		w.set(at)
		wg.Add(1)
		go func() {
			defer wg.Done()
			added, err := w.app.TopUpResult(ctx, u.ID, token)
			if err != nil {
				t.Error(err)
			}
			results <- added
		}()
	}
	wg.Wait()
	close(results)
	granted := 0
	for r := range results {
		if r {
			granted++
		}
	}
	if granted != 1 || pay.total() != 2 || paidTopups(t, us) != 2 {
		t.Fatalf("concurrent straddle: granted=%d charges=%d paid=%d", granted, pay.total(), paidTopups(t, us))
	}

	// A token the server never issued is refused before any charge.
	_, page := b.post("/billing/topup", "topup_token", "made-up-token-0123456789.abcdef")
	has(t, page, "That top-up form had expired, so nothing was charged.")
	if pay.total() != 2 {
		t.Fatal("a forged form token was charged")
	}
}

// F2: separate forms are separate, intentional top-ups, up to the $20 UTC
// monthly cap.
func TestF2_DistinctFormsWorkUpToTheMonthlyCap(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("distinct@example.com")
	pay := &countingPay{}
	w.app.Pay = pay
	us := w.lib("distinct@example.com")
	for i := 0; i < 4; i++ {
		b.mustGet("/billing") // each render is a new form
		_, page := b.post("/billing/topup")
		has(t, page, "Topped up.")
		w.advance(time.Second)
	}
	page := b.mustGet("/billing")
	has(t, page, "Monthly $20 top-up limit reached", "disabled")
	b.post("/billing/topup")
	sub, _ := us.Subscription(ctx)
	if pay.total() != 4 || paidTopups(t, us) != 4 || sub.TopupMicros != 4*w.app.Cfg.TopupMicros {
		t.Fatalf("charges=%d paid=%d credit=%d, want 4 top-ups", pay.total(), paidTopups(t, us), sub.TopupMicros)
	}
}

// F2: the processor charged but its reply was lost. The attempt stays
// pending (holding its cap slot), the next Billing view reconciles it with
// the processor and credits exactly once, and resubmitting the form never
// charges again.
func TestF2_LostReplyIsReconciledWithExactlyOneCredit(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("lostreply@example.com")
	pay := &countingPay{lose: true}
	w.app.Pay = pay
	u := w.user("lostreply@example.com")
	us := w.lib("lostreply@example.com")
	before, _ := us.Subscription(ctx)

	b.mustGet("/billing")
	form := b.topup
	nonce := strings.SplitN(form, ".", 2)[0]
	// The charge goes through at the processor; the reply is lost.
	if _, err := w.app.TopUpResult(ctx, u.ID, nonce); !errors.Is(err, app.ErrTopupPending) {
		t.Fatalf("lost reply: %v, want pending", err)
	}
	if n, _ := us.MonthTopups(ctx, w.now()); n != 1 {
		t.Fatalf("pending attempt holds %d cap slots, want 1", n)
	}
	if len(w.alerts.Titles()) == 0 {
		t.Error("an unknown charge outcome was not alerted")
	}
	mid, _ := us.Subscription(ctx)
	if mid.TopupMicros != before.TopupMicros || paidTopups(t, us) != 0 {
		t.Fatal("credit granted before the charge was confirmed")
	}
	pay.mu.Lock()
	pay.lose = false
	pay.mu.Unlock()
	page := b.mustGet("/billing") // reconciles
	has(t, page, "You also have $4 of top-up credit")
	b.mustGet("/billing")
	if added, err := w.app.TopUpResult(ctx, u.ID, nonce); err != nil || added {
		t.Fatal(added, err)
	}
	_, page = b.post("/billing/topup", "topup_token", form)
	has(t, page, "already processed")
	after, _ := us.Subscription(ctx)
	if pay.total() != 1 || after.TopupMicros-before.TopupMicros != w.app.Cfg.TopupMicros || paidTopups(t, us) != 1 {
		t.Fatalf("charges=%d credit=%d paid=%d", pay.total(), after.TopupMicros-before.TopupMicros, paidTopups(t, us))
	}
}

// F2: the app restarted between reserving an attempt and recording the
// processor's answer. A new process over the same database settles it from
// the processor's records: credit once if it succeeded; release (freeing
// the cap slot, no credit) if it failed or never arrived.
func TestF2_RestartMidChargeReconcilesCleanly(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("restart@example.com")
	u := w.user("restart@example.com")
	us := w.lib("restart@example.com")
	reserve := func(token string) {
		t.Helper()
		// The first transaction of TopUp, then the process "died".
		if _, fresh, err := us.ReserveTopup(ctx, store.TopupAttempt{Token: token, Key: fmt.Sprintf("topup-%d-%s", u.ID, token),
			AmountCents: 500, CreditMicros: w.app.Cfg.TopupMicros, Created: w.now()}, nil); err != nil || !fresh {
			t.Fatal(fresh, err)
		}
	}
	credit := func() int64 { s, _ := us.Subscription(ctx); return s.TopupMicros }

	// Succeeded at the processor: credited exactly once, however often reconciled.
	reserve("restart-succeeded-0001")
	pay := &countingPay{find: &billing.ChargeResult{Status: billing.ChargeSucceeded, Ref: "pi_restart_1"}}
	w.app.Pay = pay
	for i := 0; i < 3; i++ {
		if _, err := w.app.ReconcileTopUps(ctx, u.ID); err != nil {
			t.Fatal(err)
		}
	}
	if credit() != w.app.Cfg.TopupMicros || paidTopups(t, us) != 1 || pay.total() != 0 {
		t.Fatalf("succeeded: credit=%d paid=%d charges=%d", credit(), paidTopups(t, us), pay.total())
	}

	// Failed at the processor: released, no credit, the reason is kept.
	reserve("restart-failed-000002")
	pay.find = &billing.ChargeResult{Status: billing.ChargeFailed, Reason: "Your card was declined."}
	if n, _ := us.MonthTopups(ctx, w.now()); n != 2 {
		t.Fatalf("cap slots before release = %d", n)
	}
	_, _ = w.app.ReconcileTopUps(ctx, u.ID)
	if n, _ := us.MonthTopups(ctx, w.now()); n != 1 || credit() != w.app.Cfg.TopupMicros {
		t.Fatalf("failed: slots=%d credit=%d", n, credit())
	}
	att, _ := us.Topup(ctx, "restart-failed-000002")
	if att.Status != store.TopupFailed || att.Reason != "Your card was declined." {
		t.Fatalf("failed attempt: %+v", att)
	}

	// Not visible at the processor: kept pending inside the settle window,
	// and still kept pending (holding its cap slot, never released as
	// "nothing was charged") after it, so a late charge is still credited.
	reserve("restart-notfound-0003")
	pay.find = &billing.ChargeResult{Status: billing.ChargeNotFound}
	_, _ = w.app.ReconcileTopUps(ctx, u.ID)
	if att, _ := us.Topup(ctx, "restart-notfound-0003"); att.Status != store.TopupPending {
		t.Fatalf("settled inside the settle window: %+v", att)
	}
	w.advance(app.TopupSettleWindow + time.Second)
	if err := w.app.ReconcileAllTopUps(ctx); err != nil {
		t.Fatal(err)
	}
	if att, _ := us.Topup(ctx, "restart-notfound-0003"); att.Status != store.TopupPending {
		t.Fatalf("absence after the window was taken as proof: %+v", att)
	}
	if _, err := w.app.TopUpResult(ctx, u.ID, "restart-notfound-0003"); !errors.Is(err, app.ErrTopupUnconfirmed) || pay.total() != 0 {
		t.Fatalf("resubmitted unconfirmed form: %v, charges=%d", err, pay.total())
	}
	// The processor shows the charge late: credited exactly once.
	pay.find = &billing.ChargeResult{Status: billing.ChargeSucceeded, Ref: "pi_restart_late"}
	_, _ = w.app.ReconcileTopUps(ctx, u.ID)
	_, _ = w.app.ReconcileTopUps(ctx, u.ID)
	if credit() != 2*w.app.Cfg.TopupMicros || paidTopups(t, us) != 2 || pay.total() != 0 {
		t.Fatalf("final: credit=%d paid=%d charges=%d", credit(), paidTopups(t, us), pay.total())
	}
	// Resubmitting the settled form never charges and adds nothing.
	if added, err := w.app.TopUpResult(ctx, u.ID, "restart-notfound-0003"); added || err != nil || pay.total() != 0 {
		t.Fatalf("resubmitted settled form: %v %v, charges=%d", added, err, pay.total())
	}
}

// F2: a declined card releases the reservation and records why.
// F2: the browser sees the pending state when the reply is lost and the
// lookup is not possible yet, then the credit once it is reconciled.
func TestF2_PendingTopupIsShownThenSettled(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("pendingview@example.com")
	pay := &countingPay{lose: true, find: &billing.ChargeResult{Status: billing.ChargePending}}
	w.app.Pay = pay
	b.mustGet("/billing")
	_, page := b.post("/billing/topup")
	has(t, page, "being confirmed with the card processor", "charged at most once")
	hasNot(t, page, "You also have $4")
	pay.mu.Lock()
	pay.find = nil // the processor now reports the charge succeeded
	pay.mu.Unlock()
	has(t, b.mustGet("/billing?ok=pending"), "Topped up.", "You also have $4 of top-up credit")
	if pay.total() != 1 {
		t.Fatalf("charges = %d", pay.total())
	}
}

func TestF2_DeclineReleasesTheReservation(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("decline@example.com")
	u := w.user("decline@example.com")
	us := w.lib("decline@example.com")
	s, _ := us.Subscription(ctx)
	s.PaymentRef = "tok_test_chargefail_Visa_0341"
	_ = us.SaveSubscription(ctx, s)
	var d *billing.DeclineError
	if _, err := w.app.TopUpResult(ctx, u.ID, app.NewTopupToken()); !errors.As(err, &d) {
		t.Fatal(err)
	}
	if n, _ := us.MonthTopups(ctx, w.now()); n != 0 {
		t.Fatalf("declined top-up still holds %d slots", n)
	}
	sub, _ := us.Subscription(ctx)
	if sub.CardError != "Your card was declined." || sub.TopupMicros != 0 {
		t.Fatalf("%+v", sub)
	}
}

// D3: the SQLite write lock is never held across the processor call, so a
// slow charge cannot block /healthz or other learners' reads and writes.
func TestD3_SlowChargeDoesNotBlockHealthOrTheDatabase(t *testing.T) {
	w := newWorld(t)
	w.browser().signUp("slow@example.com")
	other := w.browser()
	other.signUp("bystander@example.com")
	u := w.user("slow@example.com")
	pay := &countingPay{entered: make(chan struct{}, 1), hold: make(chan struct{})}
	w.app.Pay = pay
	done := make(chan error, 1)
	go func() { _, err := w.app.TopUpResult(ctx, u.ID, app.NewTopupToken()); done <- err }()
	<-pay.entered // the charge is now in flight, and stays there

	start := time.Now()
	resp, err := http.Get(w.srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	took := time.Since(start)
	t.Logf("GET /healthz during an in-flight charge: %d in %v", resp.StatusCode, took)
	if resp.StatusCode != 200 || took > time.Second {
		t.Fatalf("/healthz blocked by the charge: %d after %v", resp.StatusCode, took)
	}
	// A write transaction and a page for someone else go straight through.
	start = time.Now()
	if _, err := w.lib("bystander@example.com").ChangeSubscription(ctx, func(s *store.Subscription) { s.CardError = "" }); err != nil {
		t.Fatal(err)
	}
	other.mustGet("/billing")
	if took := time.Since(start); took > time.Second {
		t.Fatalf("database writes blocked by the charge for %v", took)
	}
	select {
	case err := <-done:
		t.Fatalf("top-up finished while its charge was held: %v", err)
	default:
	}
	close(pay.hold)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if sub, _ := w.lib("slow@example.com").Subscription(ctx); sub.TopupMicros != w.app.Cfg.TopupMicros {
		t.Fatalf("credit after the slow charge: %d", sub.TopupMicros)
	}
}

// D4: an ended plan with leftover top-up credit. /learn sends the learner
// to Billing, so Billing must not claim drafts can be reserved.
func TestD4_EndedPlanWithCreditBillingAgreesWithLearn(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("ended@example.com")
	us := w.lib("ended@example.com")
	if err := w.app.TopUp(ctx, w.user("ended@example.com").ID, app.NewTopupToken()); err != nil {
		t.Fatal(err)
	}
	s, _ := us.Subscription(ctx)
	s.Status = "ended"
	_ = us.SaveSubscription(ctx, s)

	b.get("/learn")
	if b.path != "/billing" {
		t.Fatalf("/learn with an ended plan ended at %s", b.path)
	}
	page := b.mustGet("/billing")
	hasNot(t, page, "can reserve", "drafts at the worst-case", "Top up · $5")
	has(t, page, "need an active plan", "Your $4 of top-up credit is kept and becomes usable again when you restart your plan.", "Start again")
}

// D4: on a clock in UTC+2 just after local midnight on December 1 it is
// still November in UTC, where the learner already has four top-ups.
// Billing and App.TopUp both count UTC months, so both say capped; and
// after the UTC rollover both allow it again.
func TestD4_NonUTCClockMonthCapAgreesWithTopUp(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("zone@example.com")
	u := w.user("zone@example.com")
	for i := 0; i < 4; i++ {
		if err := w.app.TopUp(ctx, u.ID, app.NewTopupToken()); err != nil {
			t.Fatal(err)
		}
	}
	plus2 := time.FixedZone("UTC+2", 2*3600)
	w.set(time.Date(2026, 12, 1, 1, 54, 0, 0, plus2)) // = Nov 30 23:54 UTC
	page := b.mustGet("/billing")
	has(t, page, "Monthly $20 top-up limit reached. Resets December 1, 2026 UTC.", "disabled")
	if err := w.app.TopUp(ctx, u.ID, app.NewTopupToken()); !errors.Is(err, store.ErrTopupCap) {
		t.Fatalf("App.TopUp at Nov 30 23:54 UTC: %v, want the cap", err)
	}
	w.set(time.Date(2026, 12, 1, 2, 0, 30, 0, plus2)) // = Dec 1 00:00:30 UTC
	page = b.mustGet("/billing")
	hasNot(t, page, "Monthly $20 top-up limit reached")
	b.post("/billing/topup")
	if n := paidTopups(t, w.lib("zone@example.com")); n != 5 {
		t.Fatalf("after the UTC rollover: %d paid top-ups, want 5", n)
	}
}

// explainWorld is a learner looking at a scripted judgment.
func explainWorld(t *testing.T, email string, v srs.Verdict) (*world, *browser, store.Review, *ai.FakeExplainer) {
	t.Helper()
	w := newWorld(t)
	ex := &ai.FakeExplainer{}
	w.app.Explainer = ex
	b := w.browser()
	b.signUp(email)
	b.addSet("nato-alphabet")
	it, _ := b.current(b.mustGet("/study"))
	w.model.script = []ai.Judgment{{Verdict: v, Reason: ai.CriterionSentence(v)}}
	page := b.answer(it, "something else")
	has(t, page, ai.CriterionSentence(v))
	if v != srs.Right {
		has(t, page, `data-explain`, "Why?")
	}
	hist, _ := w.lib(email).History(ctx, it.ID)
	if len(hist) != 1 {
		t.Fatalf("%d reviews", len(hist))
	}
	return w, b, hist[0], ex
}

func (b *browser) explain(id int64, fetch bool, kv ...string) (int, string) {
	b.t.Helper()
	form := url.Values{"csrf": {b.csrf}, "review_id": {strconv.FormatInt(id, 10)}}
	for i := 0; i+1 < len(kv); i += 2 {
		form.Set(kv[i], kv[i+1])
	}
	req, _ := http.NewRequest("POST", b.w.srv.URL+"/study/explain", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if fetch {
		req.Header.Set("X-Requested-With", "fetch")
		req.Header.Set("Accept", "application/json")
	}
	return b.do(req)
}

// Ruling: the hot grading path makes no explainer call; "Why?" on a non-
// exact verdict makes one bounded call after the verdict, which says what
// differed and never touches the grade, review row or schedule.
func TestExplain_OnDemandAfterTheVerdictNeverChangesIt(t *testing.T) {
	for _, v := range []srs.Verdict{srs.Missed, srs.Close} {
		w, b, rv, ex := explainWorld(t, "why-"+string(v)+"@example.com", v)
		if ex.Calls() != 0 {
			t.Fatalf("%s: the hot path called the explainer %d times", v, ex.Calls())
		}
		us := w.lib("why-" + string(v) + "@example.com")
		item, _ := us.Item(ctx, rv.ItemID)
		sess, _ := us.ActiveSession(ctx)

		status, body := b.explain(rv.ID, true)
		var out struct {
			Text     string
			Fallback bool
		}
		_ = json.Unmarshal([]byte(body), &out)
		if status != 200 || out.Fallback || !strings.HasPrefix(out.Text, "What differed: you answered “something else”") || len([]rune(out.Text)) > ai.ExplainMaxChars {
			t.Fatalf("%s: %d %s", v, status, body)
		}
		if ex.Calls() != 1 {
			t.Fatalf("%s: explainer calls = %d", v, ex.Calls())
		}
		b.explain(rv.ID, true) // a second click is served from the cache
		if ex.Calls() != 1 {
			t.Fatalf("%s: repeated click called the explainer again", v)
		}
		after, _ := us.Review(ctx, rv.ID)
		itemAfter, _ := us.Item(ctx, rv.ItemID)
		sessAfter, _ := us.ActiveSession(ctx)
		hist, _ := us.History(ctx, rv.ItemID)
		if !reflect.DeepEqual(after, rv) || !reflect.DeepEqual(itemAfter.Sched, item.Sched) || len(hist) != 1 ||
			sessAfter.PendingReviewID != sess.PendingReviewID {
			t.Fatalf("%s: explaining changed the record:\n%+v\n%+v", v, rv, after)
		}
		// Without JavaScript the judgment is re-rendered with the explanation.
		status, page := b.explain(rv.ID, false)
		if status != 200 {
			t.Fatal(status)
		}
		has(t, page, "What differed: you answered “something else”", ai.CriterionSentence(v))
		hasNot(t, page, "data-explain aria-controls")
	}
}

func TestExplain_GuardsAuthOwnershipCSRFAndKind(t *testing.T) {
	w, b, rv, ex := explainWorld(t, "owner@example.com", srs.Missed)
	// Another learner cannot have someone else's answer explained.
	eve := w.browser()
	eve.signUp("eve@example.com")
	eve.addSet("nato-alphabet")
	eve.mustGet("/study")
	if status, _ := eve.explain(rv.ID, true); status != http.StatusNotFound {
		t.Fatalf("other learner: %d", status)
	}
	// A missing or wrong CSRF token is refused.
	if status, _ := b.explain(rv.ID, true, "csrf", "wrong"); status != http.StatusForbidden {
		t.Fatalf("bad csrf: %d", status)
	}
	// Signed out is sent to sign in.
	req, _ := http.NewRequest("POST", w.srv.URL+"/study/explain", strings.NewReader("review_id="+strconv.FormatInt(rv.ID, 10)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), "/signin") {
		t.Fatalf("signed out: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	if ex.Calls() != 0 {
		t.Fatalf("refused requests reached the explainer %d times", ex.Calls())
	}
	// A right answer has nothing to explain on demand.
	b.next()
	it, _ := b.current(b.mustGet("/study"))
	w.model.script = []ai.Judgment{{Verdict: srs.Right, Reason: ai.CriterionSentence(srs.Right)}}
	page := b.answer(it, it.Answer)
	hasNot(t, page, "data-explain")
	hist, _ := w.lib("owner@example.com").History(ctx, it.ID)
	if status, _ := b.explain(hist[0].ID, true); status != http.StatusUnprocessableEntity {
		t.Fatalf("right answer: %d", status)
	}
}

// Exact texts are judged word for word with their diff in place: no "Why?".
func TestExplain_NotOfferedOnExactTexts(t *testing.T) {
	w := newWorld(t)
	w.app.Explainer = &ai.FakeExplainer{}
	b := w.browser()
	b.signUp("exactwhy@example.com")
	b.mustGet("/library/texts/new")
	b.post("/library/texts", "title", "Key", "text", "Remember this line.")
	it, _ := b.current(b.mustGet("/study"))
	page := b.answer(it, "wrong")
	has(t, page, "Exact text")
	hasNot(t, page, "data-explain")
	hist, _ := w.lib("exactwhy@example.com").History(ctx, it.ID)
	if status, _ := b.explain(hist[0].ID, true); status != http.StatusUnprocessableEntity {
		t.Fatalf("exact text: %d", status)
	}
}

// Provider error, timeout, an unconfigured explainer and an exhausted
// allowance all fall back to the fixed criterion sentence: never a 500.
func TestExplain_FallsBackToTheCriterionSentence(t *testing.T) {
	cases := map[string]func(w *world){
		"error": func(w *world) { w.app.Explainer = &ai.FakeExplainer{Mode: "fail"} },
		"timeout": func(w *world) {
			w.app.Explainer = &ai.FakeExplainer{Mode: "slow"}
			w.app.Cfg.ExplainTimeout = 150 * time.Millisecond
		},
		"unconfigured": func(w *world) { w.app.Explainer = nil },
		"allowance": func(w *world) {
			us := w.lib("fallback-allowance@example.com")
			s, _ := us.Subscription(ctx)
			s.UsedMicros, s.TopupMicros = s.AllowanceMicros, 0
			_ = us.SaveSubscription(ctx, s)
		},
	}
	for name, setup := range cases {
		w, b, rv, _ := explainWorld(t, "fallback-"+name+"@example.com", srs.Missed)
		setup(w)
		start := time.Now()
		status, body := b.explain(rv.ID, true)
		var out struct {
			Text     string
			Fallback bool
		}
		_ = json.Unmarshal([]byte(body), &out)
		if status != 200 || !out.Fallback || out.Text != ai.CriterionSentence(srs.Missed) {
			t.Errorf("%s: %d %s", name, status, body)
		}
		if time.Since(start) > 3*time.Second {
			t.Errorf("%s: fallback took %v", name, time.Since(start))
		}
		if after, _ := w.lib("fallback-"+name+"@example.com").Review(ctx, rv.ID); !reflect.DeepEqual(after, rv) {
			t.Errorf("%s: review changed", name)
		}
		_, page := b.explain(rv.ID, false)
		has(t, page, "A fuller explanation isn't available right now.")
		if name != "allowance" {
			if res, _ := w.lib("fallback-" + name + "@example.com").ReservedMicros(ctx); res != 0 {
				t.Errorf("%s: %d micros left reserved", name, res)
			}
		}
	}
}
