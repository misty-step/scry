package store

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestTopupTransitionTable pins the top-up machine (specs/billing_topup.qnt
// step): every legal pair, every other pair refused with a typed error.
func TestTopupTransitionTable(t *testing.T) {
	legal := map[[2]TopupStatus]bool{
		{TopupStatusPending, TopupStatusSucceeded}: true,
		{TopupStatusPending, TopupStatusFailed}:    true,
		{TopupStatusFailed, TopupStatusSucceeded}:  true,
		{TopupStatusFailed, TopupStatusRefundDue}:  true,
	}
	all := append([]TopupStatus{"", "bogus"}, TopupStatuses...)
	for _, from := range all {
		for _, to := range all {
			err := TransitionTopup(from, to)
			if CanTransitionTopup(from, to) != (err == nil) {
				t.Fatalf("%s -> %s: CanTransitionTopup and TransitionTopup disagree", from, to)
			}
			if legal[[2]TopupStatus{from, to}] {
				if err != nil {
					t.Errorf("%s -> %s refused: %v", from, to, err)
				}
				continue
			}
			var te *TopupTransitionError
			if !errors.Is(err, ErrIllegalTransition) || !errors.As(err, &te) || te.From != from || te.To != to {
				t.Errorf("%s -> %s: want a *TopupTransitionError, got %v", from, to, err)
			}
		}
	}
	for _, s := range TopupStatuses {
		if !s.Valid() {
			t.Errorf("%s not valid", s)
		}
		if s.Terminal() != (len(topupTransitions[s]) == 0) {
			t.Errorf("%s: Terminal()=%v disagrees with the table", s, s.Terminal())
		}
	}
	// The old names stay the same values.
	if TopupPending != "pending" || TopupSucceeded != "succeeded" || TopupFailed != "failed" || TopupRefundDue != "refund_due" {
		t.Fatal("top-up state names changed")
	}
}

// TestSubscriptionTransitionTable pins the plan machine: ended never moves
// on a processor event, and a plan is only opened over none or ended.
func TestSubscriptionTransitionTable(t *testing.T) {
	legal := map[subStep]SubscriptionStatus{
		{SubNone, SubEventOpenTrial}:         SubTrialing,
		{SubNone, SubEventOpenPaid}:          SubActive,
		{SubEnded, SubEventOpenPaid}:         SubActive,
		{SubEnded, SubEventEnded}:            SubEnded,
		{SubTrialing, SubEventInvoicePaid}:   SubActive,
		{SubTrialing, SubEventInvoiceFailed}: SubPastDue,
		{SubTrialing, SubEventEnded}:         SubEnded,
		{SubActive, SubEventInvoicePaid}:     SubActive,
		{SubActive, SubEventInvoiceFailed}:   SubPastDue,
		{SubActive, SubEventEnded}:           SubEnded,
		{SubPastDue, SubEventInvoicePaid}:    SubActive,
		{SubPastDue, SubEventInvoiceFailed}:  SubPastDue,
		{SubPastDue, SubEventEnded}:          SubEnded,
	}
	for _, from := range append([]SubscriptionStatus{SubNone}, SubscriptionStatuses...) {
		for _, ev := range SubscriptionEvents {
			to, err := TransitionSubscription(from, ev)
			want, ok := legal[subStep{from, ev}]
			if ok {
				if err != nil || to != want {
					t.Errorf("%q on %s = %q, %v; want %q", from, ev, to, err, want)
				}
				continue
			}
			var te *SubscriptionTransitionError
			if !errors.Is(err, ErrIllegalTransition) || !errors.As(err, &te) || to != from {
				t.Errorf("%q on %s: want refused, got %q, %v", from, ev, to, err)
			}
			if from == SubEnded && to != SubEnded {
				t.Errorf("ended regressed to %q on %s", to, ev)
			}
		}
	}
}

type billingFixture struct {
	ctx context.Context
	s   *Store
	us  *UserStore
	uid int64
	now time.Time
}

func newBillingFixture(t *testing.T, path string) *billingFixture {
	t.Helper()
	ctx := context.Background()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	u, err := s.CreateUser(ctx, "billing@example.com", "billing", "UTC", now)
	if err != nil {
		t.Fatal(err)
	}
	us := s.For(u.ID)
	if err := us.SaveSubscription(ctx, Subscription{UserID: u.ID, CustomerRef: "cus_1", SubscriptionRef: "sub_1", PaymentRef: "pm_1",
		Status: SubActive, PeriodStart: now, PeriodEnd: now.AddDate(0, 1, 0), AllowanceMicros: 3_000_000}); err != nil {
		t.Fatal(err)
	}
	return &billingFixture{ctx: ctx, s: s, us: us, uid: u.ID, now: now}
}

func (f *billingFixture) reserve(t *testing.T, token string) {
	t.Helper()
	if _, fresh, err := f.us.ReserveTopup(f.ctx, TopupAttempt{Token: token, Key: "k-" + token, AmountCents: 500, CreditMicros: 4_000_000, Created: f.now}, nil); err != nil || !fresh {
		t.Fatalf("reserve %s: fresh=%v %v", token, fresh, err)
	}
}

func (f *billingFixture) status(t *testing.T, token string) TopupStatus {
	t.Helper()
	a, err := f.us.Topup(f.ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	return a.Status
}

// Bug 1.1: an attempt that failed gives its cap slot back. Four more
// top-ups then fill the month, and only then does the processor report
// that the failed attempt's charge succeeded after all. It must not be
// credited (that would be $25 in one month): it becomes refund_due.
func TestLateSuccessOverCapBecomesRefundDue(t *testing.T) {
	f := newBillingFixture(t, filepath.Join(t.TempDir(), "s.db"))
	f.reserve(t, "late")
	if err := f.us.FailTopup(f.ctx, "late", "pi_late", "Your card was declined.", f.now); err != nil {
		t.Fatal(err)
	}
	for _, tok := range []string{"a", "b", "c", "d"} {
		f.reserve(t, tok)
		if ok, err := f.us.SucceedTopup(f.ctx, tok, "pi_"+tok, "Top-up", f.now); err != nil || !ok {
			t.Fatalf("%s: %v %v", tok, ok, err)
		}
	}
	granted, err := f.us.SucceedTopup(f.ctx, "late", "pi_late", "Top-up", f.now)
	if granted || !errors.Is(err, ErrTopupCapExceededLate) {
		t.Fatalf("late success over the cap: granted=%v err=%v", granted, err)
	}
	if st := f.status(t, "late"); st != TopupStatusRefundDue {
		t.Fatalf("status %s, want refund_due", st)
	}
	sub, _ := f.us.Subscription(f.ctx)
	if n, _ := f.us.MonthTopups(f.ctx, f.now); sub.TopupMicros != 16_000_000 || n != TopupCap {
		t.Fatalf("credit %d slots %d; want $16 of credit and a full month", sub.TopupMicros, n)
	}
	due, err := f.s.RefundDueTopups(f.ctx)
	if err != nil || len(due) != 1 || due[0].ChargeRef != "pi_late" {
		t.Fatalf("refund list: %+v %v", due, err)
	}
	led, _ := f.us.Ledger(f.ctx)
	for _, e := range led {
		if e.Ref == "k-late-failed" || e.Ref == "pi_late" {
			t.Fatalf("ledger still has %+v", e)
		}
	}
	// refund_due is terminal: a replay and a failure change nothing.
	if granted, err := f.us.SucceedTopup(f.ctx, "late", "pi_late", "Top-up", f.now); granted || err != nil {
		t.Fatalf("replay: %v %v", granted, err)
	}
	if err := f.us.FailTopup(f.ctx, "late", "pi_late", "x", f.now); err != nil || f.status(t, "late") != TopupStatusRefundDue {
		t.Fatalf("fail after refund_due: %v", err)
	}
}

// With a free slot the late success is still credited (and its failed
// history line replaced), as before.
func TestLateSuccessWithFreeSlotIsCredited(t *testing.T) {
	f := newBillingFixture(t, filepath.Join(t.TempDir(), "s.db"))
	f.reserve(t, "late")
	if err := f.us.FailTopup(f.ctx, "late", "pi_late", "Your card was declined.", f.now); err != nil {
		t.Fatal(err)
	}
	for _, tok := range []string{"a", "b", "c"} {
		f.reserve(t, tok)
	}
	if granted, err := f.us.SucceedTopup(f.ctx, "late", "", "Top-up", f.now); err != nil || !granted {
		t.Fatalf("late success with a free slot: %v %v", granted, err)
	}
	if st := f.status(t, "late"); st != TopupStatusSucceeded {
		t.Fatalf("status %s", st)
	}
	// A succeeded attempt never regresses.
	if err := f.us.FailTopup(f.ctx, "late", "pi_late", "x", f.now); err != nil || f.status(t, "late") != TopupStatusSucceeded {
		t.Fatalf("succeeded regressed: %v", err)
	}
}

func TestSucceedTopupNeedsChargeRef(t *testing.T) {
	f := newBillingFixture(t, filepath.Join(t.TempDir(), "s.db"))
	f.reserve(t, "x")
	if _, err := f.us.SucceedTopup(f.ctx, "x", "", "Top-up", f.now); !errors.Is(err, ErrNoChargeRef) {
		t.Fatalf("no charge ref: %v", err)
	}
	if st := f.status(t, "x"); st != TopupStatusPending {
		t.Fatalf("status %s", st)
	}
}

// Bug 1.3: a cost larger than everything left never drives top-up credit
// negative; the usage row still records the full cost.
func TestSpendIsClamped(t *testing.T) {
	f := newBillingFixture(t, filepath.Join(t.TempDir(), "s.db"))
	if _, err := f.us.ChangeSubscription(f.ctx, func(s *Subscription) { s.TopupMicros = 1_000 }); err != nil {
		t.Fatal(err)
	}
	if err := f.us.Spend(f.ctx, Usage{Kind: "draft", Model: "x", CostMicros: 9_000_000}, f.now); err != nil {
		t.Fatal(err)
	}
	sub, _ := f.us.Subscription(f.ctx)
	if sub.TopupMicros != 0 || sub.UsedMicros != sub.AllowanceMicros {
		t.Fatalf("after an oversized cost: %+v", sub)
	}
	var cost int64
	if err := f.s.DB().QueryRow(`SELECT cost_micros FROM usage WHERE user_id=?`, f.uid).Scan(&cost); err != nil || cost != 9_000_000 {
		t.Fatalf("usage row: %d %v", cost, err)
	}
	if err := f.us.Spend(f.ctx, Usage{Kind: "draft", CostMicros: -1}, f.now); !errors.Is(err, ErrNegativeCost) {
		t.Fatalf("negative cost: %v", err)
	}
}

// A database whose topups table was created before refund_due existed is
// rebuilt on open: its rows survive, its indexes come back, and the new
// state can be written.
func TestTopupsTableMigratesToRefundDue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	f := newBillingFixture(t, path)
	f.reserve(t, "kept")
	if err := f.us.NoteTopupCharge(f.ctx, "kept", "pi_kept"); err != nil {
		t.Fatal(err)
	}
	legacy := strings.Replace(topupSchema, "CREATE TABLE IF NOT EXISTS topups (", "CREATE TABLE topups_legacy (", 1)
	legacy = strings.Replace(legacy, ",'refund_due'", "", 1)
	for _, q := range []string{
		legacy,
		`ALTER TABLE topups_legacy ADD COLUMN customer_ref TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE topups_legacy ADD COLUMN payment_ref TEXT NOT NULL DEFAULT ''`,
		`INSERT INTO topups_legacy SELECT ` + topupCols + ` FROM topups`,
		`DROP TABLE topups`,
		`ALTER TABLE topups_legacy RENAME TO topups`,
	} {
		if _, err := f.s.DB().Exec(q); err != nil {
			t.Fatal(q, err)
		}
	}
	if ok, err := f.s.topupsAllowRefundDue(); err != nil || ok {
		t.Fatalf("fixture is not a legacy table: %v %v", ok, err)
	}
	if _, err := f.s.DB().Exec(`UPDATE topups SET status='refund_due'`); err == nil {
		t.Fatal("legacy CHECK admitted refund_due")
	}
	f.s.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if ok, err := s.topupsAllowRefundDue(); err != nil || !ok {
		t.Fatalf("not migrated: %v %v", ok, err)
	}
	a, err := s.For(f.uid).Topup(f.ctx, "kept")
	if err != nil || a.Status != TopupStatusPending || a.ChargeRef != "pi_kept" || a.CustomerRef != "cus_1" || a.PaymentRef != "pm_1" {
		t.Fatalf("row lost in migration: %+v %v", a, err)
	}
	for _, idx := range []string{"topups_pending", "topups_charge"} {
		var n int
		if err := s.DB().QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='index' AND name=?`, idx).Scan(&n); err != nil || n != 1 {
			t.Fatalf("index %s missing: %d %v", idx, n, err)
		}
	}
	if _, err := s.DB().Exec(`UPDATE topups SET status='refund_due'`); err != nil {
		t.Fatalf("migrated table refuses refund_due: %v", err)
	}
	if _, err := s.DB().Exec(`UPDATE topups SET status='bogus'`); err == nil {
		t.Fatal("migrated table lost its CHECK")
	}
	// Opening again is a no-op.
	s.Close()
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s2.Close()
}

func TestOpenPlanFollowsTheMachine(t *testing.T) {
	f := newBillingFixture(t, filepath.Join(t.TempDir(), "s.db"))
	// Over a live plan, opening is a no-op.
	if err := f.us.OpenPlan(f.ctx, Subscription{UserID: f.uid, SubscriptionRef: "sub_2", Status: SubTrialing}); err != nil {
		t.Fatal(err)
	}
	if sub, _ := f.us.Subscription(f.ctx); sub.SubscriptionRef != "sub_1" {
		t.Fatalf("replaced a live plan: %+v", sub)
	}
	if _, err := f.us.ChangeSubscription(f.ctx, func(s *Subscription) { s.Status, s.TopupMicros = SubEnded, 123 }); err != nil {
		t.Fatal(err)
	}
	// An ended subscription is never reopened in place.
	if err := f.us.OpenPlan(f.ctx, Subscription{UserID: f.uid, SubscriptionRef: "sub_1", Status: SubActive}); err != nil {
		t.Fatal(err)
	}
	if sub, _ := f.us.Subscription(f.ctx); sub.Status != SubEnded {
		t.Fatalf("ended plan reopened in place: %+v", sub)
	}
	if err := f.us.OpenPlan(f.ctx, Subscription{UserID: f.uid, SubscriptionRef: "sub_2", Status: SubActive}); err != nil {
		t.Fatal(err)
	}
	if sub, _ := f.us.Subscription(f.ctx); sub.Status != SubActive || sub.SubscriptionRef != "sub_2" || sub.TopupMicros != 123 {
		t.Fatalf("reopen: %+v", sub)
	}
}
