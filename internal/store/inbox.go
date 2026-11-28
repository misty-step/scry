package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Suggestion is one item of a draft outline, waiting for the learner's
// approval: nothing in it is part of the library until they approve it.
// Rows with DraftID 0 record what an API token added; they go straight
// into the library (S106-4) and never wait anywhere.
type Suggestion struct {
	ID          int64
	DraftID     int64  // 0 for an item an API token added
	Agent       string // which agent sent it
	Source      string // the reading or conversation it came from
	ParentTitle string // the top-level idea (a book, a topic) to file under
	IdeaTitle   string // the idea the item would join
	IdeaSummary string
	Links       []string // titles of related ideas
	Prompt      string
	Answer      string
	Exact       bool
	Status      string
	Created     time.Time
	Joins       bool // the idea already exists in the library
	// [S139-2], from an API item only: learning resources for the card and
	// for its idea, related tags for the card, and titles of ideas to link
	// its idea to. They are attached when the item is added; they are not
	// kept on the suggestion row.
	Resources     []Resource
	IdeaResources []Resource
	Tags          []string
	RelatedIdeas  []string
}

// SourceLine is the mono line under a suggestion: "from reading agent · The Federalist Papers".
func (s Suggestion) SourceLine() string {
	parts := []string{}
	if s.Agent != "" {
		parts = append(parts, "from "+s.Agent)
	}
	if s.Source != "" {
		parts = append(parts, s.Source)
	}
	return strings.Join(parts, " · ")
}

// AddSuggestion queues material for approval.
func (u *UserStore) AddSuggestion(ctx context.Context, s Suggestion, now time.Time) (int64, error) {
	if strings.TrimSpace(s.Prompt) == "" || strings.TrimSpace(s.Answer) == "" {
		return 0, errors.New("a suggestion needs a question and an answer")
	}
	if strings.TrimSpace(s.IdeaTitle) == "" {
		return 0, errors.New("a suggestion needs an idea to join")
	}
	links, _ := json.Marshal(append([]string{}, s.Links...))
	// A draft id must be one of this learner's drafts.
	res, err := u.s.db.ExecContext(ctx, `
		INSERT INTO suggestions (user_id, draft_id, agent, source, parent_title, idea_title, idea_summary, links, prompt, answer, exact, created_at)
		SELECT ?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12
		WHERE ?2 = 0 OR EXISTS (SELECT 1 FROM drafts WHERE id = ?2 AND user_id = ?1)`,
		u.uid, s.DraftID, s.Agent, s.Source, strings.TrimSpace(s.ParentTitle), strings.TrimSpace(s.IdeaTitle), s.IdeaSummary,
		string(links), strings.TrimSpace(s.Prompt), strings.TrimSpace(s.Answer), b2i(s.Exact), ts(now))
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, ErrNotFound
	}
	return res.LastInsertId()
}

const sugCols = `g.id, g.draft_id, g.agent, g.source, g.parent_title, g.idea_title, g.idea_summary, g.links, g.prompt, g.answer, g.exact, g.status, g.created_at,
	EXISTS (SELECT 1 FROM ideas i WHERE i.user_id = g.user_id AND i.deleted_at = 0 AND i.title = g.idea_title COLLATE NOCASE)`

func scanSuggestion(row interface{ Scan(...any) error }) (Suggestion, error) {
	var s Suggestion
	var links string
	var exact, joins int
	var created int64
	err := row.Scan(&s.ID, &s.DraftID, &s.Agent, &s.Source, &s.ParentTitle, &s.IdeaTitle, &s.IdeaSummary, &links, &s.Prompt, &s.Answer,
		&exact, &s.Status, &created, &joins)
	if err != nil {
		return s, notFound(err)
	}
	_ = json.Unmarshal([]byte(links), &s.Links)
	s.Exact, s.Joins, s.Created = exact == 1, joins == 1, tm(created)
	return s, nil
}

// Suggestions lists the pending items of a draft.
func (u *UserStore) Suggestions(ctx context.Context, draftID int64) ([]Suggestion, error) {
	rows, err := u.s.db.QueryContext(ctx, `SELECT `+sugCols+` FROM suggestions g
		WHERE g.user_id = ? AND g.draft_id = ? AND g.status = 'pending' ORDER BY g.id`, u.uid, draftID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Suggestion
	for rows.Next() {
		s, err := scanSuggestion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Suggestion loads one suggestion in any state.
func (u *UserStore) Suggestion(ctx context.Context, id int64) (Suggestion, error) {
	return scanSuggestion(u.s.db.QueryRowContext(ctx, `SELECT `+sugCols+` FROM suggestions g WHERE g.id = ? AND g.user_id = ?`, id, u.uid))
}

// SuggestionCounts reports how a draft's suggestions stand.
func (u *UserStore) SuggestionCounts(ctx context.Context, draftID int64) (pending, approved, dismissed int, err error) {
	err = u.s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(status = 'pending'), 0), COALESCE(SUM(status = 'approved'), 0), COALESCE(SUM(status = 'dismissed'), 0)
		FROM suggestions WHERE user_id = ? AND draft_id = ?`, u.uid, draftID).Scan(&pending, &approved, &dismissed)
	return
}

func (u *UserStore) approve(ctx context.Context, tx *sql.Tx, s Suggestion, now time.Time) (int64, error) {
	origin, source := "agent", s.SourceLine()
	if s.DraftID != 0 {
		origin = "learn"
	}
	var ideaID, parentID int64
	var err error
	if s.ParentTitle != "" && !strings.EqualFold(s.ParentTitle, s.IdeaTitle) {
		if parentID, _, err = u.ensureIdea(ctx, tx, 0, s.ParentTitle, "", origin, now); err != nil {
			return 0, err
		}
		if ideaID, _, err = u.ensureIdea(ctx, tx, parentID, s.IdeaTitle, s.IdeaSummary, origin, now); err != nil {
			return 0, err
		}
	} else if ideaID, err = u.findIdeaByTitle(ctx, tx, 0, s.IdeaTitle); errors.Is(err, ErrNotFound) {
		if ideaID, _, err = u.ensureIdea(ctx, tx, 0, s.IdeaTitle, s.IdeaSummary, origin, now); err != nil {
			return 0, err
		}
	} else if err != nil {
		return 0, err
	}
	item := NewItem{IdeaID: ideaID, Prompt: s.Prompt, Answer: s.Answer, Exact: s.Exact, Source: source}
	if s.Exact {
		item.TextTitle = s.IdeaTitle
	}
	itemID, err := u.addItem(ctx, tx, item, now)
	if err != nil {
		return 0, err
	}
	for _, l := range s.Links {
		if other, err := u.findIdeaByTitle(ctx, tx, parentID, l); err == nil {
			if err := u.link(ctx, tx, ideaID, other); err != nil {
				return 0, err
			}
		}
	}
	// Links named by ideas approved earlier that point at this one.
	rows, err := tx.QueryContext(ctx, `
		SELECT DISTINCT idea_title FROM suggestions, json_each(suggestions.links)
		WHERE user_id = ? AND draft_id = ? AND status = 'approved' AND json_each.value = ? COLLATE NOCASE`, u.uid, s.DraftID, s.IdeaTitle)
	if err != nil {
		return 0, err
	}
	var back []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			rows.Close()
			return 0, err
		}
		back = append(back, t)
	}
	rows.Close()
	for _, t := range back {
		if other, err := u.findIdeaByTitle(ctx, tx, parentID, t); err == nil {
			if err := u.link(ctx, tx, ideaID, other); err != nil {
				return 0, err
			}
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE suggestions SET status = 'approved', decided_at = ? WHERE id = ? AND user_id = ?`, ts(now), s.ID, u.uid)
	return itemID, err
}

// Approve moves one suggestion into the library: its idea is created if
// it is not there yet, and the item joins it. It returns the new item.
func (u *UserStore) Approve(ctx context.Context, id int64, now time.Time) (int64, error) {
	var itemID int64
	err := u.tx(ctx, func(tx *sql.Tx) error {
		s, err := scanSuggestion(tx.QueryRowContext(ctx, `SELECT `+sugCols+` FROM suggestions g WHERE g.id = ? AND g.user_id = ? AND g.status = 'pending'`, id, u.uid))
		if err != nil {
			return err
		}
		itemID, err = u.approve(ctx, tx, s, now)
		return err
	})
	return itemID, err
}

// ApproveAll approves everything waiting in one draft (or, for draftID 0,
// any old inbox suggestion still pending) and returns how many items were
// added.
func (u *UserStore) ApproveAll(ctx context.Context, draftID int64, now time.Time) (int, error) {
	n := 0
	err := u.tx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT `+sugCols+` FROM suggestions g
			WHERE g.user_id = ? AND g.draft_id = ? AND g.status = 'pending' ORDER BY g.id`, u.uid, draftID)
		if err != nil {
			return err
		}
		var all []Suggestion
		for rows.Next() {
			s, err := scanSuggestion(rows)
			if err != nil {
				rows.Close()
				return err
			}
			all = append(all, s)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, s := range all {
			if _, err := u.approve(ctx, tx, s, now); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

// AddFromAgent puts items an API token sent straight into the library, in
// one transaction: each is recorded as an approved suggestion (draft 0, so
// its source line is kept on the item) and filed under its idea, which is
// created if it is new. It returns the new items' ids in order.
func (u *UserStore) AddFromAgent(ctx context.Context, items []Suggestion, now time.Time) ([]int64, error) {
	var ids []int64
	err := u.tx(ctx, func(tx *sql.Tx) error {
		ids = ids[:0]
		for _, s := range items {
			s.DraftID = 0
			if strings.TrimSpace(s.Prompt) == "" || strings.TrimSpace(s.Answer) == "" {
				return errors.New("each item needs a question and an answer")
			}
			if strings.TrimSpace(s.IdeaTitle) == "" {
				return errors.New("each item needs an idea to belong to")
			}
			s.Prompt, s.Answer, s.IdeaTitle = strings.TrimSpace(s.Prompt), strings.TrimSpace(s.Answer), strings.TrimSpace(s.IdeaTitle)
			s.ParentTitle = strings.TrimSpace(s.ParentTitle)
			links, _ := json.Marshal(append([]string{}, s.Links...))
			res, err := tx.ExecContext(ctx, `
				INSERT INTO suggestions (user_id, draft_id, agent, source, parent_title, idea_title, idea_summary, links, prompt, answer, exact, created_at)
				VALUES (?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				u.uid, s.Agent, s.Source, s.ParentTitle, s.IdeaTitle, s.IdeaSummary, string(links), s.Prompt, s.Answer, b2i(s.Exact), ts(now))
			if err != nil {
				return err
			}
			if s.ID, err = res.LastInsertId(); err != nil {
				return err
			}
			itemID, err := u.approve(ctx, tx, s, now)
			if err != nil {
				return err
			}
			if err := u.connect(ctx, tx, itemID, s, now); err != nil {
				return err
			}
			ids = append(ids, itemID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// InboxMove is what the startup migration did for one learner: how many
// old inbox suggestions it moved into their library.
type InboxMove struct {
	UserID int64
	Moved  int
}

// MoveInboxToLibrary retires the Inbox (S106-4): every agent suggestion
// still pending (draft 0) is approved into its learner's library with the
// same logic as a tap on Approve, one learner per transaction. It is
// idempotent (an approved suggestion is never pending again), so it runs
// on every open; a learner whose move fails is reported and retried on the
// next open, and never blocks anyone else's.
func (s *Store) MoveInboxToLibrary(ctx context.Context, now time.Time) ([]InboxMove, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT user_id FROM suggestions WHERE draft_id = 0 AND status = 'pending' ORDER BY user_id`)
	if err != nil {
		return nil, err
	}
	var users []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		users = append(users, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var moves []InboxMove
	var errs []error
	for _, uid := range users {
		n, err := s.For(uid).ApproveAll(ctx, 0, now)
		if err != nil {
			errs = append(errs, fmt.Errorf("user %d: %w", uid, err))
			continue
		}
		moves = append(moves, InboxMove{UserID: uid, Moved: n})
	}
	return moves, errors.Join(errs...)
}

// Dismiss drops a suggestion without adding it.
func (u *UserStore) Dismiss(ctx context.Context, id int64, now time.Time) error {
	return one(u.s.db.ExecContext(ctx, `
		UPDATE suggestions SET status = 'dismissed', decided_at = ? WHERE id = ? AND user_id = ? AND status = 'pending'`, ts(now), id, u.uid))
}

// Draft is one answer to "What do you want to know?": an outline whose
// items wait as suggestions until approved.
type Draft struct {
	ID      int64
	Query   string
	Kind    string // topic or book
	Title   string
	Status  string // drafting, ready, empty, failed, allowance
	Note    string
	Created time.Time
}

// CreateDraft records a request for new material.
func (u *UserStore) CreateDraft(ctx context.Context, query, status string, now time.Time) (int64, error) {
	res, err := u.s.db.ExecContext(ctx, `INSERT INTO drafts (user_id, query, status, created_at) VALUES (?, ?, ?, ?)`, u.uid, query, status, ts(now))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Draft loads one draft.
func (u *UserStore) Draft(ctx context.Context, id int64) (Draft, error) {
	var d Draft
	var created int64
	err := u.s.db.QueryRowContext(ctx, `SELECT id, query, kind, title, status, note, created_at FROM drafts WHERE id = ? AND user_id = ?`, id, u.uid).
		Scan(&d.ID, &d.Query, &d.Kind, &d.Title, &d.Status, &d.Note, &created)
	d.Created = tm(created)
	return d, notFound(err)
}

// UpdateDraft sets a draft's kind, title, status and note.
func (u *UserStore) UpdateDraft(ctx context.Context, d Draft) error {
	return one(u.s.db.ExecContext(ctx, `UPDATE drafts SET kind = ?, title = ?, status = ?, note = ? WHERE id = ? AND user_id = ?`,
		d.Kind, d.Title, d.Status, d.Note, d.ID, u.uid))
}

// FailStaleDrafts marks drafts that were still being written when the
// process stopped, so no page waits on them forever.
func (s *Store) FailStaleDrafts(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE drafts SET status = 'failed', note = 'Drafting was interrupted. Ask again.' WHERE status = 'drafting'`)
	return err
}
