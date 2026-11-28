package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// Connected learning ([S139-2]): learning resources on a card or an idea,
// related tags on a card, and idea links named by title before the idea
// they name is in the library.
//
// Every row carries user_id and is reached only through a UserStore, so a
// title or a tag that matches in another learner's library never links,
// relates or exposes anything. Resources and tags hang off their card or
// idea: removing either (a soft delete) leaves them in place, so they come
// back with it from Recently removed, and the sweep's purge cascades to
// them. The tables are additive (CREATE TABLE IF NOT EXISTS on every open),
// so a database from before [S139-2] opens unchanged.

const resourcesSchema = `CREATE TABLE IF NOT EXISTS learning_resources (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  item_id    INTEGER REFERENCES items(id) ON DELETE CASCADE,
  idea_id    INTEGER REFERENCES ideas(id) ON DELETE CASCADE,
  kind       TEXT NOT NULL CHECK (kind IN ('explainer','article','video','diagram','link')),
  title      TEXT NOT NULL,
  url        TEXT NOT NULL DEFAULT '',
  body       TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  CHECK ((item_id IS NULL) != (idea_id IS NULL)),
  CHECK (url != '' OR body != '')
)`

// An idea keeps each resource once: the same kind, title, url and body
// sent again (a recurring script) is not added again.
const resourcesIndexes = `CREATE INDEX IF NOT EXISTS learning_resources_item ON learning_resources(user_id, item_id);
CREATE UNIQUE INDEX IF NOT EXISTS learning_resources_idea_once ON learning_resources(user_id, idea_id, kind, title, url, body) WHERE idea_id IS NOT NULL`

const tagsSchema = `CREATE TABLE IF NOT EXISTS item_tags (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  tag     TEXT NOT NULL,
  PRIMARY KEY (item_id, tag)
)`

const tagsIndex = `CREATE INDEX IF NOT EXISTS item_tags_tag ON item_tags(user_id, tag)`

// pendingLinksSchema holds relatedIdeas titles that named no idea in the
// library yet: when an idea of that title arrives, it is linked (as drafted
// links are) and the row goes.
const pendingLinksSchema = `CREATE TABLE IF NOT EXISTS idea_pending_links (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  idea_id INTEGER NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
  title   TEXT NOT NULL COLLATE NOCASE,
  PRIMARY KEY (idea_id, title)
)`

const pendingLinksIndex = `CREATE INDEX IF NOT EXISTS idea_pending_links_title ON idea_pending_links(user_id, title)`

// Resource is one learning resource: a short explainer, article, video,
// diagram or link. URL and Body are each optional; at least one is set.
type Resource struct {
	ID    int64  `json:"-"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	URL   string `json:"url,omitempty"`
	Body  string `json:"body,omitempty"`
}

// RelatedCard is another card in the library related to one card: its
// question (never its answer), its idea and when it is due.
type RelatedCard struct {
	ID        int64
	Prompt    string
	IdeaTitle string
	Due       time.Time
}

// MaxRelated is how many related cards one card lists ([S139-2]).
const MaxRelated = 5

// NormalizeTag is how a related tag is stored and compared: trimmed and
// case-folded.
func NormalizeTag(t string) string { return strings.ToLower(strings.TrimSpace(t)) }

// connect attaches what an API item carried to the card it became and its
// idea, inside the add's transaction.
func (u *UserStore) connect(ctx context.Context, tx *sql.Tx, itemID int64, s Suggestion, now time.Time) error {
	if len(s.Resources) == 0 && len(s.IdeaResources) == 0 && len(s.Tags) == 0 && len(s.RelatedIdeas) == 0 {
		return nil
	}
	var ideaID int64
	if err := tx.QueryRowContext(ctx, `SELECT idea_id FROM items WHERE id = ? AND user_id = ?`, itemID, u.uid).Scan(&ideaID); err != nil {
		return notFound(err)
	}
	for _, r := range s.Resources {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO learning_resources (user_id, item_id, kind, title, url, body, created_at)
			SELECT ?1, ?2, ?3, ?4, ?5, ?6, ?7 WHERE EXISTS (SELECT 1 FROM items WHERE id = ?2 AND user_id = ?1)`,
			u.uid, itemID, r.Kind, r.Title, r.URL, r.Body, ts(now)); err != nil {
			return err
		}
	}
	for _, r := range s.IdeaResources {
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO learning_resources (user_id, idea_id, kind, title, url, body, created_at)
			SELECT ?1, ?2, ?3, ?4, ?5, ?6, ?7 WHERE EXISTS (SELECT 1 FROM ideas WHERE id = ?2 AND user_id = ?1)`,
			u.uid, ideaID, r.Kind, r.Title, r.URL, r.Body, ts(now)); err != nil {
			return err
		}
	}
	for _, t := range s.Tags {
		if t = NormalizeTag(t); t == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO item_tags (user_id, item_id, tag) VALUES (?, ?, ?)`, u.uid, itemID, t); err != nil {
			return err
		}
	}
	return u.linkByTitles(ctx, tx, ideaID, s.RelatedIdeas)
}

// sameTitle reports whether two idea titles name the same idea: trimmed
// and compared with Unicode case folding, so "Énergie" names "énergie"
// (SQLite's NOCASE folds ASCII letters only).
func sameTitle(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// linkByTitles links an idea to the learner's ideas of these titles
// (sameTitle, never itself). A title with no idea yet is kept, and linked
// when an idea of that title arrives (resolvePendingLinks). The titles
// are matched in Go over this learner's live ideas.
func (u *UserStore) linkByTitles(ctx context.Context, tx *sql.Tx, ideaID int64, titles []string) error {
	var ideas []struct {
		id    int64
		title string
	}
	var self string
	loaded := false
	for _, title := range titles {
		title = strings.TrimSpace(title)
		if title == "" {
			continue
		}
		if !loaded {
			rows, err := tx.QueryContext(ctx, `SELECT id, title FROM ideas WHERE user_id = ? AND deleted_at = 0 ORDER BY id`, u.uid)
			if err != nil {
				return err
			}
			for rows.Next() {
				var i struct {
					id    int64
					title string
				}
				if err := rows.Scan(&i.id, &i.title); err != nil {
					rows.Close()
					return err
				}
				if i.id == ideaID {
					self = i.title
					continue
				}
				ideas = append(ideas, i)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
			loaded = true
		}
		found := false
		for _, other := range ideas {
			if !sameTitle(other.title, title) {
				continue
			}
			found = true
			if err := u.link(ctx, tx, ideaID, other.id); err != nil {
				return err
			}
		}
		// Its own title names no other idea: an idea is never linked to
		// itself.
		if found || sameTitle(self, title) {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO idea_pending_links (user_id, idea_id, title) VALUES (?, ?, ?)`, u.uid, ideaID, title); err != nil {
			return err
		}
	}
	return nil
}

// resolvePendingLinks links a newly arrived idea to every idea of this
// learner that named its title (sameTitle) in relatedIdeas before it
// existed, and drops those pending rows.
func (u *UserStore) resolvePendingLinks(ctx context.Context, q querier, ideaID int64, title string) error {
	rows, err := q.QueryContext(ctx, `SELECT idea_id, title FROM idea_pending_links WHERE user_id = ? AND idea_id != ?`, u.uid, ideaID)
	if err != nil {
		return err
	}
	type pending struct {
		from  int64
		title string
	}
	var match []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.from, &p.title); err != nil {
			rows.Close()
			return err
		}
		if sameTitle(p.title, title) {
			match = append(match, p)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, p := range match {
		if err := u.link(ctx, q, p.from, ideaID); err != nil {
			return err
		}
		if _, err := q.ExecContext(ctx, `DELETE FROM idea_pending_links WHERE user_id = ? AND idea_id = ? AND title = ?`, u.uid, p.from, p.title); err != nil {
			return err
		}
	}
	return nil
}

func scanResources(rows *sql.Rows, err error) ([]Resource, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Resource
	for rows.Next() {
		var r Resource
		if err := rows.Scan(&r.ID, &r.Kind, &r.Title, &r.URL, &r.Body); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// IdeaResources lists an idea's own learning resources, oldest first.
func (u *UserStore) IdeaResources(ctx context.Context, ideaID int64) ([]Resource, error) {
	return scanResources(u.s.db.QueryContext(ctx, `
		SELECT r.id, r.kind, r.title, r.url, r.body FROM learning_resources r
		JOIN ideas i ON i.id = r.idea_id AND i.user_id = r.user_id
		WHERE r.user_id = ? AND r.idea_id = ? AND i.deleted_at = 0 ORDER BY r.id`, u.uid, ideaID))
}

// CardResources lists the learning resources for a card: its own, then
// its idea's, oldest first within each.
func (u *UserStore) CardResources(ctx context.Context, itemID int64) ([]Resource, error) {
	return scanResources(u.s.db.QueryContext(ctx, `
		SELECT r.id, r.kind, r.title, r.url, r.body FROM items t
		JOIN ideas i ON i.id = t.idea_id AND i.user_id = t.user_id
		JOIN learning_resources r ON r.user_id = t.user_id AND (r.item_id = t.id OR r.idea_id = t.idea_id)
		WHERE t.id = ? AND t.user_id = ? AND t.deleted_at = 0 AND i.deleted_at = 0
		ORDER BY (r.item_id IS NULL), r.id`, itemID, u.uid))
}

// ItemTags lists a card's related tags, as stored (trimmed, case-folded).
func (u *UserStore) ItemTags(ctx context.Context, itemID int64) ([]string, error) {
	rows, err := u.s.db.QueryContext(ctx, `SELECT tag FROM item_tags WHERE user_id = ? AND item_id = ? ORDER BY tag`, u.uid, itemID)
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

// RelatedCards lists up to MaxRelated other cards related to one card,
// worked out from what is stored (never a model call): first the cards of
// its own idea, then cards sharing one of its related tags, then the cards
// of ideas linked to its idea, each group in Library order. It never lists
// the card itself, a removed card (or one of a removed idea), or a line of
// an exact text (an exact card with a line number, whose question quotes
// the line before it). A stand-alone exact card, a question with an
// answer judged word for word ("Give the chemical symbol for gold" ->
// "Au"), is a card like any other and is listed.
func (u *UserStore) RelatedCards(ctx context.Context, itemID int64) ([]RelatedCard, error) {
	return u.related(ctx, itemID, MaxRelated)
}

// RelatedIDs is every card related to one card by the rules of
// RelatedCards, without its limit: what a waiting card's page must not
// cue before the card is graded ([S139-2]).
func (u *UserStore) RelatedIDs(ctx context.Context, itemID int64) (map[int64]bool, error) {
	cards, err := u.related(ctx, itemID, 0)
	if err != nil {
		return nil, err
	}
	ids := make(map[int64]bool, len(cards))
	for _, c := range cards {
		ids[c.ID] = true
	}
	return ids, nil
}

// related lists the related cards in RelatedCards' order, at most limit
// of them (no limit when limit <= 0). It stops reading as soon as it has
// limit cards.
func (u *UserStore) related(ctx context.Context, itemID int64, limit int) ([]RelatedCard, error) {
	var ideaID int64
	err := u.s.db.QueryRowContext(ctx, `SELECT t.idea_id FROM items t JOIN ideas i ON i.id = t.idea_id AND i.user_id = t.user_id
		WHERE t.id = ? AND t.user_id = ? AND t.deleted_at = 0 AND i.deleted_at = 0`, itemID, u.uid).Scan(&ideaID)
	if err != nil {
		return nil, notFound(err)
	}
	const base = `SELECT t.id, t.prompt, i.title, t.due_at FROM items t JOIN ideas i ON i.id = t.idea_id AND i.user_id = t.user_id
		WHERE t.user_id = ?1 AND t.deleted_at = 0 AND i.deleted_at = 0 AND NOT (t.exact = 1 AND t.line_no > 0) AND t.id != ?2 AND `
	groups := []string{
		base + `t.idea_id = ?3 ORDER BY t.id`,
		base + `t.id IN (SELECT o.item_id FROM item_tags o JOIN item_tags m ON m.tag = o.tag AND m.user_id = o.user_id
			WHERE o.user_id = ?1 AND m.item_id = ?2) ORDER BY i.position, i.id, t.id`,
		base + `t.idea_id IN (SELECT b_id FROM idea_links WHERE user_id = ?1 AND a_id = ?3
			UNION SELECT a_id FROM idea_links WHERE user_id = ?1 AND b_id = ?3) ORDER BY i.position, i.id, t.id`,
	}
	full := func(out []RelatedCard) bool { return limit > 0 && len(out) >= limit }
	seen := map[int64]bool{}
	var out []RelatedCard
	for _, q := range groups {
		if full(out) {
			break
		}
		rows, err := u.s.db.QueryContext(ctx, q, u.uid, itemID, ideaID)
		if err != nil {
			return nil, err
		}
		for !full(out) && rows.Next() {
			var c RelatedCard
			var due int64
			if err := rows.Scan(&c.ID, &c.Prompt, &c.IdeaTitle, &due); err != nil {
				rows.Close()
				return nil, err
			}
			if seen[c.ID] {
				continue
			}
			seen[c.ID] = true
			c.Due = tm(due)
			out = append(out, c)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
