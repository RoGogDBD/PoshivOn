#!/usr/bin/env bash
# Scenario B — production redeploy, as opposed to the clean-volume Scenario A.
#
# Scenario A (plain `docker compose up` on a fresh volume) runs 001..004 in one
# pass against an empty users table. That is NOT what production will do. There,
# 001+002+003 are already recorded in schema_migrations and users already holds
# real rows, so migrate.go skips the first three and applies 004 alone on top of
# live data. This script builds that state explicitly and runs the real app
# binary against it.
#
# Pre-existing rows seeded on purpose:
#   legacy_user — an ordinary account that logged in before the migration; must
#                 backfill to role='user', has_access=0, and never to NULL.
#   RoGogDBD    — an admin who had already logged in, so his users row exists;
#                 exercises the ON DUPLICATE KEY UPDATE branch (role changes,
#                 has_access deliberately untouched).
#
# Usage: scenario_b.sh

set -u
DB_NAME=poshivon_b
HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/../../../../.." && pwd)"
MIG="$REPO/server/migrations"

root() { docker exec -i poshivon_db mariadb -uroot -proot "$@"; }
q() { docker exec poshivon_db mariadb -uposhivon -pposhivon "$DB_NAME" -N -B -e "$1" 2>/dev/null; }

echo "=== 1. fresh database $DB_NAME ==="
root -e "DROP DATABASE IF EXISTS $DB_NAME; CREATE DATABASE $DB_NAME; GRANT ALL ON $DB_NAME.* TO 'poshivon'@'%';"

echo "=== 2. apply 001+002+003 only (no 004) ==="
for f in 001_auth_sessions 002_costing_schema 003_pricing_and_chat_delete; do
  docker exec -i poshivon_db mariadb -uposhivon -pposhivon "$DB_NAME" < "$MIG/$f.up.sql" || exit 1
  echo "applied $f"
done

echo "=== 3. record them in schema_migrations, exactly as migrate.go would ==="
q "CREATE TABLE IF NOT EXISTS schema_migrations (version VARCHAR(255) PRIMARY KEY, applied_at TIMESTAMP NOT NULL);
   INSERT INTO schema_migrations (version, applied_at) VALUES
     ('001_auth_sessions', NOW()), ('002_costing_schema', NOW()), ('003_pricing_and_chat_delete', NOW());"
echo "recorded: $(q "SELECT GROUP_CONCAT(version ORDER BY version) FROM schema_migrations;")"

echo "=== 4. seed pre-existing user rows ==="
q "INSERT INTO users (id) VALUES ('legacy_user'), ('RoGogDBD');"
echo "pre-existing users: $(q "SELECT GROUP_CONCAT(id ORDER BY id) FROM users;")"

echo "=== 5. run the real app against $DB_NAME (migrate.go applies 004 alone) ==="
docker rm -f poshivon_app_scen_b >/dev/null 2>&1
docker run -d --name poshivon_app_scen_b --network poshivon_default \
  -e DB_HOST=db -e DB_PORT=3306 -e DB_NAME="$DB_NAME" \
  -e DB_USER=poshivon -e DB_PASSWORD=poshivon \
  -e APP_HOST=0.0.0.0 -e APP_PORT=8080 poshivon-app >/dev/null || exit 1
sleep 10
echo "--- app log ---"
docker logs poshivon_app_scen_b 2>&1 | tail -10
echo "--- migration error check ---"
if docker logs poshivon_app_scen_b 2>&1 | grep -q "Ошибка применения миграций"; then
  echo "FAIL  app logged a migration error"
  exit 1
fi
echo "PASS  no 'Ошибка применения миграций' in app log"

echo "=== 6. only 004 was newly applied ==="
echo "schema_migrations now: $(q "SELECT GROUP_CONCAT(version ORDER BY version) FROM schema_migrations;")"

echo
echo "=== 7. standard schema assertions ==="
bash "$HERE/verify_004.sh" "$DB_NAME" 1
rc=$?

echo
echo "=== 8. scenario-B specific: backfill of pre-existing rows ==="
fails=0
sc() {
  local name="$1" expected="$2" actual="$3"
  if [ "$actual" = "$expected" ]; then echo "PASS  $name"
  else echo "FAIL  $name (expected '$expected', got '$actual')"; fails=$((fails + 1)); fi
}
# NOT NULL + DEFAULT must backfill the existing row, not leave it NULL.
sc "legacy_user backfilled to role='user', has_access=0" "user|0" \
  "$(q "SELECT CONCAT(role,'|',has_access) FROM users WHERE id='legacy_user';")"
sc "legacy_user role IS NOT NULL" "0" \
  "$(q "SELECT COUNT(*) FROM users WHERE role IS NULL OR has_access IS NULL;")"
# ON DUPLICATE KEY UPDATE promotes the pre-existing row's role and, by design,
# leaves has_access alone (access is has_access || role=='admin' — Decision 10).
sc "pre-existing RoGogDBD promoted to admin, has_access untouched" "admin|0" \
  "$(q "SELECT CONCAT(role,'|',has_access) FROM users WHERE id='RoGogDBD';")"
sc "RoGogDBD row not duplicated" "1" \
  "$(q "SELECT COUNT(*) FROM users WHERE id='RoGogDBD';")"
sc "created_at of pre-existing row preserved (row not recreated)" "1" \
  "$(q "SELECT COUNT(*) FROM users WHERE id='RoGogDBD' AND created_at < NOW();")"

echo
echo "=== 9. idempotency on the scenario-B database ==="
bash "$HERE/reapply_004.sh" "$DB_NAME" 1
fails=$((fails + $? + rc))

echo "----"
[ "$fails" -eq 0 ] && echo "SCENARIO B PASSED" || echo "SCENARIO B FAILED: $fails"
docker rm -f poshivon_app_scen_b >/dev/null 2>&1
exit "$fails"
