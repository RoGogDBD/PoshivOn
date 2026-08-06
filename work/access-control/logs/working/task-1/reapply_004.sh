#!/usr/bin/env bash
# Idempotency check for 004_access_control.
#
# Re-applies the very same migration file by hand through the mariadb client and
# asserts BOTH that the client exited cleanly AND that the resulting state is
# unchanged. Asserting state alone is not enough: run 1 already committed the
# admin rows, so a second run that errored halfway would still leave the admin
# count at 2 and look like success.
#
# Usage: reapply_004.sh [db_name]

set -u
DB_NAME="${1:-poshivon}"
MIGRATION="$(cd "$(dirname "$0")/../../../../.." && pwd)/server/migrations/004_access_control.up.sql"

echo "=== re-applying $MIGRATION into '$DB_NAME' ==="
out=$(docker exec -i poshivon_db mariadb -uposhivon -pposhivon "$DB_NAME" < "$MIGRATION" 2>&1)
rc=$?
echo "client exit code: $rc"
echo "client output: '${out:-<empty>}'"

fails=0
if [ "$rc" -ne 0 ]; then
  echo "FAIL  re-apply exit code is 0"
  fails=$((fails + 1))
else
  echo "PASS  re-apply exit code is 0"
fi

if echo "$out" | grep -qi "error"; then
  echo "FAIL  re-apply produced no ERROR output"
  fails=$((fails + 1))
else
  echo "PASS  re-apply produced no ERROR output"
fi

echo
echo "=== state after re-apply (must be identical) ==="
bash "$(dirname "$0")/verify_004.sh" "$DB_NAME" "${2:-1}"
fails=$((fails + $?))

echo "----"
[ "$fails" -eq 0 ] && echo "IDEMPOTENCY OK ($DB_NAME)" || echo "IDEMPOTENCY FAILED: $fails ($DB_NAME)"
exit "$fails"
