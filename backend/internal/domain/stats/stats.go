// Package stats is the domain model for aggregate control-plane statistics
// shown on the overview dashboard.
package stats

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Summary is a snapshot of fleet-wide counts.
type Summary struct {
	ServersTotal   int
	ServersOnline  int
	ServersOffline int

	InstancesTotal    int
	InstancesManaged  int
	InstancesExternal int

	DatabasesTotal  int
	DatabasesActive int

	BackupsCompleted24h int
	BackupsFailed24h    int
	LastBackupAt        *time.Time

	OperationsRunning   int
	OperationsFailed24h int

	SchedulesEnabled  int
	ChannelsEnabled   int
	RulesEnabled      int
	DestinationsTotal int
}

// Attention kinds: problems the overview surfaces, most urgent first.
const (
	KindServerOffline       = "server_offline"
	KindAgentUnsupported    = "agent_unsupported"
	KindAgentOutdated       = "agent_outdated"
	KindInstanceUnreachable = "instance_unreachable"
	KindBackupFailed        = "backup_failed"
	KindBackupCheckFailed   = "backup_check_failed"
	KindNoRecentBackup      = "no_recent_backup"
	KindDatabaseMissing     = "database_missing"
	KindOperationFailed     = "operation_failed"
)

// Severities of an attention item.
const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
)

// Attention is one problem that needs a person to look at it. ServerID and
// DatabaseID locate it for per-resource authorization (either may be Nil).
type Attention struct {
	Kind         string
	Severity     string
	ResourceType string
	ResourceID   uuid.UUID
	Name         string
	Message      string
	Since        time.Time
	ServerID     uuid.UUID
	DatabaseID   uuid.UUID
}

// AttentionPerm is the read permission needed to see an attention item.
func AttentionPerm(kind string) string {
	switch kind {
	case KindServerOffline, KindAgentOutdated, KindAgentUnsupported:
		return "server:read"
	case KindInstanceUnreachable:
		return "instance:read"
	case KindDatabaseMissing:
		return "database:read"
	case KindOperationFailed:
		return "operation:read"
	default:
		return "backup:read"
	}
}

// Insights are fleet-wide storage, data and capacity aggregates. Like Summary
// they are totals, not per-resource data, so they are not authorization-filtered.
type Insights struct {
	// DatabaseBytes is the probed size of every non-deleted database.
	DatabaseBytes int64
	// BackupBytes / BackupsStored cover completed backups still in retention.
	BackupBytes   int64
	BackupsStored int
	// Disk, memory and CPU come from the latest snapshot of online servers.
	DiskUsedBytes  int64
	DiskTotalBytes int64
	MemUsedBytes   int64
	MemTotalBytes  int64
	AvgCPUPct      *float64
	// ActiveConnections sums the probed connections of every database.
	ActiveConnections int

	// Backup coverage of active, non-system databases.
	CoverageDatabases  int // active user databases
	CoverageScheduled  int // ... with an enabled backup schedule
	CoverageBackedUp7d int // ... with a completed backup in the last 7 days
	CoverageVerified   int // ... whose latest test restore passed (30 days)

	Engines []EngineStat
	// BackupDays has one entry per UTC day for the last 7 days, oldest first.
	BackupDays []BackupDay
}

// EngineStat is the footprint of one database engine.
type EngineStat struct {
	Engine    string
	Instances int
	Databases int
	SizeBytes int64
}

// BackupDay counts backup outcomes on one UTC day.
type BackupDay struct {
	Day       time.Time
	Completed int
	Failed    int
	Bytes     int64
}

// DatabaseSize is one database in the "largest databases" list. ServerID is
// Nil for databases on external instances.
type DatabaseSize struct {
	ID           uuid.UUID
	Name         string
	InstanceID   uuid.UUID
	InstanceName string
	Engine       string
	SizeBytes    int64
	Connections  int
	ServerID     uuid.UUID
}

// ServerLoad is a server's latest resource snapshot. Metric fields are nil
// when the agent has not reported them.
type ServerLoad struct {
	ID             uuid.UUID
	Name           string
	Status         string
	CPUPct         *float64
	MemUsedBytes   *int64
	MemTotalBytes  *int64
	DiskUsedBytes  *int64
	DiskTotalBytes *int64
	CollectedAt    *time.Time
	Instances      int
	Databases      int
}

// Repository is the persistence port for aggregate statistics.
type Repository interface {
	Summary(ctx context.Context) (Summary, error)
	Insights(ctx context.Context) (Insights, error)
	// LargestDatabases returns up to limit non-system databases, largest first.
	LargestDatabases(ctx context.Context, limit int) ([]DatabaseSize, error)
	// ServerLoads returns up to limit servers, most heavily used first.
	ServerLoads(ctx context.Context, limit int) ([]ServerLoad, error)
	// Attention returns up to limit open problems, critical first, newest first.
	Attention(ctx context.Context, limit int) ([]Attention, error)
}
