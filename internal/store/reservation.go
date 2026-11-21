package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrBudget is a reservation refused because the plan's unreserved
// allowance plus top-up credit is less than the amount asked for.
var ErrBudget = errors.New("insufficient unreserved allowance")

// ErrReservationNoAccess is a reservation refused because the plan gives
// no access at the moment of the insert: it has ended, it is past_due
// and its grace has run out, or it is a no-card trial at or past its
// period_end. The check is part of the same atomic
// statement as the insert, so a plan that ends between the app's Plan()
// read and Reserve can never take a new hold.
var ErrReservationNoAccess = errors.New("plan gives no access: reservation refused")

// ErrInvalidReservationAmount is a reservation for zero or a negative
// amount. Every hold is for a positive worst-case budget.
var ErrInvalidReservationAmount = errors.New("reservation amount must be positive")

// DefaultReservationGrace is the past_due grace Reserve applies when the
// caller does not give one: the shipped plan's GraceDays (7).
const DefaultReservationGrace = 7 * 24 * time.Hour

// ReservationState is where one allowance hold is in its life. The machine
// (specs/allowance_reservations.qnt, the fixed `step`) is
//
//	(none)   --reserve--> held          Reserve: an atomic, guarded INSERT
//	held     --settle-->  settled       Settle with a cost: the row is deleted
//	                                    and the cost spent, in one transaction
//	held     --release--> released      Settle with no cost: the row is deleted
//	held     --sweep-->   swept         FailStaleReservations at boot
//	held     --expire-->  swept         SweepExpiredReservations (TTL)
//	settled|released|swept --settle|release--> itself
//	                                    a replay: nothing is spent again
//
// Only held is a row in the reservations table; the terminal states are
// its absence. Ids are random tokens used once, so a settle that finds no
// row is a replay of a settle, release or sweep that already happened, and
// is a no-op. Every pair not listed is illegal.
type ReservationState string

const (
	ReservationNone     ReservationState = "" // the id was never reserved
	ReservationHeld     ReservationState = "held"
	ReservationSettled  ReservationState = "settled"
	ReservationReleased ReservationState = "released"
	ReservationSwept    ReservationState = "swept"
)

// ReservationStates lists every state, including none.
var ReservationStates = []ReservationState{ReservationNone, ReservationHeld, ReservationSettled, ReservationReleased, ReservationSwept}

// Terminal reports whether the hold is over (its row is gone for good).
func (s ReservationState) Terminal() bool {
	return s == ReservationSettled || s == ReservationReleased || s == ReservationSwept
}

func (s ReservationState) String() string {
	if s == ReservationNone {
		return "none"
	}
	return string(s)
}

// ReservationEvent is what moves a hold.
type ReservationEvent uint8

const (
	ResEventReserve ReservationEvent = iota // Reserve
	ResEventSettle                          // Settle with a cost above zero
	ResEventRelease                         // Settle with a zero cost
	ResEventSweep                           // FailStaleReservations (startup)
	ResEventExpire                          // SweepExpiredReservations (TTL)
)

// ReservationEvents lists every reservation event.
var ReservationEvents = []ReservationEvent{ResEventReserve, ResEventSettle, ResEventRelease, ResEventSweep, ResEventExpire}

func (e ReservationEvent) String() string {
	switch e {
	case ResEventReserve:
		return "reserve"
	case ResEventSettle:
		return "settle"
	case ResEventRelease:
		return "release"
	case ResEventSweep:
		return "sweep"
	case ResEventExpire:
		return "expire"
	}
	return fmt.Sprintf("ReservationEvent(%d)", uint8(e))
}

type resStep struct {
	from ReservationState
	ev   ReservationEvent
}

// reservationTransitions is the whole hold machine. A pair that is not
// here is illegal.
var reservationTransitions = map[resStep]ReservationState{
	{ReservationNone, ResEventReserve}: ReservationHeld,

	{ReservationHeld, ResEventSettle}:  ReservationSettled,
	{ReservationHeld, ResEventRelease}: ReservationReleased,
	{ReservationHeld, ResEventSweep}:   ReservationSwept,
	{ReservationHeld, ResEventExpire}:  ReservationSwept,

	// Replays: settling or releasing a hold that is already over changes
	// nothing and spends nothing (idempotent settle).
	{ReservationSettled, ResEventSettle}:   ReservationSettled,
	{ReservationSettled, ResEventRelease}:  ReservationSettled,
	{ReservationReleased, ResEventSettle}:  ReservationReleased,
	{ReservationReleased, ResEventRelease}: ReservationReleased,
	{ReservationSwept, ResEventSettle}:     ReservationSwept,
	{ReservationSwept, ResEventRelease}:    ReservationSwept,
}

// ReservationTransitionError is a refused hold transition. It matches
// ErrIllegalTransition.
type ReservationTransitionError struct {
	From  ReservationState
	Event ReservationEvent
}

func (e *ReservationTransitionError) Error() string {
	return fmt.Sprintf("%v: reservation %s on %s", ErrIllegalTransition, e.From, e.Event)
}

func (e *ReservationTransitionError) Is(target error) bool { return target == ErrIllegalTransition }

// TransitionReservation is the state an event moves a hold to, or a
// *ReservationTransitionError when the hold cannot take it (reserving an
// id twice, sweeping a hold that is already over, settling an id that was
// never reserved).
func TransitionReservation(from ReservationState, ev ReservationEvent) (ReservationState, error) {
	if to, ok := reservationTransitions[resStep{from, ev}]; ok {
		return to, nil
	}
	return from, &ReservationTransitionError{From: from, Event: ev}
}

// settleEvent is the event a settle of this cost is.
func settleEvent(cost int64) ReservationEvent {
	if cost == 0 {
		return ResEventRelease
	}
	return ResEventSettle
}

// Reservation is one held budget.
type Reservation struct {
	ID        string
	Amount    int64
	CreatedAt time.Time
}

// graceCutoff is the earliest past_due_since (Unix seconds) that still has
// access at now: HasAccess is now - past_due_since <= grace, and
// past_due_since is stored in whole seconds, so it is ceil(now - grace).
func graceCutoff(now time.Time, grace time.Duration) int64 {
	c := now.Add(-grace)
	sec := c.Unix()
	if c.Nanosecond() > 0 {
		sec++
	}
	return sec
}

// reserveAccess is the access predicate of the Reserve statement, the same
// as app.HasAccess: active plans, trialing plans (a no-card trial, with
// customer_ref ”, only while now < period_end), and past_due plans whose
// grace has not run out. cutoff is the SQL parameter holding graceCutoff;
// nowSec the one holding now in whole Unix seconds (period_end is stored
// in whole seconds, so now < period_end is period_end > floor(now)).
func reserveAccess(cutoff, nowSec string) string {
	return `(status='active' OR (status='trialing' AND (customer_ref<>'' OR period_end>` + nowSec + `)) OR (status='past_due' AND past_due_since>0 AND past_due_since>=` + cutoff + `))`
}

// Reserve holds amount micro-dollars of the learner's allowance (and then
// top-up credit) for work about to start, checking access by the wall
// clock and DefaultReservationGrace. The app uses ReserveAt with its own
// clock and grace.
func (u *UserStore) Reserve(ctx context.Context, id string, amount int64) error {
	return u.ReserveAt(ctx, id, amount, time.Now(), DefaultReservationGrace)
}

// ReserveAt holds amount micro-dollars for work about to start, as of now.
// One atomic INSERT .. SELECT both checks that the plan gives access at
// now (active, trialing, or past_due within grace; a no-card trial only
// before its period_end) and that the unreserved
// allowance plus top-up credit covers amount, so neither can change
// between the check and the hold. The hold records now as its created_at
// for the TTL sweep. It returns ErrInvalidReservationAmount for an amount
// that is not positive, ErrReservationNoAccess when the plan gives no
// access, and ErrBudget when it does but there is not enough left (or
// there is no plan at all).
func (u *UserStore) ReserveAt(ctx context.Context, id string, amount int64, now time.Time, grace time.Duration) error {
	if amount <= 0 {
		return ErrInvalidReservationAmount
	}
	db := u.s.billingDB(ctx)
	res, err := db.ExecContext(ctx, `INSERT INTO reservations(id,user_id,amount,created_at)
 SELECT ?1,user_id,?2,?3 FROM subscriptions WHERE user_id=?4 AND `+reserveAccess("?5", "?6")+` AND
 allowance_micros-period_used_micros+topup_micros-
 COALESCE((SELECT SUM(amount) FROM reservations WHERE user_id=?4),0)>=?2`,
		id, amount, ts(now), u.uid, graceCutoff(now, grace), now.Unix())
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 1 {
		return nil
	}
	// Refused. Which reason is only for the caller's message: the refusal
	// itself was decided atomically above.
	var access bool
	err = db.QueryRowContext(ctx, `SELECT `+reserveAccess("?2", "?3")+` FROM subscriptions WHERE user_id=?1`,
		u.uid, graceCutoff(now, grace), now.Unix()).Scan(&access)
	if err == nil && !access {
		return ErrReservationNoAccess
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return ErrBudget
}

// Settle ends a hold: it deletes the row and, in the same transaction,
// spends what the work actually cost (ResEventSettle), or spends nothing
// when it cost nothing (ResEventRelease). A hold that is already over
// (settled, released, swept or expired) is a replay and returns nil
// without spending anything again, so settle is idempotent. A negative
// cost is refused with ErrNegativeCost and the hold stays held.
func (u *UserStore) Settle(ctx context.Context, id string, use Usage, now time.Time) error {
	if use.CostMicros < 0 {
		return ErrNegativeCost
	}
	return u.s.BillingTransaction(ctx, func(ctx context.Context) error {
		res, err := u.s.billingDB(ctx).ExecContext(ctx, "DELETE FROM reservations WHERE id=? AND user_id=?", id, u.uid)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		from := ReservationHeld
		if n == 0 {
			// No row: the hold is over already. Which terminal state it
			// reached is not recorded; every one of them absorbs a replay.
			from = ReservationSettled
		}
		to, err := TransitionReservation(from, settleEvent(use.CostMicros))
		if err != nil {
			return err
		}
		if from.Terminal() || to == ReservationReleased {
			return nil
		}
		return u.Spend(ctx, use, now)
	})
}

// ReservedMicros reports allowance held by work in flight.
func (u *UserStore) ReservedMicros(ctx context.Context) (int64, error) {
	var amount int64
	err := u.s.billingDB(ctx).QueryRowContext(ctx, "SELECT COALESCE(SUM(amount),0) FROM reservations WHERE user_id=?", u.uid).Scan(&amount)
	return amount, err
}

// Reservations lists the learner's holds still in flight, oldest first.
func (u *UserStore) Reservations(ctx context.Context) ([]Reservation, error) {
	rows, err := u.s.billingDB(ctx).QueryContext(ctx, "SELECT id, amount, created_at FROM reservations WHERE user_id=? ORDER BY created_at, id", u.uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Reservation
	for rows.Next() {
		var r Reservation
		var at int64
		if err := rows.Scan(&r.ID, &r.Amount, &at); err != nil {
			return nil, err
		}
		r.CreatedAt = tm(at)
		out = append(out, r)
	}
	return out, rows.Err()
}

// FailStaleReservations releases every allowance hold left in the table and
// reports how many it released. Scry is one process, so at startup no model
// or speech call is in flight: any row still here was orphaned by an unclean
// kill between Reserve and Settle and would otherwise hold that learner's
// allowance forever. Call it once at boot, before serving, never while
// requests are running. (ResEventSweep: held -> swept.)
func (s *Store) FailStaleReservations(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, "DELETE FROM reservations")
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SweepExpiredReservations releases every hold created before cutoff and
// reports how many it released (ResEventExpire: held -> swept). It is the
// running server's TTL cleanup, for holds a request abandoned (a provider
// outcome that stayed unknown is deliberately left held, so retries cannot
// spend unmetered) and for rows from before created_at was recorded
// (created_at 0). Nothing is spent for them. The TTL behind cutoff must be
// longer than the longest provider call: a call still running when its
// hold expires settles into a no-op and its usage is not metered. A zero
// cutoff releases nothing.
func (s *Store) SweepExpiredReservations(ctx context.Context, cutoff time.Time) (int64, error) {
	if cutoff.IsZero() {
		return 0, nil
	}
	res, err := s.db.ExecContext(ctx, "DELETE FROM reservations WHERE created_at < ?", cutoff.Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
