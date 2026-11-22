package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"scry/internal/srs"
	"scry/internal/starter"
)

// Idea is a node in the learner's outline. Every item belongs to one.
type Idea struct {
	ID       int64
	ParentID int64 // 0 for a top-level idea
	Title    string
	Summary  string
	Origin   string // starter:<slug>, learn, agent, or empty for hand-made
	Number   string // "2" or "2.3", its place in the outline
	Depth    int
	Items    int // items directly under this idea
	Removed  time.Time
}

// Item is one question in the library with its schedule.
type Item struct {
	ID        int64
	IdeaID    int64
	IdeaTitle string
	Prompt    string
	Answer    string
	Exact     bool   // judged word for word
	TextTitle string // for a line of an exact text, the text's name
	LineNo    int
	Source    string
	Created   time.Time
	Sched     srs.State
}

// NewItem is what it takes to add an item.
type NewItem struct {
	IdeaID    int64
	Prompt    string
	Answer    string
	Exact     bool
	TextTitle string
	LineNo    int
	Source    string
}

// ExactPrompt is the cue for one line of an exact text.
func ExactPrompt(title string, lineNo int, prev string) string {
	if lineNo <= 1 || prev == "" {
		return title + ": how does it begin?"
	}
	return title + ": what follows “" + prev + "”"
}

type querier interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}

func (u *UserStore) tx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := u.s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// Outline returns the learner's ideas in reading order, numbered like a
// table of contents, with sub-ideas nested one level.
func (u *UserStore) Outline(ctx context.Context) ([]Idea, error) {
	rows, err := u.s.db.QueryContext(ctx, `
		SELECT i.id, COALESCE(i.parent_id, 0), i.title, i.summary, i.origin,
		       (SELECT COUNT(*) FROM items t WHERE t.idea_id = i.id AND t.user_id = i.user_id AND t.deleted_at = 0)
		FROM ideas i WHERE i.user_id = ? AND i.deleted_at = 0
		ORDER BY i.position, i.id`, u.uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tops []Idea
	kids := map[int64][]Idea{}
	for rows.Next() {
		var i Idea
		if err := rows.Scan(&i.ID, &i.ParentID, &i.Title, &i.Summary, &i.Origin, &i.Items); err != nil {
			return nil, err
		}
		if i.ParentID == 0 {
			tops = append(tops, i)
		} else {
			kids[i.ParentID] = append(kids[i.ParentID], i)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []Idea
	for n, t := range tops {
		t.Number = fmt.Sprint(n + 1)
		out = append(out, t)
		for k, c := range kids[t.ID] {
			c.Number, c.Depth = fmt.Sprintf("%d.%d", n+1, k+1), 1
			out = append(out, c)
		}
	}
	return out, nil
}

// Idea loads one idea.
func (u *UserStore) Idea(ctx context.Context, id int64) (Idea, error) {
	return u.idea(ctx, u.s.db, id)
}

func (u *UserStore) idea(ctx context.Context, q querier, id int64) (Idea, error) {
	var i Idea
	err := q.QueryRowContext(ctx, `
		SELECT id, COALESCE(parent_id, 0), title, summary, origin FROM ideas
		WHERE id = ? AND user_id = ? AND deleted_at = 0`, id, u.uid).
		Scan(&i.ID, &i.ParentID, &i.Title, &i.Summary, &i.Origin)
	return i, notFound(err)
}

// Related returns the ideas an idea connects to: its links in either
// direction and, for a top-level idea, the ideas inside it.
func (u *UserStore) Related(ctx context.Context, id int64) ([]Idea, error) {
	rows, err := u.s.db.QueryContext(ctx, `
		SELECT id, COALESCE(parent_id, 0), title, summary, origin FROM ideas
		WHERE user_id = ?1 AND deleted_at = 0 AND id != ?2 AND (
		      parent_id = ?2
		   OR id IN (SELECT b_id FROM idea_links WHERE a_id = ?2 AND user_id = ?1)
		   OR id IN (SELECT a_id FROM idea_links WHERE b_id = ?2 AND user_id = ?1))
		ORDER BY (parent_id = ?2) DESC, position, id`, u.uid, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Idea
	for rows.Next() {
		var i Idea
		if err := rows.Scan(&i.ID, &i.ParentID, &i.Title, &i.Summary, &i.Origin); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

const itemCols = `t.id, t.idea_id, i.title, t.prompt, t.answer, t.exact, t.text_title, t.line_no, t.source,
	t.created_at, t.due_at, t.stability, t.difficulty, t.reps, t.lapses, t.last_review`

const itemFrom = ` FROM items t JOIN ideas i ON i.id = t.idea_id AND i.user_id = t.user_id `

func scanItem(row interface{ Scan(...any) error }) (Item, error) {
	var it Item
	var exact int
	var created, due, last int64
	err := row.Scan(&it.ID, &it.IdeaID, &it.IdeaTitle, &it.Prompt, &it.Answer, &exact, &it.TextTitle, &it.LineNo, &it.Source,
		&created, &due, &it.Sched.Stability, &it.Sched.Difficulty, &it.Sched.Reps, &it.Sched.Lapses, &last)
	it.Exact, it.Created = exact == 1, tm(created)
	it.Sched.Due, it.Sched.Last = tm(due), tm(last)
	return it, notFound(err)
}

func scanItems(rows *sql.Rows, err error) ([]Item, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// Items lists the items of an idea, including those of the ideas inside it.
func (u *UserStore) Items(ctx context.Context, ideaID int64) ([]Item, error) {
	return scanItems(u.s.db.QueryContext(ctx, `SELECT `+itemCols+itemFrom+`
		WHERE t.user_id = ?1 AND t.deleted_at = 0 AND i.deleted_at = 0 AND (i.id = ?2 OR i.parent_id = ?2)
		ORDER BY (i.id = ?2) DESC, i.position, i.id, t.id`, u.uid, ideaID))
}

// Item loads one item.
func (u *UserStore) Item(ctx context.Context, id int64) (Item, error) {
	return scanItem(u.s.db.QueryRowContext(ctx, `SELECT `+itemCols+itemFrom+`
		WHERE t.id = ? AND t.user_id = ? AND t.deleted_at = 0`, id, u.uid))
}

// IdeaPlace is where an idea sits in the outline, for naming it without
// its own title: the top-level idea above it (itself when it is top-level)
// and, for an idea inside another, its 1-based place among its siblings.
type IdeaPlace struct {
	Title   string // the idea's own title
	Topic   string // the top-level idea's title
	Nested  bool   // the idea sits inside a top-level idea
	Ordinal int    // 1-based place among the parent's ideas; 0 when top-level
}

// IdeaPlace reports where one of the learner's ideas sits.
func (u *UserStore) IdeaPlace(ctx context.Context, ideaID int64) (IdeaPlace, error) {
	var p IdeaPlace
	var parent int64
	var parentTitle string
	err := u.s.db.QueryRowContext(ctx, `
		SELECT i.title, COALESCE(i.parent_id, 0), COALESCE(p.title, ''),
		       (SELECT COUNT(*) FROM ideas s WHERE s.user_id = i.user_id AND s.parent_id = i.parent_id AND s.deleted_at = 0
		          AND (s.position < i.position OR (s.position = i.position AND s.id <= i.id)))
		FROM ideas i LEFT JOIN ideas p ON p.id = i.parent_id AND p.user_id = i.user_id
		WHERE i.id = ? AND i.user_id = ?`, ideaID, u.uid).Scan(&p.Title, &parent, &parentTitle, &p.Ordinal)
	if err != nil {
		return IdeaPlace{}, notFound(err)
	}
	p.Topic = p.Title
	if parent != 0 {
		p.Nested, p.Topic = true, parentTitle
	} else {
		p.Ordinal = 0
	}
	return p, nil
}

// Search finds items whose question, answer or idea holds every word of q.
func (u *UserStore) Search(ctx context.Context, q string) ([]Item, error) {
	words := strings.Fields(q)
	if len(words) == 0 {
		return nil, nil
	}
	if len(words) > 6 {
		words = words[:6]
	}
	where := `t.user_id = ? AND t.deleted_at = 0 AND i.deleted_at = 0`
	args := []any{u.uid}
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	for _, w := range words {
		where += ` AND (t.prompt LIKE ? ESCAPE '\' OR t.answer LIKE ? ESCAPE '\' OR i.title LIKE ? ESCAPE '\')`
		p := "%" + esc.Replace(w) + "%"
		args = append(args, p, p, p)
	}
	return scanItems(u.s.db.QueryContext(ctx, `SELECT `+itemCols+itemFrom+` WHERE `+where+` ORDER BY i.position, i.id, t.id LIMIT 100`, args...))
}

// Counts reports how many items the library holds and how many are due
// before the given moment.
func (u *UserStore) Counts(ctx context.Context, dueBefore time.Time) (items, due int, err error) {
	err = u.s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(due_at < ?), 0) FROM items WHERE user_id = ? AND deleted_at = 0`,
		ts(dueBefore), u.uid).Scan(&items, &due)
	return
}

// NextDue is when the next item falls due, or zero if the library is empty.
func (u *UserStore) NextDue(ctx context.Context) (time.Time, error) {
	var v sql.NullInt64
	err := u.s.db.QueryRowContext(ctx, `SELECT MIN(due_at) FROM items WHERE user_id = ? AND deleted_at = 0`, u.uid).Scan(&v)
	return tm(v.Int64), err
}

// CreateIdea adds an idea. parentID is 0 for a top-level idea; ideas nest
// one level, so the parent must itself be top-level.
func (u *UserStore) CreateIdea(ctx context.Context, parentID int64, title, summary, origin string, now time.Time) (int64, error) {
	var id int64
	err := u.tx(ctx, func(tx *sql.Tx) error {
		var err error
		id, _, err = u.ensureIdea(ctx, tx, parentID, title, summary, origin, now)
		return err
	})
	return id, err
}

// ensureIdea finds the idea with this title under parentID or creates it.
func (u *UserStore) ensureIdea(ctx context.Context, tx *sql.Tx, parentID int64, title, summary, origin string, now time.Time) (int64, bool, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return 0, false, errors.New("an idea needs a title")
	}
	var parent any
	if parentID != 0 {
		p, err := u.idea(ctx, tx, parentID)
		if err != nil {
			return 0, false, err
		}
		if p.ParentID != 0 {
			return 0, false, errors.New("ideas nest one level")
		}
		parent = parentID
	}
	var id int64
	err := tx.QueryRowContext(ctx, `
		SELECT id FROM ideas WHERE user_id = ? AND deleted_at = 0 AND COALESCE(parent_id, 0) = ? AND title = ? COLLATE NOCASE
		ORDER BY id LIMIT 1`, u.uid, parentID, title).Scan(&id)
	if err == nil {
		return id, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO ideas (user_id, parent_id, title, summary, origin, position, created_at)
		VALUES (?, ?, ?, ?, ?, (SELECT COALESCE(MAX(position), 0) + 1 FROM ideas WHERE user_id = ?), ?)`,
		u.uid, parent, title, summary, origin, u.uid, ts(now))
	if err != nil {
		return 0, false, err
	}
	id, _ = res.LastInsertId()
	return id, true, nil
}

// findIdeaByTitle looks anywhere in the library for an idea by title,
// preferring one under parentID.
func (u *UserStore) findIdeaByTitle(ctx context.Context, q querier, parentID int64, title string) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `
		SELECT id FROM ideas WHERE user_id = ? AND deleted_at = 0 AND title = ? COLLATE NOCASE
		ORDER BY (COALESCE(parent_id, 0) = ?) DESC, id LIMIT 1`, u.uid, strings.TrimSpace(title), parentID).Scan(&id)
	return id, notFound(err)
}

func (u *UserStore) link(ctx context.Context, q querier, a, b int64) error {
	if a == b {
		return nil
	}
	if a > b {
		a, b = b, a
	}
	// The SELECT proves both ideas are this learner's before linking them.
	_, err := q.ExecContext(ctx, `
		INSERT OR IGNORE INTO idea_links (user_id, a_id, b_id)
		SELECT ?1, ?2, ?3 WHERE (SELECT COUNT(*) FROM ideas WHERE user_id = ?1 AND id IN (?2, ?3)) = 2`, u.uid, a, b)
	return err
}

// Link connects two of the learner's ideas.
func (u *UserStore) Link(ctx context.Context, a, b int64) error { return u.link(ctx, u.s.db, a, b) }

func (u *UserStore) addItem(ctx context.Context, q querier, n NewItem, now time.Time) (int64, error) {
	if strings.TrimSpace(n.Prompt) == "" || strings.TrimSpace(n.Answer) == "" {
		return 0, errors.New("an item needs a question and an answer")
	}
	// New items are due at once. The idea must be this learner's.
	res, err := q.ExecContext(ctx, `
		INSERT INTO items (user_id, idea_id, prompt, answer, exact, text_title, line_no, source, created_at, due_at)
		SELECT ?1, id, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?9 FROM ideas WHERE id = ?2 AND user_id = ?1 AND deleted_at = 0`,
		u.uid, n.IdeaID, strings.TrimSpace(n.Prompt), strings.TrimSpace(n.Answer), b2i(n.Exact), n.TextTitle, n.LineNo, n.Source, ts(now))
	if err != nil {
		return 0, err
	}
	if c, _ := res.RowsAffected(); c == 0 {
		return 0, ErrNotFound
	}
	return res.LastInsertId()
}

// AddItem adds an item to one of the learner's ideas.
func (u *UserStore) AddItem(ctx context.Context, n NewItem, now time.Time) (int64, error) {
	return u.addItem(ctx, u.s.db, n, now)
}

func (u *UserStore) addExact(ctx context.Context, tx *sql.Tx, ideaID int64, title string, lines []string, source string, now time.Time) error {
	prev := ""
	n := 0
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		n++
		if _, err := u.addItem(ctx, tx, NewItem{IdeaID: ideaID, Prompt: ExactPrompt(title, n, prev), Answer: l,
			Exact: true, TextTitle: title, LineNo: n, Source: source}, now); err != nil {
			return err
		}
		prev = l
	}
	if n == 0 {
		return errors.New("an exact text needs at least one line")
	}
	return nil
}

// AddExactText adds a text to be learned word for word: an idea named for
// the text, holding one exact item per line.
func (u *UserStore) AddExactText(ctx context.Context, parentID int64, title string, lines []string, now time.Time) (int64, error) {
	var id int64
	err := u.tx(ctx, func(tx *sql.Tx) error {
		var err error
		if id, _, err = u.ensureIdea(ctx, tx, parentID, title, "A text learned word for word.", "", now); err != nil {
			return err
		}
		return u.addExact(ctx, tx, id, strings.TrimSpace(title), lines, "", now)
	})
	return id, err
}

// UpdateItem edits an item, including whether it is judged word for word.
func (u *UserStore) UpdateItem(ctx context.Context, id int64, prompt, answer string, exact bool) error {
	if strings.TrimSpace(prompt) == "" || strings.TrimSpace(answer) == "" {
		return errors.New("an item needs a question and an answer")
	}
	return one(u.s.db.ExecContext(ctx, `
		UPDATE items SET prompt = ?, answer = ?, exact = ? WHERE id = ? AND user_id = ? AND deleted_at = 0`,
		strings.TrimSpace(prompt), strings.TrimSpace(answer), b2i(exact), id, u.uid))
}

// RemoveItem takes an item out of the library. It can be restored until
// the sweep purges it.
func (u *UserStore) RemoveItem(ctx context.Context, id int64, now time.Time) error {
	return one(u.s.db.ExecContext(ctx,
		`UPDATE items SET deleted_at = ? WHERE id = ? AND user_id = ? AND deleted_at = 0`, ts(now), id, u.uid))
}

// RemoveIdea takes an idea, the ideas inside it and all their items out of
// the library. RestoreIdea brings them back.
func (u *UserStore) RemoveIdea(ctx context.Context, id int64, now time.Time) error {
	return u.tx(ctx, func(tx *sql.Tx) error {
		if _, err := u.idea(ctx, tx, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE items SET deleted_at = ?1 WHERE user_id = ?2 AND deleted_at = 0
			AND idea_id IN (SELECT id FROM ideas WHERE user_id = ?2 AND (id = ?3 OR parent_id = ?3))`, ts(now), u.uid, id); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			UPDATE ideas SET deleted_at = ?1 WHERE user_id = ?2 AND deleted_at = 0 AND (id = ?3 OR parent_id = ?3)`, ts(now), u.uid, id)
		return err
	})
}

// Removed lists removed ideas that can still be restored.
func (u *UserStore) Removed(ctx context.Context) ([]Idea, error) {
	rows, err := u.s.db.QueryContext(ctx, `
		SELECT i.id, i.title, i.deleted_at,
		  (SELECT COUNT(*) FROM items t WHERE t.user_id = i.user_id AND t.deleted_at = i.deleted_at
		     AND t.idea_id IN (SELECT id FROM ideas c WHERE c.user_id = i.user_id AND (c.id = i.id OR c.parent_id = i.id)))
		FROM ideas i
		WHERE i.user_id = ?1 AND i.deleted_at > 0
		  AND (i.parent_id IS NULL OR (SELECT p.deleted_at FROM ideas p WHERE p.id = i.parent_id) != i.deleted_at)
		ORDER BY i.deleted_at DESC, i.id`, u.uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Idea
	for rows.Next() {
		var i Idea
		var del int64
		if err := rows.Scan(&i.ID, &i.Title, &del, &i.Items); err != nil {
			return nil, err
		}
		i.Removed = tm(del)
		out = append(out, i)
	}
	return out, rows.Err()
}

// RestoreIdea undoes RemoveIdea.
func (u *UserStore) RestoreIdea(ctx context.Context, id int64) error {
	return u.tx(ctx, func(tx *sql.Tx) error {
		var del int64
		err := tx.QueryRowContext(ctx, `SELECT deleted_at FROM ideas WHERE id = ? AND user_id = ? AND deleted_at > 0`, id, u.uid).Scan(&del)
		if err != nil {
			return notFound(err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE items SET deleted_at = 0 WHERE user_id = ?1 AND deleted_at = ?2
			AND idea_id IN (SELECT id FROM ideas WHERE user_id = ?1 AND (id = ?3 OR parent_id = ?3))`, u.uid, del, id); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE ideas SET deleted_at = 0 WHERE user_id = ?1 AND deleted_at = ?2 AND (id = ?3 OR parent_id = ?3)`, u.uid, del, id)
		return err
	})
}

// StarterAdded reports which starter sets are in the library, by slug.
func (u *UserStore) StarterAdded(ctx context.Context) (map[string]bool, error) {
	rows, err := u.s.db.QueryContext(ctx, `
		SELECT origin FROM ideas WHERE user_id = ? AND deleted_at = 0 AND parent_id IS NULL AND origin LIKE 'starter:%'`, u.uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var o string
		if err := rows.Scan(&o); err != nil {
			return nil, err
		}
		out[strings.TrimPrefix(o, "starter:")] = true
	}
	return out, rows.Err()
}

func (u *UserStore) starterIdea(ctx context.Context, q querier, l starter.Link) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `
		SELECT c.id FROM ideas c JOIN ideas p ON p.id = c.parent_id
		WHERE c.user_id = ?1 AND p.user_id = ?1 AND c.deleted_at = 0 AND p.deleted_at = 0
		  AND p.origin = ?2 AND c.title = ?3 LIMIT 1`, u.uid, "starter:"+l.Slug, l.Title).Scan(&id)
	return id, notFound(err)
}

// AddStarterSet copies a starter set into the library: a top-level idea
// for the set, its ideas beneath it, their items, and the links between
// ideas (including links to ideas from other starter sets already there).
// Adding a set twice is a no-op; added reports whether anything changed.
func (u *UserStore) AddStarterSet(ctx context.Context, set starter.Set, now time.Time) (parentID int64, added bool, err error) {
	origin := "starter:" + set.Slug
	err = u.tx(ctx, func(tx *sql.Tx) error {
		e := tx.QueryRowContext(ctx, `
			SELECT id FROM ideas WHERE user_id = ? AND deleted_at = 0 AND parent_id IS NULL AND origin = ?`, u.uid, origin).Scan(&parentID)
		if e == nil {
			return nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		res, e := tx.ExecContext(ctx, `
			INSERT INTO ideas (user_id, title, summary, origin, position, created_at)
			VALUES (?, ?, ?, ?, (SELECT COALESCE(MAX(position), 0) + 1 FROM ideas WHERE user_id = ?), ?)`,
			u.uid, set.Title, set.Description, origin, u.uid, ts(now))
		if e != nil {
			return e
		}
		parentID, _ = res.LastInsertId()
		added = true
		for _, idea := range set.Ideas {
			res, e := tx.ExecContext(ctx, `
				INSERT INTO ideas (user_id, parent_id, title, summary, origin, position, created_at)
				VALUES (?, ?, ?, ?, ?, (SELECT COALESCE(MAX(position), 0) + 1 FROM ideas WHERE user_id = ?), ?)`,
				u.uid, parentID, idea.Title, idea.Summary, origin, u.uid, ts(now))
			if e != nil {
				return e
			}
			id, _ := res.LastInsertId()
			for _, it := range idea.Items {
				if _, e := u.addItem(ctx, tx, NewItem{IdeaID: id, Prompt: it.Q, Answer: it.A, Source: "starter · " + set.Title}, now); e != nil {
					return e
				}
			}
			if idea.Exact != nil {
				if e := u.addExact(ctx, tx, id, idea.Exact.Title, idea.Exact.Lines, "starter · "+set.Title, now); e != nil {
					return e
				}
			}
		}
		// Links: every starter link with both ends now in the library.
		present, e := u.starterSlugs(ctx, tx)
		if e != nil {
			return e
		}
		for _, other := range starter.All() {
			if !present[other.Slug] {
				continue
			}
			for _, idea := range other.Ideas {
				for _, l := range idea.Links {
					to := starter.ResolveLink(other.Slug, l)
					if other.Slug != set.Slug && to.Slug != set.Slug {
						continue
					}
					a, e1 := u.starterIdea(ctx, tx, starter.Link{Slug: other.Slug, Title: idea.Title})
					b, e2 := u.starterIdea(ctx, tx, to)
					if e1 != nil || e2 != nil {
						continue
					}
					if e := u.link(ctx, tx, a, b); e != nil {
						return e
					}
				}
			}
		}
		return nil
	})
	return parentID, added, err
}

func (u *UserStore) starterSlugs(ctx context.Context, q querier) (map[string]bool, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT origin FROM ideas WHERE user_id = ? AND deleted_at = 0 AND parent_id IS NULL AND origin LIKE 'starter:%'`, u.uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var o string
		if err := rows.Scan(&o); err != nil {
			return nil, err
		}
		out[strings.TrimPrefix(o, "starter:")] = true
	}
	return out, rows.Err()
}

// IdeaTitles lists every idea title in the library, for shaping new
// material to what the learner already has.
func (u *UserStore) IdeaTitles(ctx context.Context) ([]string, error) {
	rows, err := u.s.db.QueryContext(ctx, `SELECT title FROM ideas WHERE user_id = ? AND deleted_at = 0 ORDER BY position, id`, u.uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
