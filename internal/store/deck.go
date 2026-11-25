package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"scry/internal/deck"
)

// ImportOrigin marks the ideas an import created.
const ImportOrigin = "import"

// ImportCards adds cards straight into the learner's library in one
// transaction (S106-3): nothing waits for approval and no allowance is
// spent. Each distinct card idea becomes a top-level idea, reusing a live
// top-level idea with the same title (ignoring case) when there is one;
// cards with an empty idea go under defaultIdea, found or created the same
// way. Exact cards get their idea's title as TextTitle. source is recorded
// on every item ("Anki · Spanish.apkg").
//
// It returns the first idea a card went into, for the page to open, and the
// number of cards added. Either every card is added or none is.
func (u *UserStore) ImportCards(ctx context.Context, cards []deck.Card, defaultIdea, source string, now time.Time) (ideaID int64, n int, err error) {
	if len(cards) == 0 {
		return 0, 0, errors.New("There are no cards to import.")
	}
	defaultIdea = strings.TrimSpace(defaultIdea)
	if defaultIdea == "" {
		defaultIdea = "Imported cards"
	}
	if len(defaultIdea) > deck.MaxIdeaBytes {
		return 0, 0, errors.New("The idea title is longer than 120 characters.")
	}
	err = u.tx(ctx, func(tx *sql.Tx) error {
		type target struct {
			id    int64
			title string // as the library has it, which may differ in case
		}
		ideas := map[string]target{} // by the card's idea, within this import
		for _, c := range cards {
			c = deck.Clean(c)
			name := c.Idea
			if name == "" {
				name = defaultIdea
			}
			to, ok := ideas[name]
			if !ok {
				id, _, err := u.ensureIdea(ctx, tx, 0, name, "", ImportOrigin, now)
				if err != nil {
					return err
				}
				idea, err := u.idea(ctx, tx, id)
				if err != nil {
					return err
				}
				to = target{id, idea.Title}
				ideas[name] = to
			}
			if ideaID == 0 {
				ideaID = to.id
			}
			item := NewItem{IdeaID: to.id, Prompt: c.Question, Answer: c.Answer, Exact: c.Exact, Source: source}
			if c.Exact {
				item.TextTitle = to.title
			}
			if _, err := u.addItem(ctx, tx, item, now); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	return ideaID, n, nil
}

// DeckCards lists every card in the learner's library, for the Markdown
// and CSV exports: live cards of live ideas, in outline order (each
// top-level idea, then the ideas inside it), oldest card first within an
// idea. A card's Idea is the title of the idea it sits in, so a card inside
// a sub-idea carries the sub-idea's own title; importing the export back
// makes each of those a top-level idea.
func (u *UserStore) DeckCards(ctx context.Context) ([]deck.Card, error) {
	rows, err := u.s.db.QueryContext(ctx, `
		SELECT i.title, t.prompt, t.answer, t.exact
		FROM items t
		JOIN ideas i ON i.id = t.idea_id AND i.user_id = t.user_id
		LEFT JOIN ideas p ON p.id = i.parent_id AND p.user_id = i.user_id
		WHERE t.user_id = ? AND t.deleted_at = 0 AND i.deleted_at = 0 AND COALESCE(p.deleted_at, 0) = 0
		ORDER BY COALESCE(p.position, i.position), COALESCE(p.id, i.id), i.parent_id IS NOT NULL, i.position, i.id, t.id`, u.uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []deck.Card
	for rows.Next() {
		var c deck.Card
		var exact int
		if err := rows.Scan(&c.Idea, &c.Question, &c.Answer, &exact); err != nil {
			return nil, err
		}
		c.Exact = exact == 1
		out = append(out, c)
	}
	return out, rows.Err()
}
