package web

import (
	"scry/internal/app"
	"scry/internal/store"
	"testing"
	"time"
)

func TestG15_AllBillingDatesUseLearnerTimezone(t *testing.T) {
	end := time.Date(2026, 12, 1, 1, 0, 0, 0, time.UTC)
	s := &Server{App: &app.App{Cfg: app.DefaultConfig(), Now: func() time.Time { return end.Add(-24 * time.Hour) }}}
	c := &rc{user: store.User{TZ: "America/Los_Angeles"}}
	v := s.billingView(c, store.Subscription{Status: "active", PeriodEnd: end, TrialEnd: end}, []store.LedgerEntry{{At: end}})
	if v.Sub.PeriodEnd.Day() != 30 || v.Sub.TrialEnd.Day() != 30 || v.Ledger[0].At.Day() != 30 {
		t.Fatalf("mixed billing zones: %+v", v)
	}
}
