"use client";

// Overview insight cards. Data comes from GET /v1/overview, already filtered to what the caller may read.

import Link from "next/link";

import { ArrowRight, BarChart3, Database, HardDrive, Server, ShieldCheck } from "lucide-react";

import { Card, CardHeader, StatusBadge, Time } from "@/components/ui";
import { Tooltip } from "@/components/ui/tooltip";
import { engineLabel } from "@/lib/engines";
import { formatBytes } from "@/lib/format";
import type { Overview } from "@/lib/types";

// ---- shared pieces ----------------------------------------------------------

function pct(used?: number | null, total?: number | null): number | null {
  if (used == null || !total) return null;
  return Math.min(100, Math.max(0, (used / total) * 100));
}

function toneFor(p: number | null): "" | " is-warning" | " is-danger" {
  if (p == null) return "";
  if (p >= 90) return " is-danger";
  if (p >= 75) return " is-warning";
  return "";
}

/** Meter is a labelled usage bar; the number is always printed, so colour is never the only signal. */
function Meter({
  label,
  value,
  detail,
  usage = true,
}: {
  label: string;
  value: number | null;
  detail?: string;
  /** Usage meters turn amber/red when nearly full; coverage meters (where more is better) stay neutral. */
  usage?: boolean;
}) {
  return (
    <div className="meter">
      <div className="meter-head">
        <span className="muted">{label}</span>
        <span className="meter-value">
          {value == null ? "—" : `${Math.round(value)}%`}
          {detail ? <span className="muted"> · {detail}</span> : null}
        </span>
      </div>
      <div
        className={`meter-track${usage ? toneFor(value) : ""}`}
        role="meter"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={value == null ? undefined : Math.round(value)}
        aria-valuetext={value == null ? "No data" : `${Math.round(value)}%${detail ? `, ${detail}` : ""}`}
      >
        <span className="meter-fill" style={{ width: `${value ?? 0}%` }} />
      </div>
    </div>
  );
}

function Figure({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div className="insight-figure">
      <div className="text-xs muted">{label}</div>
      <div className="insight-figure-value">{value}</div>
      {hint ? <div className="text-xs muted truncate">{hint}</div> : null}
    </div>
  );
}

function ViewAll({ href }: { href: string }) {
  return (
    <Link href={href} className="link text-sm inline-flex items-center gap-1">
      View all <ArrowRight size={13} aria-hidden />
    </Link>
  );
}

// ---- storage & capacity -----------------------------------------------------

export function StorageCard({ data }: { data: Overview }) {
  const { storage, resources } = data;
  const disk = pct(storage.disk_used_bytes, storage.disk_total_bytes);
  const mem = pct(resources.mem_used_bytes, resources.mem_total_bytes);
  const hasServers = storage.disk_total_bytes > 0 || resources.mem_total_bytes > 0;
  return (
    <Card className="overflow-hidden">
      <CardHeader title="Storage & capacity" icon={<HardDrive size={15} aria-hidden className="text-muted" />} />
      <div className="card-body stack-sm">
        <div className="insight-figures">
          <Figure
            label="Data stored"
            value={formatBytes(storage.database_bytes, "0 B")}
            hint={`${data.databases.total} database${data.databases.total === 1 ? "" : "s"}`}
          />
          <Figure
            label="Backup storage"
            value={formatBytes(storage.backup_bytes, "0 B")}
            hint={`${storage.backups_stored} backup${storage.backups_stored === 1 ? "" : "s"} kept`}
          />
          <Figure
            label="Connections"
            value={String(resources.active_connections)}
            hint="Open right now"
          />
        </div>
        {hasServers ? (
          <>
            <Meter
              label="Server disk"
              value={disk}
              detail={`${formatBytes(storage.disk_used_bytes)} of ${formatBytes(storage.disk_total_bytes)}`}
            />
            <Meter
              label="Server memory"
              value={mem}
              detail={`${formatBytes(resources.mem_used_bytes)} of ${formatBytes(resources.mem_total_bytes)}`}
            />
            <Meter label="Average CPU" value={resources.avg_cpu_pct} />
          </>
        ) : (
          <p className="muted text-xs m-0">Connect a server to see disk, memory and CPU across your fleet.</p>
        )}
      </div>
    </Card>
  );
}

// ---- data insights: engines + backup coverage ------------------------------

// Fixed engine → hue mapping: colour follows the engine, never its rank.
const ENGINE_COLOR: Record<string, string> = {
  postgres: "var(--chart-2)",
  mysql: "var(--chart-3)",
  mariadb: "var(--chart-4)",
};

function EngineSplit({ engines }: { engines: Overview["engines"] }) {
  const total = engines.reduce((n, e) => n + e.size_bytes, 0);
  const totalDbs = engines.reduce((n, e) => n + e.databases, 0);
  // Size when it is known, otherwise database count.
  const weight = (e: Overview["engines"][number]) => (total > 0 ? e.size_bytes : e.databases);
  const sum = total > 0 ? total : totalDbs;
  if (engines.length === 0) return null;
  return (
    <div className="insight-group">
      <div className="text-xs muted">By engine</div>
      {sum > 0 ? (
        <div className="engine-split" aria-hidden>
          {engines.map((e) =>
            weight(e) > 0 ? (
              <span key={e.engine} className="engine-split-cell" style={{ flexGrow: weight(e) }}>
                <Tooltip
                  describe={false}
                  content={`${engineLabel(e.engine)}: ${formatBytes(e.size_bytes, "0 B")}, ${e.databases} database${e.databases === 1 ? "" : "s"}`}
                >
                  <span className="engine-split-seg" style={{ background: ENGINE_COLOR[e.engine] ?? "var(--chart-1)" }} />
                </Tooltip>
              </span>
            ) : null,
          )}
        </div>
      ) : null}
      <ul className="engine-legend">
        {engines.map((e) => (
          <li key={e.engine}>
            <span className="legend-swatch" style={{ background: ENGINE_COLOR[e.engine] ?? "var(--chart-1)" }} aria-hidden />
            <span className="font-medium">{engineLabel(e.engine)}</span>
            <span className="muted">
              {e.databases} database{e.databases === 1 ? "" : "s"} · {e.instances} database server
              {e.instances === 1 ? "" : "s"} · {formatBytes(e.size_bytes, "0 B")}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

export function DataInsightsCard({ data }: { data: Overview }) {
  const c = data.coverage;
  const ofDbs = (n: number) => (c.databases > 0 ? (n / c.databases) * 100 : null);
  return (
    <Card className="overflow-hidden">
      <CardHeader title="Data insights" icon={<ShieldCheck size={15} aria-hidden className="text-muted" />} />
      <div className="card-body stack-sm">
        {c.databases === 0 ? (
          <p className="muted text-xs m-0">No active databases yet.</p>
        ) : (
          <>
            <Meter usage={false} label="On a backup schedule" value={ofDbs(c.scheduled)} detail={`${c.scheduled} of ${c.databases}`} />
            <Meter usage={false} label="Backed up in last 7 days" value={ofDbs(c.backed_up_7d)} detail={`${c.backed_up_7d} of ${c.databases}`} />
            <Meter usage={false} label="Test restore passed" value={ofDbs(c.verified)} detail={`${c.verified} of ${c.databases}`} />
          </>
        )}
        <EngineSplit engines={data.engines} />
      </div>
    </Card>
  );
}

// ---- backups, last 7 days ---------------------------------------------------

const dayLabel = (day: string) =>
  new Date(`${day}T00:00:00Z`).toLocaleDateString(undefined, { weekday: "short", timeZone: "UTC" });

export function BackupTrendCard({ days }: { days: Overview["backup_days"] }) {
  const max = Math.max(1, ...days.map((d) => d.completed + d.failed));
  const completed = days.reduce((n, d) => n + d.completed, 0);
  const failed = days.reduce((n, d) => n + d.failed, 0);
  const rate = completed + failed > 0 ? Math.round((completed / (completed + failed)) * 100) : null;
  return (
    <Card className="overflow-hidden">
      <CardHeader
        title="Backups, last 7 days"
        icon={<BarChart3 size={15} aria-hidden className="text-muted" />}
        actions={<ViewAll href="/backups" />}
      />
      <div className="card-body stack-sm">
        <div className="insight-figures">
          <Figure label="Completed" value={String(completed)} />
          <Figure label="Failed" value={String(failed)} />
          <Figure label="Success rate" value={rate == null ? "—" : `${rate}%`} />
        </div>
        <div className="backup-bars" aria-hidden>
          {days.map((d) => {
            const total = d.completed + d.failed;
            return (
              <Tooltip
                key={d.day}
                describe={false}
                content={
                  <>
                    <strong>{new Date(`${d.day}T00:00:00Z`).toLocaleDateString(undefined, { dateStyle: "medium", timeZone: "UTC" })}</strong>
                    <br />
                    {d.completed} completed · {d.failed} failed
                    {d.bytes > 0 ? ` · ${formatBytes(d.bytes)}` : ""}
                  </>
                }
              >
                <span className="backup-bar">
                  <span className="backup-bar-plot">
                    <span className="backup-bar-stack" style={{ height: `${(total / max) * 100}%` }}>
                      {d.failed > 0 ? <span className="backup-bar-failed" style={{ flexGrow: d.failed }} /> : null}
                      {d.completed > 0 ? <span className="backup-bar-ok" style={{ flexGrow: d.completed }} /> : null}
                    </span>
                  </span>
                  <span className="backup-bar-label">{dayLabel(d.day)}</span>
                </span>
              </Tooltip>
            );
          })}
        </div>
        <div className="engine-legend engine-legend-inline text-xs" aria-hidden>
          <span>
            <span className="legend-swatch backup-bar-ok" /> Completed
          </span>
          <span>
            <span className="legend-swatch backup-bar-failed" /> Failed
          </span>
        </div>
        <table className="sr-only">
          <caption>Backups per day, last 7 days (UTC)</caption>
          <thead>
            <tr>
              <th scope="col">Day</th>
              <th scope="col">Completed</th>
              <th scope="col">Failed</th>
            </tr>
          </thead>
          <tbody>
            {days.map((d) => (
              <tr key={d.day}>
                <th scope="row">{d.day}</th>
                <td>{d.completed}</td>
                <td>{d.failed}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Card>
  );
}

// ---- largest databases ------------------------------------------------------

export function LargestDatabasesCard({ items }: { items: Overview["largest_databases"] }) {
  const max = Math.max(1, ...items.map((d) => d.size_bytes));
  return (
    <Card className="overflow-hidden">
      <CardHeader
        title="Largest databases"
        icon={<Database size={15} aria-hidden className="text-muted" />}
        actions={<ViewAll href="/databases" />}
      />
      {items.length === 0 ? (
        <p className="muted text-sm m-0 p-4">No databases to show yet.</p>
      ) : (
        <ul className="recent-list">
          {items.map((d) => (
            <li key={d.id}>
              <Link href={`/databases/${d.id}`} className="insight-row">
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-medium">{d.name}</span>
                  <span className="block truncate text-xs muted">
                    {d.instance_name} · {engineLabel(d.engine)}
                    {d.active_connections > 0
                      ? ` · ${d.active_connections} connection${d.active_connections === 1 ? "" : "s"}`
                      : ""}
                  </span>
                </span>
                <span className="size-bar hide-sm" aria-hidden>
                  <span style={{ width: `${(d.size_bytes / max) * 100}%` }} />
                </span>
                <span className="text-sm tabular-nums min-w-16 text-right">{formatBytes(d.size_bytes)}</span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}

// ---- server health ----------------------------------------------------------

function MiniMeter({ label, value }: { label: string; value: number | null }) {
  return (
    <span className="mini-meter" title={value == null ? `${label}: no data` : `${label}: ${Math.round(value)}%`}>
      <span className="mini-meter-label">{label}</span>
      <span className={`meter-track${toneFor(value)}`} aria-hidden>
        <span className="meter-fill" style={{ width: `${value ?? 0}%` }} />
      </span>
      <span className="mini-meter-value">{value == null ? "—" : `${Math.round(value)}%`}</span>
    </span>
  );
}

export function ServerHealthCard({ items, total }: { items: Overview["server_loads"]; total: number }) {
  return (
    <Card className="overflow-hidden">
      <CardHeader
        title="Server health"
        icon={<Server size={15} aria-hidden className="text-muted" />}
        description={total > items.length ? `Busiest ${items.length} of ${total}` : undefined}
        actions={<ViewAll href="/servers" />}
      />
      {items.length === 0 ? (
        <p className="muted text-sm m-0 p-4">No servers connected. External database servers don&apos;t report host metrics.</p>
      ) : (
        <ul className="recent-list">
          {items.map((s) => {
            const live = s.status === "online";
            return (
              <li key={s.id}>
                <Link href={`/servers/${s.id}`} className="insight-row server-row">
                  <span className="min-w-0 server-row-name">
                    <span className="flex items-center gap-2">
                      <span className="truncate font-medium">{s.name}</span>
                      <StatusBadge status={s.status} />
                    </span>
                    <span className="block truncate text-xs muted">
                      {s.instances} instance{s.instances === 1 ? "" : "s"} · {s.databases} database
                      {s.databases === 1 ? "" : "s"}
                      {s.collected_at ? (
                        <>
                          {" · "}
                          <Time value={s.collected_at} />
                        </>
                      ) : null}
                    </span>
                  </span>
                  <span className="server-row-meters">
                    <MiniMeter label="CPU" value={live ? (s.cpu_pct ?? null) : null} />
                    <MiniMeter label="Mem" value={live ? pct(s.mem_used_bytes, s.mem_total_bytes) : null} />
                    <MiniMeter label="Disk" value={live ? pct(s.disk_used_bytes, s.disk_total_bytes) : null} />
                  </span>
                </Link>
              </li>
            );
          })}
        </ul>
      )}
    </Card>
  );
}
