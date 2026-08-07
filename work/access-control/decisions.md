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

---

## Task 2: Доменный сервис доступа

**Status:** Done
**Commit:** 4191929 (service + tests), 804e010 + 718626b (review fixes)
**Agent:** access-service-writer
**Summary:** Added `server/internal/service/access.go` — the domain types (`Role`, `AccessState`, `UserRecord`, `AccessRequest`), the `UserRepository`/`AccessRequestRepository` interfaces verbatim from the tech-spec Data Models block, and `AccessService` with `EnsureUser`, `GetAccessState`, `CreateRequest`, `SetAccess`; `ErrForbidden` and `ErrConflict` joined the existing domain errors in `costing.go`. Decision 10 lives in exactly one unexported helper, `hasEffectiveAccess`, which both `GetAccessState` and `CreateRequest` go through, so nothing anywhere checks the raw `has_access` alone. The repository signals "zero rows affected" (Decision 5) by returning `ErrConflict` from `CreateRequest` — the signature is fixed by the spec, so the contract had to travel through the error, and the service wraps with `%w` so `errors.Is` survives.

**Deviations:** None from the task's "What to do". Three additions beyond the literal list, each from a review finding: `requireLogin` bounds the login by `users.id`'s column width (255, counted in runes so the boundary matches MariaDB's reading of `VARCHAR(255)`), `SetAccess` carries a doc comment stating it performs no authorization check of its own, and the test suite grew past the 10 TDD-Anchor cases to 20 top-level tests (36 leaf assertions incl. subtests) covering invalid arguments, infrastructure-failure paths, and the partial-failure ordering.

**Reviews:**

*Design round 1 (tests written, no implementation yet):*
- test-reviewer (design): 2 minor + 1 low + 1 informational, all addressed → [logs/working/task-2/test-reviewer-design-1.json](logs/working/task-2/test-reviewer-design-1.json)

*Code round 1:*
- code-reviewer: 3 minor, non-blocking → [logs/working/task-2/code-reviewer-1.json](logs/working/task-2/code-reviewer-1.json)
- security-auditor: 1 medium + 3 low + 2 verified-clean → [logs/working/task-2/security-auditor-1.json](logs/working/task-2/security-auditor-1.json)
- test-reviewer (full): PASS, findings array empty → [logs/working/task-2/test-reviewer-1.json](logs/working/task-2/test-reviewer-1.json)

**Notes carried forward:**
- **For Task 3 — the `SetAccess` snapshot race.** `SetAccess` decides whether to call `DecideRequest` from the `RequestStatus` it already read in `GetUser`, not from a fresh read. A second read would not close the gap, because nothing at this layer is transactional, so it would only add a query. Two concrete consequences when a `CreateRequest` interleaves: a request created after the snapshot stays `pending` forever (access has since been granted, so the next `CreateRequest` short-circuits on `ErrConflict` and never overwrites it), and in the revoke direction a request the user submits right after the flag is cleared can be stamped `approved`/`rejected` with this admin's `decided_by` though nobody reviewed it. `has_access` itself is never wrong — the flag is written with exactly the value the admin asked for — so this costs only the request row and the accuracy of the `decided_by` audit trail (Decision 5). The real fix is one transaction in the repository, which is Task 3's layer; whether a dangling `pending` row in the admin list is worth that is a product call nobody has made yet.
- **For Task 3 — no rollback across the two writes.** `SetAccess` writes the flag and then decides the request as two separate calls. A failure of the second surfaces an error to the caller with `has_access` already changed and not rolled back. This is now pinned by a test as deliberate current behaviour, not left implicit; making the pair atomic is again a repository-layer question.
- **For Task 4 — consume the rule, do not re-derive it.** `hasEffectiveAccess` is unexported on purpose: `GetAccessState(...).HasAccess` is the intended entry point and already returns the effective flag. Middleware that re-writes `has_access || role == 'admin'` in the handler package would put a second copy of Decision 10 in the tree and defeat the reason this task exists. Note that `AccessState.HasAccess` is the *effective* value while `UserRecord.HasAccess` stays the raw column — the handler is serialising the effective one, which keeps the tech-spec's client-side `has_access || role=='admin'` correct either way.
- **For Task 4 — `AccessService` has no `ListUsers`.** The repository interface has one (fixed by Data Models) but the service does not, since this task's method list did not include it. The admin-list endpoint will need one added to `AccessService` rather than reaching past it to the repository.
- **For Task 4/5 — two declared-but-unused members.** `ErrForbidden` is declared per spec and consumed by the middleware later; `AccessRequestRepository.GetRequest` may turn out genuinely unused, since neither `AccessState` nor `UserRecord` carries `DecidedBy`/`DecidedAt` and the admin-list description does not ask for decision metadata. Both are fixed by the spec, so neither was removed here — worth confirming they get used or recording them as reserved.
- Checked and holding, no action: no SQL or injection surface (the file has zero direct DB access, interfaces only); no user-enumeration or timing oracle (the login always comes from the caller's own session or an admin-only path); every error is wrapped with a fixed, non-parameterised prefix, so no repository detail reaches the caller through the message; `EnsureUser` cannot reset `role`/`has_access` structurally, since its signature has no such parameters (Decision 11).

**Verification:**
- Go is not installed on this host. Everything ran in the pre-existing `golang:1.25-alpine` image with `--network host` (the default bridge cannot reach `proxy.golang.org`) and a persistent module/build cache. Both the code-reviewer and the test-reviewer independently reproduced the suite the same way.
- `go build ./...` → clean, exit 0. `go vet ./...` → clean. `gofmt -l .` → no output.
- `go test ./internal/service/... -count=1` → `ok github.com/RoGogDBD/PoshivOn/internal/service`. All 20 `TestAccessService_*` top-level cases pass (36 leaf results with subtests); the 5 pre-existing `TestCostingService_*` are untouched and still pass.
- TDD order was real: the tests were written first and confirmed red against the missing implementation (build failed on undefined `UserRecord`/`RoleUser`/`AccessRequest`) before `access.go` existed.
