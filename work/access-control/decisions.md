# Decisions Log: Access Control

Agent reports on completed tasks. Each entry is written by the agent that executed the task.

---

## Task 1: Миграция схемы доступа

**Status:** Done
**Commit:** 77b5cb4 (migration), f7e35b4 + ec5b31c (verification harness per review)
**Agent:** migration-writer
**Summary:** Added `server/migrations/004_access_control.up.sql` — `role`/`has_access`/`email`/`display_name` on `users`, Yandex identity columns on `oauth_sessions`, the `access_requests` table, the `(role, has_access)` index, and seeding of the two administrators. The SQL is byte-identical to the tech-spec Data Models block; every statement is idempotent (`IF NOT EXISTS` plus `INSERT ... ON DUPLICATE KEY UPDATE`), so a manual re-run neither fails nor duplicates admin rows. Because this is pure SQL with no Go test runtime, the TDD anchor is an executable check harness kept in `logs/working/task-1/`, written before the migration and confirmed to fail on the pre-004 schema.

**Deviations:** None in the migration itself. One addition beyond the task's "only the migration file changes": three verification scripts were committed under `logs/working/task-1/` (`verify_004.sh`, `reapply_004.sh`, `scenario_b.sh`) at the test-reviewer's request, so the checks behind the acceptance criteria are reproducible rather than existing only as pasted terminal output. They live in the feature work directory, not in `server/`, so the migration remains byte-identical to the spec for anyone diffing it.

**Reviews:**

*Design round 1 (before the SQL was finalised):*
- test-reviewer (design): 5 major + 1 minor, all fixed → [logs/working/task-1/test-reviewer-design-1.json](logs/working/task-1/test-reviewer-design-1.json)

*Code round 1:*
- code-reviewer: 3 minor, non-blocking → [logs/working/task-1/code-reviewer-1.json](logs/working/task-1/code-reviewer-1.json)
- security-auditor: no blockers; 1 low (new) + 1 medium (carried over from the tech-spec round) → [logs/working/task-1/security-auditor-1.json](logs/working/task-1/security-auditor-1.json)
- test-reviewer (full): PASS, 2 minor harness fixes applied → [logs/working/task-1/test-reviewer-1.json](logs/working/task-1/test-reviewer-1.json)

**Notes carried forward:**
- **For Task 4 (`EnsureUser`) — collation.** `users.id` has no explicit `COLLATE` and inherits the database default `utf8mb4_uca1400_ai_ci`, which is case- and accent-insensitive. Admin identity therefore depends on an ambient server setting rather than an explicit one, and Decision 11 has no normalisation step. Inherited from migration 002, benign for the two current logins (verified live: `'rogogdbd'` collides with the seeded row instead of creating a second admin). Raised by the security auditor as a tech-spec/task-level decision, deliberately not patched inside this migration.
- **Admin state asymmetry.** `ON DUPLICATE KEY UPDATE role = 'admin'` intentionally does not touch `has_access`, so an administrator who had already logged in keeps `has_access=0` while a freshly inserted one gets `1`. Both pass the gate, since access is `has_access || role=='admin'` (Decision 10).
- **Pattern caveat.** `ADD CONSTRAINT chk_users_role` validates every existing row at `ALTER` time, unlike a plain `ADD COLUMN`. Trivial at current data volume; worth remembering before reusing this pattern on a large table.
- **Public repo exposure — resolved, accepted.** `github.com/RoGogDBD/PoshivOn` is public (confirmed via the GitHub API: `private: false`), so the two seeded admin Yandex logins are permanently and publicly visible in git history, irreversibly. Not an auth bypass (a login is not a credential), but it publishes a confirmed two-account target list for phishing. Raised by the security auditor as low severity; surfaced to the user directly (outside this task's own review flow, since the finding only reached decisions.md after Task 1 had already closed). **User decision: accept as-is.** Documented as an explicit trade-off in tech-spec.md's Risks table rather than left as an unweighed byproduct of Decision 4. No code or migration change; repo visibility is a GitHub setting, out of this feature's scope.
- Checked and holding, no action: no SQL injection surface (static file, no interpolation, confirmed by full read); `role` and `status` are `NOT NULL`, so the CHECK-vs-NULL concern cannot arise — verified live that `role=NULL` fails with 1048 before any CHECK runs, and an out-of-enum value fails with 4025; FK and `ON DELETE CASCADE` verified live (orphan insert rejected 1452, cascade delete leaves no orphans); idempotency additionally confirmed under a simulated mid-file crash (first two `ALTER`s applied, then the whole file re-run) — no error, no duplication.

**Verification:**
- `verify_004.sh` — 22 schema assertions (column types/nullability/defaults, CHECK clause text, tightly scoped FK + `ON DELETE CASCADE`, PK, index column order, admin cardinality and case-sensitive identity) → all pass. Confirmed failing on the pre-004 schema first.
- Scenario A, clean volume, `docker compose up -d db app` → no `Ошибка применения миграций`; `schema_migrations` contains `004_access_control`; exactly 2 admins.
- Scenario B, production redeploy (`001`-`003` pre-recorded with live user rows, real `migrate.go` applying `004` alone) → all assertions pass; pre-existing rows backfill to `role='user'`, `has_access=0`, never NULL; `created_at` preserved.
- `reapply_004.sh` — manual re-run of the same file: client exit 0, no `ERROR` output, state unchanged, admin count still 2, no duplicated schema objects.
