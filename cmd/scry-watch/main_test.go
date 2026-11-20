package main

import "testing"

// Pass 2: with no SCRY_WATCH_HEALTH_URL the watcher probes scry on the
// PORT scry itself listens on, so a custom PORT in scry.env is not a
// false outage; an explicit SCRY_WATCH_HEALTH_URL still wins.
func TestHealthURLFollowsPORT(t *testing.T) {
	for _, tc := range []struct{ name, port, explicit, want string }{
		{"default port", "", "", "http://127.0.0.1:8080/healthz"},
		{"custom PORT", "41763", "", "http://127.0.0.1:41763/healthz"},
		{"PORT with whitespace", " 9090 ", "", "http://127.0.0.1:9090/healthz"},
		{"explicit URL wins", "41763", "http://localhost:7000/healthz", "http://localhost:7000/healthz"},
		{"blank explicit URL falls back", "41763", "  ", "http://127.0.0.1:41763/healthz"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PORT", tc.port)
			t.Setenv("SCRY_WATCH_HEALTH_URL", tc.explicit)
			got, err := healthURL()
			if err != nil || got != tc.want {
				t.Fatalf("healthURL() = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
	for _, bad := range []string{"0", "65536", "http", "80/evil", "-1", "08080"} {
		t.Setenv("PORT", bad)
		t.Setenv("SCRY_WATCH_HEALTH_URL", "")
		if got, err := healthURL(); err == nil {
			t.Errorf("PORT=%q accepted as %q", bad, got)
		}
	}
}
