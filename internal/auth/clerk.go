package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"

	clerk "github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/jwks"
	"github.com/clerk/clerk-sdk-go/v2/jwt"
	"github.com/clerk/clerk-sdk-go/v2/user"
)

type Clerk struct {
	PublishableKey, SecretKey string
	// Resolve is a stand-in seam; production always uses the official SDK.
	Resolve func(context.Context, string, string) (string, string, error)
}

func (c *Clerk) Name() string { return "clerk" }
func (c *Clerk) SignUp(context.Context, string, string) (string, error) {
	return "", errors.New("use Clerk sign-up")
}
func (c *Clerk) SignIn(context.Context, string, string) (string, error) {
	return "", errors.New("use Clerk sign-in")
}
func (c *Clerk) Domain() string {
	parts := strings.SplitN(c.PublishableKey, "_", 3)
	if len(parts) != 3 {
		return ""
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(parts[2], "="))
	host := strings.TrimSuffix(string(raw), "$")
	if err != nil || strings.ContainsAny(host, "/: \\") || !strings.Contains(host, ".") {
		return ""
	}
	return host
}
func (c *Clerk) Configured() bool { return c.SecretKey != "" && c.Domain() != "" }
func (c *Clerk) Verify(ctx context.Context, token, origin string) (string, string, error) {
	if c.Resolve != nil {
		return c.Resolve(ctx, token, origin)
	}
	if !c.Configured() {
		return "", "", errors.New("sign-in not configured")
	}
	cfg := &clerk.ClientConfig{BackendConfig: clerk.BackendConfig{Key: &c.SecretKey}}
	type statusClaims struct {
		Status string `json:"sts"`
	}
	claims, err := jwt.Verify(ctx, &jwt.VerifyParams{CustomClaimsConstructor: func(context.Context) any { return &statusClaims{} }, Token: token, JWKSClient: jwks.NewClient(cfg), AuthorizedPartyHandler: func(azp string) bool { return azp == origin }})
	if err != nil {
		return "", "", ErrBadCredentials
	}
	if status, ok := claims.Custom.(*statusClaims); ok && status.Status != "" && status.Status != "active" {
		return "", "", ErrBadCredentials
	}
	u, err := user.NewClient(cfg).Get(ctx, claims.Subject)
	if err != nil || u == nil || u.Banned || u.Locked {
		return "", "", ErrBadCredentials
	}
	return verifiedIdentity(u)
}
func verifiedIdentity(u *clerk.User) (string, string, error) {
	if u == nil || u.Banned || u.Locked {
		return "", "", ErrBadCredentials
	}
	for _, e := range u.EmailAddresses {
		if u.PrimaryEmailAddressID != nil && e.ID == *u.PrimaryEmailAddressID && e.Verification != nil && e.Verification.Status == "verified" {
			return u.ID, e.EmailAddress, nil
		}
	}
	return "", "", errors.New("verify your email before continuing")
}
func (c *Clerk) CSRF(subject string) string {
	h := hmac.New(sha256.New, []byte(c.SecretKey))
	h.Write([]byte(subject))
	return hex.EncodeToString(h.Sum(nil))
}
