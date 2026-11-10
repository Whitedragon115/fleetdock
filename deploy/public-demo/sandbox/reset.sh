#!/bin/bash
# Keeps the public demo's sandbox in shape (runs in its own container):
#   - every 5 seconds, ends visitors' statements running longer than
#     DEMO_MAX_QUERY_SECONDS. This matters on MySQL: its own timeout covers
#     SELECT only, and a statement keeps running after Fleetdock gives up on
#     it, so this is what stops long UPDATEs, procedures and the like;
#   - every DEMO_RESET_INTERVAL_SECONDS, drops each demo database and loads
#     the sample data again, throwing away whatever visitors changed.
#     Fleetdock's console accounts keep their grants (MySQL keeps
#     database-level grants when a database is dropped).
set -u
DBS="${DEMO_DATABASES:-shop analytics billing}"
INTERVAL="${DEMO_RESET_INTERVAL_SECONDS:-3600}"
MAXQ="${DEMO_MAX_QUERY_SECONDS:-45}"
SEED_DIR="${SEED_DIR:-/sandbox/seed}"
# Credentials come from MYSQL_HOST / MYSQL_PWD in the environment.
MYSQL="${SANDBOX_MYSQL:-mysql -uroot}"
ts() { date -u +%FT%TZ; }

reset_db() {
  db="$1"
  if $MYSQL -e "DROP DATABASE IF EXISTS \`$db\`; CREATE DATABASE \`$db\`;" && $MYSQL "$db" < "$SEED_DIR/$db.sql"; then
    echo "$(ts) reset $db"
  else
    echo "$(ts) reset $db FAILED"
  fi
}

until $MYSQL -e "SELECT 1" >/dev/null 2>&1; do sleep 2; done
echo "$(ts) watching the sandbox (reset every ${INTERVAL}s, statements limited to ${MAXQ}s)"
last=$(date +%s)
while true; do
  for id in $($MYSQL -N -e "SELECT ID FROM information_schema.PROCESSLIST
      WHERE USER LIKE 'fleetdock\\_r_\\_%' AND COMMAND NOT IN ('Sleep', 'Killed') AND TIME > $MAXQ" 2>/dev/null); do
    $MYSQL -e "KILL CONNECTION $id" 2>/dev/null && echo "$(ts) ended connection $id (statement over ${MAXQ}s)"
  done
  now=$(date +%s)
  if [ $((now - last)) -ge "$INTERVAL" ]; then
    for db in $DBS; do reset_db "$db"; done
    last=$now
  fi
  sleep 5
done
