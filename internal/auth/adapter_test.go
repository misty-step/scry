package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestG11_IdentityErrorMappingAndRequest(t *testing.T) {
	for _, tc := range []struct {
		message string
		want    error
	}{
		{"EMAIL_EXISTS", ErrEmailTaken}, {"WEAK_PASSWORD : short", ErrWeakPassword}, {"INVALID_EMAIL", ErrInvalidEmail},
		{"INVALID_LOGIN_CREDENTIALS", ErrBadCredentials}, {"EMAIL_NOT_FOUND", ErrBadCredentials}, {"INVALID_PASSWORD", ErrBadCredentials}, {"USER_DISABLED", ErrBadCredentials},
	} {
		t.Run(tc.message, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("key") != "local-test-key" || r.URL.Path != "/v1/accounts:signUp" {
					t.Error("wrong request")
				}
				var b map[string]any
				_ = json.NewDecoder(r.Body).Decode(&b)
				if b["email"] != "p@example.com" || b["password"] != "test-password" || b["returnSecureToken"] != true {
					t.Error("wrong payload")
				}
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": tc.message}})
			}))
			defer srv.Close()
			a := IdentityToolkit{APIKey: "local-test-key", BaseURL: srv.URL, HTTP: srv.Client()}
			_, err := a.SignUp(context.Background(), "p@example.com", "test-password")
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
}
