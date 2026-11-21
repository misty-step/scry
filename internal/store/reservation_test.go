package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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

// planAt saves the learner's plan with a status and past_due_since.
func planAt(t *testing.T, us *UserStore, status SubscriptionStatus, pastDueSince time.Time) {
	t.Helper()
	if _, err := us.ChangeSubscription(context.Background(), func(s *Subscription) {
		s.Status, s.PastDueSince = status, pastDueSince
	}); err != nil {
		t.Fatal(err)
	}
}

func heldCount(t *testing.T, s *Store) int {
	t.Helper()
	var n int
	if err := s.DB().QueryRow("SELECT COUNT(*) FROM reservations").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The Reserve statement's access guard is app.HasAccess to the
// nanosecond: past_due keeps access while now - past_due_since <= grace,
// with past_due_since stored in whole seconds and now not.
func TestReserveGuardFollowsHasAccess(t *testing.T) {
	ctx := context.Background()
	s, users := reservationStore(t, 1, 1_000_000)
	us := users[0]
	grace := 7 * 24 * time.Hour
	base := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	half := base.Add(grace + 500*time.Millisecond) // half a second past base's grace
	for i, tc := range []struct {
		status SubscriptionStatus
		since  time.Time
		now    time.Time
		want   error
	}{
		{SubTrialing, time.Time{}, half, nil},
		{SubActive, time.Time{}, half, nil},
		{SubPastDue, base, base.Add(grace), nil},                                      // exactly grace: still in
		{SubPastDue, base, base.Add(grace + time.Nanosecond), ErrReservationNoAccess}, // just past it
		{SubPastDue, base, half, ErrReservationNoAccess},                              // 7d 0.5s
		{SubPastDue, base.Add(time.Second), half, nil},                                // 7d - 0.5s
		{SubPastDue, base, base, nil},
		{SubPastDue, time.Time{}, half, ErrReservationNoAccess}, // past_due with no since has no access
		{SubEnded, time.Time{}, half, ErrReservationNoAccess},
		{SubEnded, base, base, ErrReservationNoAccess},
	} {
		planAt(t, us, tc.status, tc.since)
		sub, _ := us.Subscription(ctx)
		hasAccess := sub.Status == SubTrialing || sub.Status == SubActive ||
			(sub.Status == SubPastDue && tc.now.Sub(sub.PastDueSince) <= grace) // app.HasAccess
		if hasAccess != (tc.want == nil) {
			t.Fatalf("%d: case disagrees with HasAccess", i)
		}
		id := fmt.Sprintf("g%d", i)
		err := us.ReserveAt(ctx, id, 10, tc.now, grace)
		if !errors.Is(err, tc.want) || (tc.want == nil) != (err == nil) {
			t.Fatalf("%d: %s since %v at %v = %v; want %v", i, tc.status, tc.since, tc.now, err, tc.want)
		}
		if err == nil {
			if err := us.Settle(ctx, id, Usage{}, tc.now); err != nil {
				t.Fatal(err)
			}
		}
	}
	if n := heldCount(t, s); n != 0 {
		t.Fatalf("%d holds left", n)
	}
	// A learner with no plan at all is out of budget, as before.
	u, err := s.CreateUser(ctx, "noplan@example.com", "noplan", "UTC", base)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.For(u.ID).ReserveAt(ctx, "np", 1, base, grace); !errors.Is(err, ErrBudget) {
		t.Fatalf("no plan: %v", err)
	}
}

// TOCTOU: the app reads the plan (Plan/HasAccess) and only then reserves.
// A plan that ends, or whose grace runs out, in between is refused by the
// Reserve statement itself.
func TestReserveRefusedWhenAccessEndsAfterPlanRead(t *testing.T) {
	ctx := context.Background()
	s, users := reservationStore(t, 1, 1_000_000)
	us := users[0]
	grace := 7 * 24 * time.Hour
	now := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)

	if sub, _ := us.Subscription(ctx); sub.Status != SubActive { // what Plan() saw
		t.Fatal(sub.Status)
	}
	planAt(t, us, SubEnded, time.Time{}) // the plan ends (a webhook) before the hold
	if err := us.ReserveAt(ctx, "late", 10, now, grace); !errors.Is(err, ErrReservationNoAccess) {
		t.Fatalf("reserve on an ended plan: %v", err)
	}

	planAt(t, us, SubPastDue, now.Add(-grace)) // the last instant of grace...
	if err := us.ReserveAt(ctx, "edge", 10, now, grace); err != nil {
		t.Fatalf("reserve at the end of grace: %v", err)
	}
	// ...and one second later, still before any re-read, it has run out.
	if err := us.ReserveAt(ctx, "expired", 10, now.Add(time.Second), grace); !errors.Is(err, ErrReservationNoAccess) {
		t.Fatalf("reserve after grace: %v", err)
	}
	if n := heldCount(t, s); n != 1 {
		t.Fatalf("%d holds; want only the one inside grace", n)
	}
	// Reserve without a clock checks the wall clock with the default grace.
	planAt(t, us, SubPastDue, time.Now().Add(-DefaultReservationGrace-time.Minute))
	if err := us.Reserve(ctx, "wall", 1); !errors.Is(err, ErrReservationNoAccess) {
		t.Fatalf("Reserve after the default grace: %v", err)
	}
	planAt(t, us, SubActive, time.Time{})
	if err := us.Reserve(ctx, "wall", 1); err != nil {
		t.Fatalf("Reserve on an active plan: %v", err)
	}
}

// Concurrent reserves racing the end of the plan: once the end has
// committed, every reserve that starts afterwards is refused, and what
// was held before never exceeds the budget.
func TestReserveRacingPlanEnd(t *testing.T) {
	ctx := context.Background()
	s, users := reservationStore(t, 1, 1_000_000)
	us := users[0]
	var ended atomic.Bool
	var wg sync.WaitGroup
	var bad atomic.Int64
	for g := 0; g < 6; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				after := ended.Load()
				err := us.ReserveAt(ctx, fmt.Sprintf("c%d-%d", g, i), 100, time.Now(), DefaultReservationGrace)
				if after && err == nil {
					bad.Add(1)
				}
				if errors.Is(err, ErrReservationNoAccess) {
					return
				}
				if err != nil && !errors.Is(err, ErrBudget) {
					t.Error(err)
					return
				}
			}
		}(g)
	}
	time.Sleep(2 * time.Millisecond)
	planAt(t, us, SubEnded, time.Time{})
	ended.Store(true)
	wg.Wait()
	if bad.Load() != 0 {
		t.Fatalf("%d reservations taken after the plan ended", bad.Load())
	}
	held, _ := us.ReservedMicros(ctx)
	if held > 1_000_000 {
		t.Fatalf("held %d over the budget", held)
	}
	if err := us.ReserveAt(ctx, "after", 1, time.Now(), DefaultReservationGrace); !errors.Is(err, ErrReservationNoAccess) {
		t.Fatalf("after the end: %v", err)
	}
	_ = s
}

func TestReserveRejectsNonPositiveAmount(t *testing.T) {
	ctx := context.Background()
	s, users := reservationStore(t, 1, 1_000_000)
	for _, amount := range []int64{0, -1, -1_000_000, math.MinInt64} {
		if err := users[0].ReserveAt(ctx, fmt.Sprintf("neg%d", amount), amount, time.Now(), DefaultReservationGrace); !errors.Is(err, ErrInvalidReservationAmount) {
			t.Fatalf("reserve %d = %v", amount, err)
		}
	}
	if n := heldCount(t, s); n != 0 {
		t.Fatalf("%d holds from non-positive amounts", n)
	}
	sub, _ := users[0].Subscription(ctx)
	if sub.UsedMicros != 0 || sub.TopupMicros != 0 {
		t.Fatalf("plan changed: %+v", sub)
	}
}

// A settle at a negative cost is refused and the hold stays held.
func TestSettleNegativeCostKeepsHold(t *testing.T) {
	ctx := context.Background()
	_, users := reservationStore(t, 1, 1_000_000)
	us := users[0]
	if err := us.Reserve(ctx, "n", 500); err != nil {
		t.Fatal(err)
	}
	if err := us.Settle(ctx, "n", Usage{CostMicros: -1}, time.Now()); !errors.Is(err, ErrNegativeCost) {
		t.Fatalf("negative settle: %v", err)
	}
	if held, _ := us.ReservedMicros(ctx); held != 500 {
		t.Fatalf("held %d", held)
	}
}

// The TTL sweep releases holds created before the cutoff second, keeps
// the rest, spends nothing, and a late settle of a swept hold is a no-op.
func TestSweepExpiredReservations(t *testing.T) {
	ctx := context.Background()
	s, users := reservationStore(t, 2, 1_000_000)
	t0 := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	ttl := time.Hour
	grace := DefaultReservationGrace
	for _, r := range []struct {
		u  int
		id string
		at time.Time
	}{
		{0, "old", t0.Add(-time.Second)},
		{1, "old-other-learner", t0.Add(-ttl)},           // swept for every learner
		{0, "edge", t0},                                  // created in the cutoff's second: kept
		{0, "edge-frac", t0.Add(900 * time.Millisecond)}, // same second: kept
		{1, "young", t0.Add(30 * time.Minute)},
	} {
		if err := users[r.u].ReserveAt(ctx, r.id, 100, r.at, grace); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := s.SweepExpiredReservations(ctx, time.Time{}); n != 0 || err != nil {
		t.Fatalf("zero cutoff swept %d, %v", n, err)
	}
	n, err := s.SweepExpiredReservations(ctx, t0.Add(999*time.Millisecond)) // the sweep at now = t0+ttl+0.999s
	if err != nil || n != 2 {
		t.Fatalf("swept %d, %v; want 2", n, err)
	}
	rows, _ := users[0].Reservations(ctx)
	if len(rows) != 2 || rows[0].ID != "edge" || rows[1].ID != "edge-frac" || !rows[0].CreatedAt.Equal(t0) {
		t.Fatalf("learner 0 holds %+v", rows)
	}
	if rows, _ := users[1].Reservations(ctx); len(rows) != 1 || rows[0].ID != "young" {
		t.Fatalf("learner 1 holds %+v", rows)
	}
	// The owner of a swept hold settles late: nothing is spent.
	if err := users[0].Settle(ctx, "old", Usage{Kind: "draft", CostMicros: 70}, t0); err != nil {
		t.Fatal(err)
	}
	if sub, _ := users[0].Subscription(ctx); sub.UsedMicros != 0 {
		t.Fatalf("settle after sweep spent %d", sub.UsedMicros)
	}
	if n, _ := s.SweepExpiredReservations(ctx, t0.Add(time.Second)); n != 2 {
		t.Fatalf("next second swept %d; want the two edge holds", n)
	}
}

// A reservations table from before created_at gains the column on open
// (idempotently); its old rows read 0 and are released by the TTL sweep,
// and FailStaleReservations still clears everything.
func TestReservationsCreatedAtMigration(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.CreateUser(ctx, "legacy@example.com", "legacy", "UTC", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	us := s.For(u.ID)
	if err := us.SaveSubscription(ctx, Subscription{SubscriptionRef: "sub_l", Status: SubActive, AllowanceMicros: 1000}); err != nil {
		t.Fatal(err)
	}
	// Recreate the table exactly as Slice 12 shipped it.
	for _, q := range []string{
		`DROP TABLE reservations`,
		`CREATE TABLE reservations(id TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id), amount INTEGER NOT NULL CHECK(amount>0))`,
		fmt.Sprintf(`INSERT INTO reservations(id,user_id,amount) VALUES ('legacy-1', %d, 300), ('legacy-2', %d, 200)`, u.ID, u.ID),
	} {
		if _, err := s.DB().Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	for i := 0; i < 2; i++ { // the migration is idempotent
		if s, err = Open(path); err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		cols, err := s.columns(ctx, s.db, "main", "reservations")
		if err != nil || strings.Join(cols, ",") != "id,user_id,amount,created_at" {
			t.Fatalf("columns %v %v", cols, err)
		}
		if i == 0 {
			s.Close()
		}
	}
	defer s.Close()
	us = s.For(u.ID)
	rows, err := us.Reservations(ctx)
	if err != nil || len(rows) != 2 || !rows[0].CreatedAt.IsZero() {
		t.Fatalf("legacy rows %+v %v", rows, err)
	}
	if held, _ := us.ReservedMicros(ctx); held != 500 {
		t.Fatalf("legacy holds %d", held)
	}
	now := time.Now()
	if err := us.ReserveAt(ctx, "new", 100, now, DefaultReservationGrace); err != nil {
		t.Fatal(err)
	}
	if n, err := s.SweepExpiredReservations(ctx, now.Add(-time.Hour)); err != nil || n != 2 {
		t.Fatalf("sweep of legacy rows: %d %v", n, err)
	}
	if rows, _ := us.Reservations(ctx); len(rows) != 1 || rows[0].ID != "new" || rows[0].CreatedAt.Unix() != now.Unix() {
		t.Fatalf("after sweep %+v", rows)
	}
	if n, err := s.FailStaleReservations(ctx); err != nil || n != 1 {
		t.Fatalf("FailStaleReservations %d %v", n, err)
	}
}
