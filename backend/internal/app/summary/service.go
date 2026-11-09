// Package summaryapp implements the overview-dashboard use case.
package summaryapp

import (
	"context"

	statsdom "github.com/Fleetdock/fleetdock/backend/internal/domain/stats"
)

// Service returns aggregate control-plane statistics.
type Service struct {
	repo statsdom.Repository
}

// NewService wires the summary service.
func NewService(repo statsdom.Repository) *Service { return &Service{repo: repo} }

// Attention returns open problems, critical first (at most limit).
func (s *Service) Attention(ctx context.Context, limit int) ([]statsdom.Attention, error) {
	return s.repo.Attention(ctx, limit)
}

// Insights returns fleet-wide storage, data and capacity aggregates.
func (s *Service) Insights(ctx context.Context) (statsdom.Insights, error) {
	return s.repo.Insights(ctx)
}

// LargestDatabases returns up to limit databases, largest first.
func (s *Service) LargestDatabases(ctx context.Context, limit int) ([]statsdom.DatabaseSize, error) {
	return s.repo.LargestDatabases(ctx, limit)
}

// ServerLoads returns up to limit servers, most heavily used first.
func (s *Service) ServerLoads(ctx context.Context, limit int) ([]statsdom.ServerLoad, error) {
	return s.repo.ServerLoads(ctx, limit)
}

// Get returns the current fleet summary.
func (s *Service) Get(ctx context.Context) (statsdom.Summary, error) {
	return s.repo.Summary(ctx)
}
