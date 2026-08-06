#!/usr/bin/env bash
# Verification harness for Task 1 — migration 004_access_control.
#
# This migration is pure SQL, so its TDD anchor is this executable check script
# rather than a Go test file. It was written BEFORE the migration existed and was
# confirmed to fail on the pre-004 schema (001+002+003), then to pass after it.
#
# Usage: verify_004.sh [db_name] [expect_migration_row]
#   db_name              database to inspect (default: poshivon)
#   expect_migration_row 1 = require the 004_access_control row in schema_migrations
#                        (default 1; pass 0 when the file was applied by hand via
#                        the mariadb client, which does not touch schema_migrations)
#
# Exit code = number of failed assertions (0 = all pass).

set -u

DB_NAME="${1:-poshivon}"
EXPECT_MIGRATION_ROW="${2:-1}"

q() { docker exec poshivon_db mariadb -uposhivon -pposhivon "$DB_NAME" -N -B -e "$1" 2>/dev/null; }

fails=0
check() {
  local name="$1" expected="$2" actual="$3"
  if [ "$actual" = "$expected" ]; then
    echo "PASS  $name"
  else
    echo "FAIL  $name"
    echo "        expected: '$expected'"
    echo "        actual:   '$actual'"
    fails=$((fails + 1))
  fi
}

echo "=== schema assertions on database '$DB_NAME' ==="

# --- 1. users: full column definitions, not mere presence -------------------
# Type/Null/Default all matter. In particular `role` MUST be NOT NULL: a NULL
# role would make `role IN ('user','admin')` evaluate to UNKNOWN, which a CHECK
# constraint treats as satisfied — so a nullable role silently defeats
# chk_users_role. Same reasoning for has_access, which gates every request.
# NB: information_schema quotes string defaults ('user'), unlike SHOW COLUMNS (user).
check "users.role definition" "varchar(16)|NO|'user'" \
  "$(q "SELECT CONCAT(column_type,'|',is_nullable,'|',IFNULL(column_default,'<none>')) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='users' AND column_name='role';")"

check "users.has_access definition" "tinyint(1)|NO|0" \
  "$(q "SELECT CONCAT(column_type,'|',is_nullable,'|',IFNULL(column_default,'<none>')) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='users' AND column_name='has_access';")"

check "users.email definition" "varchar(255)|YES" \
  "$(q "SELECT CONCAT(column_type,'|',is_nullable) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='users' AND column_name='email';")"

check "users.display_name definition" "varchar(255)|YES" \
  "$(q "SELECT CONCAT(column_type,'|',is_nullable) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='users' AND column_name='display_name';")"

# --- 2. oauth_sessions identity columns -------------------------------------
for col in yandex_login yandex_email yandex_display_name; do
  check "oauth_sessions.$col definition" "varchar(255)|YES" \
    "$(q "SELECT CONCAT(column_type,'|',is_nullable) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='oauth_sessions' AND column_name='$col';")"
done

# --- 3. access_requests table ------------------------------------------------
check "access_requests table exists" "1" \
  "$(q "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='access_requests';")"

check "access_requests column definitions" \
  "user_id:varchar(255):NO,status:varchar(16):NO,created_at:timestamp:NO,decided_at:timestamp:YES,decided_by:varchar(255):YES" \
  "$(q "SELECT GROUP_CONCAT(CONCAT(column_name,':',column_type,':',is_nullable) ORDER BY ordinal_position) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='access_requests';")"

check "access_requests.status default" "'pending'" \
  "$(q "SELECT IFNULL(column_default,'<none>') FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='access_requests' AND column_name='status';")"

check "access_requests PK = user_id" "user_id" \
  "$(q "SELECT GROUP_CONCAT(column_name ORDER BY seq_in_index) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='access_requests' AND index_name='PRIMARY';")"

# --- 4. FK scoped tightly ----------------------------------------------------
# user_settings, chats and calculations already carry an identically shaped
# FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE. Every predicate
# below (schema + table + constraint name, on BOTH sides of the join) is load
# bearing: without them this assertion could be satisfied by one of those three
# pre-existing FKs while our own FK is missing or wrong.
check "access_requests FK -> users(id) ON DELETE CASCADE" "user_id->users.id:CASCADE" \
  "$(q "SELECT CONCAT(k.column_name,'->',k.referenced_table_name,'.',k.referenced_column_name,':',r.delete_rule)
        FROM information_schema.key_column_usage k
        JOIN information_schema.referential_constraints r
          ON  r.constraint_schema = k.constraint_schema
          AND r.constraint_name   = k.constraint_name
          AND r.table_name        = k.table_name
        WHERE k.constraint_schema = DATABASE()
          AND k.table_name        = 'access_requests'
          AND k.constraint_name   = 'fk_access_requests_user';")"

# --- 5. CHECK constraints: assert the clause text, not just the name ---------
check "chk_users_role clause" "\`role\` in ('user','admin')" \
  "$(q "SELECT check_clause FROM information_schema.check_constraints WHERE constraint_schema=DATABASE() AND table_name='users' AND constraint_name='chk_users_role';")"

check "chk_access_requests_status clause" "\`status\` in ('pending','approved','rejected')" \
  "$(q "SELECT check_clause FROM information_schema.check_constraints WHERE constraint_schema=DATABASE() AND table_name='access_requests' AND constraint_name='chk_access_requests_status';")"

# --- 6. index, including column ORDER ---------------------------------------
# (role, has_access) — order matters for the admin-listing query plan.
check "idx_users_role_has_access column order" "1:role,2:has_access" \
  "$(q "SELECT GROUP_CONCAT(CONCAT(seq_in_index,':',column_name) ORDER BY seq_in_index) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='users' AND index_name='idx_users_role_has_access';")"

# --- 7. migration recorded ---------------------------------------------------
if [ "$EXPECT_MIGRATION_ROW" = "1" ]; then
  check "schema_migrations has 004_access_control" "1" \
    "$(q "SELECT COUNT(*) FROM schema_migrations WHERE version='004_access_control';")"
fi

# --- 8. admin seed: cardinality AND case-sensitive identity ------------------
# Two separate assertions on purpose. COUNT alone would accept two wrong logins;
# a set check alone would accept extra rows. The identity check uses BINARY
# because the default utf8mb4 collation is case-insensitive, so a seeded
# 'rogogdbd' would wrongly satisfy a plain `=` comparison against 'RoGogDBD'.
check "COUNT(admins) = 2" "2" \
  "$(q "SELECT COUNT(*) FROM users WHERE role='admin';")"

check "admin logins are exactly the two expected (case-sensitive)" "2" \
  "$(q "SELECT COUNT(*) FROM users WHERE role='admin' AND BINARY id IN (BINARY 'RoGogDBD', BINARY 'irina2000aleshina');")"

check "no admin row with unexpected casing" "0" \
  "$(q "SELECT COUNT(*) FROM users WHERE role='admin' AND BINARY id NOT IN (BINARY 'RoGogDBD', BINARY 'irina2000aleshina');")"

# --- 9. no duplicated schema objects (guards a non-idempotent re-apply) ------
# A re-apply that silently created a second index/constraint would show up here
# even though the admin row count stayed at 2.
check "exactly one idx_users_role_has_access-like index" "1" \
  "$(q "SELECT COUNT(DISTINCT index_name) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='users' AND index_name LIKE 'idx_users_role_has_access%';")"

check "exactly one chk_users_role-like constraint" "1" \
  "$(q "SELECT COUNT(*) FROM information_schema.check_constraints WHERE constraint_schema=DATABASE() AND table_name='users' AND constraint_name LIKE 'chk_users_role%';")"

check "exactly one fk_access_requests_user-like FK" "1" \
  "$(q "SELECT COUNT(*) FROM information_schema.referential_constraints WHERE constraint_schema=DATABASE() AND table_name='access_requests' AND constraint_name LIKE 'fk_access_requests_user%';")"

echo "----"
if [ "$fails" -eq 0 ]; then
  echo "ALL CHECKS PASSED ($DB_NAME)"
else
  echo "$fails CHECK(S) FAILED ($DB_NAME)"
fi
exit "$fails"
