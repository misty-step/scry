package store

import (
	"bytes"
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"scry/internal/deck"
)

func deckStore(t *testing.T) (*Store, *UserStore, *UserStore) {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "deck.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	ctx := context.Background()
	a, err := s.CreateUser(ctx, "ada@example.com", "ada", "UTC", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateUser(ctx, "bo@example.com", "bo", "UTC", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return s, s.For(a.ID), s.For(b.ID)
}

var importCards = []deck.Card{
	{Idea: "Spanish", Question: "What is \"cat\"?", Answer: "gato"},
	{Idea: "", Question: "Closest planet to the Sun?", Answer: "Mercury"},
	{Idea: "spanish", Question: "Count to three.", Answer: "uno, dos, tres", Exact: true},
	{Idea: "Poems", Question: "Sonnet 18 begins?", Answer: "Shall I compare thee\nto a summer's day?\n\n---\n# Q: ?", Exact: true},
	{Idea: "  Spanish  ", Question: "  padded?  ", Answer: "  yes  "},
}

func TestS106_3_StoreImportCards(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	_, ada, bo := deckStore(t)
	existing, err := ada.CreateIdea(ctx, 0, "SPANISH", "", "", now)
	if err != nil {
		t.Fatal(err)
	}
	// A removed idea of the same name, and a sub-idea of it, are not reused.
	gone, _ := ada.CreateIdea(ctx, 0, "Poems", "", "", now)
	if err := ada.RemoveIdea(ctx, gone, now); err != nil {
		t.Fatal(err)
	}
	parent, _ := ada.CreateIdea(ctx, 0, "Literature", "", "", now)
	if _, err := ada.CreateIdea(ctx, parent, "Poems", "", "", now); err != nil {
		t.Fatal(err)
	}

	ideaID, n, err := ada.ImportCards(ctx, importCards, "My deck", "import · cards.csv", now)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(importCards) || ideaID != existing {
		t.Fatalf("ideaID %d (want %d), n %d", ideaID, existing, n)
	}
	items, err := ada.Items(ctx, existing)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("Spanish items: %+v", items)
	}
	for _, it := range items {
		if it.Source != "import · cards.csv" {
			t.Errorf("source %q", it.Source)
		}
		if it.Exact != (it.TextTitle == "SPANISH") {
			t.Errorf("exact %v text title %q", it.Exact, it.TextTitle)
		}
		if !it.Sched.Due.Equal(now) {
			t.Errorf("not due at once: %v", it.Sched.Due)
		}
	}
	if items[2].Prompt != "padded?" || items[2].Answer != "yes" {
		t.Errorf("not trimmed: %+v", items[2])
	}

	outline, _ := ada.Outline(ctx)
	var titles []string
	for _, i := range outline {
		titles = append(titles, i.Number+" "+i.Title)
	}
	want := []string{"1 SPANISH", "2 Literature", "2.1 Poems", "3 My deck", "4 Poems"}
	if !reflect.DeepEqual(titles, want) {
		t.Fatalf("outline %q", titles)
	}
	for _, i := range outline {
		if i.Title == "My deck" && i.Origin != ImportOrigin {
			t.Errorf("origin %q", i.Origin)
		}
	}
	// The other learner's library is untouched.
	if cards, _ := bo.DeckCards(ctx); len(cards) != 0 {
		t.Fatalf("bo sees %+v", cards)
	}
}

func TestS106_3_StoreImportIsAllOrNothing(t *testing.T) {
	ctx := context.Background()
	_, ada, _ := deckStore(t)
	bad := append(append([]deck.Card{}, importCards...), deck.Card{Idea: "Broken", Question: "no answer?", Answer: " "})
	if _, _, err := ada.ImportCards(ctx, bad, "My deck", "", time.Now()); err == nil {
		t.Fatal("imported a card with no answer")
	}
	if outline, _ := ada.Outline(ctx); len(outline) != 0 {
		t.Fatalf("partial import left %+v", outline)
	}
	if _, _, err := ada.ImportCards(ctx, nil, "My deck", "", time.Now()); err == nil {
		t.Fatal("imported nothing without saying so")
	}
}

func TestS106_3_DeckCardsOutlineOrder(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	_, ada, _ := deckStore(t)
	top, _ := ada.CreateIdea(ctx, 0, "Book", "", "", now)
	other, _ := ada.CreateIdea(ctx, 0, "Other", "", "", now)
	ch1, _ := ada.CreateIdea(ctx, top, "Chapter 1", "", "", now)
	gone, _ := ada.CreateIdea(ctx, 0, "Gone", "", "", now)
	add := func(idea int64, q string) int64 {
		id, err := ada.AddItem(ctx, NewItem{IdeaID: idea, Prompt: q, Answer: "a"}, now)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	add(other, "o1")
	add(ch1, "c1")
	add(top, "b1")
	removed := add(top, "b2")
	add(gone, "g1")
	if err := ada.RemoveItem(ctx, removed, now); err != nil {
		t.Fatal(err)
	}
	if err := ada.RemoveIdea(ctx, gone, now); err != nil {
		t.Fatal(err)
	}
	if _, err := ada.AddExactText(ctx, other, "Ode", []string{"line one", "line two"}, now); err != nil {
		t.Fatal(err)
	}
	got, err := ada.DeckCards(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []deck.Card{
		{Idea: "Book", Question: "b1", Answer: "a"},
		{Idea: "Chapter 1", Question: "c1", Answer: "a"},
		{Idea: "Other", Question: "o1", Answer: "a"},
		{Idea: "Ode", Question: "Ode: how does it begin?", Answer: "line one", Exact: true},
		{Idea: "Ode", Question: "Ode: what follows “line one”", Answer: "line two", Exact: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

// TestS106_3_StoreImportExportRoundTrip: a Markdown or CSV export imports
// back with every idea, question, answer and exact flag intact.
func TestS106_3_StoreImportExportRoundTrip(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	s, ada, _ := deckStore(t)
	if _, _, err := ada.ImportCards(ctx, importCards, "My deck", "", now); err != nil {
		t.Fatal(err)
	}
	exported, err := ada.DeckCards(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []deck.Card{
		{Idea: "Spanish", Question: "What is \"cat\"?", Answer: "gato"},
		{Idea: "Spanish", Question: "Count to three.", Answer: "uno, dos, tres", Exact: true},
		{Idea: "Spanish", Question: "padded?", Answer: "yes"},
		{Idea: "My deck", Question: "Closest planet to the Sun?", Answer: "Mercury"},
		{Idea: "Poems", Question: "Sonnet 18 begins?", Answer: "Shall I compare thee\nto a summer's day?\n\n---\n# Q: ?", Exact: true},
	}
	if !reflect.DeepEqual(exported, want) {
		t.Fatalf("exported %+v", exported)
	}

	for i, format := range []string{"md", "csv"} {
		var buf bytes.Buffer
		var back []deck.Card
		if format == "md" {
			if err := deck.WriteMarkdown(&buf, exported); err != nil {
				t.Fatal(err)
			}
			cards, structured, err := deck.ParseMarkdown(buf.String())
			if err != nil || !structured {
				t.Fatalf("md: %v %v", structured, err)
			}
			back = cards
		} else {
			if err := deck.WriteCSV(&buf, exported); err != nil {
				t.Fatal(err)
			}
			if back, err = deck.ParseDelimited(buf.Bytes(), ','); err != nil {
				t.Fatal(err)
			}
		}
		u, err := s.CreateUser(ctx, format+"@example.com", format, "UTC", now)
		if err != nil {
			t.Fatal(i, err)
		}
		fresh := s.For(u.ID)
		if _, n, err := fresh.ImportCards(ctx, back, "Unused default", "", now); err != nil || n != len(want) {
			t.Fatalf("%s: n %d err %v", format, n, err)
		}
		again, err := fresh.DeckCards(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(again, want) {
			t.Fatalf("%s round trip:\n got %+v\nwant %+v", format, again, want)
		}
	}
}
