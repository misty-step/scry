package app

import (
	"context"
	"time"

	"scry/internal/store"
)

// Run does Scry's housekeeping until ctx ends: it watches its own
// health, renews plans, takes backups and sweeps expired rows. Anything
// that goes wrong is alerted, so an outage is noticed without anyone
// being asked.
func (a *App) Run(ctx context.Context) {
	health := time.NewTicker(30 * time.Second)
	renew := time.NewTicker(time.Minute)
	sweep := time.NewTicker(time.Hour)
	defer health.Stop()
	defer renew.Stop()
	defer sweep.Stop()
	var backup <-chan time.Time
	if a.Cfg.BackupDir != "" && a.Cfg.BackupInterval > 0 {
		t := time.NewTicker(a.Cfg.BackupInterval)
		defer t.Stop()
		backup = t.C
		a.Backup(ctx) // one at start, so a fresh deploy is never without a snapshot
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-health.C:
			a.Watch(ctx)
		case <-renew.C:
			if err := a.RunRenewals(ctx); err != nil {
				a.Alert.Notify(ctx, "Plan renewals are failing", err.Error())
			}
			if err := a.ReconcileAllTopUps(ctx); err != nil {
				a.Alert.Notify(ctx, "Top-up reconciliation is failing", err.Error())
			}
		case <-sweep.C:
			if err := a.Store.Sweep(ctx, a.now(), 30*24*time.Hour); err != nil {
				a.Alert.Notify(ctx, "Housekeeping is failing", err.Error())
			}
			a.SweepReservations(ctx)
		case <-backup:
			a.Backup(ctx)
		}
	}
}

// SweepReservations releases allowance holds older than ReservationTTL
// that nobody settled, and reports how many it released.
func (a *App) SweepReservations(ctx context.Context) int64 {
	n, err := a.Store.SweepExpiredReservations(ctx, a.now().Add(-ReservationTTL))
	if err != nil {
		a.Alert.Notify(ctx, "Housekeeping is failing", "reservation sweep: "+err.Error())
		return 0
	}
	if n > 0 {
		a.Log.WarnContext(ctx, "released expired allowance reservations", "count", n, "ttl", ReservationTTL.String())
	}
	return n
}

// Watch checks Scry's own health and raises an alert when it is unwell.
func (a *App) Watch(ctx context.Context) {
	if err := a.Health(ctx); err != nil {
		a.Alert.Notify(ctx, "Scry is unhealthy", "The database cannot be written: "+err.Error())
		return
	}
	if a.Cfg.BackupDir != "" && a.Cfg.BackupInterval > 0 {
		if _, at := store.LatestSnapshot(a.Cfg.BackupDir); !at.IsZero() && time.Since(at) > 3*a.Cfg.BackupInterval {
			a.Alert.Notify(ctx, "Backups are stale", "The newest snapshot is from "+at.UTC().Format(time.RFC3339))
		}
	}
}

// Backup writes a snapshot of the database and prunes old ones.
func (a *App) Backup(ctx context.Context) (string, error) {
	path, err := a.Store.Snapshot(ctx, a.Cfg.BackupDir, a.now())
	if err == nil {
		err = store.PruneSnapshots(a.Cfg.BackupDir, max(a.Cfg.BackupKeep, 1))
	}
	if err != nil {
		a.Alert.Notify(ctx, "Backup failed", err.Error())
		return "", err
	}
	a.Log.InfoContext(ctx, "backup written", "path", path)
	return path, nil
}
