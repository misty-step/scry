package web

import (
	"context"
	"sort"
	"time"
)

// removedRow is one line of Settings → Recently removed: a removed idea
// (with the ideas and cards inside it) or a card removed on its own, each
// with "Restore", kept 30 days until the sweep purges it (S106-5).
type removedRow struct {
	Kind    string // "idea" or "card"
	ID      int64
	Title   string // the idea's title, or the card's question
	Detail  string // "26 items", or "Card · <its idea>"
	Removed time.Time
}

// recentlyRemoved lists removed ideas and removed cards, newest first.
func recentlyRemoved(ctx context.Context, c *rc) ([]removedRow, error) {
	ideas, err := c.us.Removed(ctx)
	if err != nil {
		return nil, err
	}
	cards, err := c.us.RemovedCards(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]removedRow, 0, len(ideas)+len(cards))
	for _, i := range ideas {
		rows = append(rows, removedRow{Kind: "idea", ID: i.ID, Title: i.Title, Detail: plural(i.Items, "item", "items"), Removed: i.Removed})
	}
	for _, t := range cards {
		rows = append(rows, removedRow{Kind: "card", ID: t.ID, Title: truncate(90, t.Prompt), Detail: "Card · " + t.IdeaTitle, Removed: t.Removed})
	}
	sort.SliceStable(rows, func(a, b int) bool { return rows[a].Removed.After(rows[b].Removed) })
	return rows, nil
}
