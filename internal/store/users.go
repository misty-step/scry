package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ErrEmailTaken means an account already uses that email.
var ErrEmailTaken = errors.New("email already has an account")

// User is an account. There is one kind of user: no roles, no admins.
type User struct {
	ID        int64
	Email     string
	Subject   string // the auth provider's id for this person
	TZ        string
	Theme     string // system, light or dark
	About     string // what the learner told Scry about themself
	CreatedAt time.Time
}

// Location is the learner's time zone, falling back to UTC.
func (u User) Location() *time.Location {
	if loc, err := time.LoadLocation(u.TZ); err == nil {
		return loc
	}
	return time.UTC
}

const userCols = `id, email, auth_subject, tz, theme, about, created_at`

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var u User
	var created int64
	err := row.Scan(&u.ID, &u.Email, &u.Subject, &u.TZ, &u.Theme, &u.About, &created)
	u.CreatedAt = tm(created)
	return u, notFound(err)
}

// CreateUser adds an account.
func (s *Store) CreateUser(ctx context.Context, email, subject, tz string, now time.Time) (User, error) {
	if _, err := time.LoadLocation(tz); err != nil || tz == "" {
		tz = "UTC"
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (email, auth_subject, tz, created_at) VALUES (?, ?, ?, ?)`, email, subject, tz, ts(now))
	if isUnique(err) {
		return User{}, ErrEmailTaken
	}
	if err != nil {
		return User{}, err
	}
	id, _ := res.LastInsertId()
	return s.UserByID(ctx, id)
}

// UserByID loads an account.
func (s *Store) UserByID(ctx context.Context, id int64) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id))
}

// UserBySubject loads the account for an auth provider subject.
func (s *Store) UserBySubject(ctx context.Context, subject string) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE auth_subject = ?`, subject))
}

// UserByEmail loads the account for an email.
func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE email = ?`, email))
}

// CountUsers counts accounts.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// Session is a signed-in browser.
type Session struct {
	UserID  int64
	CSRF    string
	Expires time.Time
}

// CreateSession stores a browser session under the hash of its cookie.
func (s *Store) CreateSession(ctx context.Context, tokenHash string, userID int64, csrf string, now, expires time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, user_id, csrf, created_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
		tokenHash, userID, csrf, ts(now), ts(expires))
	return err
}

// SessionByHash finds a live session.
func (s *Store) SessionByHash(ctx context.Context, tokenHash string, now time.Time) (Session, error) {
	var se Session
	var exp int64
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id, csrf, expires_at FROM sessions WHERE token_hash = ? AND expires_at > ?`, tokenHash, ts(now)).
		Scan(&se.UserID, &se.CSRF, &exp)
	se.Expires = tm(exp)
	return se, notFound(err)
}

// ExtendSession pushes a live session's expiry out, so a learner who
// keeps coming back stays signed in.
func (s *Store) ExtendSession(ctx context.Context, tokenHash string, expires time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET expires_at = ? WHERE token_hash = ?`, ts(expires), tokenHash)
	return err
}

// DeleteSession signs a browser out.
func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// Sweep removes expired sessions and purges items and ideas removed more
// than keepRemoved ago.
func (s *Store) Sweep(ctx context.Context, now time.Time, keepRemoved time.Duration) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, ts(now)); err != nil {
		return err
	}
	cut := ts(now.Add(-keepRemoved))
	if _, err := s.db.ExecContext(ctx, `DELETE FROM items WHERE deleted_at > 0 AND deleted_at < ?`, cut); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM ideas WHERE deleted_at > 0 AND deleted_at < ?`, cut)
	return err
}

// AgentToken is a credential a learner gave to one of their agents.
type AgentToken struct {
	ID       int64
	UserID   int64
	Name     string
	CanAdd   bool // may add straight to the library; otherwise suggests only
	Created  time.Time
	LastUsed time.Time
}

// TokenByHash resolves an agent's bearer token to its owner.
func (s *Store) TokenByHash(ctx context.Context, hash string, now time.Time) (AgentToken, error) {
	var t AgentToken
	var created, used int64
	var canAdd int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, can_add, created_at, last_used FROM agent_tokens WHERE token_hash = ? AND revoked_at = 0`, hash).
		Scan(&t.ID, &t.UserID, &t.Name, &canAdd, &created, &used)
	if err != nil {
		return t, notFound(err)
	}
	t.CanAdd, t.Created, t.LastUsed = canAdd == 1, tm(created), tm(used)
	_, _ = s.db.ExecContext(ctx, `UPDATE agent_tokens SET last_used = ? WHERE id = ?`, ts(now), t.ID)
	return t, nil
}

// MarkEvent records a processor event id and reports whether it is new.
func (s *Store) MarkEvent(ctx context.Context, id string, now time.Time) (bool, error) {
	res, err := s.billingDB(ctx).ExecContext(ctx, `INSERT OR IGNORE INTO processed_events (id, at) VALUES (?, ?)`, id, ts(now))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// UserStore is one learner's view of the database. Every statement it
// runs carries the learner's id, so it cannot read or change anyone else's
// rows.
type UserStore struct {
	s   *Store
	uid int64
}

// For returns the store scoped to one learner.
func (s *Store) For(userID int64) *UserStore { return &UserStore{s: s, uid: userID} }

// UserID is the learner this store is scoped to.
func (u *UserStore) UserID() int64 { return u.uid }

// SetUserTheme stores the learner's colour mode. A scripted save from the
// Settings page carries the page's own tab token (themeTab, made fresh on
// every page load) and its choice number on that page (themeSeq, 1, 2, …).
// Within one tab the write is a compare-and-set: it applies only when it is
// newer than the save stored from that same tab, so a save held up on the
// way and overtaken by the same page's keepalive flush of a later choice
// cannot overwrite it. A save from any other tab (another already-open
// Settings page, another device) always applies: across tabs the last save
// to arrive wins, as for any HTTP form. No clock is involved.
//
// A plain form submit (no script) carries no tab or number (themeTab == ""
// or themeSeq <= 0); it always applies and clears the stored pair.
// It reports whether the mode was written.
func (u *UserStore) SetUserTheme(ctx context.Context, theme, themeTab string, themeSeq int64) (bool, error) {
	var (
		res sql.Result
		err error
	)
	if themeTab == "" || themeSeq <= 0 {
		res, err = u.s.db.ExecContext(ctx, `UPDATE users SET theme = ?, theme_tab = '', theme_seq = 0 WHERE id = ?`, theme, u.uid)
	} else {
		res, err = u.s.db.ExecContext(ctx, `UPDATE users SET theme = ?, theme_tab = ?, theme_seq = ? WHERE id = ? AND (theme_tab != ? OR theme_seq < ?)`,
			theme, themeTab, themeSeq, u.uid, themeTab, themeSeq)
	}
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// SetAbout stores what the learner told Scry about themself.
func (u *UserStore) SetAbout(ctx context.Context, about string) error {
	_, err := u.s.db.ExecContext(ctx, `UPDATE users SET about = ? WHERE id = ?`, about, u.uid)
	return err
}

// SetTZ stores the learner's time zone.
func (u *UserStore) SetTZ(ctx context.Context, tz string) error {
	if _, err := time.LoadLocation(tz); err != nil || tz == "" {
		return nil
	}
	_, err := u.s.db.ExecContext(ctx, `UPDATE users SET tz = ? WHERE id = ?`, tz, u.uid)
	return err
}

// CreateToken stores a new agent token by its hash.
func (u *UserStore) CreateToken(ctx context.Context, name, hash string, canAdd bool, now time.Time) error {
	_, err := u.s.db.ExecContext(ctx,
		`INSERT INTO agent_tokens (user_id, name, token_hash, can_add, created_at) VALUES (?, ?, ?, ?, ?)`,
		u.uid, name, hash, b2i(canAdd), ts(now))
	return err
}

// Tokens lists the learner's live agent tokens.
func (u *UserStore) Tokens(ctx context.Context) ([]AgentToken, error) {
	rows, err := u.s.db.QueryContext(ctx,
		`SELECT id, name, can_add, created_at, last_used FROM agent_tokens WHERE user_id = ? AND revoked_at = 0 ORDER BY id`, u.uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AgentToken
	for rows.Next() {
		t := AgentToken{UserID: u.uid}
		var canAdd int
		var created, used int64
		if err := rows.Scan(&t.ID, &t.Name, &canAdd, &created, &used); err != nil {
			return nil, err
		}
		t.CanAdd, t.Created, t.LastUsed = canAdd == 1, tm(created), tm(used)
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeToken turns an agent token off.
func (u *UserStore) RevokeToken(ctx context.Context, id int64, now time.Time) error {
	return one(u.s.db.ExecContext(ctx,
		`UPDATE agent_tokens SET revoked_at = ? WHERE id = ? AND user_id = ? AND revoked_at = 0`, ts(now), id, u.uid))
}
