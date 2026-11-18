// Package auth puts the email-and-password provider behind a small
// interface. Scry keeps its own browser sessions; the provider only
// answers "who is this, and is the password right".
package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"scry/internal/safeerr"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Errors every adapter maps its failures onto.
var (
	ErrEmailTaken     = errors.New("that email already has an account")
	ErrBadCredentials = errors.New("email or password is wrong")
	ErrWeakPassword   = errors.New("password is too weak")
	ErrInvalidEmail   = errors.New("email is not valid")
)

// Provider signs people up and checks their passwords.
type Provider interface {
	Name() string
	// SignUp creates the identity and returns the provider's stable id for it.
	SignUp(ctx context.Context, email, password string) (subject string, err error)
	// SignIn checks a password and returns the same stable id.
	SignIn(ctx context.Context, email, password string) (subject string, err error)
}

// Local is the test-mode adapter. It needs no keys: credentials live in
// the app's own database, hashed with bcrypt.
type Local struct {
	DB   *sql.DB
	Cost int // bcrypt cost; 0 means the library default
}

func (l *Local) Name() string { return "test" }

func (l *Local) SignUp(ctx context.Context, email, password string) (string, error) {
	cost := l.Cost
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", ErrWeakPassword
	}
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	subject := "local_" + hex.EncodeToString(raw)
	_, err = l.DB.ExecContext(ctx, `INSERT INTO local_credentials (subject, email, password_hash) VALUES (?, ?, ?)`, subject, email, string(hash))
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return "", ErrEmailTaken
	}
	return subject, err
}

// A fixed hash to compare against when the email is unknown, so timing
// does not reveal which emails have accounts.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("scry-no-such-user"), bcrypt.MinCost)

func (l *Local) SignIn(ctx context.Context, email, password string) (string, error) {
	var subject, hash string
	err := l.DB.QueryRowContext(ctx, `SELECT subject, password_hash FROM local_credentials WHERE email = ?`, email).Scan(&subject, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return "", ErrBadCredentials
	}
	if err != nil {
		return "", err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return "", ErrBadCredentials
	}
	return subject, nil
}

// IdentityToolkit is the real adapter: Google Identity Platform (Firebase
// Authentication) email-and-password accounts over its REST API.
// The API key comes from the environment (IDENTITY_API_KEY).
type IdentityToolkit struct {
	APIKey  string
	BaseURL string // defaults to the public endpoint; tests point it at a fake
	HTTP    *http.Client
}

func (i *IdentityToolkit) Name() string { return "identity-toolkit" }

func (i *IdentityToolkit) call(ctx context.Context, method, email, password string) (result string, retErr error) {
	defer func() { retErr = safeerr.Redact(retErr, i.APIKey) }()
	base := i.BaseURL
	if base == "" {
		base = "https://identitytoolkit.googleapis.com"
	}
	client := i.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	body, _ := json.Marshal(map[string]any{"email": email, "password": password, "returnSecureToken": true})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/accounts:"+method+"?key="+i.APIKey, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("auth provider unreachable: %w", err)
	}
	defer resp.Body.Close()
	var out struct {
		LocalID string `json:"localId"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("auth provider: status %d", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusOK && out.LocalID != "" {
		return out.LocalID, nil
	}
	msg := out.Error.Message
	switch {
	case strings.HasPrefix(msg, "EMAIL_EXISTS"):
		return "", ErrEmailTaken
	case strings.HasPrefix(msg, "WEAK_PASSWORD"):
		return "", ErrWeakPassword
	case strings.HasPrefix(msg, "INVALID_EMAIL"), strings.HasPrefix(msg, "MISSING_EMAIL"):
		return "", ErrInvalidEmail
	case strings.HasPrefix(msg, "INVALID_LOGIN_CREDENTIALS"), strings.HasPrefix(msg, "EMAIL_NOT_FOUND"),
		strings.HasPrefix(msg, "INVALID_PASSWORD"), strings.HasPrefix(msg, "USER_DISABLED"), strings.HasPrefix(msg, "MISSING_PASSWORD"):
		return "", ErrBadCredentials
	}
	return "", fmt.Errorf("auth provider: status %d %s", resp.StatusCode, msg)
}

func (i *IdentityToolkit) SignUp(ctx context.Context, email, password string) (string, error) {
	return i.call(ctx, "signUp", email, password)
}

func (i *IdentityToolkit) SignIn(ctx context.Context, email, password string) (string, error) {
	return i.call(ctx, "signInWithPassword", email, password)
}
