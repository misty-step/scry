package web

import (
	"strings"

	"scry/internal/auth"
)

// cspDirectives is the fixed order directives appear in the header, so the
// policy is deterministic and easy to diff.
var cspDirectives = []string{
	"default-src", "script-src", "style-src", "img-src", "font-src", "connect-src",
	"frame-src", "worker-src", "base-uri", "form-action", "frame-ancestors", "report-uri",
}

// csp accumulates sources per directive. Adding a source never replaces
// another, and a source is kept once, in the order it was first added, so
// integrations compose instead of overwriting one another.
type csp struct {
	sources map[string][]string
}

func newCSP() *csp { return &csp{sources: map[string][]string{}} }

func (p *csp) add(directive string, sources ...string) {
	have := p.sources[directive]
	for _, src := range sources {
		if src == "" {
			continue
		}
		dup := false
		for _, h := range have {
			if h == src {
				dup = true
				break
			}
		}
		if !dup {
			have = append(have, src)
		}
	}
	p.sources[directive] = have
}

func (p *csp) String() string {
	var b strings.Builder
	for _, d := range cspDirectives {
		srcs, ok := p.sources[d]
		if !ok || len(srcs) == 0 {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("; ")
		}
		b.WriteString(d)
		b.WriteByte(' ')
		b.WriteString(strings.Join(srcs, " "))
	}
	return b.String()
}

// Stripe Elements (card field, 3D Secure) and its fraud signals. Origins are
// Stripe's published CSP guidance for Stripe.js v3.
func (p *csp) stripe() {
	p.add("script-src", "https://js.stripe.com", "https://*.js.stripe.com", "https://m.stripe.network")
	p.add("frame-src", "https://js.stripe.com", "https://*.js.stripe.com", "https://hooks.stripe.com", "https://m.stripe.network")
	p.add("connect-src", "https://api.stripe.com", "https://r.stripe.com", "https://m.stripe.network", "https://q.stripe.com", "https://errors.stripe.com")
	p.add("img-src", "https://*.stripe.com")
	// Stripe Elements writes inline styles on the host wrapper it mounts.
	p.add("style-src", "'unsafe-inline'")
}

// Clerk JS v5 from the instance's Frontend API, its bot protection
// (Cloudflare Turnstile and Clerk protect), avatars and workers.
func (p *csp) clerk(origin string) {
	p.add("script-src", origin, "https://challenges.cloudflare.com", "https://*.protect.clerk.com")
	p.add("frame-src", "https://challenges.cloudflare.com", "https://*.protect.clerk.com")
	p.add("connect-src", origin, "https://challenges.cloudflare.com", "https://*.protect.clerk.com:*", "https://clerk-telemetry.com")
	p.add("img-src", origin, "https://img.clerk.com")
	p.add("worker-src", "'self'", "blob:")
	p.add("style-src", "'unsafe-inline'")
}

// policy is the Content-Security-Policy for every response: the page's own
// origin, plus exactly what each configured third party needs.
func (s *Server) policy() string {
	p := newCSP()
	p.add("default-src", "'self'")
	p.add("script-src", "'self'")
	p.add("style-src", "'self'")
	p.add("img-src", "'self'", "data:")
	p.add("font-src", "'self'")
	p.add("connect-src", "'self'")
	if s.App.Pay != nil && s.App.Pay.PublishableKey() != "" {
		p.stripe()
	}
	if provider, ok := s.App.Auth.(*auth.Clerk); ok && provider.Configured() {
		p.clerk("https://" + provider.Domain())
	}
	if len(p.sources["frame-src"]) == 0 {
		p.add("frame-src", "'none'")
	}
	p.add("base-uri", "'none'")
	p.add("form-action", "'self'")
	p.add("frame-ancestors", "'none'")
	p.add("report-uri", clientErrorPath)
	return p.String()
}
