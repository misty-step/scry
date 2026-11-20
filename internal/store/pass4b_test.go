package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// pass4bTopup is a learner on a plan with one pending $5 top-up attempt
// whose processor id is chargeRef ("" for none saved yet).
func pass4bTopup(t *testing.T, chargeRef string) (*Store, *UserStore, string) {
	t.Helper()
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "p4b.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	u, err := s.CreateUser(ctx, "p4b@example.com", "p4b", "UTC", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	us := s.For(u.ID)
	if err := us.SaveSubscription(ctx, Subscription{CustomerRef: "cus_p4b", SubscriptionRef: "sub_p4b", PaymentRef: "pm_p4b", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	att, fresh, err := us.ReserveTopup(ctx, TopupAttempt{Token: "tok-p4b-0000000000001", Key: "topup-p4b-1", AmountCents: 500, CreditMicros: 4000000, Created: time.Now()},
		func(Subscription) error { return nil })
	if err != nil || !fresh {
		t.Fatal(fresh, err)
	}
	if err := us.NoteTopupCharge(ctx, att.Token, chargeRef); err != nil {
		t.Fatal(err)
	}
	return s, us, att.Token
}

// pass4bState is the attempt, credit and ledger as SQLite holds them.
func pass4bState(t *testing.T, s *Store, us *UserStore, token string) string {
	t.Helper()
	ctx := context.Background()
	att, err := us.Topup(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	sub, err := us.Subscription(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var paid, blank int
	var refs string
	s.DB().QueryRow("SELECT count(*), coalesce(group_concat(ref),'') FROM ledger WHERE kind='topup' AND status='paid'").Scan(&paid, &refs)
	s.DB().QueryRow("SELECT count(*) FROM ledger WHERE ref=''").Scan(&blank)
	return fmt.Sprintf("%s|charge_ref=%s|credit=%d|paid_ledger=%d|paid_refs=%s|blank_ledger_refs=%d", att.Status, att.ChargeRef, sub.TopupMicros, paid, refs, blank)
}

// Pass 4b: SucceedTopup with an empty chargeRef never clears the saved
// charge_ref and books the paid line under it, exactly once.
func TestP4b_SucceedTopupEmptyRefUsesSavedChargeRef(t *testing.T) {
	ctx := context.Background()
	s, us, token := pass4bTopup(t, "pi_saved")
	for i := 0; i < 3; i++ {
		ref := ""
		if i == 2 {
			ref = "pi_saved" // a genuine settlement afterwards is a no-op
		}
		granted, err := us.SucceedTopup(ctx, token, ref, "Top-up", time.Now())
		if err != nil || granted != (i == 0) {
			t.Fatalf("call %d: granted=%t err=%v", i, granted, err)
		}
		if got, want := pass4bState(t, s, us, token), "succeeded|charge_ref=pi_saved|credit=4000000|paid_ledger=1|paid_refs=pi_saved|blank_ledger_refs=0"; got != want {
			t.Fatalf("call %d: %s; want %s", i, got, want)
		}
	}
}

// Pass 4b: SucceedTopup with an empty chargeRef and nothing saved is an
// error and changes nothing: still pending, no credit, no ledger line.
func TestP4b_SucceedTopupWithNoChargeRefAtAllIsRefused(t *testing.T) {
	ctx := context.Background()
	s, us, token := pass4bTopup(t, "")
	before := pass4bState(t, s, us, token)
	granted, err := us.SucceedTopup(ctx, token, "", "Top-up", time.Now())
	if !errors.Is(err, ErrNoChargeRef) || granted {
		t.Fatalf("granted=%t err=%v; want ErrNoChargeRef", granted, err)
	}
	if got, want := pass4bState(t, s, us, token), "pending|charge_ref=|credit=0|paid_ledger=0|paid_refs=|blank_ledger_refs=0"; got != want || got != before {
		t.Fatalf("%s; want %s", got, want)
	}
	var lines int
	s.DB().QueryRow("SELECT count(*) FROM ledger").Scan(&lines)
	if lines != 0 {
		t.Fatalf("%d ledger lines", lines)
	}
	// A genuine settlement still credits once.
	if granted, err := us.SucceedTopup(ctx, token, "pi_late", "Top-up", time.Now()); err != nil || !granted {
		t.Fatal(granted, err)
	}
	if got, want := pass4bState(t, s, us, token), "succeeded|charge_ref=pi_late|credit=4000000|paid_ledger=1|paid_refs=pi_late|blank_ledger_refs=0"; got != want {
		t.Fatalf("%s; want %s", got, want)
	}
}
