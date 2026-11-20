package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// reservationStore is a fresh database with n learners, each on an active
// plan with allowance micro-dollars of unspent allowance.
func reservationStore(t *testing.T, n int, allowance int64) (*Store, []*UserStore) {
	t.Helper()
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "reservations.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	var out []*UserStore
	for i := 0; i < n; i++ {
		u, err := s.CreateUser(ctx, fmt.Sprintf("r%d@example.com", i), fmt.Sprintf("r%d", i), "UTC", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		us := s.For(u.ID)
		if err := us.SaveSubscription(ctx, Subscription{SubscriptionRef: fmt.Sprintf("sub_r%d", i), CustomerRef: fmt.Sprintf("cus_r%d", i), Status: "active", AllowanceMicros: allowance}); err != nil {
			t.Fatal(err)
		}
		out = append(out, us)
	}
	return s, out
}

// Slice 12: an unclean kill between Reserve and Settle leaves rows that
// would hold a learner's allowance forever; the startup sweep releases them
// for every learner and the allowance is usable again.
func TestFailStaleReservationsReleasesOrphansAcrossUsers(t *testing.T) {
	ctx := context.Background()
	s, users := reservationStore(t, 3, 1_000_000)
	// Learner 0 has their whole allowance held by an orphan; learner 1 has
	// two smaller orphans; learner 2 has none.
	if err := users[0].Reserve(ctx, "orphan-a", 1_000_000); err != nil {
		t.Fatal(err)
	}
	if err := users[1].Reserve(ctx, "orphan-b1", 300_000); err != nil {
		t.Fatal(err)
	}
	if err := users[1].Reserve(ctx, "orphan-b2", 400_000); err != nil {
		t.Fatal(err)
	}
	if err := users[0].Reserve(ctx, "blocked", 1); !errors.Is(err, ErrBudget) {
		t.Fatalf("orphan did not block the allowance: %v", err)
	}
	if err := users[1].Reserve(ctx, "blocked", 400_000); !errors.Is(err, ErrBudget) {
		t.Fatalf("orphans did not block the allowance: %v", err)
	}

	n, err := s.FailStaleReservations(ctx)
	if err != nil || n != 3 {
		t.Fatalf("FailStaleReservations = %d, %v; want 3, nil", n, err)
	}
	for i, us := range users {
		held, err := us.ReservedMicros(ctx)
		if err != nil || held != 0 {
			t.Fatalf("learner %d still holds %d (%v)", i, held, err)
		}
	}
	var left int
	if err := s.DB().QueryRow("SELECT COUNT(*) FROM reservations").Scan(&left); err != nil || left != 0 {
		t.Fatalf("reservations left: %d (%v)", left, err)
	}
	// The calls that were refused before the sweep now succeed.
	if err := users[0].Reserve(ctx, "after-a", 1_000_000); err != nil {
		t.Fatalf("learner 0 still blocked after the sweep: %v", err)
	}
	if err := users[1].Reserve(ctx, "after-b", 1_000_000); err != nil {
		t.Fatalf("learner 1 still blocked after the sweep: %v", err)
	}
	// Spend is untouched by the sweep: nothing was charged for orphans.
	for i, us := range users {
		sub, err := us.Subscription(ctx)
		if err != nil || sub.UsedMicros != 0 {
			t.Fatalf("learner %d used %d (%v)", i, sub.UsedMicros, err)
		}
	}
}

func TestFailStaleReservationsEmptyTable(t *testing.T) {
	s, _ := reservationStore(t, 1, 1_000_000)
	n, err := s.FailStaleReservations(context.Background())
	if n != 0 || err != nil {
		t.Fatalf("FailStaleReservations on an empty table = %d, %v; want 0, nil", n, err)
	}
}

// The ordinary path is unchanged: Reserve holds, Settle releases the hold
// and charges what was used, and a second Settle is a no-op.
func TestReserveSettleUnchangedBySweep(t *testing.T) {
	ctx := context.Background()
	s, users := reservationStore(t, 1, 1_000_000)
	us := users[0]
	if _, err := s.FailStaleReservations(ctx); err != nil {
		t.Fatal(err)
	}
	if err := us.Reserve(ctx, "r1", 600_000); err != nil {
		t.Fatal(err)
	}
	if held, _ := us.ReservedMicros(ctx); held != 600_000 {
		t.Fatalf("held %d", held)
	}
	if err := us.Reserve(ctx, "r2", 600_000); !errors.Is(err, ErrBudget) {
		t.Fatalf("over-reservation allowed: %v", err)
	}
	now := time.Now()
	if err := us.Settle(ctx, "r1", Usage{Kind: "judge", Model: "test", CostMicros: 250_000}, now); err != nil {
		t.Fatal(err)
	}
	if err := us.Settle(ctx, "r1", Usage{Kind: "judge", Model: "test", CostMicros: 250_000}, now); err != nil {
		t.Fatal(err)
	}
	if held, _ := us.ReservedMicros(ctx); held != 0 {
		t.Fatalf("held after settle %d", held)
	}
	sub, err := us.Subscription(ctx)
	if err != nil || sub.UsedMicros != 250_000 {
		t.Fatalf("used %d (%v); want one settle of 250000", sub.UsedMicros, err)
	}
	if err := us.Reserve(ctx, "r3", 750_000); err != nil {
		t.Fatalf("remaining allowance not reservable: %v", err)
	}
}
