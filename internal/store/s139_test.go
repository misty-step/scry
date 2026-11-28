package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func s139Store(t *testing.T) (*UserStore, time.Time) {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "s139.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	now := time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC)
	u, err := st.CreateUser(context.Background(), "s139@example.com", "s139", "UTC", now)
	if err != nil {
		t.Fatal(err)
	}
	return st.For(u.ID), now
}

func s139Linked(t *testing.T, us *UserStore, a, b int64) bool {
	t.Helper()
	rel, err := us.Related(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range rel {
		if i.ID == b {
			return true
		}
	}
	return false
}

// [S139-2] relatedIdeas matches idea titles with Unicode case folding, not
// SQLite's ASCII-only NOCASE: "Énergie" names the idea "énergie", both when
// that idea is already in the library and when it arrives afterwards.
func TestStory_S139_2_RelatedIdeasMatchAccentedTitlesInAnyCase(t *testing.T) {
	ctx := context.Background()
	us, now := s139Store(t)
	ids, err := us.AddFromAgent(ctx, []Suggestion{
		{Agent: "t", IdeaTitle: "énergie", Prompt: "Unité de l'énergie ?", Answer: "Le joule"},
		{Agent: "t", IdeaTitle: "Physique", Prompt: "Qu'est-ce que la puissance ?", Answer: "L'énergie par seconde", RelatedIdeas: []string{"  Énergie "}},
		// A title not in the library yet, in another case than the idea
		// that arrives later.
		{Agent: "t", IdeaTitle: "Chimie", Prompt: "Symbole de l'or ?", Answer: "Au", RelatedIdeas: []string{"ÉLECTRON"}},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	idea := func(itemID int64) int64 {
		it, err := us.Item(ctx, itemID)
		if err != nil {
			t.Fatal(err)
		}
		return it.IdeaID
	}
	energie, physique, chimie := idea(ids[0]), idea(ids[1]), idea(ids[2])
	if !s139Linked(t, us, physique, energie) {
		t.Error(`relatedIdeas "Énergie" did not link the existing idea "énergie"`)
	}
	later, err := us.AddFromAgent(ctx, []Suggestion{{Agent: "t", IdeaTitle: "électron", Prompt: "Charge de l'électron ?", Answer: "Négative"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !s139Linked(t, us, chimie, idea(later[0])) {
		t.Error(`pending relatedIdeas "ÉLECTRON" did not link the later idea "électron"`)
	}
	var pending int
	if err := us.s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM idea_pending_links WHERE user_id = ?`, us.uid).Scan(&pending); err != nil || pending != 0 {
		t.Errorf("pending links left = %d (%v), want 0", pending, err)
	}
	// Related cards follow the link both ways.
	rel, _ := us.RelatedCards(ctx, ids[1])
	if len(rel) != 1 || rel[0].ID != ids[0] {
		t.Errorf("related cards of the Physique card = %+v, want the énergie card", rel)
	}
}

// [S139-2] Only a line of an exact text is left out of related cards. A
// stand-alone exact card (a question whose answer is judged word for word,
// "Give the chemical symbol for gold" -> "Au") is related like any other;
// the lines of a multi-line exact text in the same idea are not.
func TestStory_S139_2_RelatedCardsKeepStandAloneExactCardsAndDropExactTextLines(t *testing.T) {
	ctx := context.Background()
	us, now := s139Store(t)
	ids, err := us.AddFromAgent(ctx, []Suggestion{
		{Agent: "t", IdeaTitle: "Elements", Prompt: "Which metal has atomic number 79?", Answer: "Gold", Tags: []string{"metals"}},
		{Agent: "t", IdeaTitle: "Elements", Prompt: "Give the chemical symbol for gold?", Answer: "Au", Exact: true},
		{Agent: "t", IdeaTitle: "Symbols", Prompt: "Give the chemical symbol for silver?", Answer: "Ag", Exact: true, Tags: []string{"Metals"}},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	gold, err := us.Item(ctx, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	// A two-line exact text in the same idea: its lines are never listed.
	if err := us.tx(ctx, func(tx *sql.Tx) error {
		return us.addExact(ctx, tx, gold.IdeaID, "Element rhyme", []string{"Hydrogen helium lithium", "Beryllium boron carbon"}, "", now)
	}); err != nil {
		t.Fatal(err)
	}
	lines := 0
	items, _ := us.Items(ctx, gold.IdeaID)
	for _, it := range items {
		if it.Exact && it.LineNo > 0 {
			lines++
		}
	}
	if lines != 2 {
		t.Fatalf("%d exact-text lines in the idea, want 2", lines)
	}
	rel, err := us.RelatedCards(ctx, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range rel {
		got = append(got, c.Prompt)
	}
	want := []string{"Give the chemical symbol for gold?", "Give the chemical symbol for silver?"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("related cards = %q, want the two stand-alone exact cards (same idea, then shared tag) and no exact-text line", got)
	}
	ids2, _ := us.RelatedIDs(ctx, ids[0])
	if len(ids2) != 2 || !ids2[ids[1]] || !ids2[ids[2]] {
		t.Errorf("RelatedIDs = %v", ids2)
	}
}

// [S139-2] At most five related cards, and RelatedIDs (what a waiting card
// holds back) is every related card, without the limit.
func TestStory_S139_2_RelatedCardsStopAtFive(t *testing.T) {
	ctx := context.Background()
	us, now := s139Store(t)
	var batch []Suggestion
	for i := 0; i < 8; i++ {
		batch = append(batch, Suggestion{Agent: "t", IdeaTitle: "Many", Prompt: "Question " + string(rune('A'+i)) + "?", Answer: "Answer"})
	}
	ids, err := us.AddFromAgent(ctx, batch, now)
	if err != nil {
		t.Fatal(err)
	}
	rel, _ := us.RelatedCards(ctx, ids[0])
	if len(rel) != MaxRelated || rel[0].ID != ids[1] || rel[4].ID != ids[5] {
		t.Errorf("related = %d cards %+v, want the next five in Library order", len(rel), rel)
	}
	all, _ := us.RelatedIDs(ctx, ids[0])
	if len(all) != 7 {
		t.Errorf("RelatedIDs = %d cards, want all 7 others", len(all))
	}
}
