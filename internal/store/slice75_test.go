package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"scry/internal/srs"
)

// slice75World is a learner with one idea of three cards: the parent P,
// and two others. P has been reviewed (so it has a schedule to keep).
func slice75World(t *testing.T) (*Store, *UserStore, time.Time, int64, int64) {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "s75.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	now := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)
	u, err := st.CreateUser(ctx, "learner@example.com", "learner", "UTC", now)
	if err != nil {
		t.Fatal(err)
	}
	us := st.For(u.ID)
	idea, err := us.CreateIdea(ctx, 0, "Idea", "", "", now)
	if err != nil {
		t.Fatal(err)
	}
	p, err := us.AddItem(ctx, NewItem{IdeaID: idea, Prompt: "parent q", Answer: "parent a"}, now.Add(-72*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	// P was last reviewed two days ago and is due an hour ago.
	sched := srs.State{Stability: 1.2, Difficulty: 7.1, Reps: 4, Lapses: 3, Last: now.Add(-48 * time.Hour), Due: now.Add(-time.Hour)}
	if _, err := us.RecordReview(ctx, Review{ItemID: p, At: sched.Last, Answer: "x", Verdict: srs.Missed, Reason: "r"}, sched, false); err != nil {
		t.Fatal(err)
	}
	return st, us, now, idea, p
}

func dueIDs(t *testing.T, us *UserStore, before time.Time) []int64 {
	t.Helper()
	items, err := us.DueItems(context.Background(), before, 100)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	return ids
}

func contains(ids []int64, id int64) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

var twoBridges = []BridgeCard{{"bridge one q", "bridge one a"}, {"bridge two q", "bridge two a"}}

// Slice 75: accepting bridges adds them to the parent's idea and holds the
// parent out of every due queue path, without touching its schedule.
func TestSlice75_Store_AcceptHoldsTheParentOutOfEveryQueuePath(t *testing.T) {
	ctx := context.Background()
	_, us, now, idea, p := slice75World(t)
	before, _ := us.Item(ctx, p)
	if !contains(dueIDs(t, us, now.Add(time.Hour)), p) {
		t.Fatal("the parent should be due before it is held")
	}
	res, err := us.AcceptBridges(ctx, p, "key-1", twoBridges, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Bridges) != 2 || res.Duplicate {
		t.Fatalf("accept = %+v", res)
	}
	for _, b := range res.Bridges {
		it, err := us.Item(ctx, b)
		if err != nil || it.IdeaID != idea || it.Source != "bridge" || it.Sched.Reps != 0 {
			t.Fatalf("bridge %d = %+v, %v", b, it, err)
		}
	}
	// DueItems.
	due := dueIDs(t, us, now.Add(time.Hour))
	if contains(due, p) || !contains(due, res.Bridges[0]) || !contains(due, res.Bridges[1]) {
		t.Fatalf("due = %v, parent %d, bridges %v", due, p, res.Bridges)
	}
	// Counts and NextDue.
	items, dueN, err := us.Counts(ctx, now.Add(time.Hour))
	if err != nil || items != 3 || dueN != 2 {
		t.Fatalf("counts = %d items, %d due, %v", items, dueN, err)
	}
	// StartSession refuses a held item even when asked for it directly.
	sid, err := us.StartSession(ctx, []int64{p, res.Bridges[0]}, now)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := us.ActiveSession(ctx)
	if s.ID != sid || len(s.Entries) != 1 || s.Entries[0].Item.ID != res.Bridges[0] {
		t.Fatalf("session entries = %+v", s.Entries)
	}
	// The parent's schedule is exactly as it was.
	after, _ := us.Item(ctx, p)
	if after.Sched != before.Sched {
		t.Fatalf("schedule changed: %+v -> %+v", before.Sched, after.Sched)
	}
	st, _ := us.BridgeState(ctx, p)
	if st.HeldBy != 2 {
		t.Fatalf("held by %d", st.HeldBy)
	}
	bst, _ := us.BridgeState(ctx, res.Bridges[1])
	if bst.BridgeFor != p {
		t.Fatalf("bridge state = %+v", bst)
	}
}

// Slice 75: a hold made while the parent waits in an open session takes it
// out of that session at once (entries, Current, and the answer guard).
func TestSlice75_Store_AHoldTakesTheParentOutOfAnOpenSession(t *testing.T) {
	ctx := context.Background()
	_, us, now, idea, p := slice75World(t)
	other, _ := us.AddItem(ctx, NewItem{IdeaID: idea, Prompt: "other", Answer: "o"}, now)
	sid, err := us.StartSession(ctx, []int64{p, other}, now)
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := us.ActiveSession(ctx); s.Current().Item.ID != p {
		t.Fatal("the parent should be current before the hold")
	}
	if _, err := us.AcceptBridges(ctx, p, "key-open", twoBridges, now); err != nil {
		t.Fatal(err)
	}
	s, _ := us.ActiveSession(ctx)
	if cur := s.Current(); cur == nil || cur.Item.ID != other {
		t.Fatalf("current after hold = %+v", cur)
	}
	// An answer to the held parent racing the hold is refused.
	_, err = us.RecordReview(ctx, Review{ItemID: p, SessionID: sid, At: now, Answer: "a", Verdict: srs.Right, Reason: "r"}, srs.State{}, true)
	if !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("answering a held card = %v", err)
	}
}

// Slice 75: the same preview accepted twice (a retry, a double submit) or
// concurrently writes its bridges once; a different preview for a card
// already held is refused.
func TestSlice75_Store_AcceptIsIdempotentAndSafeUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	st, us, now, _, p := slice75World(t)
	var wg sync.WaitGroup
	results := make([]BridgeAccept, 8)
	errs := make([]error, 8)
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := "same-key"
			if i%2 == 1 {
				key = fmt.Sprintf("other-key-%d", i)
			}
			results[i], errs[i] = us.AcceptBridges(ctx, p, key, twoBridges, now)
		}(i)
	}
	wg.Wait()
	var made [][]int64
	for i := range 8 {
		switch {
		case errs[i] == nil:
			made = append(made, results[i].Bridges)
		case errors.Is(errs[i], ErrAlreadyHeld):
		default:
			t.Fatalf("accept %d: %v", i, errs[i])
		}
	}
	var rows int
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM item_bridges`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Fatalf("%d bridge rows, want 2", rows)
	}
	var bridgeItems int
	st.DB().QueryRow(`SELECT COUNT(*) FROM items WHERE source = 'bridge'`).Scan(&bridgeItems)
	if bridgeItems != 2 {
		t.Fatalf("%d bridge items, want 2", bridgeItems)
	}
	for _, m := range made {
		if fmt.Sprint(m) != fmt.Sprint(made[0]) {
			t.Fatalf("successful accepts disagree: %v", made)
		}
	}
	// A plain sequential retry of the winning key is a duplicate.
	var key string
	st.DB().QueryRow(`SELECT accept_key FROM item_bridges LIMIT 1`).Scan(&key)
	again, err := us.AcceptBridges(ctx, p, key, twoBridges, now)
	if err != nil || !again.Duplicate || len(again.Bridges) != 2 {
		t.Fatalf("retry = %+v, %v", again, err)
	}
	// A key used for one card cannot be replayed on another.
	idea, _ := us.CreateIdea(ctx, 0, "Other", "", "", now)
	q, _ := us.AddItem(ctx, NewItem{IdeaID: idea, Prompt: "q2", Answer: "a2"}, now)
	if _, err := us.AcceptBridges(ctx, q, key, twoBridges, now); !errors.Is(err, ErrBridgeKeyReused) {
		t.Fatalf("replayed key = %v", err)
	}
}

// Slice 75: bridges are scoped to their learner: another learner can
// neither accept bridges for, release, nor read the state of a card that
// is not theirs.
func TestSlice75_Store_BridgesRespectAccountOwnership(t *testing.T) {
	ctx := context.Background()
	st, us, now, _, p := slice75World(t)
	if _, err := us.AcceptBridges(ctx, p, "mine", twoBridges, now); err != nil {
		t.Fatal(err)
	}
	mallory, _ := st.CreateUser(ctx, "mallory@example.com", "mallory", "UTC", now)
	ms := st.For(mallory.ID)
	if _, err := ms.AcceptBridges(ctx, p, "theirs", twoBridges, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign accept = %v", err)
	}
	if _, err := ms.ReleaseHold(ctx, p, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign release = %v", err)
	}
	if s, _ := ms.BridgeState(ctx, p); s != (BridgeState{}) {
		t.Fatalf("foreign state = %+v", s)
	}
	if s, _ := us.BridgeState(ctx, p); s.HeldBy != 2 {
		t.Fatalf("own hold disturbed: %+v", s)
	}
}

// Slice 75: an Incorrect on a bridge keeps the hold; the first Correct or
// Almost on each bridge ends it; the parent comes back once no
// bridge is left, due exactly when its own schedule says.
func TestSlice75_Store_SuccessfulRatingsReleaseTheParentAutomatically(t *testing.T) {
	ctx := context.Background()
	_, us, now, _, p := slice75World(t)
	before, _ := us.Item(ctx, p)
	res, _ := us.AcceptBridges(ctx, p, "k", twoBridges, now)
	b1, b2 := res.Bridges[0], res.Bridges[1]
	review := func(item int64, v srs.Verdict, at time.Time) {
		t.Helper()
		it, _ := us.Item(ctx, item)
		next := srs.Next(it.Sched, v, at)
		if _, err := us.RecordReview(ctx, Review{ItemID: item, At: at, Answer: "a", Verdict: v, Reason: "r"}, next, false); err != nil {
			t.Fatal(err)
		}
	}
	review(b1, srs.Missed, now.Add(time.Minute))
	if s, _ := us.BridgeState(ctx, p); s.HeldBy != 2 {
		t.Fatalf("after an Incorrect: held by %d, want 2", s.HeldBy)
	}
	review(b2, srs.Right, now.Add(2*time.Minute))
	if s, _ := us.BridgeState(ctx, p); s.HeldBy != 1 {
		t.Fatalf("after a Correct: held by %d, want 1", s.HeldBy)
	}
	if contains(dueIDs(t, us, now.Add(time.Hour)), p) {
		t.Fatal("one bridge left: the parent must stay held")
	}
	// A practice attempt never masters a bridge.
	if _, err := us.RecordReview(ctx, Review{ItemID: b1, At: now.Add(3 * time.Minute), Answer: "a", Verdict: srs.Right, Reason: "r", Practice: true}, srs.State{}, false); err != nil {
		t.Fatal(err)
	}
	if s, _ := us.BridgeState(ctx, p); s.HeldBy != 1 {
		t.Fatalf("after practice: held by %d, want 1", s.HeldBy)
	}
	review(b1, srs.Close, now.Add(4*time.Minute))
	if s, _ := us.BridgeState(ctx, p); s.HeldBy != 0 {
		t.Fatalf("after an Almost: held by %d, want 0", s.HeldBy)
	}
	if !contains(dueIDs(t, us, now.Add(time.Hour)), p) {
		t.Fatal("the parent should be back in the due queue")
	}
	after, _ := us.Item(ctx, p)
	if after.Sched != before.Sched {
		t.Fatalf("release changed the schedule: %+v -> %+v", before.Sched, after.Sched)
	}
	for _, b := range []int64{b1, b2} {
		parent, reason, _, err := us.BridgeEnd(ctx, b)
		if err != nil || parent != p || reason != "mastered" {
			t.Fatalf("bridge %d end = %d %q %v", b, parent, reason, err)
		}
	}
}

// Slice 75: releasing by hand ends every bridge at once (idempotently);
// the bridges stay as ordinary cards and the parent keeps its schedule.
func TestSlice75_Store_ManualReleaseEndsTheHold(t *testing.T) {
	ctx := context.Background()
	_, us, now, _, p := slice75World(t)
	before, _ := us.Item(ctx, p)
	res, _ := us.AcceptBridges(ctx, p, "k", twoBridges, now)
	n, err := us.ReleaseHold(ctx, p, now.Add(time.Minute))
	if err != nil || n != 2 {
		t.Fatalf("release = %d, %v", n, err)
	}
	if n, err := us.ReleaseHold(ctx, p, now.Add(2*time.Minute)); err != nil || n != 0 {
		t.Fatalf("second release = %d, %v", n, err)
	}
	due := dueIDs(t, us, now.Add(time.Hour))
	if !contains(due, p) || !contains(due, res.Bridges[0]) {
		t.Fatalf("due after release = %v", due)
	}
	if s, _ := us.BridgeState(ctx, res.Bridges[0]); s.BridgeFor != 0 {
		t.Fatalf("a released bridge is still a bridge: %+v", s)
	}
	after, _ := us.Item(ctx, p)
	if after.Sched != before.Sched {
		t.Fatal("manual release changed the schedule")
	}
	// A fresh set can be accepted after a release.
	if _, err := us.AcceptBridges(ctx, p, "k2", twoBridges[:1], now.Add(3*time.Minute)); err != nil {
		t.Fatalf("re-accept after release: %v", err)
	}
}

// Slice 75: removing a bridge card ends its bridge (as removed), and the
// parent is released when it was the last; removing the parent ends its
// hold. Trashing the idea stops the hold while it is in the trash, and
// restoring the idea restores the hold of unmastered bridges.
func TestSlice75_Store_DeletionAndArchivalAreConsistent(t *testing.T) {
	ctx := context.Background()
	_, us, now, idea, p := slice75World(t)
	res, _ := us.AcceptBridges(ctx, p, "k", twoBridges, now)
	if err := us.RemoveItem(ctx, res.Bridges[0], now); err != nil {
		t.Fatal(err)
	}
	if s, _ := us.BridgeState(ctx, p); s.HeldBy != 1 {
		t.Fatalf("after removing one bridge: held by %d", s.HeldBy)
	}
	if _, reason, _, _ := us.BridgeEnd(ctx, res.Bridges[0]); reason != "removed" {
		t.Fatalf("removed bridge reason = %q", reason)
	}
	// Archival: the whole idea to the trash and back.
	if err := us.RemoveIdea(ctx, idea, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if s, _ := us.BridgeState(ctx, p); s.HeldBy != 0 {
		t.Fatalf("a trashed bridge still holds: %d", s.HeldBy)
	}
	if err := us.RestoreIdea(ctx, idea); err != nil {
		t.Fatal(err)
	}
	if s, _ := us.BridgeState(ctx, p); s.HeldBy != 1 {
		t.Fatalf("after restore: held by %d, want 1 (the removed bridge stays removed)", s.HeldBy)
	}
	if contains(dueIDs(t, us, now.Add(time.Hour)), p) {
		t.Fatal("restored with an unmastered bridge: the parent must be held")
	}
	if err := us.RemoveItem(ctx, res.Bridges[1], now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if !contains(dueIDs(t, us, now.Add(time.Hour)), p) {
		t.Fatal("last bridge removed: the parent should be due again")
	}
	// Removing a held parent ends its hold.
	res2, err := us.AcceptBridges(ctx, p, "k2", twoBridges, now.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := us.RemoveItem(ctx, p, now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	for _, b := range res2.Bridges {
		if s, _ := us.BridgeState(ctx, b); s.BridgeFor != 0 {
			t.Fatalf("bridge of a removed parent is still active: %+v", s)
		}
	}
}

// Slice 75: bridges do not nest, and a held card cannot take a second set.
func TestSlice75_Store_RefusesNestedOrDoubleHolds(t *testing.T) {
	ctx := context.Background()
	_, us, now, _, p := slice75World(t)
	res, _ := us.AcceptBridges(ctx, p, "k", twoBridges, now)
	if _, err := us.AcceptBridges(ctx, p, "k-other", twoBridges, now); !errors.Is(err, ErrAlreadyHeld) {
		t.Fatalf("second set = %v", err)
	}
	if _, err := us.AcceptBridges(ctx, res.Bridges[0], "k-nested", twoBridges, now); !errors.Is(err, ErrIsBridge) {
		t.Fatalf("nested = %v", err)
	}
}

// Slice 75: LatestVerdicts reads the newest graded verdicts per card,
// practice attempts left out.
func TestSlice75_Store_LatestVerdictsSkipPractice(t *testing.T) {
	ctx := context.Background()
	_, us, now, idea, p := slice75World(t)
	q, _ := us.AddItem(ctx, NewItem{IdeaID: idea, Prompt: "q", Answer: "a"}, now)
	add := func(item int64, v srs.Verdict, at time.Time, practice bool) {
		if _, err := us.RecordReview(ctx, Review{ItemID: item, At: at, Answer: "a", Verdict: v, Reason: "r", Practice: practice}, srs.Next(srs.State{}, v, at), false); err != nil {
			t.Fatal(err)
		}
	}
	add(q, srs.Missed, now.Add(time.Minute), false)
	add(q, srs.Right, now.Add(2*time.Minute), true)
	add(q, srs.Missed, now.Add(3*time.Minute), false)
	got, err := us.LatestVerdicts(ctx, []int64{p, q}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(got[q]) != "[missed missed]" || fmt.Sprint(got[p]) != "[missed]" {
		t.Fatalf("latest = %v", got)
	}
}

// Slice 75: the migration is additive. A database from before the slice
// (no item_bridges table) opens with every existing row and every
// existing table's definition unchanged, and gains the new table.
func TestSlice75_Store_MigrationIsAdditive(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)
	u, _ := st.CreateUser(ctx, "old@example.com", "old", "UTC", now)
	us := st.For(u.ID)
	idea, _ := us.CreateIdea(ctx, 0, "Idea", "", "", now)
	item, _ := us.AddItem(ctx, NewItem{IdeaID: idea, Prompt: "q", Answer: "a"}, now)
	us.RecordReview(ctx, Review{ItemID: item, At: now, Answer: "a", Verdict: srs.Missed, Reason: "r"}, srs.Next(srs.State{}, srs.Missed, now), false)
	// Make it a pre-Slice-75 database.
	if _, err := st.DB().Exec(`DROP INDEX item_bridges_parent; DROP TABLE item_bridges`); err != nil {
		t.Fatal(err)
	}
	schemaOf := func(db *Store) map[string]string {
		rows, err := db.DB().Query(`SELECT name, sql FROM sqlite_master WHERE sql IS NOT NULL`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		m := map[string]string{}
		for rows.Next() {
			var n, s string
			rows.Scan(&n, &s)
			m[n] = s
		}
		return m
	}
	countAll := func(db *Store) string {
		var out []string
		for _, tb := range []string{"users", "ideas", "items", "reviews", "subscriptions"} {
			var n int
			db.DB().QueryRow(`SELECT COUNT(*) FROM ` + tb).Scan(&n)
			out = append(out, fmt.Sprintf("%s=%d", tb, n))
		}
		sort.Strings(out)
		return fmt.Sprint(out)
	}
	oldSchema, oldCounts := schemaOf(st), countAll(st)
	st.Close()

	st2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	newSchema := schemaOf(st2)
	for name, sql := range oldSchema {
		if newSchema[name] != sql {
			t.Errorf("%s changed:\n%s\n->\n%s", name, sql, newSchema[name])
		}
	}
	if _, ok := newSchema["item_bridges"]; !ok {
		t.Fatal("item_bridges was not created")
	}
	if got := countAll(st2); got != oldCounts {
		t.Fatalf("rows changed: %s -> %s", oldCounts, got)
	}
	us2 := st2.For(u.ID)
	if it, err := us2.Item(ctx, item); err != nil || it.Prompt != "q" || it.Sched.Reps != 1 {
		t.Fatalf("old item = %+v, %v", it, err)
	}
	// And it works.
	if _, err := us2.AcceptBridges(ctx, item, "k", twoBridges, now); err != nil {
		t.Fatal(err)
	}
}

// Slice 75 (pass 4): a release sent while the idea is in the Trash (a
// form opened before trashing it) is the documented no-op: the suspended
// bridges are not ended, so restoring the idea restores the hold; a
// release of the visible hold after restore still ends it.
func TestSlice75_Store_StaleReleaseWhileTrashedIsANoOp(t *testing.T) {
	ctx := context.Background()
	st, us, now, idea, p := slice75World(t)
	res, err := us.AcceptBridges(ctx, p, "k", twoBridges, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := us.RemoveIdea(ctx, idea, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if n, err := us.ReleaseHold(ctx, p, now.Add(2*time.Minute)); err != nil || n != 0 {
		t.Fatalf("stale release while trashed = %d, %v; want a no-op", n, err)
	}
	var ended int
	st.DB().QueryRow(`SELECT COUNT(*) FROM item_bridges WHERE parent_item_id = ? AND ended_at != 0`, p).Scan(&ended)
	if ended != 0 {
		t.Fatalf("%d suspended bridges were ended by a stale release", ended)
	}
	if err := us.RestoreIdea(ctx, idea); err != nil {
		t.Fatal(err)
	}
	if s, _ := us.BridgeState(ctx, p); s.HeldBy != 2 {
		t.Fatalf("after restore: held by %d, want 2", s.HeldBy)
	}
	if contains(dueIDs(t, us, now.Add(time.Hour)), p) {
		t.Fatal("restored and held: the parent must stay out of the queue")
	}
	if n, err := us.ReleaseHold(ctx, p, now.Add(3*time.Minute)); err != nil || n != 2 {
		t.Fatalf("release of the visible hold = %d, %v; want 2", n, err)
	}
	if _, reason, _, _ := us.BridgeEnd(ctx, res.Bridges[1]); reason != "released" {
		t.Fatalf("released bridge reason = %q", reason)
	}
	if !contains(dueIDs(t, us, now.Add(time.Hour)), p) {
		t.Fatal("released: the parent should be due again")
	}
}
