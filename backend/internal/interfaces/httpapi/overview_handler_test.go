package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	authapp "github.com/Fleetdock/fleetdock/backend/internal/app/auth"
	summaryapp "github.com/Fleetdock/fleetdock/backend/internal/app/summary"
	authz "github.com/Fleetdock/fleetdock/backend/internal/domain/authz"
	statsdom "github.com/Fleetdock/fleetdock/backend/internal/domain/stats"
)

type fakeStats struct {
	summary   statsdom.Summary
	attention []statsdom.Attention
	insights  statsdom.Insights
	databases []statsdom.DatabaseSize
	servers   []statsdom.ServerLoad
}

func (f fakeStats) Insights(context.Context) (statsdom.Insights, error) { return f.insights, nil }
func (f fakeStats) LargestDatabases(_ context.Context, limit int) ([]statsdom.DatabaseSize, error) {
	return f.databases[:min(limit, len(f.databases))], nil
}
func (f fakeStats) ServerLoads(_ context.Context, limit int) ([]statsdom.ServerLoad, error) {
	return f.servers[:min(limit, len(f.servers))], nil
}

func (f fakeStats) Summary(context.Context) (statsdom.Summary, error) { return f.summary, nil }
func (f fakeStats) Attention(_ context.Context, limit int) ([]statsdom.Attention, error) {
	if len(f.attention) > limit {
		return f.attention[:limit], nil
	}
	return f.attention, nil
}

func getOverview(t *testing.T, repo fakeStats, p *authapp.Principal) overviewResponse {
	t.Helper()
	h := NewOverviewHandler(summaryapp.NewService(repo), nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/overview", nil)
	req = req.WithContext(withPrincipal(req.Context(), p))
	rr := httptest.NewRecorder()
	h.Overview(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body)
	}
	var out overviewResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestOverviewAttentionIsFilteredByScope(t *testing.T) {
	mine, other := uuid.New(), uuid.New()
	myDB := uuid.New()
	repo := fakeStats{
		summary: statsdom.Summary{ServersTotal: 2, InstancesTotal: 1, DestinationsTotal: 1, SchedulesEnabled: 3},
		attention: []statsdom.Attention{
			{Kind: statsdom.KindServerOffline, Severity: "critical", ResourceType: "server", ResourceID: other, Name: "theirs", ServerID: other, Since: time.Now()},
			{Kind: statsdom.KindBackupFailed, Severity: "critical", ResourceType: "backup", ResourceID: uuid.New(), Name: "orders", ServerID: mine, DatabaseID: myDB},
			{Kind: statsdom.KindDatabaseMissing, Severity: "warning", ResourceType: "database", ResourceID: uuid.New(), Name: "elsewhere", ServerID: other, DatabaseID: uuid.New()},
			// External instance: no server, so only a global grant covers it.
			{Kind: statsdom.KindInstanceUnreachable, Severity: "critical", ResourceType: "instance", ResourceID: uuid.New(), Name: "ext"},
		},
	}

	scoped := authapp.NewPrincipalWithGrants(uuid.New(), "a@example.com", []authz.Grant{
		{Permission: "backup:read", Scope: authz.Scope{Type: authz.ScopeDatabase, ID: myDB}},
		{Permission: "server:read", Scope: authz.Scope{Type: authz.ScopeServer, ID: mine}},
	})
	out := getOverview(t, repo, scoped)
	if len(out.Attention) != 1 || out.Attention[0].Name != "orders" {
		t.Fatalf("scoped user sees %+v, want only orders", out.Attention)
	}
	if out.Setup.Servers != 2 || out.Setup.Destinations != 1 || out.Setup.Schedules != 3 || out.Setup.Channels != 0 {
		t.Errorf("setup = %+v", out.Setup)
	}

	admin := authapp.NewPrincipal(uuid.New(), "b@example.com", "server:read", "instance:read", "database:read", "backup:read")
	if got := getOverview(t, repo, admin); len(got.Attention) != 4 {
		t.Errorf("admin sees %d items, want 4", len(got.Attention))
	}

	nobody := authapp.NewPrincipal(uuid.New(), "c@example.com")
	got := getOverview(t, repo, nobody)
	if got.Attention == nil || len(got.Attention) != 0 {
		t.Errorf("attention = %#v, want empty list", got.Attention)
	}
}

func TestOverviewAttentionIsCapped(t *testing.T) {
	var many []statsdom.Attention
	for i := 0; i < 3*maxAttention; i++ {
		many = append(many, statsdom.Attention{Kind: statsdom.KindDatabaseMissing, Severity: "warning", ResourceType: "database", ResourceID: uuid.New()})
	}
	admin := authapp.NewPrincipal(uuid.New(), "b@example.com", "database:read")
	if got := getOverview(t, fakeStats{attention: many}, admin); len(got.Attention) != maxAttention {
		t.Errorf("got %d items, want %d", len(got.Attention), maxAttention)
	}
}

func TestOverviewInsightsAreFilteredByScope(t *testing.T) {
	mine, other := uuid.New(), uuid.New()
	myDB := uuid.New()
	cpu := 42.5
	repo := fakeStats{
		insights: statsdom.Insights{
			DatabaseBytes: 3 << 30, BackupBytes: 1 << 30, BackupsStored: 4, AvgCPUPct: &cpu,
			CoverageDatabases: 3, CoverageScheduled: 2,
			Engines:    []statsdom.EngineStat{{Engine: "postgres", Instances: 1, Databases: 3, SizeBytes: 3 << 30}},
			BackupDays: []statsdom.BackupDay{{Day: time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC), Completed: 2, Failed: 1}},
		},
		databases: []statsdom.DatabaseSize{
			{ID: uuid.New(), Name: "big-theirs", ServerID: other, SizeBytes: 2 << 30},
			{ID: myDB, Name: "orders", ServerID: other, SizeBytes: 1 << 30},
			{ID: uuid.New(), Name: "external"}, // no server: global grant only
		},
		servers: []statsdom.ServerLoad{
			{ID: other, Name: "theirs", Status: "online"},
			{ID: mine, Name: "mine", Status: "online", CPUPct: &cpu},
		},
	}

	scoped := authapp.NewPrincipalWithGrants(uuid.New(), "a@example.com", []authz.Grant{
		{Permission: "database:read", Scope: authz.Scope{Type: authz.ScopeDatabase, ID: myDB}},
		{Permission: "server:read", Scope: authz.Scope{Type: authz.ScopeServer, ID: mine}},
	})
	out := getOverview(t, repo, scoped)
	if len(out.LargestDatabases) != 1 || out.LargestDatabases[0].Name != "orders" {
		t.Errorf("databases = %+v, want only orders", out.LargestDatabases)
	}
	if len(out.ServerLoads) != 1 || out.ServerLoads[0].Name != "mine" || *out.ServerLoads[0].CPUPct != cpu {
		t.Errorf("servers = %+v, want only mine", out.ServerLoads)
	}
	if out.Storage.DatabaseBytes != 3<<30 || out.Storage.BackupsStored != 4 || out.Coverage.Scheduled != 2 {
		t.Errorf("totals = %+v / %+v", out.Storage, out.Coverage)
	}
	if len(out.BackupDays) != 1 || out.BackupDays[0].Day != "2026-11-08" || out.BackupDays[0].Failed != 1 {
		t.Errorf("backup days = %+v", out.BackupDays)
	}

	admin := authapp.NewPrincipal(uuid.New(), "b@example.com", "server:read", "database:read")
	if got := getOverview(t, repo, admin); len(got.LargestDatabases) != 3 || len(got.ServerLoads) != 2 {
		t.Errorf("admin sees %d databases / %d servers, want 3 / 2", len(got.LargestDatabases), len(got.ServerLoads))
	}

	nobody := getOverview(t, repo, authapp.NewPrincipal(uuid.New(), "c@example.com"))
	if nobody.LargestDatabases == nil || len(nobody.LargestDatabases) != 0 || len(nobody.ServerLoads) != 0 {
		t.Errorf("nobody sees %+v / %+v", nobody.LargestDatabases, nobody.ServerLoads)
	}
}
