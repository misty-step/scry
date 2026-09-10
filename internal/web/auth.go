package web

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type csrfKey struct{}

func securityHeaders(w http.ResponseWriter, secure bool) {
	w.Header().Set("Cache-Control", "private, no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; font-src 'self'; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'; object-src 'none'")
	if secure {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
	}
}

func localHostname(host string) bool {
	if host == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

func remoteIP(r *http.Request) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}, false
	}
	ip, err := netip.ParseAddr(host)
	return ip.Unmap(), err == nil && ip.Zone() == ""
}

func (s *server) authorized(r *http.Request) bool {
	// The canonical Host is required even on the backend listener. Neither
	// Forwarded nor X-Forwarded-* participates in any authority decision.
	if !strings.EqualFold(r.Host, s.origin.Host) {
		return false
	}
	ip, ok := remoteIP(r)
	if !ok {
		return false
	}
	if s.cfg.Mode == "development" {
		return ip.IsLoopback()
	}
	if !s.cfg.TrustProxy || !s.peers[ip] {
		return false
	}
	ids := r.Header.Values("X-ExeDev-UserID")
	return len(ids) == 1 && hmac.Equal([]byte(ids[0]), []byte(s.cfg.OwnerID))
}

func (s *server) cookieName() string {
	if s.cfg.Mode == "production" {
		return "__Host-scry-session"
	}
	return "scry-development-session"
}

func (s *server) signature(message string) string {
	mac := hmac.New(sha256.New, []byte(s.cfg.Secret))
	mac.Write([]byte(s.cfg.OwnerID))
	mac.Write([]byte{0})
	mac.Write([]byte(message))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *server) session(r *http.Request) (string, bool) {
	c, err := r.Cookie(s.cookieName())
	if err != nil {
		return "", false
	}
	parts := strings.Split(c.Value, ".")
	if len(parts) != 3 || len(parts[1]) != 43 {
		return "", false
	}
	issued, err := strconv.ParseInt(parts[0], 10, 64)
	now := time.Now().Unix()
	if err != nil || issued > now+30 || issued < now-int64(12*time.Hour/time.Second) {
		return "", false
	}
	if !hmac.Equal([]byte(parts[2]), []byte(s.signature("session:"+parts[0]+"."+parts[1]))) {
		return "", false
	}
	return c.Value, true
}

func (s *server) newSession(w http.ResponseWriter) string {
	value := strconv.FormatInt(time.Now().Unix(), 10) + "." + randomToken()
	value += "." + s.signature("session:"+value)
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: value, Path: "/", HttpOnly: true, Secure: s.cfg.Mode == "production", SameSite: http.SameSiteStrictMode, MaxAge: 43200})
	return value
}

func (s *server) deny(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: "", Path: "/", HttpOnly: true, Secure: s.cfg.Mode == "production", SameSite: http.SameSiteStrictMode, MaxAge: -1})
	w.Header().Set("Clear-Site-Data", "\"cache\", \"storage\"")
	if isHTMX(r) {
		w.Header().Set("HX-Redirect", "/")
	}
	if wantsJSON(r) {
		jsonResponse(w, http.StatusForbidden, map[string]string{"error": "Private access required."})
		return
	}
	s.render(w, r, http.StatusForbidden, page{View: "access", Title: "Private access required"})
}

func (s *server) sameOrigin(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		return false
	}
	origins := r.Header.Values("Origin")
	if len(origins) > 0 {
		return len(origins) == 1 && origins[0] == s.origin.String()
	}
	// Older browsers can omit Origin on a same-origin form POST. A canonical
	// Referer is the only fallback, never a forwarded header or arbitrary Host.
	u, err := url.Parse(r.Header.Get("Referer"))
	return err == nil && u.Scheme == s.origin.Scheme && u.Host == s.origin.Host && u.User == nil
}

func (s *server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			s.deny(w, r)
			return
		}
		value, valid := s.session(r)
		mutation := r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions
		if mutation {
			if !valid || !s.sameOrigin(r) {
				s.csrfFailure(w, r)
				return
			}
			if !s.parseForm(w, r) {
				return
			}
			tokens := r.PostForm["csrf"]
			if len(tokens) != 1 || !hmac.Equal([]byte(tokens[0]), []byte(s.signature("csrf:"+value))) {
				s.csrfFailure(w, r)
				return
			}
		} else if !valid {
			value = s.newSession(w)
		}
		ctx := context.WithValue(r.Context(), csrfKey{}, s.signature("csrf:"+value))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *server) csrfFailure(w http.ResponseWriter, r *http.Request) {
	if wantsJSON(r) {
		jsonResponse(w, http.StatusForbidden, map[string]string{"error": "The private form expired or came from another site. Reload before trying again."})
		return
	}
	s.render(w, r, http.StatusForbidden, page{View: "error", Title: "Reload this private form", Error: "This form expired or came from another site. Nothing was changed by this request. Copy any unsaved text, then reload and try again."})
}
