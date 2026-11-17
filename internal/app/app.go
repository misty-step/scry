// Package app is Scry's behaviour: accounts, the plan and its allowance,
// study sessions, drafting, and the agents' inbox. It sits between the
// web layer and the store, and talks to the outside world only through
// the adapter interfaces.
package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/mail"
	"strings"
	"sync"
	"time"

	"scry/internal/ai"
	"scry/internal/alert"
	"scry/internal/auth"
	"scry/internal/billing"
	"scry/internal/speech"
	"scry/internal/store"
)

// Config is the product's numbers. Prices cover costs plus a small margin.
type Config struct {
	TestMode bool

	PriceCents      int64 // the monthly base
	AllowanceMicros int64 // metered model usage included each month, in micro-dollars
	TrialMicros     int64 // metered usage included in the trial
	TopupCents      int64 // price of one top-up
	TopupMicros     int64 // metered usage one top-up adds
	TrialDays       int

	SessionBudget time.Duration // how long a study session should take
	GraceDays     int           // access kept while a failed card is retried

	BackupDir      string
	BackupInterval time.Duration
	BackupKeep     int
}

// DefaultConfig is the plan as shipped.
func DefaultConfig() Config {
	return Config{
		PriceCents:      500,
		AllowanceMicros: 3_000_000,
		TrialMicros:     1_000_000,
		TopupCents:      500,
		TopupMicros:     4_000_000,
		TrialDays:       7,
		SessionBudget:   5 * time.Minute,
		GraceDays:       7,
		BackupKeep:      14,
	}
}

// App is the running product.
type App struct {
	Store  *store.Store
	Auth   auth.Provider
	Pay    billing.Processor
	Model  ai.Model
	Speech speech.Transcriber
	Alert  alert.Notifier
	Log    *slog.Logger
	Now    func() time.Time
	Cfg    Config

	drafts sync.WaitGroup
}

// FieldError is a mistake the learner can fix, said in words and shown
// under the field it belongs to.
type FieldError struct {
	Field string
	Msg   string
}

func (e *FieldError) Error() string { return e.Msg }

func fieldErr(field, msg string) error { return &FieldError{Field: field, Msg: msg} }

func (a *App) now() time.Time { return a.Now().UTC() }

// SignUp creates an account with email and password.
func (a *App) SignUp(ctx context.Context, email, password, tz string) (store.User, error) {
	email = strings.TrimSpace(email)
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || !strings.Contains(email[strings.LastIndex(email, "@")+1:], ".") {
		return store.User{}, fieldErr("email", "That doesn't look like an email address.")
	}
	if len(password) < 8 {
		return store.User{}, fieldErr("password", "Use at least 8 characters.")
	}
	if len(password) > 200 {
		return store.User{}, fieldErr("password", "Use at most 200 characters.")
	}
	if _, err := a.Store.UserByEmail(ctx, email); err == nil {
		return store.User{}, fieldErr("email", "That email already has an account.")
	}
	subject, err := a.Auth.SignUp(ctx, email, password)
	switch {
	case errors.Is(err, auth.ErrEmailTaken):
		return store.User{}, fieldErr("email", "That email already has an account.")
	case errors.Is(err, auth.ErrWeakPassword):
		return store.User{}, fieldErr("password", "That password is too easy to guess. Try a longer one.")
	case errors.Is(err, auth.ErrInvalidEmail):
		return store.User{}, fieldErr("email", "That doesn't look like an email address.")
	case err != nil:
		a.Alert.Notify(ctx, "Sign-up is failing", err.Error())
		return store.User{}, err
	}
	u, err := a.Store.CreateUser(ctx, email, subject, tz, a.now())
	if errors.Is(err, store.ErrEmailTaken) {
		return store.User{}, fieldErr("email", "That email already has an account.")
	}
	return u, err
}

// SignIn checks an email and password.
func (a *App) SignIn(ctx context.Context, email, password string) (store.User, error) {
	email = strings.TrimSpace(email)
	if email == "" || password == "" {
		return store.User{}, fieldErr("password", "Enter your email and password.")
	}
	subject, err := a.Auth.SignIn(ctx, email, password)
	if errors.Is(err, auth.ErrBadCredentials) || errors.Is(err, auth.ErrInvalidEmail) {
		return store.User{}, fieldErr("password", "That email and password don't match.")
	}
	if err != nil {
		a.Alert.Notify(ctx, "Sign-in is failing", err.Error())
		return store.User{}, err
	}
	u, err := a.Store.UserBySubject(ctx, subject)
	if errors.Is(err, store.ErrNotFound) {
		// The provider knows them but Scry does not: finish the sign-up.
		return a.Store.CreateUser(ctx, email, subject, "UTC", a.now())
	}
	return u, err
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

// SessionTTL is how long a browser stays signed in.
const SessionTTL = 30 * 24 * time.Hour

// NewSession signs a browser in and returns its cookie value.
func (a *App) NewSession(ctx context.Context, userID int64) (string, error) {
	token := randomToken(32)
	now := a.now()
	return token, a.Store.CreateSession(ctx, hashToken(token), userID, randomToken(18), now, now.Add(SessionTTL))
}

// SessionUser resolves a cookie value to its learner.
func (a *App) SessionUser(ctx context.Context, token string) (store.User, store.Session, error) {
	if token == "" {
		return store.User{}, store.Session{}, store.ErrNotFound
	}
	se, err := a.Store.SessionByHash(ctx, hashToken(token), a.now())
	if err != nil {
		return store.User{}, store.Session{}, err
	}
	u, err := a.Store.UserByID(ctx, se.UserID)
	return u, se, err
}

// KeepSession extends a session that is past half its life. It reports
// whether the cookie should be sent again with the new expiry.
func (a *App) KeepSession(ctx context.Context, token string, se store.Session) bool {
	if se.Expires.Sub(a.now()) > SessionTTL/2 {
		return false
	}
	return a.Store.ExtendSession(ctx, hashToken(token), a.now().Add(SessionTTL)) == nil
}

// SignOut ends a browser session.
func (a *App) SignOut(ctx context.Context, token string) error {
	return a.Store.DeleteSession(ctx, hashToken(token))
}

// CreateAgentToken makes a credential the learner can hand to one of
// their agents. The plain token is returned once and never stored.
func (a *App) CreateAgentToken(ctx context.Context, userID int64, name string, canAdd bool) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fieldErr("name", "Give the agent a name, such as “reading agent”.")
	}
	if len(name) > 40 {
		return "", fieldErr("name", "Keep the name under 40 characters.")
	}
	token := "scry_" + randomToken(30)
	return token, a.Store.For(userID).CreateToken(ctx, name, hashToken(token), canAdd, a.now())
}

// AgentFromToken resolves an agent's bearer token.
func (a *App) AgentFromToken(ctx context.Context, bearer string) (store.AgentToken, error) {
	if !strings.HasPrefix(bearer, "scry_") {
		return store.AgentToken{}, store.ErrNotFound
	}
	return a.Store.TokenByHash(ctx, hashToken(bearer), a.now())
}

// EndOfDay is the first moment of tomorrow where the learner lives.
// Anything due before it is due today.
func EndOfDay(u store.User, now time.Time) time.Time {
	loc := u.Location()
	n := now.In(loc)
	return time.Date(n.Year(), n.Month(), n.Day()+1, 0, 0, 0, 0, loc)
}

// Health reports whether Scry can do its job right now.
func (a *App) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return a.Store.Check(ctx)
}
