package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	clerk "github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/jwks"
	"github.com/clerk/clerk-sdk-go/v2/jwt"
	"github.com/clerk/clerk-sdk-go/v2/user"
)

// Cache bounds for Clerk. The SDK's jwks.Client does not cache (every Get
// is an HTTPS call to the Backend API) and user.Get is one too, so without
// these each signed-in page view made two synchronous round trips to Clerk.
const (
	// DefaultJWKSTTL is how long a fetched key set is trusted before it is
	// fetched again.
	DefaultJWKSTTL = 10 * time.Minute
	// DefaultUserTTL is how long a verified, active identity is reused. A
	// ban, lock or lost verification is noticed within this window; the
	// session token itself is still verified on every request.
	DefaultUserTTL = 60 * time.Second
	// DefaultUserCacheSize bounds the identity cache.
	DefaultUserCacheSize = 1024
	// jwksMissFloor is the least time between two key-set fetches caused
	// by an unknown kid, so tokens with made-up kids cannot turn every
	// request into a Clerk API call. A real rotation publishes the new key
	// well before signing with it.
	jwksMissFloor = 30 * time.Second
)

var errUnknownKey = errors.New("session token signed with an unknown key")

// Clerk verifies Clerk session tokens. The zero value plus PublishableKey
// and SecretKey is ready to use: the SDK clients and caches are created on
// first use, so a struct literal is enough.
//
// Every call to Verify checks the token's signature, expiry, issuer, azp
// and sts. Only the key set and the identity lookup are cached, and only
// successes are: errors, banned or locked accounts and unverified emails are
// never remembered.
type Clerk struct {
	PublishableKey, SecretKey string
	// Resolve is a stand-in seam; production always uses the official SDK.
	Resolve func(context.Context, string, string) (string, string, error)

	// Now is the clock for token expiry and both caches; nil is time.Now.
	Now func() time.Time
	// FetchJWKS and LookupUser stand in for the Clerk Backend API in tests;
	// nil uses the SDK clients, built once and reused.
	FetchJWKS  func(context.Context) (*clerk.JSONWebKeySet, error)
	LookupUser func(context.Context, string) (*clerk.User, error)
	// Cache bounds; zero means the defaults above.
	JWKSTTL, UserTTL time.Duration
	MaxUsers         int

	apiURL string // package tests only: point the SDK clients at a stand-in

	clients    sync.Once
	jwksClient *jwks.Client
	userClient *user.Client

	fetching sync.Mutex // one key-set fetch at a time
	mu       sync.Mutex // guards keys, keysAt and users
	keys     *clerk.JSONWebKeySet
	keysAt   time.Time
	users    map[string]cachedIdentity
}

type cachedIdentity struct {
	id, email string
	expires   time.Time
}

func (c *Clerk) Name() string { return "clerk" }

// Mode is "clerk-live" for a production instance (pk_live_), "clerk-test"
// otherwise.
func (c *Clerk) Mode() string {
	if strings.HasPrefix(c.PublishableKey, "pk_live_") {
		return "clerk-live"
	}
	return "clerk-test"
}

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

func (c *Clerk) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

type clockFunc func() time.Time

func (f clockFunc) Now() time.Time { return f() }

func (c *Clerk) sdk() (*jwks.Client, *user.Client) {
	c.clients.Do(func() {
		key := c.SecretKey
		jcfg := &clerk.ClientConfig{BackendConfig: clerk.BackendConfig{Key: &key}}
		ucfg := &clerk.ClientConfig{BackendConfig: clerk.BackendConfig{Key: &key}}
		if c.apiURL != "" {
			u := c.apiURL
			jcfg.URL, ucfg.URL = &u, &u
		}
		c.jwksClient, c.userClient = jwks.NewClient(jcfg), user.NewClient(ucfg)
	})
	return c.jwksClient, c.userClient
}

func (c *Clerk) fetchJWKS(ctx context.Context) (*clerk.JSONWebKeySet, error) {
	if c.FetchJWKS != nil {
		return c.FetchJWKS(ctx)
	}
	client, _ := c.sdk()
	return client.Get(ctx, &jwks.GetParams{})
}

func (c *Clerk) lookupUser(ctx context.Context, subject string) (*clerk.User, error) {
	if c.LookupUser != nil {
		return c.LookupUser(ctx, subject)
	}
	_, client := c.sdk()
	return client.Get(ctx, subject)
}

func (c *Clerk) jwksTTL() time.Duration {
	if c.JWKSTTL > 0 {
		return c.JWKSTTL
	}
	return DefaultJWKSTTL
}

func (c *Clerk) userTTL() time.Duration {
	if c.UserTTL > 0 {
		return c.UserTTL
	}
	return DefaultUserTTL
}

func (c *Clerk) maxUsers() int {
	if c.MaxUsers > 0 {
		return c.MaxUsers
	}
	return DefaultUserCacheSize
}

// cachedKey looks kid up in the cached key set. fresh says the set is
// within its TTL; recent says it was fetched within the kid-miss floor.
func (c *Clerk) cachedKey(kid string, now time.Time) (key *clerk.JSONWebKey, fresh, recent bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.keys == nil {
		return nil, false, false
	}
	age := now.Sub(c.keysAt)
	fresh, recent = age >= 0 && age < c.jwksTTL(), age >= 0 && age < jwksMissFloor
	if fresh {
		key = findKey(c.keys, kid)
	}
	return key, fresh, recent
}

func findKey(set *clerk.JSONWebKeySet, kid string) *clerk.JSONWebKey {
	for _, k := range set.Keys {
		if k != nil && k.KeyID == kid {
			return k
		}
	}
	return nil
}

// key returns the public key a token names, from the cached key set while
// it is fresh. The set is fetched again when it expires, or early when a
// token names a kid it does not hold (a key rotation).
func (c *Clerk) key(ctx context.Context, kid string) (*clerk.JSONWebKey, error) {
	if kid == "" {
		return nil, errUnknownKey
	}
	if k, fresh, recent := c.cachedKey(kid, c.now()); k != nil {
		return k, nil
	} else if fresh && recent {
		return nil, errUnknownKey
	}
	c.fetching.Lock()
	defer c.fetching.Unlock()
	// Another request may have fetched while this one waited.
	if k, fresh, recent := c.cachedKey(kid, c.now()); k != nil {
		return k, nil
	} else if fresh && recent {
		return nil, errUnknownKey
	}
	set, err := c.fetchJWKS(ctx)
	if err != nil {
		return nil, err
	}
	if set == nil || len(set.Keys) == 0 {
		return nil, errors.New("clerk returned no signing keys")
	}
	c.mu.Lock()
	c.keys, c.keysAt = set, c.now()
	c.mu.Unlock()
	if k := findKey(set, kid); k != nil {
		return k, nil
	}
	return nil, errUnknownKey
}

func (c *Clerk) Verify(ctx context.Context, token, origin string) (string, string, error) {
	if c.Resolve != nil {
		return c.Resolve(ctx, token, origin)
	}
	if !c.Configured() {
		return "", "", errors.New("sign-in not configured")
	}
	// The kid only chooses which cached key to try; the token is trusted
	// only after jwt.Verify checks the signature with that key.
	unverified, err := jwt.Decode(ctx, &jwt.DecodeParams{Token: token})
	if err != nil {
		return "", "", ErrBadCredentials
	}
	key, err := c.key(ctx, unverified.KeyID)
	if err != nil {
		return "", "", ErrBadCredentials
	}
	type statusClaims struct {
		Status string `json:"sts"`
	}
	claims, err := jwt.Verify(ctx, &jwt.VerifyParams{
		Token:                   token,
		JWK:                     key,
		Clock:                   clockFunc(c.now),
		CustomClaimsConstructor: func(context.Context) any { return &statusClaims{} },
		AuthorizedPartyHandler:  func(azp string) bool { return azp == origin },
	})
	if err != nil {
		return "", "", ErrBadCredentials
	}
	if status, ok := claims.Custom.(*statusClaims); ok && status.Status != "" && status.Status != "active" {
		return "", "", ErrBadCredentials
	}
	if claims.Subject == "" {
		return "", "", ErrBadCredentials
	}
	return c.identity(ctx, claims.Subject)
}

// identity is the verified, active Clerk account behind subject, from the
// cache while it is fresh. Only a verified active identity is cached.
func (c *Clerk) identity(ctx context.Context, subject string) (string, string, error) {
	now := c.now()
	c.mu.Lock()
	if e, ok := c.users[subject]; ok {
		if now.Before(e.expires) {
			c.mu.Unlock()
			return e.id, e.email, nil
		}
		delete(c.users, subject)
	}
	c.mu.Unlock()
	u, err := c.lookupUser(ctx, subject)
	if err != nil || u == nil || u.ID != subject {
		return "", "", ErrBadCredentials
	}
	id, email, err := verifiedIdentity(u)
	if err != nil {
		return "", "", err
	}
	c.remember(subject, cachedIdentity{id: id, email: email, expires: c.now().Add(c.userTTL())})
	return id, email, nil
}

func (c *Clerk) remember(subject string, e cachedIdentity) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.users == nil {
		c.users = map[string]cachedIdentity{}
	}
	if _, ok := c.users[subject]; !ok && len(c.users) >= c.maxUsers() {
		now := c.now()
		for k, v := range c.users {
			if !now.Before(v.expires) {
				delete(c.users, k)
			}
		}
		for len(c.users) >= c.maxUsers() {
			oldest, at := "", time.Time{}
			for k, v := range c.users {
				if oldest == "" || v.expires.Before(at) {
					oldest, at = k, v.expires
				}
			}
			delete(c.users, oldest)
		}
	}
	c.users[subject] = e
}

// cachedUsers is the identity cache's size, for tests.
func (c *Clerk) cachedUsers() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.users)
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
