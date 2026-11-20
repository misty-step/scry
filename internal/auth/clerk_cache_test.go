package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	clerk "github.com/clerk/clerk-sdk-go/v2"
	jose "github.com/go-jose/go-jose/v4"
	josejwt "github.com/go-jose/go-jose/v4/jwt"
)

const testOrigin = "https://scry.example.test"

var testPublishable = "pk_test_" + base64.RawStdEncoding.EncodeToString([]byte("example.clerk.accounts.dev$"))

// clerkRig is a Clerk instance in miniature: signing keys, a key set, a
// user directory, a clock the test moves, and counters for every upstream
// call the adapter makes.
type clerkRig struct {
	t       *testing.T
	mu      sync.Mutex
	clock   time.Time
	signers map[string]*rsa.PrivateKey
	keys    []string // kids published in the key set
	users   map[string]*clerk.User
	userErr error

	jwksCalls, userCalls atomic.Int64
	c                    *Clerk
}

func newClerkRig(t *testing.T) *clerkRig {
	t.Helper()
	r := &clerkRig{t: t, clock: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC), signers: map[string]*rsa.PrivateKey{}, users: map[string]*clerk.User{}}
	r.addKey("kid_1", true)
	r.c = &Clerk{PublishableKey: testPublishable, SecretKey: "sk_test_standin", Now: r.now, FetchJWKS: r.fetch, LookupUser: r.lookup}
	return r
}

func (r *clerkRig) now() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.clock
}

func (r *clerkRig) advance(d time.Duration) {
	r.mu.Lock()
	r.clock = r.clock.Add(d)
	r.mu.Unlock()
}

func (r *clerkRig) addKey(kid string, publish bool) {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		r.t.Fatal(err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.signers[kid] = k
	if publish {
		r.keys = append(r.keys, kid)
	}
}

func (r *clerkRig) publish(kid string) {
	r.mu.Lock()
	r.keys = append(r.keys, kid)
	r.mu.Unlock()
}

func (r *clerkRig) keySet() *clerk.JSONWebKeySet {
	r.mu.Lock()
	defer r.mu.Unlock()
	set := &clerk.JSONWebKeySet{}
	for _, kid := range r.keys {
		set.Keys = append(set.Keys, &clerk.JSONWebKey{Key: &r.signers[kid].PublicKey, KeyID: kid, Algorithm: "RS256", Use: "sig"})
	}
	return set
}

func (r *clerkRig) fetch(context.Context) (*clerk.JSONWebKeySet, error) {
	r.jwksCalls.Add(1)
	return r.keySet(), nil
}

func (r *clerkRig) lookup(_ context.Context, id string) (*clerk.User, error) {
	r.userCalls.Add(1)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.userErr != nil {
		return nil, r.userErr
	}
	u, ok := r.users[id]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *u
	return &cp, nil
}

func (r *clerkRig) addUser(id, email, status string) {
	emailID := "idn_" + id
	r.mu.Lock()
	r.users[id] = &clerk.User{ID: id, PrimaryEmailAddressID: &emailID,
		EmailAddresses: []*clerk.EmailAddress{{ID: emailID, EmailAddress: email, Verification: &clerk.Verification{Status: status}}}}
	r.mu.Unlock()
}

func (r *clerkRig) setErr(err error) {
	r.mu.Lock()
	r.userErr = err
	r.mu.Unlock()
}

func (r *clerkRig) setUser(id string, change func(*clerk.User)) {
	r.mu.Lock()
	change(r.users[id])
	r.mu.Unlock()
}

type tokenOpts struct {
	kid, signWith, sub, azp, sts, iss string
	exp                               time.Duration
	noKid                             bool
}

func (r *clerkRig) token(o tokenOpts) string {
	r.t.Helper()
	if o.kid == "" {
		o.kid = "kid_1"
	}
	opts := (&jose.SignerOptions{}).WithType("JWT")
	if !o.noKid {
		opts = opts.WithHeader("kid", o.kid)
	}
	if o.signWith == "" {
		o.signWith = o.kid
	}
	if o.azp == "" {
		o.azp = testOrigin
	}
	if o.iss == "" {
		o.iss = "https://example.clerk.accounts.dev"
	}
	if o.exp == 0 {
		o.exp = time.Hour
	}
	r.mu.Lock()
	key := r.signers[o.signWith]
	r.mu.Unlock()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, opts)
	if err != nil {
		r.t.Fatal(err)
	}
	now := r.now()
	claims := map[string]any{"iss": o.iss, "sub": o.sub, "azp": o.azp, "iat": now.Add(-time.Second).Unix(), "nbf": now.Add(-time.Second).Unix(), "exp": now.Add(o.exp).Unix()}
	if o.sts != "" {
		claims["sts"] = o.sts
	}
	tok, err := josejwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		r.t.Fatal(err)
	}
	return tok
}

func (r *clerkRig) verify(tok string) (string, string, error) {
	return r.c.Verify(context.Background(), tok, testOrigin)
}

func (r *clerkRig) mustVerify(tok, wantID string) {
	r.t.Helper()
	id, _, err := r.verify(tok)
	if err != nil || id != wantID {
		r.t.Fatalf("verify = %q, %v; want %q", id, err, wantID)
	}
}

func (r *clerkRig) calls(jwks, users int64) {
	r.t.Helper()
	if got := r.jwksCalls.Load(); got != jwks {
		r.t.Fatalf("JWKS fetches = %d, want %d", got, jwks)
	}
	if got := r.userCalls.Load(); got != users {
		r.t.Fatalf("user lookups = %d, want %d", got, users)
	}
}

// Slice 12: within their TTLs the key set and the verified identity are
// reused, so a signed-in page view makes no Clerk API call; after the TTL
// each is fetched again.
func TestClerkCachesKeySetAndIdentityWithinTTL(t *testing.T) {
	r := newClerkRig(t)
	r.addUser("user_a", "a@example.com", "verified")
	for i := 0; i < 5; i++ {
		r.mustVerify(r.token(tokenOpts{sub: "user_a"}), "user_a")
	}
	r.calls(1, 1)

	r.advance(DefaultUserTTL + time.Second) // identity expires, keys do not
	r.mustVerify(r.token(tokenOpts{sub: "user_a"}), "user_a")
	r.calls(1, 2)

	r.advance(DefaultJWKSTTL) // both expire
	r.mustVerify(r.token(tokenOpts{sub: "user_a"}), "user_a")
	r.mustVerify(r.token(tokenOpts{sub: "user_a"}), "user_a")
	r.calls(2, 3)
}

// A token signed with a key the cached set does not have yet (a rotation)
// fetches the set again at once, without waiting out the TTL or a restart.
func TestClerkKeyRotationRefreshesKeySet(t *testing.T) {
	r := newClerkRig(t)
	r.addUser("user_a", "a@example.com", "verified")
	r.mustVerify(r.token(tokenOpts{sub: "user_a"}), "user_a")
	r.calls(1, 1)

	// Clerk publishes kid_2 and starts signing with it.
	r.addKey("kid_2", false)
	r.advance(jwksMissFloor + 5*time.Second) // well inside both TTLs
	r.publish("kid_2")
	r.mustVerify(r.token(tokenOpts{kid: "kid_2", sub: "user_a"}), "user_a")
	r.calls(2, 1)
	// The refreshed set serves both keys from cache.
	r.mustVerify(r.token(tokenOpts{kid: "kid_1", sub: "user_a"}), "user_a")
	r.mustVerify(r.token(tokenOpts{kid: "kid_2", sub: "user_a"}), "user_a")
	r.calls(2, 1)

	// An unknown kid right after a fetch is refused without hammering
	// Clerk; after the floor it may fetch once more, and is still refused.
	r.addKey("kid_rogue", false)
	for i := 0; i < 5; i++ {
		if _, _, err := r.verify(r.token(tokenOpts{kid: "kid_rogue", sub: "user_a"})); err == nil {
			t.Fatal("unpublished key accepted")
		}
	}
	r.calls(2, 1)
	r.advance(jwksMissFloor + time.Second)
	if _, _, err := r.verify(r.token(tokenOpts{kid: "kid_rogue", sub: "user_a"})); err == nil {
		t.Fatal("unpublished key accepted")
	}
	r.calls(3, 1)
}

// The token is checked in full on every request, even when the identity
// and keys are cached: a forged signature, an expired token, another
// origin, a pending session or a foreign issuer never get through.
func TestClerkAlwaysVerifiesTokenEvenWhenCached(t *testing.T) {
	r := newClerkRig(t)
	r.addUser("user_a", "a@example.com", "verified")
	r.addKey("kid_forger", false)
	r.mustVerify(r.token(tokenOpts{sub: "user_a"}), "user_a")
	for name, tok := range map[string]string{
		"forged signature": r.token(tokenOpts{kid: "kid_1", signWith: "kid_forger", sub: "user_a"}),
		"expired":          r.token(tokenOpts{sub: "user_a", exp: -time.Minute}),
		"wrong azp":        r.token(tokenOpts{sub: "user_a", azp: "https://evil.example.test"}),
		"pending session":  r.token(tokenOpts{sub: "user_a", sts: "pending"}),
		"foreign issuer":   r.token(tokenOpts{sub: "user_a", iss: "https://evil.example.test"}),
		"empty subject":    r.token(tokenOpts{sub: ""}),
		"not a token":      "garbage",
		"tampered payload": tamper(r.token(tokenOpts{sub: "user_a"})),
		"no kid header":    r.token(tokenOpts{noKid: true, sub: "user_a"}),
	} {
		if _, _, err := r.verify(tok); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// A token that expires while cached is refused once the clock passes it.
	short := r.token(tokenOpts{sub: "user_a", exp: 10 * time.Second})
	r.mustVerify(short, "user_a")
	r.advance(11 * time.Second)
	if _, _, err := r.verify(short); err == nil {
		t.Fatal("expired token accepted from cache")
	}
	if _, _, err := r.verify(r.token(tokenOpts{sub: "user_a", sts: "active"})); err != nil {
		t.Fatalf("active session refused: %v", err)
	}
}

func tamper(tok string) string {
	parts := strings.Split(tok, ".")
	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims map[string]any
	_ = json.Unmarshal(raw, &claims)
	claims["sub"] = "user_admin"
	raw, _ = json.Marshal(claims)
	parts[1] = base64.RawURLEncoding.EncodeToString(raw)
	return strings.Join(parts, ".")
}

// Failures are never cached: a lookup error, a banned or locked account
// and an unverified email are asked about again on the next request, and
// a fix upstream takes effect at once.
func TestClerkNeverCachesFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		breakIt func(r *clerkRig)
		fix     func(r *clerkRig)
	}{
		{"lookup error", func(r *clerkRig) { r.setErr(errors.New("clerk is down")) }, func(r *clerkRig) { r.setErr(nil) }},
		{"banned", func(r *clerkRig) { r.setUser("user_a", func(u *clerk.User) { u.Banned = true }) }, func(r *clerkRig) { r.setUser("user_a", func(u *clerk.User) { u.Banned = false }) }},
		{"locked", func(r *clerkRig) { r.setUser("user_a", func(u *clerk.User) { u.Locked = true }) }, func(r *clerkRig) { r.setUser("user_a", func(u *clerk.User) { u.Locked = false }) }},
		{"unverified", func(r *clerkRig) {
			r.setUser("user_a", func(u *clerk.User) { u.EmailAddresses[0].Verification.Status = "unverified" })
		}, func(r *clerkRig) {
			r.setUser("user_a", func(u *clerk.User) { u.EmailAddresses[0].Verification.Status = "verified" })
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newClerkRig(t)
			r.addUser("user_a", "a@example.com", "verified")
			tc.breakIt(r)
			for i := 0; i < 3; i++ {
				if _, _, err := r.verify(r.token(tokenOpts{sub: "user_a"})); err == nil {
					t.Fatal("accepted")
				}
			}
			r.calls(1, 3)
			if n := r.c.cachedUsers(); n != 0 {
				t.Fatalf("failure cached: %d entries", n)
			}
			tc.fix(r)
			r.mustVerify(r.token(tokenOpts{sub: "user_a"}), "user_a")
			r.calls(1, 4)
		})
	}
	// A key-set fetch failure is not cached either.
	r := newClerkRig(t)
	r.addUser("user_a", "a@example.com", "verified")
	var fail atomic.Bool
	fail.Store(true)
	r.c.FetchJWKS = func(ctx context.Context) (*clerk.JSONWebKeySet, error) {
		if fail.Load() {
			r.jwksCalls.Add(1)
			return nil, errors.New("clerk is down")
		}
		return r.fetch(ctx)
	}
	for i := 0; i < 2; i++ {
		if _, _, err := r.verify(r.token(tokenOpts{sub: "user_a"})); err == nil {
			t.Fatal("accepted without keys")
		}
	}
	fail.Store(false)
	r.mustVerify(r.token(tokenOpts{sub: "user_a"}), "user_a")
	r.calls(3, 1)
	// A lookup that answers for another user is refused.
	r.c.LookupUser = func(ctx context.Context, id string) (*clerk.User, error) { return r.lookup(ctx, "user_a") }
	if _, _, err := r.verify(r.token(tokenOpts{sub: "user_b"})); err == nil {
		t.Fatal("identity for another subject accepted")
	}
}

// The identity cache is bounded; the oldest entries go first.
func TestClerkIdentityCacheIsBounded(t *testing.T) {
	r := newClerkRig(t)
	r.c.MaxUsers = 3
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("user_%d", i)
		r.addUser(id, id+"@example.com", "verified")
		r.mustVerify(r.token(tokenOpts{sub: id}), id)
		r.advance(time.Second)
		if n := r.c.cachedUsers(); n > 3 {
			t.Fatalf("cache grew to %d", n)
		}
	}
	before := r.userCalls.Load()
	r.mustVerify(r.token(tokenOpts{sub: "user_5"}), "user_5") // newest: cached
	if r.userCalls.Load() != before {
		t.Fatal("newest identity was evicted")
	}
	r.mustVerify(r.token(tokenOpts{sub: "user_0"}), "user_0") // oldest: evicted
	if r.userCalls.Load() != before+1 {
		t.Fatal("oldest identity was not evicted")
	}
	// Zero-valued bounds on a struct literal mean the defaults.
	if z := (&Clerk{}); z.jwksTTL() != DefaultJWKSTTL || z.userTTL() != DefaultUserTTL || z.maxUsers() != DefaultUserCacheSize {
		t.Fatal("zero bounds are not the defaults")
	}
}

// Many requests at once share one key-set fetch and one cached identity,
// race-free.
func TestClerkCacheConcurrent(t *testing.T) {
	r := newClerkRig(t)
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("user_%d", i)
		r.addUser(id, id+"@example.com", "verified")
	}
	toks := make([]string, 8)
	for i := range toks {
		toks[i] = r.token(tokenOpts{sub: fmt.Sprintf("user_%d", i)})
	}
	var wg sync.WaitGroup
	errs := make(chan error, 400)
	for g := 0; g < 400; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			want := fmt.Sprintf("user_%d", g%8)
			id, _, err := r.verify(toks[g%8])
			if err != nil || id != want {
				errs <- fmt.Errorf("goroutine %d: %q %v", g, id, err)
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if n := r.jwksCalls.Load(); n != 1 {
		t.Fatalf("concurrent requests fetched the key set %d times", n)
	}
	// Racing first requests may each look a user up once; after that the
	// cache answers.
	before := r.userCalls.Load()
	if before < 8 || before > 400 {
		t.Fatalf("lookups %d", before)
	}
	for i := range toks {
		r.mustVerify(toks[i], fmt.Sprintf("user_%d", i))
	}
	if r.userCalls.Load() != before {
		t.Fatal("warm cache looked users up again")
	}
}

// A plain struct literal, as cmd/scry builds it, uses the SDK clients:
// created once, reused, and their responses cached.
func TestClerkStructLiteralUsesCachedSDKClients(t *testing.T) {
	r := newClerkRig(t)
	r.addUser("user_a", "a@example.com", "verified")
	var jwksHits, userHits atomic.Int64
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "Bearer sk_test_standin" {
			http.Error(w, "no key", http.StatusUnauthorized)
			return
		}
		switch {
		case strings.HasSuffix(req.URL.Path, "/jwks"):
			jwksHits.Add(1)
			k := r.signers["kid_1"]
			jwk := jose.JSONWebKey{Key: &k.PublicKey, KeyID: "kid_1", Algorithm: "RS256", Use: "sig"}
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{jwk}})
		case strings.HasSuffix(req.URL.Path, "/users/user_a"):
			userHits.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"object": "user", "id": "user_a", "primary_email_address_id": "idn_a",
				"email_addresses": []any{map[string]any{"id": "idn_a", "email_address": "a@example.com", "verification": map[string]any{"status": "verified"}}}})
		default:
			http.NotFound(w, req)
		}
	}))
	defer api.Close()
	c := &Clerk{PublishableKey: testPublishable, SecretKey: "sk_test_standin", Now: r.now, apiURL: api.URL + "/v1"}
	j1, u1 := c.sdk()
	j2, u2 := c.sdk()
	if j1 == nil || u1 == nil || j1 != j2 || u1 != u2 {
		t.Fatal("SDK clients are not created once and reused")
	}
	for i := 0; i < 4; i++ {
		id, email, err := c.Verify(context.Background(), r.token(tokenOpts{sub: "user_a"}), testOrigin)
		if err != nil || id != "user_a" || email != "a@example.com" {
			t.Fatalf("verify %q %q %v", id, email, err)
		}
	}
	if jwksHits.Load() != 1 || userHits.Load() != 1 {
		t.Fatalf("Clerk API hits: jwks %d users %d, want 1 and 1", jwksHits.Load(), userHits.Load())
	}
}

func TestClerkMode(t *testing.T) {
	if (&Clerk{PublishableKey: "pk_live_x"}).Mode() != "clerk-live" || (&Clerk{PublishableKey: "pk_test_x"}).Mode() != "clerk-test" {
		t.Fatal("mode")
	}
}

// Pass 2: requests that arrive while a key-set fetch is in flight share
// its outcome, an error included. Twelve concurrent sign-ins during a
// Clerk JWKS outage (HTTP 503) make one upstream call, not twelve; once
// the fetch has finished, the next request tries again and, with Clerk
// back, succeeds.
func TestClerkConcurrentJWKSFailureSharesOneFetch(t *testing.T) {
	const n = 12
	r := newClerkRig(t)
	var jwksHits, joins atomic.Int64
	var down atomic.Bool
	down.Store(true)
	release := make(chan struct{})
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/jwks"):
			jwksHits.Add(1)
			if down.Load() {
				<-release // hold the outage open until every request has joined
				http.Error(w, `{"errors":[{"code":"unavailable"}]}`, http.StatusServiceUnavailable)
				return
			}
			k := r.signers["kid_1"]
			jwk := jose.JSONWebKey{Key: &k.PublicKey, KeyID: "kid_1", Algorithm: "RS256", Use: "sig"}
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{jwk}})
		case strings.HasSuffix(req.URL.Path, "/users/user_a"):
			_ = json.NewEncoder(w).Encode(map[string]any{"object": "user", "id": "user_a", "primary_email_address_id": "idn_a",
				"email_addresses": []any{map[string]any{"id": "idn_a", "email_address": "a@example.com", "verification": map[string]any{"status": "verified"}}}})
		default:
			http.NotFound(w, req)
		}
	}))
	defer api.Close()
	c := &Clerk{PublishableKey: testPublishable, SecretKey: "sk_test_standin", Now: r.now, apiURL: api.URL + "/v1",
		onJoin: func() { joins.Add(1) }}
	tok := r.token(tokenOpts{sub: "user_a"})

	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := c.Verify(context.Background(), tok, testOrigin)
			errs <- err
		}()
	}
	deadline := time.Now().Add(10 * time.Second)
	for jwksHits.Load() < 1 || joins.Load() < n-1 {
		if time.Now().After(deadline) {
			close(release)
			t.Fatalf("requests did not join one fetch: upstream %d, joined %d", jwksHits.Load(), joins.Load())
		}
		time.Sleep(time.Millisecond)
	}
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err == nil {
			t.Fatal("signed in with no key set")
		}
	}
	if got := jwksHits.Load(); got != 1 {
		t.Fatalf("%d concurrent requests during a 503 made %d upstream JWKS fetches, want 1", n, got)
	}
	c.mu.Lock()
	inFlight, keys := c.inFlight, c.keys
	c.mu.Unlock()
	if inFlight != nil || keys != nil {
		t.Fatal("failed fetch left in flight or cached")
	}

	down.Store(false)
	id, _, err := c.Verify(context.Background(), tok, testOrigin)
	if err != nil || id != "user_a" {
		t.Fatalf("after recovery: %q %v", id, err)
	}
	if got := jwksHits.Load(); got != 2 {
		t.Fatalf("upstream JWKS fetches after recovery = %d, want 2", got)
	}
}

// A request that gives up while waiting on a shared fetch leaves promptly,
// and its cancellation does not fail the fetch for everyone else.
func TestClerkJWKSFetchOutlivesCanceledRequest(t *testing.T) {
	r := newClerkRig(t)
	r.addUser("user_a", "a@example.com", "verified")
	started, release := make(chan struct{}), make(chan struct{})
	r.c.FetchJWKS = func(ctx context.Context) (*clerk.JSONWebKeySet, error) {
		close(started)
		<-release
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return r.fetch(ctx)
	}
	tok := r.token(tokenOpts{sub: "user_a"})
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { _, _, err := r.c.Verify(ctx, tok, testOrigin); first <- err }()
	<-started
	cancel()
	if err := <-first; err == nil {
		t.Fatal("canceled request signed in")
	}
	close(release)
	r.mustVerify(tok, "user_a")
	r.calls(1, 1)
}

// Pass 2: a cached identity's lifetime counts from when its lookup began.
// A lookup slower than the TTL is used once but not cached; a faster one
// expires TTL after it started, not TTL after it answered.
func TestClerkIdentityTTLCountsFromLookupStart(t *testing.T) {
	r := newClerkRig(t)
	r.addUser("user_a", "a@example.com", "verified")
	tok := r.token(tokenOpts{sub: "user_a"})
	slow := func(d time.Duration) func(context.Context, string) (*clerk.User, error) {
		return func(ctx context.Context, id string) (*clerk.User, error) {
			r.advance(d) // the clock moves while user.Get is in flight
			return r.lookup(ctx, id)
		}
	}

	r.c.LookupUser = slow(DefaultUserTTL + time.Second)
	r.mustVerify(tok, "user_a")
	if n := r.c.cachedUsers(); n != 0 {
		t.Fatalf("identity older than its TTL was cached (%d entries)", n)
	}
	r.c.LookupUser = r.lookup
	r.mustVerify(tok, "user_a")
	r.calls(1, 2) // the stale answer was not reused

	r.advance(DefaultUserTTL + time.Second) // let that entry expire
	r.c.LookupUser = slow(DefaultUserTTL * 2 / 3)
	r.mustVerify(tok, "user_a")
	r.calls(1, 3)
	r.c.LookupUser = r.lookup
	r.advance(DefaultUserTTL/3 - time.Second) // TTL-1s since the lookup began
	r.mustVerify(tok, "user_a")
	r.calls(1, 3)
	r.advance(2 * time.Second) // TTL+1s since it began, TTL*2/3 since it answered
	r.mustVerify(tok, "user_a")
	r.calls(1, 4)
}
