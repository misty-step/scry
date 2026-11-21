package app_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"scry/internal/app"
	"scry/internal/billing"
)

// The app reserves by its own clock and grace: a plan that ended, or whose
// grace ran out, after the caller's Plan() read is refused atomically and
// reported as ErrPlanEnded; a plan with access but no budget left is
// ErrAllowance, as before.
func TestAppReserveFollowsAccess(t *testing.T) {
	w := newBillingWorld(t, filepath.Join(t.TempDir(), "reserve.db"))
	sub, _ := w.us.Subscription(w.ctx)
	if _, err := w.app.Reserve(w.ctx, w.user.ID, 1000); err != nil {
		t.Fatalf("trial: %v", err)
	}
	if _, err := w.app.Reserve(w.ctx, w.user.ID, sub.AllowanceMicros); !errors.Is(err, app.ErrAllowance) {
		t.Fatalf("over budget: %v", err)
	}
	w.deliver(t, billing.Event{ID: "evt_fail", Kind: billing.InvoiceFailed, SubscriptionRef: sub.SubscriptionRef, Reason: "declined"})
	grace := time.Duration(w.app.Cfg.GraceDays) * 24 * time.Hour
	w.advance(grace)
	if _, err := w.app.Reserve(w.ctx, w.user.ID, 1000); err != nil {
		t.Fatalf("last moment of grace: %v", err)
	}
	if _, err := w.app.Plan(w.ctx, w.user.ID); err != nil {
		t.Fatalf("Plan at the last moment of grace: %v", err)
	}
	w.advance(time.Second) // grace runs out after the Plan() read
	if _, err := w.app.Reserve(w.ctx, w.user.ID, 1000); !errors.Is(err, app.ErrPlanEnded) {
		t.Fatalf("after grace: %v", err)
	}
	w.deliver(t, billing.Event{ID: "evt_end", Kind: billing.SubscriptionEnded, SubscriptionRef: sub.SubscriptionRef})
	if _, err := w.app.Reserve(w.ctx, w.user.ID, 1); !errors.Is(err, app.ErrPlanEnded) {
		t.Fatalf("ended: %v", err)
	}
	if held, _ := w.us.ReservedMicros(w.ctx); held != 2000 {
		t.Fatalf("held %d; want the two holds taken with access", held)
	}
}

// Housekeeping releases holds older than ReservationTTL (an abandoned
// request, or an unknown provider outcome kept held on purpose) and keeps
// younger ones; nothing is spent for them.
func TestSweepReservationsReleasesExpiredHolds(t *testing.T) {
	w := newBillingWorld(t, filepath.Join(t.TempDir(), "ttl.db"))
	if _, err := w.app.Reserve(w.ctx, w.user.ID, 5000); err != nil {
		t.Fatal(err)
	}
	w.advance(app.ReservationTTL / 2)
	if _, err := w.app.Reserve(w.ctx, w.user.ID, 3000); err != nil {
		t.Fatal(err)
	}
	if n := w.app.SweepReservations(w.ctx); n != 0 {
		t.Fatalf("swept %d before any hold expired", n)
	}
	w.advance(app.ReservationTTL/2 + time.Second)
	if n := w.app.SweepReservations(w.ctx); n != 1 {
		t.Fatalf("swept %d; want the first hold", n)
	}
	if held, _ := w.us.ReservedMicros(w.ctx); held != 3000 {
		t.Fatalf("held %d; want the younger hold", held)
	}
	w.advance(app.ReservationTTL)
	if n := w.app.SweepReservations(w.ctx); n != 1 {
		t.Fatalf("swept %d; want the second hold", n)
	}
	sub, _ := w.us.Subscription(w.ctx)
	if sub.UsedMicros != 0 {
		t.Fatalf("sweep spent %d", sub.UsedMicros)
	}
}
