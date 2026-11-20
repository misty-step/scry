package store

import (
	"context"
	"errors"
	"time"
)

var ErrBudget = errors.New("insufficient unreserved allowance")

func (u *UserStore) Reserve(ctx context.Context, id string, amount int64) error {
	res, err := u.s.db.ExecContext(ctx, `INSERT INTO reservations(id,user_id,amount)
 SELECT ?,user_id,? FROM subscriptions WHERE user_id=? AND
 allowance_micros-period_used_micros+topup_micros-
 COALESCE((SELECT SUM(amount) FROM reservations WHERE user_id=?),0)>=?`, id, amount, u.uid, u.uid, amount)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrBudget
	}
	return nil
}
func (u *UserStore) Settle(ctx context.Context, id string, use Usage, now time.Time) error {
	return u.s.BillingTransaction(ctx, func(ctx context.Context) error {
		res, err := u.s.billingDB(ctx).ExecContext(ctx, "DELETE FROM reservations WHERE id=? AND user_id=?", id, u.uid)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		if use.CostMicros == 0 {
			return nil
		}
		return u.Spend(ctx, use, now)
	})
}

// ReservedMicros reports allowance held by work in flight.
func (u *UserStore) ReservedMicros(ctx context.Context) (int64, error) {
	var amount int64
	err := u.s.db.QueryRowContext(ctx, "SELECT COALESCE(SUM(amount),0) FROM reservations WHERE user_id=?", u.uid).Scan(&amount)
	return amount, err
}

// FailStaleReservations releases every allowance hold left in the table and
// reports how many it released. Scry is one process, so at startup no model
// or speech call is in flight: any row still here was orphaned by an unclean
// kill between Reserve and Settle and would otherwise hold that learner's
// allowance forever. Call it once at boot, before serving, never while
// requests are running.
func (s *Store) FailStaleReservations(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, "DELETE FROM reservations")
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
