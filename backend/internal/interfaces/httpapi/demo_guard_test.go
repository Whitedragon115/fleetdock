package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Every allowlisted pattern must be a registered route, or a typo would
// silently switch a demo feature off.
func TestDemoAllowedRoutesExist(t *testing.T) {
	src, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatal(err)
	}
	registered := map[string]bool{}
	for _, m := range regexp.MustCompile(`mux\.HandleFunc\("([A-Z]+ [^"]+)"`).FindAllStringSubmatch(string(src), -1) {
		registered[m[1]] = true
	}
	for pattern := range demoAllowed {
		if !registered[pattern] {
			t.Errorf("demo route %q is not registered in router.go", pattern)
		}
	}
}

// Nothing administrative may be on the allowlist.
func TestDemoAllowlistHasNoAdminRoutes(t *testing.T) {
	forbidden := []string{
		"/v1/auth/login", "/v1/users", "/v1/roles", "/v1/tokens", "/v1/api-tokens", "/v1/agent-tokens",
		"/v1/backup", "/v1/moves", "/v1/notification", "/v1/alert-rules", "/v1/audit", "/agent/", "/install.sh",
		"/credentials", "/public-access", "/db-users", "/grants", "/processes", "/dump", "/import",
		"/v1/profile/password", "/v1/fleet-runs", "/metrics",
	}
	for pattern := range demoAllowed {
		for _, f := range forbidden {
			if strings.Contains(pattern, f) {
				t.Errorf("demo allowlist contains %q", pattern)
			}
		}
		if strings.HasPrefix(pattern, "POST /v1/instances") || strings.HasPrefix(pattern, "PATCH /v1/instances") ||
			strings.HasPrefix(pattern, "DELETE /v1/") && !strings.Contains(pattern, "table-views") && !strings.Contains(pattern, "saved-queries") ||
			pattern == "POST /v1/servers" || pattern == "POST /v1/databases" {
			t.Errorf("demo allowlist contains %q", pattern)
		}
	}
}

func TestDemoGuard(t *testing.T) {
	mux := http.NewServeMux()
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	mux.HandleFunc("GET /v1/databases", ok)
	mux.HandleFunc("POST /v1/instances", ok)
	mux.HandleFunc("POST /v1/auth/login", ok)
	h := demoGuard(mux, false, mux)

	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{"GET", "/v1/databases", http.StatusOK},
		{"POST", "/v1/instances", http.StatusForbidden},
		{"POST", "/v1/auth/login", http.StatusForbidden},
		{"GET", "/v1/unknown", http.StatusNotFound},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		if rec.Code != tc.want {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, rec.Code, tc.want)
		}
	}

	// The write limit applies per client IP.
	limited := false
	for i := 0; i < demoWritesPerMinute+1; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/unknown-write", nil))
		if rec.Code == http.StatusTooManyRequests {
			limited = true
		}
	}
	if !limited {
		t.Error("writes were not rate limited")
	}
}
