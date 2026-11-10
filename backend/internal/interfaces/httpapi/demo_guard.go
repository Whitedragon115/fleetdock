package httpapi

import (
	"net/http"
	"time"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// demoAllowed is every route the public demo serves, by its ServeMux pattern.
// Anything else (accounts, roles, tokens, servers and agents, adding or
// changing database servers, backups, notifications, the audit log, ...)
// answers 403 before authentication even runs. It is an allowlist on purpose:
// a route added later stays off in the demo until it is reviewed and listed.
//
// The data routes still go through the normal per-database permission checks,
// and the visitor account holds no instance:write, so the console cannot run
// on system databases or with the server's admin login.
var demoAllowed = map[string]bool{
	"GET /healthz":         true,
	"GET /readyz":          true,
	"GET /v1/version":      true,
	"GET /openapi.yaml":    true,
	"GET /docs":            true,
	"POST /v1/auth/demo":   true,
	"GET /v1/auth/me":      true,
	"POST /v1/auth/logout": true,
	"GET /v1/profile":      true,
	"GET /v1/overview":     true,

	// Read-only lists the dashboard shows.
	"GET /v1/servers":        true,
	"GET /v1/instances":      true,
	"GET /v1/instances/{id}": true,
	"GET /v1/databases":      true,
	"GET /v1/databases/{id}": true,
	"GET /v1/operations":     true,

	// Data browser: tables, rows, structure.
	"GET /v1/databases/{id}/tables":                        true,
	"GET /v1/databases/{id}/tables/{table}/rows":           true,
	"GET /v1/databases/{id}/tables/{table}/schema":         true,
	"GET /v1/databases/{id}/tables/{table}/export":         true,
	"POST /v1/databases/{id}/tables/{table}/export":        true,
	"POST /v1/databases/{id}/tables/{table}/browse":        true,
	"POST /v1/databases/{id}/tables/{table}/rows":          true,
	"PATCH /v1/databases/{id}/tables/{table}/rows":         true,
	"POST /v1/databases/{id}/tables/{table}/rows/delete":   true,
	"POST /v1/databases/{id}/tables":                       true,
	"PATCH /v1/databases/{id}/tables/{table}":              true,
	"POST /v1/databases/{id}/tables/{table}/drop":          true,
	"POST /v1/databases/{id}/tables/{table}/truncate":      true,
	"POST /v1/databases/{id}/tables/{table}/rename":        true,
	"POST /v1/databases/{id}/tables/{table}/indexes":       true,
	"POST /v1/databases/{id}/tables/{table}/indexes/drop":  true,
	"GET /v1/databases/{id}/tables/{table}/foreign-keys":   true,
	"GET /v1/databases/{id}/tables/{table}/referenced-by":  true,
	"GET /v1/databases/{id}/objects":                       true,
	"GET /v1/databases/{id}/columns":                       true,
	"GET /v1/databases/{id}/diagram":                       true,
	"GET /v1/databases/{id}/table-views":                   true,
	"POST /v1/databases/{id}/table-views":                  true,
	"PATCH /v1/databases/{id}/table-views/{viewId}":        true,
	"DELETE /v1/databases/{id}/table-views/{viewId}":       true,
	"GET /v1/databases/{id}/changes":                       true,
	"POST /v1/databases/{id}/changes/undo":                 true,
	"POST /v1/databases/{id}/changes/{changeId}/undo":      true,
	"GET /v1/databases/{id}/schema/snapshots":              true,
	"GET /v1/databases/{id}/schema/snapshots/{snapshotId}": true,
	"GET /v1/databases/{id}/index-insights":                true,
	"GET /v1/search/schema":                                true,

	// SQL console.
	"POST /v1/databases/{id}/query":              true,
	"POST /v1/databases/{id}/query/{qid}/cancel": true,
	"POST /v1/databases/{id}/explain":            true,
	"POST /v1/databases/{id}/export":             true,
	"GET /v1/databases/{id}/query-history":       true,
	"GET /v1/saved-queries":                      true,
	"POST /v1/saved-queries":                     true,
	"PATCH /v1/saved-queries/{id}":               true,
	"DELETE /v1/saved-queries/{id}":              true,
}

// Per client IP: every API request, and requests that change something.
const (
	demoRequestsPerMinute = 600
	demoWritesPerMinute   = 90
	demoLoginsPerMinute   = 20
)

// demoGuard limits the API to demoAllowed and rate limits each client IP.
// Paths the mux does not know fall through to its 404.
func demoGuard(mux *http.ServeMux, trustProxy bool, next http.Handler) http.Handler {
	all := newLoginLimiter(demoRequestsPerMinute, time.Minute, trustProxy)
	all.message = "too many requests to the demo; wait a minute"
	writes := newLoginLimiter(demoWritesPerMinute, time.Minute, trustProxy)
	writes.message = "too many changes in a minute; wait a little and try again"
	logins := newLoginLimiter(demoLoginsPerMinute, time.Minute, trustProxy)
	logins.message = "too many demo sign-ins; try again in a minute"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := mux.Handler(r); pattern != "" && !demoAllowed[pattern] {
			writeError(w, apperr.Forbidden("this is turned off in the public demo"))
			return
		}
		ip := all.clientIP(r)
		limiter := (*loginLimiter)(nil)
		switch {
		case !all.Allow(ip):
			limiter = all
		case r.Method != http.MethodGet && r.Method != http.MethodHead && !writes.Allow(ip):
			limiter = writes
		case r.URL.Path == "/v1/auth/demo" && !logins.Allow(ip):
			limiter = logins
		}
		if limiter != nil {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, http.StatusTooManyRequests, errorBody{Error: errorDetail{Code: "rate_limited", Message: limiter.message}})
			return
		}
		next.ServeHTTP(w, r)
	})
}
