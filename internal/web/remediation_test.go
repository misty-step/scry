package web_test

import (
	"context"
	"encoding/binary"
	"errors"
	"scry/internal/ai"
	"scry/internal/app"
	"scry/internal/billing"
	"scry/internal/speech"
	"scry/internal/store"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
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
	token := app.NewTopupToken() // the same form, resubmitted
	for i := 0; i < 3; i++ {
		if err := w.app.TopUp(ctx, u.ID, token); err != nil {
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

func TestG5_FailedEventCanBeRedelivered(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("event@example.com")
	u := w.user("event@example.com")
	us := w.store.For(u.ID)
	sub, _ := us.Subscription(ctx)
	ev := billing.Event{ID: "retryable", Kind: billing.InvoicePaid, SubscriptionRef: sub.SubscriptionRef, At: w.now(), PeriodStart: w.now(), PeriodEnd: w.now().AddDate(0, 1, 0), AmountCents: 500}
	_, err := w.store.DB().Exec("CREATE TRIGGER fail_event BEFORE UPDATE ON subscriptions BEGIN SELECT RAISE(FAIL,'temporary failure'); END")
	if err != nil {
		t.Fatal(err)
	}
	if err = w.app.ApplyEvent(ctx, ev); err == nil {
		t.Fatal("expected write failure")
	}
	_, err = w.store.DB().Exec("DROP TRIGGER fail_event")
	if err != nil {
		t.Fatal(err)
	}
	if err = w.app.ApplyEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}
	sub, _ = us.Subscription(ctx)
	if sub.Status != "active" {
		t.Fatalf("redelivery lost: %s", sub.Status)
	}
	if err = w.app.ApplyEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = w.store.DB().QueryRow("SELECT count(*) FROM ledger WHERE ref='retryable'").Scan(&n)
	if n != 1 {
		t.Fatalf("ledger effects: %d", n)
	}
}

func TestG10_EmptyAnswerCallsJudgeOnce(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("empty@example.com")
	b.addSet("nato-alphabet")
	it, _ := b.current(b.mustGet("/study"))
	before := w.model.judgeCalls()
	b.answer(it, "")
	if w.model.judgeCalls() != before+1 {
		t.Fatal("empty answer bypassed judge")
	}
}

func wavSecond() []byte {
	b := make([]byte, 44+48000*2)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], 48000)
	binary.LittleEndian.PutUint32(b[28:], 96000)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], 96000)
	// A quiet tone, not digital silence: silence is "didn't catch that".
	for i := 44; i+1 < len(b); i += 2 {
		binary.LittleEndian.PutUint16(b[i:], uint16(int16(1000*((i/2)%40-20)/20)))
	}
	return b
}
func TestG12_SpeechDurationComesFromAudio(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("speech@example.com")
	u := w.user("speech@example.com")
	us := w.store.For(u.ID)
	before, _ := us.Subscription(ctx)
	if _, err := w.app.Transcribe(ctx, u, speech.Audio{Data: wavSecond(), MIME: "audio/wav", Seconds: 0.01}); err != nil {
		t.Fatal(err)
	}
	after, _ := us.Subscription(ctx)
	if after.UsedMicros-before.UsedMicros != 50 {
		t.Fatalf("client controlled metering: %d", after.UsedMicros-before.UsedMicros)
	}
}

type meteringPay struct {
	billing.Fake
	during func()
}

func (p meteringPay) SetCancelAtPeriodEnd(context.Context, string, bool) error {
	p.during()
	return nil
}
func TestG16_CancelCannotOverwriteConcurrentUsage(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("atomic@example.com")
	u := w.user("atomic@example.com")
	us := w.store.For(u.ID)
	w.app.Pay = meteringPay{during: func() {
		if err := us.Spend(ctx, store.Usage{Kind: "judge", Model: "test", CostMicros: 73}, w.now()); err != nil {
			t.Fatal(err)
		}
	}}
	if _, err := w.app.SetCancel(ctx, u.ID, true); err != nil {
		t.Fatal(err)
	}
	sub, _ := us.Subscription(ctx)
	if sub.UsedMicros != 73 || !sub.CancelAtEnd {
		t.Fatalf("lost concurrent update: %+v", sub)
	}
}

func TestG17_LongAnswerNeverSplitsRune(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("rune@example.com")
	b.addSet("nato-alphabet")
	it, _ := b.current(b.mustGet("/study"))
	b.answer(it, strings.Repeat("a", 1999)+"🙂")
	w.model.mu.Lock()
	defer w.model.mu.Unlock()
	if len(w.model.judged) != 1 {
		t.Fatal("missing model call")
	}
	got := w.model.judged[0].Answer
	if !utf8.ValidString(got) || len(got) > 2000 {
		t.Fatalf("invalid UTF-8 answer: %q", got[len(got)-8:])
	}
}

func TestG2_UnknownProviderOutcomeKeepsReservation(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("uncertain@example.com")
	u := w.user("uncertain@example.com")
	w.model.err = errors.New("connection lost after request")
	if _, err := w.app.Learn(ctx, u, "physics"); err != nil {
		t.Fatal(err)
	}
	w.app.WaitDrafts()
	var n int
	if err := w.store.DB().QueryRow("SELECT count(*) FROM reservations WHERE user_id=?", u.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatal("unknown provider cost was released, permitting unbounded retries")
	}
}

type replayChargePay struct{ billing.Fake }

func (replayChargePay) Charge(context.Context, string, string, int64, string, string) (string, error) {
	return "same-confirmed-payment", nil
}
func TestG1_OnePaymentReferenceCannotCreditTwice(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("replay@example.com")
	u := w.user("replay@example.com")
	w.app.Pay = replayChargePay{}
	if err := w.app.TopUp(ctx, u.ID, app.NewTopupToken()); err != nil {
		t.Fatal(err)
	}
	w.advance(2 * time.Minute)
	if err := w.app.TopUp(ctx, u.ID, app.NewTopupToken()); err != nil {
		t.Fatal(err)
	}
	sub, _ := w.store.For(u.ID).Subscription(ctx)
	if sub.TopupMicros != w.app.Cfg.TopupMicros {
		t.Fatalf("same payment credited twice: %d", sub.TopupMicros)
	}
}
