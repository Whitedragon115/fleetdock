// Package demoapp sets up and maintains the public demo (FLEETDOCK_DEMO_MODE).
//
// The demo is meant to be reachable by anyone, so it is built to give
// visitors nothing beyond a disposable sandbox:
//
//   - one shared visitor account, signed in automatically (POST /v1/auth/demo),
//     whose role can read and change data but not administer anything;
//   - the API limited to an allowlist of data routes (httpapi/demo_guard.go),
//     so servers, instances, users, backups and settings cannot be changed
//     and nothing can be pointed at another host;
//   - a single external instance, the sandbox database server, which the
//     deployment isolates and resets (deploy/public-demo). Its data access is
//     "managed": the console and data browser run as Fleetdock's per-database
//     roles, never as the sandbox's admin login.
//
// This package creates the account, role and instance on start, and keeps the
// metadata visitors leave behind (query history, saved queries, table views)
// from growing without bound.
package demoapp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	instanceapp "github.com/Fleetdock/fleetdock/backend/internal/app/instance"
	userapp "github.com/Fleetdock/fleetdock/backend/internal/app/user"
	"github.com/Fleetdock/fleetdock/backend/internal/config"
)

// RoleName is the visitor account's role.
const RoleName = "demo-visitor"

// InstanceName is the sandbox instance's name in the dashboard.
const InstanceName = "Demo sandbox"

// rolePermissions is everything a visitor may do. No instance:write, so the
// console never runs on a system database or with an admin login, and no
// backup, user, token or settings permission at all.
var rolePermissions = []string{"server:read", "instance:read", "database:read", "database:write"}

// Cleaner deletes what the visitor account left in the metadata database.
type Cleaner interface {
	CleanupDemo(ctx context.Context, userID uuid.UUID, before time.Time) (int64, error)
}

// Service bootstraps and maintains the demo.
type Service struct {
	cfg       config.Demo
	users     *userapp.Service
	instances *instanceapp.Service
	cleaner   Cleaner
	userID    uuid.UUID
}

// NewService wires the service.
func NewService(cfg config.Demo, users *userapp.Service, instances *instanceapp.Service, cleaner Cleaner) *Service {
	return &Service{cfg: cfg, users: users, instances: instances, cleaner: cleaner}
}

// Bootstrap creates (or repairs) the visitor role and account and the sandbox
// instance, and imports the sandbox's databases. Safe to run on every start.
func (s *Service) Bootstrap(ctx context.Context) error {
	if err := s.ensureRole(ctx); err != nil {
		return fmt.Errorf("demo role: %w", err)
	}
	if err := s.ensureUser(ctx); err != nil {
		return fmt.Errorf("demo account: %w", err)
	}
	id, err := s.ensureInstance(ctx)
	if err != nil {
		return fmt.Errorf("demo sandbox instance: %w", err)
	}
	// The sandbox may still be starting: retry the import for a while.
	for attempt := 0; ; attempt++ {
		_, err = s.instances.ImportDatabases(ctx, id, nil)
		if err == nil || attempt >= 30 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if err != nil {
		slog.Warn("demo: could not list the sandbox's databases yet; the probe will retry", "error", err.Error())
	}
	slog.Info("demo mode ready", "account", s.cfg.Email, "sandbox", s.cfg.SandboxHost)
	return nil
}

// Run deletes old visitor metadata every few minutes until ctx ends.
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		if s.userID != uuid.Nil && s.cleaner != nil {
			n, err := s.cleaner.CleanupDemo(ctx, s.userID, time.Now().Add(-s.cfg.CleanupAge))
			if err != nil {
				slog.Warn("demo: cleanup", "error", err.Error())
			} else if n > 0 {
				slog.Info("demo: removed old visitor history, saved queries and views", "rows", n)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Service) ensureRole(ctx context.Context) error {
	roles, err := s.users.ListRoles(ctx)
	if err != nil {
		return err
	}
	name := RoleName
	desc := "Public demo visitors: browse and change data in the sandbox databases."
	for _, r := range roles {
		if r.Name == RoleName {
			_, err := s.users.UpdateRole(ctx, r.ID.String(), userapp.RoleInput{Description: &desc, Permissions: rolePermissions})
			return err
		}
	}
	_, err = s.users.CreateRole(ctx, userapp.RoleInput{Name: &name, Description: &desc, Permissions: rolePermissions})
	return err
}

func (s *Service) ensureUser(ctx context.Context) error {
	email := strings.ToLower(strings.TrimSpace(s.cfg.Email))
	users, err := s.users.List(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		if strings.EqualFold(u.Email, email) {
			s.userID = u.ID
			// Keep it a visitor, whatever happened to it before.
			role, active := RoleName, "active"
			_, err := s.users.Update(ctx, uuid.Nil, u.ID.String(), userapp.UpdateInput{Role: &role, Status: &active})
			return err
		}
	}
	// Nobody signs in with this password (demo mode turns password login
	// off); it only satisfies the account model.
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return err
	}
	u, err := s.users.Create(ctx, userapp.CreateInput{
		Name: "Demo visitor", Email: email, Password: hex.EncodeToString(buf), Role: RoleName,
	})
	if err != nil {
		return err
	}
	s.userID = u.ID
	return nil
}

func (s *Service) ensureInstance(ctx context.Context) (string, error) {
	managed := "managed"
	tls := "disable" // the sandbox is on a private, internal-only network
	list, err := s.instances.List(ctx, instanceapp.ListParams{Kind: "external", Limit: 100})
	if err != nil {
		return "", err
	}
	for _, inst := range list.Items {
		if inst.Name != InstanceName {
			continue
		}
		if string(inst.Engine) != s.cfg.SandboxEngine {
			return "", fmt.Errorf("the %q instance is %s, but the sandbox is now %s; start with a fresh metadata database",
				InstanceName, inst.Engine, s.cfg.SandboxEngine)
		}
		host, port, user, pw := s.cfg.SandboxHost, s.cfg.SandboxPort, s.cfg.SandboxUser, s.cfg.SandboxPassword
		_, err := s.instances.Update(ctx, inst.ID.String(), instanceapp.UpdateInput{
			Host: &host, Port: &port, Username: &user, Password: &pw, TLSMode: &tls,
			DataAccess: instanceapp.DataAccessInput{Mode: &managed},
		})
		return inst.ID.String(), err
	}
	inst, err := s.instances.Register(ctx, instanceapp.RegisterInput{
		Kind:   "external",
		Name:   InstanceName,
		Engine: s.cfg.SandboxEngine,
		// Display only; the probe reports the server's real version.
		EngineVersion: map[string]string{"mysql": "8.4", "mariadb": "11", "postgres": "16"}[s.cfg.SandboxEngine],
		Host:          s.cfg.SandboxHost,
		Port:          s.cfg.SandboxPort,
		Username:      s.cfg.SandboxUser,
		Password:      s.cfg.SandboxPassword,
		TLSMode:       tls,
		Labels:        map[string]string{"demo": "true"},
		DataAccess:    instanceapp.DataAccessInput{Mode: &managed},
	})
	if err != nil {
		return "", err
	}
	return inst.ID.String(), nil
}
