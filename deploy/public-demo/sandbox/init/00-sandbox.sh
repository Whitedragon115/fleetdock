#!/bin/bash
# Sets up the public demo's sandbox MySQL server on its first start
# (docker-entrypoint-initdb.d; the data directory is a tmpfs, so that is every
# container start).
#
#   fleetdock_admin   Fleetdock's login: all privileges on the demo databases
#                     (with GRANT OPTION), plus CREATE USER and RELOAD to
#                     manage Fleetdock's per-database console accounts. No
#                     FILE, PROCESS, SUPER or any other global privilege, and
#                     nothing on the system databases. Visitors never run SQL
#                     as it.
#
# Visitors' SQL runs as Fleetdock's fleetdock_ro_* / fleetdock_rw_* accounts,
# which only get privileges on their one database.
set -euo pipefail
: "${SANDBOX_ADMIN_PASSWORD:?set SANDBOX_ADMIN_PASSWORD}"
DBS="${DEMO_DATABASES:-shop analytics billing}"
SEED_DIR="${SEED_DIR:-/sandbox/seed}"
if [ -z "${SANDBOX_MYSQL:-}" ]; then
  SANDBOX_MYSQL="mysql --protocol=socket -uroot -p${MYSQL_ROOT_PASSWORD}"
fi
case "$SANDBOX_ADMIN_PASSWORD" in
  *[\'\"\\]*) echo "SANDBOX_ADMIN_PASSWORD must not contain quotes or backslashes" >&2; exit 1 ;;
esac

$SANDBOX_MYSQL <<SQL
CREATE USER 'fleetdock_admin'@'%' IDENTIFIED BY '${SANDBOX_ADMIN_PASSWORD}';
GRANT CREATE USER, RELOAD ON *.* TO 'fleetdock_admin'@'%';
SQL

for db in $DBS; do
  echo "sandbox: seeding $db"
  $SANDBOX_MYSQL -e "CREATE DATABASE \`$db\`; GRANT ALL PRIVILEGES ON \`$db\`.* TO 'fleetdock_admin'@'%' WITH GRANT OPTION;"
  $SANDBOX_MYSQL "$db" < "$SEED_DIR/$db.sql"
done
echo "sandbox: ready ($DBS)"
