package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// TestReservationTransitionTable pins the allowance hold machine
// (specs/allowance_reservations.qnt, `step`): every legal pair of
// (ReservationState, ReservationEvent), and every other pair refused with
// a typed error that matches ErrIllegalTransition.
func TestReservationTransitionTable(t *testing.T) {
	legal := map[resStep]ReservationState{
		{ReservationNone, ResEventReserve}: ReservationHeld,
		{ReservationHeld, ResEventSettle}:  ReservationSettled,
		{ReservationHeld, ResEventRelease}: ReservationReleased,
		{ReservationHeld, ResEventSweep}:   ReservationSwept,
		{ReservationHeld, ResEventExpire}:  ReservationSwept,
		// idempotent replays of a settle on a hold that is over
		{ReservationSettled, ResEventSettle}:   ReservationSettled,
		{ReservationSettled, ResEventRelease}:  ReservationSettled,
		{ReservationReleased, ResEventSettle}:  ReservationReleased,
		{ReservationReleased, ResEventRelease}: ReservationReleased,
		{ReservationSwept, ResEventSettle}:     ReservationSwept,
		{ReservationSwept, ResEventRelease}:    ReservationSwept,
	}
	pairs := 0
	for _, from := range ReservationStates {
		for _, ev := range ReservationEvents {
			pairs++
			to, err := TransitionReservation(from, ev)
			if want, ok := legal[resStep{from, ev}]; ok {
				if err != nil || to != want {
					t.Errorf("%s --%s--> %s, %v; want %s", from, ev, to, err, want)
				}
				continue
			}
			var te *ReservationTransitionError
			if !errors.Is(err, ErrIllegalTransition) || !errors.As(err, &te) || te.From != from || te.Event != ev || to != from {
				t.Errorf("%s --%s--> %s accepted or untyped: %v", from, ev, to, err)
			}
		}
	}
	if pairs != 5*5 || len(reservationTransitions) != len(legal) {
		t.Fatalf("%d pairs checked, %d legal in the table; want 25 and %d", pairs, len(reservationTransitions), len(legal))
	}
	// Terminal states only ever lead to themselves; held is left by every
	// ending event; nothing but reserve leaves none.
	for _, from := range ReservationStates {
		for _, ev := range ReservationEvents {
			to, err := TransitionReservation(from, ev)
			if err == nil && from.Terminal() && to != from {
				t.Errorf("terminal %s left on %s to %s", from, ev, to)
			}
		}
	}
	if ReservationNone.Terminal() || ReservationHeld.Terminal() || !ReservationSwept.Terminal() {
		t.Error("Terminal() is wrong")
	}
	if settleEvent(0) != ResEventRelease || settleEvent(1) != ResEventSettle {
		t.Error("settleEvent is wrong")
	}
}

// ------------------------------------------------------------------ model

const (
	rAllowance = 1000 // a month's allowance (scaled micros)
	rTrial     = 600  // the trial's allowance
	rTopup     = 400  // one top-up's credit
	rGrace     = 6 * time.Hour
	rTTL       = time.Hour
)

type resHold struct {
	amount  int64
	created time.Time
}

// reservationMachine drives one learner's real, file-backed SQLite store
// and checks it against a reference model of the subscription row and the
// hold machine (TransitionReservation).
type reservationMachine struct {
	ctx  context.Context
	path string
	st   *Store
	us   *UserStore
	uid  int64
	now  time.Time
	refs int

	// subscription row
	status       SubscriptionStatus
	pastDueSince time.Time
	allowance    int64
	used         int64
	topup        int64
	usages       int // usage rows

	held   map[string]resHold
	states map[string]ReservationState // every id ever issued
	ids    int

	restarted bool // the last action was a crash and restart
}

func (m *reservationMachine) hasAccess() bool {
	switch m.status {
	case SubTrialing, SubActive:
		return true
	case SubPastDue:
		return !m.pastDueSince.IsZero() && m.now.Sub(m.pastDueSince) <= rGrace
	}
	return false
}

func (m *reservationMachine) reserved() int64 {
	var n int64
	for _, h := range m.held {
		n += h.amount
	}
	return n
}

func (m *reservationMachine) available() int64 {
	return m.allowance - m.used + m.topup - m.reserved()
}

// move applies an event to an id in the model, by the transition table.
func (m *reservationMachine) move(t *rapid.T, id string, ev ReservationEvent) {
	to, err := TransitionReservation(m.states[id], ev)
	if err != nil {
		t.Fatalf("model: %v", err)
	}
	m.states[id] = to
	if to != ReservationHeld {
		delete(m.held, id)
	}
}

func (m *reservationMachine) sub(t *rapid.T) Subscription {
	sub, err := m.us.Subscription(m.ctx)
	if err != nil {
		t.Fatal(err)
	}
	return sub
}

func (m *reservationMachine) usageRows(t *rapid.T) int {
	var n int
	if err := m.st.db.QueryRow(`SELECT COUNT(*) FROM usage WHERE user_id = ?`, m.uid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (m *reservationMachine) pick(t *rapid.T, label string, want func(ReservationState) bool) (string, bool) {
	var ids []string
	for id, st := range m.states {
		if want(st) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return "", false
	}
	sort.Strings(ids)
	return rapid.SampledFrom(ids).Draw(t, label), true
}

// ---------------------------------------------------------------- actions

// Reserve asks for a budget within or over what is left, on whatever the
// plan is now (trialing, active, past_due within or after grace, ended).
// The store must agree with the model exactly: refused without access
// whatever the budget, refused over budget, held otherwise.
func (m *reservationMachine) Reserve(t *rapid.T) {
	avail := m.available()
	var amount int64
	if avail >= 1 && rapid.Bool().Draw(t, "within") {
		amount = rapid.Int64Range(1, avail).Draw(t, "amount")
	} else {
		amount = max(avail, 0) + rapid.Int64Range(1, 300).Draw(t, "over")
	}
	m.ids++
	id := fmt.Sprintf("res-%d", m.ids)
	err := m.us.ReserveAt(m.ctx, id, amount, m.now, rGrace)
	switch {
	case !m.hasAccess():
		if !errors.Is(err, ErrReservationNoAccess) {
			t.Fatalf("reserve %d on %s (past due since %v, now %v) = %v; want ErrReservationNoAccess",
				amount, m.status, m.pastDueSince, m.now, err)
		}
	case amount > avail:
		if !errors.Is(err, ErrBudget) {
			t.Fatalf("reserve %d with %d available = %v; want ErrBudget", amount, avail, err)
		}
	default:
		if err != nil {
			t.Fatalf("reserve %d with %d available on %s = %v", amount, avail, m.status, err)
		}
		m.move(t, id, ResEventReserve)
		m.held[id] = resHold{amount: amount, created: m.now}
	}
}

// BadReserve asks for a hold that can never be taken: a non-positive
// amount, or an id that is already held. Nothing changes.
func (m *reservationMachine) BadReserve(t *rapid.T) {
	if id, ok := m.pick(t, "held", func(s ReservationState) bool { return s == ReservationHeld }); ok && rapid.Bool().Draw(t, "duplicate") {
		if _, err := TransitionReservation(ReservationHeld, ResEventReserve); !errors.Is(err, ErrIllegalTransition) {
			t.Fatalf("model allows reserving a held id: %v", err)
		}
		if err := m.us.ReserveAt(m.ctx, id, 1, m.now, rGrace); err == nil {
			t.Fatalf("reserved held id %s twice", id)
		}
		return
	}
	amount := rapid.SampledFrom([]int64{0, -1, -500, -1 << 62}).Draw(t, "bad_amount")
	m.ids++
	if err := m.us.ReserveAt(m.ctx, fmt.Sprintf("res-%d", m.ids), amount, m.now, rGrace); !errors.Is(err, ErrInvalidReservationAmount) {
		t.Fatalf("reserve %d = %v; want ErrInvalidReservationAmount", amount, err)
	}
}

// Settle ends a held reservation at a zero, partial, full or over-reserved
// cost (an unpriced model, a long recording), or tries a negative cost.
func (m *reservationMachine) Settle(t *rapid.T) {
	id, ok := m.pick(t, "held", func(s ReservationState) bool { return s == ReservationHeld })
	if !ok {
		t.Skip("nothing held")
	}
	amount := m.held[id].amount
	var cost int64
	switch kind := rapid.SampledFrom([]string{"zero", "partial", "full", "over", "negative"}).Draw(t, "cost_kind"); kind {
	case "zero":
	case "partial":
		cost = rapid.Int64Range(1, max(1, amount-1)).Draw(t, "cost")
	case "full":
		cost = amount
	case "over":
		cost = amount + rapid.Int64Range(1, 2*amount+rAllowance).Draw(t, "extra")
	case "negative":
		cost = -rapid.Int64Range(1, 1000).Draw(t, "neg")
	}
	err := m.us.Settle(m.ctx, id, Usage{Kind: "judge", Model: "test-judge", CostMicros: cost}, m.now)
	if cost < 0 {
		if !errors.Is(err, ErrNegativeCost) {
			t.Fatalf("settle at %d = %v; want ErrNegativeCost", cost, err)
		}
		return // still held, nothing spent
	}
	if err != nil {
		t.Fatalf("settle %s at %d: %v", id, cost, err)
	}
	m.move(t, id, settleEvent(cost))
	if cost > 0 {
		left := max(0, m.allowance-m.used)
		m.topup = max(0, m.topup-max(0, cost-left))
		m.used = max(0, min(m.allowance, m.used+cost))
		m.usages++
	}
}

// DoubleSettle replays a settle on a hold that is already over (settled,
// released, swept, expired) or on an id that was never reserved. It is a
// no-op: it returns nil and spends nothing.
func (m *reservationMachine) DoubleSettle(t *rapid.T) {
	id, ok := m.pick(t, "over", ReservationState.Terminal)
	if !ok || rapid.Bool().Draw(t, "never_reserved") {
		id = "never-reserved"
	} else if _, err := TransitionReservation(m.states[id], ResEventSettle); err != nil {
		t.Fatalf("model refuses a replay on %s: %v", m.states[id], err)
	}
	before, usages, held := m.sub(t), m.usageRows(t), m.reserved()
	cost := rapid.Int64Range(0, 3*rAllowance).Draw(t, "cost")
	if err := m.us.Settle(m.ctx, id, Usage{Kind: "draft", Model: "test-draft", CostMicros: cost}, m.now); err != nil {
		t.Fatalf("replayed settle of %s (%s): %v", id, m.states[id], err)
	}
	after := m.sub(t)
	if after.UsedMicros != before.UsedMicros || after.TopupMicros != before.TopupMicros || m.usageRows(t) != usages {
		t.Fatalf("replayed settle of %s (%s) spent: used %d->%d, topup %d->%d, usage rows %d->%d",
			id, m.states[id], before.UsedMicros, after.UsedMicros, before.TopupMicros, after.TopupMicros, usages, m.usageRows(t))
	}
	if r, _ := m.us.ReservedMicros(m.ctx); r != held {
		t.Fatalf("replayed settle changed holds: %d -> %d", held, r)
	}
}

// AdvanceClock moves time on (by a ragged amount, so whole-second
// boundaries are crossed both ways) and, usually, runs the TTL sweep the
// way housekeeping does.
func (m *reservationMachine) AdvanceClock(t *rapid.T) {
	d := time.Duration(rapid.Int64Range(int64(time.Second), int64(3*time.Hour)).Draw(t, "advance"))
	m.now = m.now.Add(d)
	if !rapid.Bool().Draw(t, "sweep") {
		return
	}
	cutoff := m.now.Add(-rTTL)
	want := 0
	for id, h := range m.held {
		if h.created.Unix() < cutoff.Unix() {
			m.move(t, id, ResEventExpire)
			want++
		}
	}
	n, err := m.st.SweepExpiredReservations(m.ctx, cutoff)
	if err != nil || n != int64(want) {
		t.Fatalf("SweepExpiredReservations = %d, %v; want %d", n, err, want)
	}
	rows, err := m.us.Reservations(m.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if m.now.Sub(r.CreatedAt) > rTTL+time.Second {
			t.Fatalf("hold %s outlived the TTL: created %v, now %v", r.ID, r.CreatedAt, m.now)
		}
	}
}

// SimulateCrashAndRestart closes the store with holds still in flight,
// reopens the same file and runs the startup sweep. Every hold is
// released, nothing is spent, and the plan is exactly as it was.
func (m *reservationMachine) SimulateCrashAndRestart(t *rapid.T) {
	before := m.sub(t)
	if err := m.st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err := Open(m.path)
	if err != nil {
		t.Fatal(err)
	}
	m.st, m.us = st, st.For(m.uid)
	want := len(m.held)
	n, err := st.FailStaleReservations(m.ctx)
	if err != nil || n != int64(want) {
		t.Fatalf("FailStaleReservations = %d, %v; want %d", n, err, want)
	}
	for id := range m.held {
		m.move(t, id, ResEventSweep)
	}
	if r, err := m.us.ReservedMicros(m.ctx); err != nil || r != 0 {
		t.Fatalf("after restart %d still reserved (%v)", r, err)
	}
	if after := m.sub(t); after != before {
		t.Fatalf("restart changed the plan:\n%+v\n%+v", before, after)
	}
	m.restarted = true
}

// PeriodReset is a paid monthly invoice: active, a fresh allowance.
func (m *reservationMachine) PeriodReset(t *rapid.T) {
	if m.status == SubEnded {
		t.Skip("an ended plan is not invoiced")
	}
	if _, err := m.us.ChangeSubscription(m.ctx, func(s *Subscription) {
		s.Status, s.PastDueSince, s.CardError = SubActive, time.Time{}, ""
		s.AllowanceMicros, s.UsedMicros = rAllowance, 0
	}); err != nil {
		t.Fatal(err)
	}
	m.status, m.pastDueSince, m.allowance, m.used = SubActive, time.Time{}, rAllowance, 0
}

// TopupCredit adds one top-up's credit (only a plan with access may top up).
func (m *reservationMachine) TopupCredit(t *rapid.T) {
	if !m.hasAccess() {
		t.Skip("no access: top-up refused")
	}
	if _, err := m.us.ChangeSubscription(m.ctx, func(s *Subscription) { s.TopupMicros += rTopup }); err != nil {
		t.Fatal(err)
	}
	m.topup += rTopup
}

// StatusChange is a failed invoice (past_due, since the first failure),
// the end of the plan, or, once ended, a new plan under a new ref.
func (m *reservationMachine) StatusChange(t *rapid.T) {
	switch to := rapid.SampledFrom([]SubscriptionStatus{SubPastDue, SubEnded, SubActive}).Draw(t, "status"); {
	case m.status == SubEnded && to == SubActive:
		m.refs++
		if err := m.us.OpenPlan(m.ctx, Subscription{SubscriptionRef: fmt.Sprintf("sub_%d", m.refs), CustomerRef: "cus_r",
			Status: SubActive, AllowanceMicros: rAllowance}); err != nil {
			t.Fatal(err)
		}
		m.status, m.pastDueSince, m.allowance, m.used = SubActive, time.Time{}, rAllowance, 0
	case m.status == SubEnded || to == SubActive:
		t.Skip("no such move")
	case to == SubPastDue:
		at := m.now
		if _, err := m.us.ChangeSubscription(m.ctx, func(s *Subscription) {
			s.Status = SubPastDue
			if s.PastDueSince.IsZero() {
				s.PastDueSince = at
			}
		}); err != nil {
			t.Fatal(err)
		}
		if m.pastDueSince.IsZero() {
			m.pastDueSince = time.Unix(at.Unix(), 0).UTC() // stored in whole seconds
		}
		m.status = SubPastDue
	default:
		if _, err := m.us.ChangeSubscription(m.ctx, func(s *Subscription) { s.Status = SubEnded }); err != nil {
			t.Fatal(err)
		}
		m.status = SubEnded
	}
}

// ------------------------------------------------------------- invariants

// Check runs after every action.
func (m *reservationMachine) Check(t *rapid.T) {
	sub := m.sub(t)
	// inv_non_negative_balances
	if sub.UsedMicros < 0 || sub.UsedMicros > sub.AllowanceMicros {
		t.Fatalf("period_used_micros %d outside [0, %d]", sub.UsedMicros, sub.AllowanceMicros)
	}
	if sub.TopupMicros < 0 {
		t.Fatalf("topup_micros %d is negative", sub.TopupMicros)
	}
	reserved, err := m.us.ReservedMicros(m.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if reserved < 0 {
		t.Fatalf("reserved_micros %d is negative", reserved)
	}
	// inv_no_leaked_reservations_after_recovery
	if m.restarted && reserved != 0 {
		t.Fatalf("%d still reserved right after restart", reserved)
	}
	m.restarted = false
	// the store is the model
	if sub.Status != m.status || sub.AllowanceMicros != m.allowance || sub.UsedMicros != m.used || sub.TopupMicros != m.topup ||
		!sub.PastDueSince.Equal(m.pastDueSince) {
		t.Fatalf("plan %s allowance %d used %d topup %d past due %v; model %s %d %d %d %v",
			sub.Status, sub.AllowanceMicros, sub.UsedMicros, sub.TopupMicros, sub.PastDueSince,
			m.status, m.allowance, m.used, m.topup, m.pastDueSince)
	}
	if reserved != m.reserved() {
		t.Fatalf("reserved %d, model %d", reserved, m.reserved())
	}
	if n := m.usageRows(t); n != m.usages {
		t.Fatalf("%d usage rows, model %d (a settle spent twice or not at all)", n, m.usages)
	}
	rows, err := m.us.Reservations(m.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(m.held) {
		t.Fatalf("%d rows held, model %d", len(rows), len(m.held))
	}
	for _, r := range rows {
		h, ok := m.held[r.ID]
		if !ok || m.states[r.ID] != ReservationHeld || h.amount != r.Amount || r.CreatedAt.Unix() != h.created.Unix() {
			t.Fatalf("row %+v; model %+v %s", r, h, m.states[r.ID])
		}
	}
}

// TestRapidReservationStateMachine drives a real, file-backed SQLite store
// (closed and reopened on the same file by SimulateCrashAndRestart) through
// arbitrary sequences of reserve, settle, replayed settle, clock and TTL
// sweep, crash and restart, period reset, top-up and plan status changes,
// checking the invariants of specs/allowance_reservations.qnt after every
// step.
func TestRapidReservationStateMachine(t *testing.T) {
	dir := rapidDir(t)
	var n atomic.Int64
	rapid.Check(t, func(t *rapid.T) {
		ctx := context.Background()
		path := filepath.Join(dir, fmt.Sprintf("r%d.db", n.Add(1)))
		st, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		m := &reservationMachine{ctx: ctx, path: path, st: st,
			now:  time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC).Add(time.Duration(rapid.Int64Range(0, int64(time.Second)).Draw(t, "start_nanos"))),
			held: map[string]resHold{}, states: map[string]ReservationState{}}
		defer func() { m.st.Close() }()
		u, err := st.CreateUser(ctx, "holder@example.com", "holder", "UTC", m.now)
		if err != nil {
			t.Fatal(err)
		}
		m.uid, m.us = u.ID, st.For(u.ID)
		m.refs++
		if err := m.us.OpenPlan(ctx, Subscription{SubscriptionRef: "sub_1", CustomerRef: "cus_r", Status: SubTrialing,
			AllowanceMicros: rTrial, PeriodStart: m.now, PeriodEnd: m.now.AddDate(0, 0, 7)}); err != nil {
			t.Fatal(err)
		}
		m.status, m.allowance = SubTrialing, rTrial
		m.Check(t)
		t.Repeat(rapid.StateMachineActions(m))
	})
}
