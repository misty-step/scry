// Package app is Scry's behaviour: accounts, the plan and its allowance,
// study sessions, drafting, and the API tokens' additions. It sits between the
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
	// BuildSHA is the commit this binary was built from, for /healthz.
	BuildSHA string

	PriceCents      int64 // the monthly base
	AllowanceMicros int64 // metered model usage included each month, in micro-dollars
	TrialMicros     int64 // metered usage included in the no-card trial: its hard cap
	CardTrialMicros int64 // what the trial includes once a card is on file
	TopupCents      int64 // price of one top-up
	TopupMicros     int64 // metered usage one top-up adds
	TrialDays       int

	SessionBudget  time.Duration // how long a study session should take
	ExplainTimeout time.Duration // bound on one on-demand explanation; zero means ai.ExplainTimeout
	GraceDays      int           // access kept while a failed card is retried

	BackupDir      string
	BackupInterval time.Duration
	BackupKeep     int
}

// DefaultConfig is the plan as shipped.
func DefaultConfig() Config {
	return Config{
		PriceCents:      500,
		AllowanceMicros: 3_000_000,
		// The no-card trial's cap: $0.50 of metered usage, enforced at
		// admission. A hold is only admitted while what the trial has
		// spent, plus what its settles wrote off (period overage), plus
		// what is held leaves room for it (store.ReserveAt), and a settle
		// never draws more than its hold (store.SettleMetered). A provider
		// that bills more than a call's hold can still take the trial's
		// actual cost past $0.50 by that overage, which is recorded and
		// logged, never charged, and closes the trial to further holds.
		// Holds are priced per model (BudgetMicros, SpeechBudgetMicros) and
		// sized so that is rare; one worst case of each surface (a
		// gpt-6.2-sol draft is $0.23) fits inside the cap at once.
		TrialMicros:     500_000,
		CardTrialMicros: 1_000_000,
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
	Store *store.Store
	Auth  auth.Provider
	Pay   billing.Processor
	Model ai.Model
	// Explainer writes on-demand explanations after a verdict; nil means
	// the fixed criterion sentence is all there is.
	Explainer ai.Explainer
	// Bridger drafts bridge cards for a struggling card (Slice 75); nil
	// means bridges cannot be drafted right now.
	Bridger ai.Bridger
	Speech  speech.Transcriber
	Alert   alert.Notifier
	Log     *slog.Logger
	Now     func() time.Time
	Cfg     Config

	drafts sync.WaitGroup
	// topupsInFlight holds the top-up keys being charged by this process.
	topupsInFlight sync.Map
	// subInFlight holds the learners whose card step this process has with
	// the processor right now.
	subInFlight sync.Map
	explained   explainCache
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
	if err != nil {
		return u, err
	}
	a.startTrialFor(ctx, u)
	return u, nil
}

// startTrialFor starts a new account's no-card trial. A failure here does
// not fail the sign-up: the account exists, and Plan starts the trial on
// the learner's next request.
func (a *App) startTrialFor(ctx context.Context, u store.User) {
	if err := a.StartNoCardTrial(ctx, u.ID); err != nil {
		a.Log.ErrorContext(ctx, "no-card trial could not be started at sign-up", "err", err, "user", u.ID)
	}
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
		return a.CreateUser(ctx, email, subject, "UTC")
	}
	return u, err
}

// CreateUser records an account the auth provider already knows (a first
// Clerk sign-in) and starts its no-card trial.
func (a *App) CreateUser(ctx context.Context, email, subject, tz string) (store.User, error) {
	u, err := a.Store.CreateUser(ctx, email, subject, tz, a.now())
	if err != nil {
		return u, err
	}
	a.startTrialFor(ctx, u)
	return u, nil
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
// their scripts or tools; every token may add cards to their library. The
// plain token is returned once and never stored.
func (a *App) CreateAgentToken(ctx context.Context, userID int64, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fieldErr("name", "Give the token a name, such as “reading script”.")
	}
	if len(name) > 40 {
		return "", fieldErr("name", "Keep the name under 40 characters.")
	}
	token := "scry_" + randomToken(30)
	return token, a.Store.For(userID).CreateToken(ctx, name, hashToken(token), a.now())
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
