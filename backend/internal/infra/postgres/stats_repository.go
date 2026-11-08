package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/jackc/pgx/v5/pgxpool"

	statsdom "github.com/Fleetdock/fleetdock/backend/internal/domain/stats"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
	"github.com/Fleetdock/fleetdock/backend/internal/version"
)

// StatsRepository is the Postgres adapter for statsdom.Repository.
type StatsRepository struct {
	pool *pgxpool.Pool
}

// NewStatsRepository builds a stats repository.
func NewStatsRepository(pool *pgxpool.Pool) *StatsRepository { return &StatsRepository{pool: pool} }

var _ statsdom.Repository = (*StatsRepository)(nil)

// Summary runs the aggregate queries behind the overview dashboard in one
// round-trip using a single SELECT of scalar sub-queries.
func (r *StatsRepository) Summary(ctx context.Context) (statsdom.Summary, error) {
	const q = `
		SELECT
			(SELECT count(*) FROM servers WHERE deleted_at IS NULL),
			(SELECT count(*) FROM servers WHERE deleted_at IS NULL AND status = 'online'),
			(SELECT count(*) FROM servers WHERE deleted_at IS NULL AND status = 'offline'),
			(SELECT count(*) FROM instances WHERE deleted_at IS NULL),
			(SELECT count(*) FROM instances WHERE deleted_at IS NULL AND kind = 'managed'),
			(SELECT count(*) FROM instances WHERE deleted_at IS NULL AND kind = 'external'),
			(SELECT count(*) FROM databases WHERE deleted_at IS NULL),
			(SELECT count(*) FROM databases WHERE deleted_at IS NULL AND status = 'active'),
			(SELECT count(*) FROM backups WHERE status = 'completed' AND created_at > now() - interval '24 hours'),
			(SELECT count(*) FROM backups WHERE status = 'failed' AND created_at > now() - interval '24 hours'),
			(SELECT max(completed_at) FROM backups WHERE status = 'completed'),
			(SELECT count(*) FROM jobs WHERE status = 'running'),
			(SELECT count(*) FROM jobs WHERE status = 'failed' AND created_at > now() - interval '24 hours'),
			(SELECT count(*) FROM backup_schedules WHERE enabled),
			(SELECT count(*) FROM notification_channels WHERE enabled),
			(SELECT count(*) FROM alert_rules WHERE enabled),
			(SELECT count(*) FROM backup_destinations WHERE deleted_at IS NULL)`
	var s statsdom.Summary
	err := r.pool.QueryRow(ctx, q).Scan(
		&s.ServersTotal, &s.ServersOnline, &s.ServersOffline,
		&s.InstancesTotal, &s.InstancesManaged, &s.InstancesExternal,
		&s.DatabasesTotal, &s.DatabasesActive,
		&s.BackupsCompleted24h, &s.BackupsFailed24h, &s.LastBackupAt,
		&s.OperationsRunning, &s.OperationsFailed24h,
		&s.SchedulesEnabled, &s.ChannelsEnabled, &s.RulesEnabled,
		&s.DestinationsTotal,
	)
	if err != nil {
		return statsdom.Summary{}, apperr.Internal(fmt.Errorf("summary: %w", err))
	}
	return s, nil
}

// attentionSQL gathers every open problem in one statement. Each branch yields
// (kind, severity, resource_type, resource_id, name, message, since,
// server_id, database_id); the server/database ids drive authorization.
const attentionSQL = `
WITH latest_backup AS (
	SELECT DISTINCT ON (database_id) database_id, id, status, error, created_at
	FROM backups
	WHERE status IN ('completed','failed') AND created_at > now() - interval '7 days'
	ORDER BY database_id, created_at DESC
), latest_check AS (
	SELECT DISTINCT ON (database_id) database_id, id, verify_status, verify_error, verified_at
	FROM backups
	WHERE verify_status IN ('passed','failed') AND verified_at > now() - interval '7 days'
	ORDER BY database_id, verified_at DESC
), items AS (
	SELECT 'server_offline' AS kind, 'critical' AS severity, 'server' AS resource_type, s.id AS resource_id,
	       s.name, 'Not responding: its agent has stopped reporting in.' AS message,
	       COALESCE(s.last_heartbeat_at, s.updated_at) AS since, s.id AS server_id, NULL::uuid AS database_id
	FROM servers s
	WHERE s.deleted_at IS NULL AND s.status IN ('offline','error')

	UNION ALL
	-- Agents older than this control plane ($2, empty for a development build)
	-- or older than the oldest it supports ($3). Only plain x.y.z versions
	-- (optionally v-prefixed, with a suffix) are compared; offline servers are
	-- reported above already.
	SELECT CASE WHEN av.v < string_to_array($3, '.')::int[] THEN 'agent_unsupported' ELSE 'agent_outdated' END,
	       CASE WHEN av.v < string_to_array($3, '.')::int[] THEN 'critical' ELSE 'warning' END,
	       'server', s.id, s.name,
	       'Its agent is version ' || s.agent_version || '; this Fleetdock is ' || $2 || '. Update the agent from the server''s page.',
	       COALESCE(s.last_heartbeat_at, s.updated_at), s.id, NULL
	FROM servers s
	CROSS JOIN LATERAL (
		SELECT string_to_array(substring(s.agent_version from '^v?([0-9]+\.[0-9]+\.[0-9]+)'), '.')::int[] AS v
	) av
	WHERE s.deleted_at IS NULL AND s.status NOT IN ('offline','error') AND $2 <> ''
	  AND av.v IS NOT NULL AND av.v < string_to_array($2, '.')::int[]

	UNION ALL
	SELECT 'instance_unreachable', 'critical', 'instance', i.id,
	       i.name, COALESCE(NULLIF(i.health->>'error',''), 'Fleetdock cannot connect to it.'),
	       (i.health->>'checked_at')::timestamptz, i.server_id, NULL
	FROM instances i
	WHERE i.deleted_at IS NULL AND i.health->>'status' = 'unreachable'

	UNION ALL
	SELECT 'backup_failed', 'critical', 'backup', lb.id,
	       d.name, COALESCE(NULLIF(lb.error,''), 'The latest backup failed.'),
	       lb.created_at, i.server_id, d.id
	FROM latest_backup lb
	JOIN databases d ON d.id = lb.database_id AND d.deleted_at IS NULL
	JOIN instances i ON i.id = d.instance_id
	WHERE lb.status = 'failed'

	UNION ALL
	SELECT 'backup_check_failed', 'warning', 'backup', lc.id,
	       d.name, COALESCE(NULLIF(lc.verify_error,''), 'The latest backup could not be restored in a test.'),
	       lc.verified_at, i.server_id, d.id
	FROM latest_check lc
	JOIN databases d ON d.id = lc.database_id AND d.deleted_at IS NULL
	JOIN instances i ON i.id = d.instance_id
	WHERE lc.verify_status = 'failed'

	UNION ALL
	SELECT 'no_recent_backup', 'warning', 'database', d.id,
	       d.name, 'Has a backup schedule but no successful backup in the last 7 days.',
	       COALESCE((SELECT max(b.completed_at) FROM backups b WHERE b.database_id = d.id AND b.status = 'completed'), d.created_at),
	       i.server_id, d.id
	FROM databases d
	JOIN instances i ON i.id = d.instance_id
	WHERE d.deleted_at IS NULL AND d.status = 'active'
	  AND d.created_at < now() - interval '1 day'
	  AND EXISTS (SELECT 1 FROM backup_schedules bs WHERE bs.database_id = d.id AND bs.enabled)
	  AND NOT EXISTS (SELECT 1 FROM backups b WHERE b.database_id = d.id AND b.status = 'completed'
	                  AND b.completed_at > now() - interval '7 days')
	  AND NOT EXISTS (SELECT 1 FROM latest_backup lb WHERE lb.database_id = d.id AND lb.status = 'failed')

	UNION ALL
	SELECT 'database_missing', 'warning', 'database', d.id,
	       d.name, 'No longer found on ' || i.name || '. It may have been dropped or renamed outside Fleetdock.',
	       COALESCE(d.missing_since, d.updated_at), i.server_id, d.id
	FROM databases d
	JOIN instances i ON i.id = d.instance_id
	WHERE d.deleted_at IS NULL AND d.status = 'missing'

	UNION ALL
	SELECT 'operation_failed', 'warning', 'operation', j.id,
	       j.type, COALESCE(NULLIF(j.error,''), 'The task failed.'),
	       COALESCE(j.completed_at, j.updated_at),
	       COALESCE(j.server_id, ji.server_id, dsi.server_id, bi.server_id),
	       COALESCE(jd.id, bd.id)
	FROM jobs j
	LEFT JOIN instances ji ON j.resource_type = 'instance' AND ji.id = j.resource_id
	LEFT JOIN databases jd ON j.resource_type = 'database' AND jd.id = j.resource_id
	LEFT JOIN instances dsi ON dsi.id = jd.instance_id
	LEFT JOIN backups jb ON j.resource_type = 'backup' AND jb.id = j.resource_id
	LEFT JOIN databases bd ON bd.id = jb.database_id
	LEFT JOIN instances bi ON bi.id = bd.instance_id
	WHERE j.status = 'failed' AND j.created_at > now() - interval '24 hours'
	  AND j.type <> 'backup'
	  -- a later success of the same task on the same resource resolves it
	  AND NOT EXISTS (SELECT 1 FROM jobs j2 WHERE j2.type = j.type AND j2.resource_id IS NOT DISTINCT FROM j.resource_id
	                  AND j2.status = 'succeeded' AND j2.created_at > j.created_at)
)
SELECT kind, severity, resource_type, resource_id, name, message, since, server_id, database_id
FROM items
ORDER BY severity = 'critical' DESC, since DESC NULLS LAST
LIMIT $1`

// Attention returns up to limit open problems, critical first, newest first.
func (r *StatsRepository) Attention(ctx context.Context, limit int) ([]statsdom.Attention, error) {
	current, minAgent := "", version.MinAgentVersion
	if parts, _, ok := version.Parse(version.Version); ok {
		current = fmt.Sprintf("%d.%d.%d", parts[0], parts[1], parts[2])
	}
	rows, err := r.pool.Query(ctx, attentionSQL, limit, current, minAgent)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("attention: %w", err))
	}
	defer rows.Close()
	var out []statsdom.Attention
	for rows.Next() {
		var a statsdom.Attention
		var since *time.Time
		var serverID, databaseID *uuid.UUID
		if err := rows.Scan(&a.Kind, &a.Severity, &a.ResourceType, &a.ResourceID, &a.Name, &a.Message,
			&since, &serverID, &databaseID); err != nil {
			return nil, apperr.Internal(fmt.Errorf("attention scan: %w", err))
		}
		if since != nil {
			a.Since = *since
		}
		if serverID != nil {
			a.ServerID = *serverID
		}
		if databaseID != nil {
			a.DatabaseID = *databaseID
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, apperr.Internal(fmt.Errorf("attention rows: %w", err))
	}
	return out, nil
}

// insightsSQL computes the fleet-wide storage, coverage and capacity totals in
// one round-trip. Server metrics only count online servers: an offline
// server's last snapshot is stale.
const insightsSQL = `
WITH user_dbs AS (
	SELECT d.id FROM databases d
	WHERE d.deleted_at IS NULL AND d.status = 'active' AND NOT d.system
), latest_verify AS (
	SELECT DISTINCT ON (database_id) database_id, verify_status
	FROM backups
	WHERE verify_status IN ('passed','failed') AND verified_at > now() - interval '30 days'
	ORDER BY database_id, verified_at DESC
), live_health AS (
	SELECT h.* FROM server_health h
	JOIN servers s ON s.id = h.server_id
	WHERE s.deleted_at IS NULL AND s.status = 'online'
)
SELECT
	(SELECT COALESCE(sum(size_bytes), 0)::bigint FROM databases WHERE deleted_at IS NULL),
	(SELECT COALESCE(sum(size_bytes), 0)::bigint FROM backups WHERE status = 'completed'),
	(SELECT count(*) FROM backups WHERE status = 'completed'),
	(SELECT COALESCE(sum(disk_used_bytes), 0)::bigint FROM live_health),
	(SELECT COALESCE(sum(disk_total_bytes), 0)::bigint FROM live_health),
	(SELECT COALESCE(sum(mem_used_bytes), 0)::bigint FROM live_health),
	(SELECT COALESCE(sum(mem_total_bytes), 0)::bigint FROM live_health),
	(SELECT avg(cpu_pct)::float8 FROM live_health),
	(SELECT COALESCE(sum(active_connections), 0)::int FROM databases WHERE deleted_at IS NULL),
	(SELECT count(*) FROM user_dbs),
	(SELECT count(*) FROM user_dbs u
	 WHERE EXISTS (SELECT 1 FROM backup_schedules bs WHERE bs.database_id = u.id AND bs.enabled)),
	(SELECT count(*) FROM user_dbs u
	 WHERE EXISTS (SELECT 1 FROM backups b WHERE b.database_id = u.id AND b.status = 'completed'
	               AND b.completed_at > now() - interval '7 days')),
	(SELECT count(*) FROM user_dbs u JOIN latest_verify lv ON lv.database_id = u.id
	 WHERE lv.verify_status = 'passed')`

const enginesSQL = `
SELECT i.engine, count(DISTINCT i.id), count(d.id), COALESCE(sum(d.size_bytes), 0)::bigint
FROM instances i
LEFT JOIN databases d ON d.instance_id = i.id AND d.deleted_at IS NULL
WHERE i.deleted_at IS NULL
GROUP BY i.engine
ORDER BY 4 DESC, 1`

const backupDaysSQL = `
SELECT day::date,
       count(b.id) FILTER (WHERE b.status = 'completed'),
       count(b.id) FILTER (WHERE b.status = 'failed'),
       COALESCE(sum(b.size_bytes) FILTER (WHERE b.status = 'completed'), 0)::bigint
FROM generate_series((now() AT TIME ZONE 'UTC')::date - 6, (now() AT TIME ZONE 'UTC')::date, interval '1 day') AS day
LEFT JOIN backups b ON b.status IN ('completed','failed')
	AND (b.created_at AT TIME ZONE 'UTC')::date = day::date
GROUP BY day
ORDER BY day`

// Insights returns fleet-wide storage, data and capacity aggregates.
func (r *StatsRepository) Insights(ctx context.Context) (statsdom.Insights, error) {
	var in statsdom.Insights
	err := r.pool.QueryRow(ctx, insightsSQL).Scan(
		&in.DatabaseBytes, &in.BackupBytes, &in.BackupsStored,
		&in.DiskUsedBytes, &in.DiskTotalBytes, &in.MemUsedBytes, &in.MemTotalBytes, &in.AvgCPUPct,
		&in.ActiveConnections,
		&in.CoverageDatabases, &in.CoverageScheduled, &in.CoverageBackedUp7d, &in.CoverageVerified,
	)
	if err != nil {
		return statsdom.Insights{}, apperr.Internal(fmt.Errorf("insights: %w", err))
	}

	rows, err := r.pool.Query(ctx, enginesSQL)
	if err != nil {
		return statsdom.Insights{}, apperr.Internal(fmt.Errorf("insights engines: %w", err))
	}
	for rows.Next() {
		var e statsdom.EngineStat
		if err := rows.Scan(&e.Engine, &e.Instances, &e.Databases, &e.SizeBytes); err != nil {
			rows.Close()
			return statsdom.Insights{}, apperr.Internal(fmt.Errorf("insights engines scan: %w", err))
		}
		in.Engines = append(in.Engines, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return statsdom.Insights{}, apperr.Internal(fmt.Errorf("insights engines rows: %w", err))
	}

	rows, err = r.pool.Query(ctx, backupDaysSQL)
	if err != nil {
		return statsdom.Insights{}, apperr.Internal(fmt.Errorf("insights backups: %w", err))
	}
	defer rows.Close()
	for rows.Next() {
		var d statsdom.BackupDay
		if err := rows.Scan(&d.Day, &d.Completed, &d.Failed, &d.Bytes); err != nil {
			return statsdom.Insights{}, apperr.Internal(fmt.Errorf("insights backups scan: %w", err))
		}
		in.BackupDays = append(in.BackupDays, d)
	}
	if err := rows.Err(); err != nil {
		return statsdom.Insights{}, apperr.Internal(fmt.Errorf("insights backups rows: %w", err))
	}
	return in, nil
}

// LargestDatabases returns up to limit non-system databases, largest first.
func (r *StatsRepository) LargestDatabases(ctx context.Context, limit int) ([]statsdom.DatabaseSize, error) {
	const q = `
		SELECT d.id, d.name, i.id, i.name, i.engine, d.size_bytes, d.active_connections, i.server_id
		FROM databases d
		JOIN instances i ON i.id = d.instance_id AND i.deleted_at IS NULL
		WHERE d.deleted_at IS NULL AND NOT d.system AND d.status <> 'missing'
		ORDER BY d.size_bytes DESC, d.name
		LIMIT $1`
	rows, err := r.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("largest databases: %w", err))
	}
	defer rows.Close()
	var out []statsdom.DatabaseSize
	for rows.Next() {
		var d statsdom.DatabaseSize
		var serverID *uuid.UUID
		if err := rows.Scan(&d.ID, &d.Name, &d.InstanceID, &d.InstanceName, &d.Engine,
			&d.SizeBytes, &d.Connections, &serverID); err != nil {
			return nil, apperr.Internal(fmt.Errorf("largest databases scan: %w", err))
		}
		if serverID != nil {
			d.ServerID = *serverID
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, apperr.Internal(fmt.Errorf("largest databases rows: %w", err))
	}
	return out, nil
}

// ServerLoads returns up to limit servers: unhealthy ones first, then by their
// highest CPU, memory or disk utilisation.
func (r *StatsRepository) ServerLoads(ctx context.Context, limit int) ([]statsdom.ServerLoad, error) {
	const q = `
		SELECT s.id, s.name, s.status, h.cpu_pct::float8,
		       h.mem_used_bytes, h.mem_total_bytes, h.disk_used_bytes, h.disk_total_bytes, h.collected_at,
		       (SELECT count(*) FROM instances i WHERE i.server_id = s.id AND i.deleted_at IS NULL),
		       (SELECT count(*) FROM databases d JOIN instances i ON i.id = d.instance_id
		        WHERE i.server_id = s.id AND i.deleted_at IS NULL AND d.deleted_at IS NULL)
		FROM servers s
		LEFT JOIN server_health h ON h.server_id = s.id
		WHERE s.deleted_at IS NULL
		ORDER BY s.status IN ('offline','error') DESC,
		         GREATEST(h.cpu_pct::float8,
		                  100.0 * h.mem_used_bytes / NULLIF(h.mem_total_bytes, 0),
		                  100.0 * h.disk_used_bytes / NULLIF(h.disk_total_bytes, 0)) DESC NULLS LAST,
		         s.name
		LIMIT $1`
	rows, err := r.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("server loads: %w", err))
	}
	defer rows.Close()
	var out []statsdom.ServerLoad
	for rows.Next() {
		var s statsdom.ServerLoad
		if err := rows.Scan(&s.ID, &s.Name, &s.Status, &s.CPUPct, &s.MemUsedBytes, &s.MemTotalBytes,
			&s.DiskUsedBytes, &s.DiskTotalBytes, &s.CollectedAt, &s.Instances, &s.Databases); err != nil {
			return nil, apperr.Internal(fmt.Errorf("server loads scan: %w", err))
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, apperr.Internal(fmt.Errorf("server loads rows: %w", err))
	}
	return out, nil
}
