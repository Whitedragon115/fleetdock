# Fleetdock public demo

A Fleetdock install that anyone can open: visitors are signed in automatically
and can browse, edit and query three sample MySQL databases (`shop`,
`analytics`, `billing`). Nothing they do can reach beyond a disposable sandbox,
and the sandbox is restored every hour.

```bash
cd deploy/public-demo
cp .env.example .env && chmod 600 .env
# fill in .env: the address, and a fresh secret for every empty value
#   openssl rand -base64 33 | tr -d '/+=\n'
docker compose up --build -d
docker compose ps        # wait for fleetdock and sandbox to be healthy
```

Requires Docker Compose 2.23 or later. Updating: `git pull --ff-only && docker compose up --build -d`.
Only Caddy publishes ports (80 and 443); open nothing else.

## What visitors can and cannot do

They **can** open the dashboard, the data browser and the SQL console on the
sample databases. They can read, insert, update and delete rows; create,
alter and drop tables and indexes; run any SQL the database allows; export
results; and save queries and table views.

They **cannot**:

- sign in with a password (the admin account is unusable from outside);
- see or change users, roles, API tokens, the audit log or settings;
- add, change or remove servers, database servers or databases, or point
  Fleetdock at any other host;
- use backups, restores, moves, notifications, the external-database gateway,
  agent enrolment or the install script;
- change the shared account's profile or password, or sign other visitors out.

## How it is kept safe

**Fleetdock (`FLEETDOCK_DEMO_MODE=true`):**

- `POST /v1/auth/demo` signs anyone in as the shared visitor account
  (`demo@fleetdock.local`). Its role `demo-visitor` holds only `server:read`,
  `instance:read`, `database:read` and `database:write`. It has no
  `instance:write`, so the console refuses system databases and never runs
  with the server's admin login.
- The API serves an **allowlist** of routes (`backend/internal/interfaces/httpapi/demo_guard.go`).
  Every other route answers 403 before authentication runs. That includes
  password sign-in. A route added to Fleetdock later stays off until it is
  added to the list.
- Per client IP: 600 requests a minute, 90 changes a minute, 20 demo sign-ins
  a minute.
- The sandbox instance uses **managed data access**. Every console and
  data-browser request runs as Fleetdock's per-database account
  (`fleetdock_ro_…` or `fleetdock_rw_…`), which MySQL itself confines to that
  database. The read account only has `SELECT`.
- Visitors' query history, saved queries and table views are deleted after an
  hour (`DEMO_CLEANUP_AGE`). The audit log and the change journal are kept for
  a short time only.

**The sandbox MySQL server (8.4 LTS):**

- It is on an `internal` Docker network: no internet access, and no route to
  Fleetdock's own metadata database, which is on a separate network.
- Fleetdock's login there, `fleetdock_admin`, holds privileges on the three
  demo databases plus `CREATE USER` and `RELOAD`, which it needs for its
  console accounts. It has no `FILE`, `PROCESS`, `SUPER` or other admin
  privilege, and nothing on the system databases. MySQL's `SYSTEM_USER`
  protection stops it from touching `root`.
- Visitors' accounts cannot:
  - read or write server files (`LOAD_FILE`, `INTO OUTFILE`, `LOAD DATA`);
    `local_infile` is off and `secure_file_priv` is `NULL`;
  - create accounts, databases, events or plugins;
  - read `mysql.user` or `performance_schema`;
  - change global settings;
  - see the other demo databases.
- Its data lives in a 768 MB tmpfs. CPU (1 core), memory (1.5 GB), processes,
  connections (60 in total, 20 per account) and temporary tables are capped.
  The binary log is off. If a visitor fills the disk, only the sandbox stops
  accepting writes, until the next reset. A restart of the container also
  brings back fresh data.
- `sandbox-reset` checks every 5 seconds and ends any visitor statement that
  has run for more than 45 seconds. On MySQL this is the real limit: the
  server's own timeout covers `SELECT` only, and a statement keeps running
  after Fleetdock stops waiting for it (30 seconds).
- Every `DEMO_RESET_INTERVAL_SECONDS` (default: one hour), it drops each demo
  database and loads the sample data again. That takes about a second per
  database.

## Things to know

- **Visitors share one account and one sandbox.** One visitor can see what
  another saved, or drop a table another is looking at, until the next reset.
  The banner in the dashboard says so. Don't put anything private in it.
- **Use a domain**, so Caddy serves HTTPS. With only an IP address the site is
  plain HTTP.
- Keep `.env` at mode 600. Never commit it or paste it anywhere.
- Logs: `docker compose logs -f fleetdock sandbox-reset`.
- Wipe everything: `docker compose down -v`.
