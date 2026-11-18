// Package auth contains the Clerk browser identity adapter and a local test stand-in.
package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"

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
