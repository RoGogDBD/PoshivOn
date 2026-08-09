# Task 12 — Test Audit (access-control, Phase 1 / Tasks 1–9)

**Auditor:** test-master (hostile read-through)
**Date:** 2026-08-09
**Commit audited:** `f300c07` (main, working tree clean except `work/access-control/tasks/10-12.md`)
**Baseline:** `work/access-control/tech-spec.md` → "Testing Strategy" (Unit / Integration / Client / E2E / CI)
**Method:** full read of all 7 test files from scratch (5 866 lines), bullet-by-bullet against tech-spec, litmus test applied per test ("if the core logic line is deleted, does this test still pass?"). `go test ./...` run once as supporting evidence only.

---

## 1. Overall verdict

**PASS with 5 minor findings. No load-bearing test is missing, and no load-bearing test is weak.**

All five of the load-bearing checks named in `tasks/12.md` are present in code, assert the behaviour tech-spec demands (state and side effects, not just status codes or "no error"), and **actually executed** in this audit run — not skipped. Every bullet in tech-spec's Testing Strategy maps to a real test with a discriminating assertion. The negative-path discipline tech-spec asks for ("флаг не изменился") is implemented as a reusable helper (`assertAccessUnchanged`, `assertUntouched`) and is applied consistently across the rejection branches.

The findings below are 5 minor issues: two tests whose assertions are weaker than their comments claim, one genuinely tautological test (redundant with a real one that exists elsewhere), one indirection in the `RowsAffected` test, and one small uncovered route/status pair. None of them hides a bug; none of them warrants a follow-up fix task on its own, though items G-1 and G-3 are worth a 10-minute cleanup if anyone touches those files.

### Supporting evidence — `go test ./...`

```
docker run --rm --network host -v .../server:/app -w /app -v poshivon_gomodcache:/go/pkg/mod \
  -e TEST_DB_DSN="poshivon:poshivon@tcp(127.0.0.1:3306)/poshivon_test?..." golang:1.25-alpine \
  go test ./... -v
```

| Package | Result |
|---|---|
| `cmd` | ok 0.007s |
| `internal/auth` | ok 0.027s |
| `internal/config` | ok 0.004s |
| `internal/handler` | ok 0.031s |
| `internal/repository` | ok 0.175s |
| `internal/service` | ok 0.004s |
| `internal/db` | no test files |

- **384 test cases (129 top-level, rest subtests) — 384 PASS, 0 FAIL, 0 SKIP.**
- **No environment caveat applies.** `grep SKIP` on the verbose log returns nothing. Every `postgres` subtree of the repository contract ran (`--- PASS: TestAccessRepoContract_*/postgres` × 18), and all 3 `internal/auth` store tests ran against live MariaDB. The DB half of the contract was genuinely exercised, not silently no-op'd.
- The only two `t.Skip` sites in the whole server tree are `store_test.go:46` and `access_repo_test.go:62`, both guarded exactly by `TEST_DB_DSN` as tech-spec intends. There is no disabled or commented-out test anywhere.
- CI backstop verified: `.github/workflows/test.yml` runs a `mariadb:11.4` service container, applies migrations, and passes `TEST_DB_DSN` to `go test ./... -v` (line 95–96). The "silently skipped in CI" failure mode tech-spec warns about is closed.

### Pyramid balance

| Level | What | Count (top-level) | Assessment |
|---|---|---|---|
| Unit (stubs) | `service/access_test.go`, middleware-in-isolation half of `middleware_test.go`, pure functions in `auth_test.go` | ~40 | Healthy |
| Integration (route, real `BuildRoutes`) | `access_test.go`, route half of `middleware_test.go`, `http_test.go` | ~30 | Healthy |
| Contract (dual-store) | `repository/access_repo_test.go` × {memory, postgres} | 18 × 2 | Healthy |
| DB-backed (store) | `auth/store_test.go` | 3 | Healthy |
| E2E | none — declared out of scope by tech-spec | 0 | Correctly scoped |

The two-level structure tech-spec prescribes (rules by unit test, *application of rules to routes* by integration test through the real `main.go` wiring) is implemented literally: `newRouteFixture` calls `BuildRoutes(RouteDeps{...})` — the same function `main.go` calls — rather than reconstructing the chain. This is the single most important structural property of the suite and it holds. The rationale is even documented in the fixture comment (`access_test.go:20-34`): Task 4 shipped middleware that was written, tested, and wired to nothing, and no test noticed.

Mock discipline: only two stub types exist in the handler package (`stubUserRepo`, `stubRequestRepo`) plus `stubSessionStore` and `fixtureResolver`. The `AccessService` under test is always the **real** one, and the costing store in route tests is the **real** `MemoryRepository`. No test asserts "a mock was called" without also asserting the resulting state. No test exceeds 3 mocked dependencies.

---

## 2. Per-file findings

### 2.1 `server/internal/service/access_test.go` (741 lines) — Unit tests on stubs

| Tech-spec bullet | Status | Note |
|---|---|---|
| Пользователь без доступа и без заявки: `has_access=false`, `request_status=""` | **present** | `TestAccessService_NoAccessNoRequest_ReturnsEmptyState` — also asserts Login/DisplayName/Email/Role carry through |
| `admin` + `has_access=false` считается имеющим доступ (US-14) | **present** | `TestAccessService_AdminWithoutAccessFlag_HasAccess` |
| Создание заявки пользователем без доступа → `pending` | **present** | `..._CreateRequest_ByUserWithoutAccess_CreatesPending`; asserts stored row status **and** that `DecideRequest` was not called |
| `CreateRequest` → `ErrConflict` при нарушении ключа | **present** | `..._CreateRequest_RepoZeroRowsAffected_ReturnsErrConflict`; asserts the stored request was **not mutated** by the rejected attempt |
| Заявка от пользователя с доступом → `ErrConflict`, обращения к репозиторию нет | **present** | `..._UserAlreadyHasAccess_ReturnsErrConflictWithoutRepoCall`; both routes to "has access" (flag, admin role) as subtests; asserts `createCalls == 0` **and** `len(requests) == 0` |
| Новая заявка после `rejected` разрешена | **present** | `..._CreateRequest_AfterRejected_Allowed`; asserts prior `DecidedBy`/`DecidedAt` survive |
| `SetAccess(true)` → флаг + `approved` + `decided_by` | **present** | `..._Grant_SetsFlagAndApprovesRequestWithDecidedBy` |
| `SetAccess(false)` → снимает флаг + `rejected` | **present** | `..._Revoke_UnsetsFlagAndRejectsRequest`; uses a *different* admin login (`second-admin`) so a hardcoded `decided_by` would fail |
| `SetAccess` для несуществующего логина → `ErrNotFound` | **present** | `..._SetAccess_UnknownLogin_ReturnsErrNotFound`; asserts neither repo was touched |

**Beyond the bullet list (all justified, none redundant):** repository-failure propagation for `ListUsers` and `CreateRequest` (an infra failure must not be reported as a conflict — a real class of bug); `SetAccess` partial-failure ordering in both directions (flag write fails → request untouched; decide fails → flag already written, documented as recorded-behaviour-not-ideal); a 15-case `ErrInvalidArgument` table including a Cyrillic login that is over the column width in *characters* but under it in *bytes*; the inclusive boundary at exactly `maxLoginLength`.

**Litmus:** all pass. The stubs implement real semantics (map-backed state) rather than returning canned values, so `TestAccessService_SetAccess_Grant...` fails if `SetAccess` stops calling the repo, and `..._UserAlreadyHasAccess_...` fails if the early-return branch is deleted. There is no mock-returns-X-assert-X pattern anywhere in this file.

**Anti-pattern check:** `t.Parallel()` used throughout with per-test stub construction — no shared state.

---

### 2.2 `server/internal/repository/access_repo_test.go` (1 054 lines) — Contract tests, dual store

| Tech-spec bullet | Status | Note |
|---|---|---|
| `EnsureUser` создаёт `role='user'`, `has_access=false` | **present** | `contractEnsureUserCreatesUser` — also asserts empty `RequestStatus`, nil `RequestedAt`, non-zero `CreatedAt` |
| **`EnsureUser` не сбрасывает `admin`/`has_access`** (Decision 11) | **present** | See §3.1 |
| `EnsureUser` обновляет `email`/`display_name` | **present** | `contractEnsureUserUpdatesProfileFields`; also asserts `CreatedAt` is **not** rewritten |
| `GetUser` возвращает `role`/`has_access` непустыми (ловит потерянный gorm-тег) | **present** | `contractGetUserReturnsRoleAndAccess`; explicit `Role == ""` assertion, exactly the lost-tag signature |
| `GetUser` для несуществующего → `ErrNotFound` | **present** | |
| `SetAccess` в обе стороны, отдельными тестами | **present** | See §3.2 |
| `SetAccess` для несуществующего → `ErrNotFound` | **present** | |
| `ListUsers` возвращает всех, включая созданных только входом | **present** | `contractListUsersIncludesLoginOnlyUsers`; also asserts `request_status`/`requested_at` arrive via the join |
| `CreateRequest` дважды при `pending` → `ErrConflict`, по факту `RowsAffected()==0` | **present** (split) | See §3.3 and finding G-3 |
| `DecideRequest` сохраняет `decided_by`/`decided_at` | **present** | |
| `DecideRequest` без строки заявки — не ошибка, `SetAccess` такой вызов не делает | **present** (split) | Repo half in `contractDecideRequestNoRowIsNotError` (also asserts no request row is conjured); service half in `service/access_test.go:..._NoExistingRequest_DoesNotCallDecideRequest` |
| Повторный `CreateRequest` после `rejected`: `RowsAffected()!=0`, статус `pending`, прошлое решение сохранено | **present** | See §3.3 |
| Существующие пути записи не сломаны (Decision 13) | **present, exceeded** | `..._ExistingWritePathsSurviveAccessColumns` (DB-only, needs the live CHECK constraint) covers `UpsertSettings`, `CreateChat` **and** `AppendCalculation`; plus `..._WritePathsCreateUserRow` and `..._WritePathsPreserveAdminRoleAndAccess` run the same three paths across both stores |

**Beyond the bullet list:** `..._SetAccessRepeatedGrantIsNotNotFound` and `..._SetAccessRevokeWhenAlreadyRevoked` — the naive `RowsAffected == 0 → ErrNotFound` mapping would turn a second admin click into a 404; both directions covered. `..._LoginIdentityIsCaseAndSpaceSensitive` — a privilege-escalation regression (pre-migration-005, `rogogdbd` resolved to the admin row `RoGogDBD`), asserted three ways: lookalike must be `ErrNotFound`, must not inherit access, and must not overwrite the owner's profile.

**Structural quality:** the dual-store harness is real, not decorative — `runAgainstBothStores` runs the identical contract function through a factory, `seedAdmin` deliberately bypasses the interface (there is no role-setting method, by design), and `TestMain` purges fixtures by `LOWER(id) LIKE` (correct after 005's binary collation made plain `LIKE` case-sensitive). Logins are unique per run via nanosecond + atomic counter, so the shared, non-recreated schema cannot cross-contaminate.

**Litmus:** all pass. Every state assertion reads back through `GetUser`/`GetRequest` rather than trusting the write call's return.

---

### 2.3 `server/internal/auth/store_test.go` (240 lines) — Session store, DB-only

| Tech-spec bullet | Status | Note |
|---|---|---|
| `CreateSession` сохраняет `yandex_login`/`_email`/`_display_name` | **present** | `TestAuthStore_CreateSessionPersistsIdentity`; reads back via `FindByRefreshHash`, asserts all three via `assertIdentity` (checks `.Valid` **and** `.String`) |
| **`UpdateSessionTokens` сохраняет личность при ротации** | **present** | `TestAuthStore_UpdateSessionTokensPreservesIdentity`; rotates to a *new* hash, re-finds by the new hash, asserts same session ID, updated access token, **and** all three identity fields intact |
| Сессия с `NULL` `yandex_login` читается без ошибки, поля пустые | **present** | `TestAuthStore_NullYandexLoginRoundTrips`; inserts a pre-migration row with raw SQL (the only way to produce it), asserts `!Valid && String == ""` on all three |

All three ran (not skipped) in this audit. `cleanupSession` uses `t.Cleanup` per test, so the shared schema stays clean even on failure.

**Litmus:** all pass. The rotation test is the one that matters and it is correctly built as a *separate* test rather than an extra assertion on the create test — a fresh-login path would never exercise `UPDATE`, which is exactly where the identity columns would be dropped from the statement.

---

### 2.4 `server/internal/handler/auth_test.go` (878 lines) — `POST /auth/yandex/code`

| Tech-spec bullet | Status | Note |
|---|---|---|
| Успешный вход создаёт строку `users` через `EnsureUser` **до** любого действия в панели | **present, strong** | `TestHandleYandexCode_CreatesUserBeforeAnyPanelAction`; asserts exact `EnsureUser` arguments, **ordering** via `callLog.indexOf` (`EnsureUser` before `CreateSession`), the `Authorization: OAuth <token>` header seen by the fake Yandex, exactly one profile fetch, all three identity fields on the created session, and both cookies set |
| Без `Origin` при непустом allowlist → 403, сессия не создаётся | **present** | See §3.4 |
| Посторонний `Origin` → 403, сессия не создаётся | **present** | See §3.4 |

**Beyond the bullet list:** allowed-Origin positive control; profile-fetch failure → 502, no session, no `EnsureUser`, no cookies; empty login in profile → same; `EnsureUser` failure → 500, no session, no cookies, **and no `3306`/`connection refused` in the body**; `CreateSession` failure after a successful `EnsureUser` → 500, exactly one `EnsureUser`, no cookies, no `1146`/`oauth_sessions` in the body; missing `default_email` is not an error and lands as SQL `NULL` not `""`; the refresh-cookie/hash inversion check (`session.RefreshTokenHash != refreshCookie` and `== HashRefreshToken(refreshCookie)`); the name fallback chain; `ResolveSession` decoupled from `*AuthHandler` (enforced compile-time via a package-level `var _ SessionResolver = ...`); the 7 rejection slugs.

The fake Yandex is an `httptest.NewServer` with `/token` and `/info` routed through config URLs — the seam tech-spec names.

**Litmus:** all pass except finding G-2 (`TestAuthHandlerHasNoLegacyTokenEntryPoint`), detailed below.

---

### 2.5 `server/internal/handler/access_test.go` (1 153 lines) — `/api/v1/access/*`, `/api/v1/admin/*`

| Tech-spec bullet | Status | Note |
|---|---|---|
| `GET /access/me` без кук → 401 | **present** | Asserts slug `access_cookie_missing`, not just 401 |
| `GET /access/me` с сессией без `yandex_login` → 401 `session_identity_missing` | **present** | |
| `GET /access/me` без доступа → 200, `has_access=false` | **present** | Decodes the body by **JSON field names** (wire contract) and asserts login/role/request_status/email/contact_email |
| `POST /access/requests` дважды → 201, затем 409 | **present** | Asserts slug `conflict`, that the request stayed `pending` (not overwritten), and that no access was granted |
| `GET /admin/users` без кук → 401; неадминистратором → 403 | **present** | Both assert the response body does **not** contain the user list — a leak check on the rejection path |
| `GET /admin/users` администратором → 200, список содержит всех | **present** | Full membership check + per-user `has_access`/`role`/`request_status`, including that the admin's **raw** flag stays `false` (not replaced by the effective right) |
| `RequireAccess` пропускает `admin` + `has_access=false` (US-14) на реально навешенном middleware | **present** | See `middleware_test.go` §2.6 |
| `POST /admin/users/{other}/access` неадминистратором → 403 **и флаг не изменился** | **present** | `assertAccessUnchanged` |
| `POST /admin/users/{self}/access` неадминистратором → 403 **и флаг не изменился** (US-13) | **present** | `assertAccessUnchanged` + explicit self-check |
| Тот же запрос без кук → 401 **и флаг не изменился** | **present** | With a *valid* Origin, so it proves `RequireAuth` rejects — not `RequireSameOrigin` |
| `POST /admin/users/{unknown}/access` администратором → 404 **и ни один флаг не изменился** | **present** | Also asserts the unknown user was not conjured into the store |
| `granted=true` → 204, флаг выставлен; `granted=false` → 204, флаг снят | **present** | Also asserts 204 has an empty body, request status `approved`→`rejected`, and `decided_by` = the **session** login |
| Тело с неизвестным полем → 400 | **present, exceeded** | 4 malformed-body shapes + `{}`/`{"granted":null}` (the zero-value-means-revoke trap) + entirely absent body; every case asserts the parser's text does **not** leak and the flags are unchanged |
| **Origin, все 5 ветвей Decision 8**, каждая с проверкой «флаг не изменился» | **present** | See §3.4 |
| Ошибка репозитория с SQL-текстом → 500 без текста; то же для 400/404/429/503/504 | **present** | See §3.5 |

**Beyond the bullet list:** `TestUsersRoutesAreClosedByAccessChain` (the four-state gate check on `/api/v1/users/chats`, with a real data assertion on the 200 branch so "route passed" cannot be confused with "route 404'd"); `TestAuthMutatingRoutesRequireSameOrigin` (all three mutating `/auth/*` routes × {no Origin, foreign Origin}, plus GET `/auth/status`, `/auth/me` explicitly asserted **not** 403); `/health` and `/metrics` stay open; `TestAccessHandler_RepositoryFailureAfterGatePasses` — a genuinely sharp test that distinguishes "the gate itself failed" from "the handler failed after the gate passed" via `getErrByLogin`, without which the error handling inside `ListUsers`/`SetAccess`/`CreateRequest` would be covered by nothing.

**Litmus:** all pass except finding G-1 (`TestAccessMe_ContactEmailComesFromConfig`, empty-string case).

---

### 2.6 `server/internal/handler/middleware_test.go` (986 lines) — `RequireAuth`/`RequireAccess`/`RequireAdmin`/`RequireSameOrigin`

Covers both levels tech-spec asks for: middleware in isolation (a fake `next` that records call count *and* the identity placed in context), then the same invariants on routes assembled by the production `BuildRoutes`.

| Area | Status | Note |
|---|---|---|
| `RequireAuth` без кук / без refresh | **present** | Distinct slugs per case; `next.calls == 0` asserted every time |
| `RequireAuth` сессия без личности → `session_identity_missing` | **present** | 3 variants: SQL `NULL`, `""`, whitespace-only — all fail closed |
| `RequireAuth` кладёт личность в контекст | **present** | Asserts the full `Identity` struct, not merely that `next` ran |
| `RequireAuth` слаги отказа неизменны | **present** | All 6 client-contract slugs |
| `RequireAuth` неизвестная ошибка резолвера не течёт | **present** | Asserts `session_not_found` slug **and** absence of `3306`/`connection refused` |
| `session_identity_missing` не входит в retry-список клиента | **present** | Prevents a client refresh loop — a real, non-obvious failure mode |
| **`RequireAccess` пропускает admin с `has_access=false` (US-14)** | **present ×2** | Isolated (`..._AdminBypassesHasAccessFalse`) **and** on real routes (`..._AdminWithoutFlagPasses`, which additionally asserts the response contains the admin's **own** chat, so "passed the gate" ≠ "answered anything"). Tech-spec explicitly asks for both levels — this is not redundancy |
| `RequireAccess` gate table (with/without access/unknown login) | **present** | + `forbidden` slug on the 403s |
| `RequireAccess`/`RequireAdmin` без личности fail closed → 401 | **present** | Guards against a mis-ordered chain being a hole rather than a degradation |
| `RequireAccess` при сбое хранилища → 500, не пропуск, без утечки | **present** | |
| `RequireAdmin` gate table | **present** | Including "has access but no role" → 403 |
| `RequireSameOrigin` — empty allowlist, allowlist, safe methods, mutating methods, full matrix on real routes | **present** | See §3.4 |

**Litmus:** all pass. The `recordingHandler` counts calls *and* captures context, so "middleware rejected" and "middleware passed but the next handler happened to error" are never conflated.

---

### 2.7 `server/internal/handler/http_test.go` (814 lines) — product routes after the session-identity migration

| Tech-spec bullet | Status | Note |
|---|---|---|
| `/users/chats`, `/users/settings`, `/users/chats/{id}/calculate` без кук → 401 на каждом | **present** | Asserts slug + that Petrov's secret chat title does not appear in the rejection body + `assertUntouched` on his data afterwards |
| Каждый из них пользователем без доступа → 403 (US-16) | **present, strong** | Asserts `forbidden` slug, that Petrov's data is untouched, **and** that nothing appeared in Ivanov's own space — a "write first, reject later" implementation would differ only in that second check |
| Пользователем с доступом → 200 с данными **его** владельца, для каждого маршрута | **present** | All 7 route shapes: POST/GET `/settings`, POST/GET `/chats`, POST `/calculate`, GET `/calculations`, DELETE `/chats/{id}`, POST `/restore`, POST `/market-feedback` (503-not-404, proving the address parsed). Asserts `user_id` in the body, not just the status. One weak assertion here — finding G-4 |
| Данные другого владельца недостижимы (US-15) | **present, strong** | 6 sub-cases: foreign login in query on settings (18 vs 99 owner marker — genuinely discriminating), in query on chats, in body on create (400 via `DisallowUnknownFields`, so the channel doesn't exist rather than being ignored), foreign chat ID on delete (404 + untouched), on reading calculations (empty), and on writing a calculation (lands in the caller's own space, victim untouched) |
| `POST /auth/yandex` → 404 (US-17) | **present** | Via `BuildRoutes` in `access_test.go:TestBuildRoutes_LegacyYandexTokenRouteIsGone`. Also over-covered by `auth_test.go` — finding G-2 |
| `writeAPIDomainError` — все ветки без утечки | **present** | See §3.5 |

**Beyond the bullet list:** `TestRegisterRoutes_LegacyUserIDSegment_NotFound` — 10 old-form addresses, each asserting 404 **with the `route not found` slug** (so the extra segment isn't silently parsed into another `switch` branch), no leaked data in the body, and Petrov untouched.

**Structural quality:** preconditions and post-checks go straight into `fixture.costing` (the real `MemoryRepository`), never through the handler under test. This is the correct discipline — a consistently-wrong implementation would otherwise look consistently right.

---

## 3. The five load-bearing checks

### 3.1 Admin-demotion regression (Decision 11) — **PASS, exceeded**

`access_repo_test.go:200-231`, `TestAccessRepoContract_EnsureUserPreservesAdminRoleAndAccess`.

Sequence: `EnsureUser` (first login) → `seedAdmin` (sets `role='admin'`, `has_access=TRUE` out-of-band, since no interface method may set a role) → `EnsureUser` (second login) → `GetUser` and assert **both** fields independently:

```go
if record.Role != service.RoleAdmin { t.Errorf("... повторный вход разжаловал администратора") }
if !record.HasAccess              { t.Errorf("... повторный вход снял флаг доступа") }
```

This is a read-back assertion on two separate fields, not an error check. It runs against **both** `MemoryRepository` and `PostgresRepository` (`--- PASS: .../postgres` confirmed in the run log).

**Exceeded:** `TestAccessRepoContract_WritePathsPreserveAdminRoleAndAccess` extends the same invariant to the three *existing* write paths (`UpsertSettings`, `CreateChat`, `AppendCalculation`), each of which also touches the `users` row. Tech-spec does not require this. It closes the same Decision-11 hole through a different door — saving settings or running one calculation would otherwise demote the admin. This is the strongest single test in the suite.

Additionally, `service/access_test.go:316` (`..._EnsureUser_DelegatesWithoutResettingRoleAndAccess`) pins the service layer: arguments pass through verbatim and the service itself does not attempt to rewrite role or flag.

**Litmus:** if `EnsureUser`'s conflict clause were changed to overwrite `role`/`has_access`, this test fails on both stores. Confirmed non-tautological.

### 3.2 `SetAccess(granted=false)` DB-tested — **PASS, and it genuinely ran**

Both directions are separate contract functions, not two asserts in one test — deliberately, per the in-file comment (`access_repo_test.go:351-353`): a write path that can only set `true` looks entirely healthy in the granting direction.

- `contractSetAccessGrant` (line 326): `EnsureUser` → `SetAccess(true)` → `GetUser` → `!record.HasAccess` fails.
- `contractSetAccessRevoke` (line 354): `EnsureUser` → `SetAccess(true)` → `SetAccess(false)` → `GetUser` → `record.HasAccess != false` fails. The intermediate `true` matters: revoking from an already-`false` row would be a no-op and would pass against a broken revoke path.

**Ran against the real repository — not skipped.** Verbatim from the run log:

```
--- PASS: TestAccessRepoContract_SetAccessRevoke (0.01s)
    --- PASS: TestAccessRepoContract_SetAccessRevoke/memory (0.00s)
    --- PASS: TestAccessRepoContract_SetAccessRevoke/postgres (0.01s)
```

**No environment caveat.** `TEST_DB_DSN` was set, MariaDB answered, the `postgres` subtest executed and passed. The Go zero-value hazard tech-spec names (an unset `bool` in a struct literal that "works" against a map but never writes `false` through SQL) is genuinely covered. Reinforced by `contractSetAccessRevokeWhenAlreadyRevoked`, which pins the `RowsAffected == 0` no-op case so it is not mistaken for `ErrNotFound`.

### 3.3 `CreateRequest` upsert semantics via `RowsAffected` — **PASS, with one indirection (G-3)**

Coverage is split across two tests, and between them all three required cases are checked **by count**, not only by error:

`TestAccessRepoContract_CreateRequestRowsAffectedSemantics` (line 759, DB-only — MySQL's affected-row counting is the whole point, so a memory run would be meaningless):

| Case | Assertion | Tech-spec asks for |
|---|---|---|
| first insert | `insert.RowsAffected != 1` → fail | 1+ |
| duplicate while `pending` | `repeat.RowsAffected != 0` → fail | `== 0` |
| re-request after `rejected` | `resubmit.RowsAffected != 2` → fail | `!= 0` |

(2 is MariaDB's count for a row genuinely changed by `ON DUPLICATE KEY UPDATE`; the test is stricter than the spec, which is correct — it also proves the DSN does not carry `clientFoundRows=true`, which would silently turn the 0 into a 1 and break the `ErrConflict` mapping.)

`contractCreateRequestConflictWhilePending` (line 593, both stores) covers the **error** half: second `CreateRequest` → `errors.Is(err, ErrConflict)`, plus a full read-back of the untouched request (`pending`, original `CreatedAt`, `DecidedAt == nil`, `DecidedBy == ""`) and of the user row's joined `RequestStatus`.

`contractCreateRequestAfterRejectedSucceeds` (line 716, both stores) covers the resubmission: status returns to `pending`, and `DecidedBy` / `DecidedAt` are compared **against the values captured before** the resubmit (`resubmitted.DecidedAt.Equal(*rejected.DecidedAt)`) — not against a hardcoded literal. Prior-decision preservation is genuinely asserted.

**One indirection (finding G-3):** the `RowsAffected` test executes `createRequestSQL` directly via `db.Exec` rather than calling `requests.CreateRequest`. The file mitigates this correctly by importing the **production constant** rather than copying the SQL (`upsert := createRequestSQL`, with a comment explaining exactly why). So a divergence between test and production SQL is impossible. What remains unasserted at the `RowsAffected` level is the *mapping* `RowsAffected == 0 → ErrConflict` inside `CreateRequest` — but that mapping is covered against the live Postgres repo by `contractCreateRequestConflictWhilePending/postgres`. Combined coverage is complete; the split is a readability cost, not a hole.

### 3.4 Origin-check branch matrix (Decision 8) — **PASS, all five branches, each with a state re-check**

`middleware_test.go:839`, `TestRequireSameOrigin_Matrix`, on routes built by the production `BuildRoutes`, driving `POST /api/v1/admin/users/ivanov/access` as an admin so that a wrongly-permitted request would visibly flip a flag:

| # | Tech-spec branch | Test case | Expected | Flag re-checked |
|---|---|---|---|---|
| 1 | no `Origin` → 403 | "без заголовка Origin при непустом списке" | 403 | `assertAccessUnchanged` |
| 1b | no `Origin`, empty allowlist → 403 | "без заголовка Origin при пустом списке" | 403 | `assertAccessUnchanged` |
| 2 | foreign `Origin` + non-empty allowlist → 403 | "посторонний Origin при непустом списке" | 403 | `assertAccessUnchanged` |
| 3 | `Origin == r.Host` + **empty** allowlist → passes | "Origin совпадает с r.Host при пустом списке — проходит (same-origin fallback)" | 204 | asserts flag **was** set |
| 4 | foreign `Origin` + empty allowlist → **still 403** | "посторонний Origin при пустом списке — всё равно 403, а не пропуск" | 403 | `assertAccessUnchanged` |
| 5 | `GET` with missing/foreign `Origin` → not blocked | `TestRequireSameOrigin_MatrixSafeMethod` (2 cases) | 200 | `assertAccessUnchanged` |

Plus two positive/negative controls not in the spec list: allowed `Origin` from a non-empty allowlist → 204 with the flag actually set (so the 403 cases are not passing because *everything* 403s); and scheme mismatch (`http://` origin when `cookieSecure=true`) → 403.

The critical property tech-spec calls out — "Каждый случай проверяется по факту: флаг доступа не изменился" — is implemented on **every** rejected branch via `assertAccessUnchanged(before)`, which diffs a full snapshot of every user's flag and also catches users appearing or disappearing. The pass branches assert the inverse (`hasAccess("ivanov")` is true), so a fixture that silently rejects everything would fail.

**Branch 3 is the one that silently flipped direction during the tech-spec revision, and it is directly and explicitly asserted** — both at route level (case 3 above) and in isolation (`TestRequireSameOrigin_EmptyAllowlistFallsBackToHost`, 5 sub-cases including the https-in-prod variant).

**Login-CSRF variant on `POST /auth/yandex/code`:** covered in `auth_test.go` by `TestHandleYandexCode_MissingOriginRejected` and `TestHandleYandexCode_ForeignOriginRejected`. Both assert far more than the 403: **no session created**, **no `EnsureUser` call**, **zero token exchanges with the fake Yandex** (`fake.tokenCalls.Load() != 0`), and **no cookies set**. The "rejected before any side effect" property is asserted four ways. `TestHandleYandexCode_AllowedOriginPasses` is the positive control. Additionally, `access_test.go:TestAuthMutatingRoutesRequireSameOrigin` re-checks all three mutating `/auth/*` routes through the real `BuildRoutes` wiring, and asserts GET `/auth/status`/`/auth/me` are **not** 403.

Also covered beyond the spec: `null` Origin → 403; `PUT`/`PATCH`/`DELETE` are in the mutating set, not just `POST`; `HEAD` is exempt alongside `GET`.

### 3.5 No internal error text leaks across ALL branches (Decision 17) — **PASS, all eight**

`http_test.go:34-85` defines `domainErrorCases()` with an explicit comment that it enumerates **all** branches, not only the new ones. Every case wraps the same realistic payload (`internalDetail` = SQL text + table name + a DSN fragment with credentials and port).

| Branch | Case | Fixed body asserted | Leak assertion |
|---|---|---|---|
| 400 | `ErrInvalidArgument` | `invalid_request` | ✓ |
| 403 | `ErrForbidden` | `forbidden` | ✓ |
| 404 | `ErrNotFound` | `not_found` | ✓ |
| 409 | `ErrConflict` | `conflict` | ✓ |
| 429 | `deepseek: rate_limit_exceeded` | `rate_limited` | ✓ |
| 500 | unclassified | `internal_error` | ✓ |
| 503 | `deepseek: service_unavailable` | `service_unavailable` | ✓ |
| 504 | `deepseek: timeout while waiting` | `timeout` | ✓ |

`TestWriteAPIDomainError_NoInternalTextInAnyBranch` asserts, per branch: the status code, the exact fixed slug, `Content-Type: application/json`, and the absence of **seven** distinct fragments — `SELECT`, `users`, `RoGogDBD`, `dsn=`, `poshivon`, `deepseek`, `get user`. **The pre-existing branches this feature did not touch (400/404/429/503/504) are covered exactly as thoroughly as the new ones (403/409).** No gap here.

Two complements that make this more than a unit test:

- `TestWriteAPIDomainError_LogsOriginalError` — same 8 branches, asserts the removed text **is** in the server log. Without this, "no leak" could be satisfied by discarding the error entirely, destroying 500-diagnosability.
- `TestWriteAPIDomainError_NilErrorDoesNotPanic` — the text-matching branches call `err.Error()`; on `nil` that is a panic instead of a response.

Route-level confirmation that the handlers actually route through this function rather than writing errors directly: `access_test.go:TestWriteAPIDomainError_NoInternalTextLeak` (5 routes, unconditional repo failure) and `TestAccessHandler_RepositoryFailureAfterGatePasses` (4 targeted failures *after* the gate passes — otherwise the gate would reject first and the handler's own error path would be untested). Leak checks also appear on the 400-parser path (`assertInvalidRequest`, screening for `unknown field`, `json:`, `cannot unmarshal`, `EOF`), on the 401 resolver path (`..._UnknownResolverErrorIsNotLeaked`), and on two login-failure paths in `auth_test.go`.

---

## 4. Deliberate exclusions — both correctly scoped, neither is a gap

**Client tests: none — CONFIRMED CORRECT.**
`client/package.json` has scripts `{dev, build, preview}` — no `test` script — and devDependencies `{@tailwindcss/vite, @vitejs/plugin-react, tailwindcss, vite}` — no test runner, no assertion library, no jsdom. A filesystem sweep of `client/` (excluding `node_modules`) for `*.test.*`, `*.spec.*`, `__tests__` returns **nothing**. So no client test file is silently missing; there is no infrastructure for one to exist in.
Reviewer routing verified in frontmatter: `tasks/7.md` → `reviewers: [code-reviewer, security-auditor]`; `tasks/8.md` → `reviewers: [code-reviewer, security-auditor]`. Neither lists `test-reviewer`, exactly as tech-spec's "Client tests" subsection prescribes ("в клиентских задачах `test-reviewer` заменён на `code-reviewer` — рецензировать нечего"). Correctly scoped.

**`server/internal/service/notifier_test.go` — CONFIRMED OUT OF SCOPE.**
The file does not exist (`ls` confirms). Tech-spec marks the whole notifier subsection **(Phase 2)** and `tasks/12.md` explicitly instructs not to flag it. Phase 1 ships no notifier, so there is nothing to test. **Not counted as a gap.** For the record, when Task 16 lands, the tech-spec bullet requiring `net/mail.ReadMessage` round-tripping (rather than substring checks) and the CRLF-header-injection case will need to be audited then — substring assertions there would be a real finding.

**E2E tests: none** — tech-spec declares no runner and no CI job exists; covered by the Agent Verification Plan via Playwright MCP. Correctly scoped.

---

## 5. Concrete gaps

Five minor findings. None blocks; none corresponds to a load-bearing check; all are described precisely enough to scope a fix without re-reading the files.

### G-1 (minor, anti-pattern / tautological sub-case) — the empty-`CONTACT_EMAIL` case tests nothing

**File:** `server/internal/handler/access_test.go:349-375`, `TestAccessMe_ContactEmailComesFromConfig`, third loop iteration (`contactEmail: ""`).

**Problem:** the fixture substitutes its own default whenever the option is empty (`access_test.go:83-86`: `if contactEmail == "" { contactEmail = fixtureContactEmail }`), and the test then computes `want = fixtureContactEmail` for that same case. The empty value never reaches `NewAccessHandler`. The sub-case asserts that the fixture's default equals the fixture's default. Its comment claims the opposite — "Пустое значение CONTACT_EMAIL — штатная конфигурация по умолчанию, и оно тоже обязано доехать как есть, а не подмениться дефолтом фикстуры" — so the file currently reads as if empty `CONTACT_EMAIL` pass-through were covered when it is not. Empty `CONTACT_EMAIL` is the **default production configuration**, so this is the one value most likely to ship.

**Litmus:** if `NewAccessHandler` substituted a hardcoded fallback for an empty contact email, this test would still pass.

**Fix:** give `fixtureOptions` an explicit way to express "empty on purpose" — e.g. change `contactEmail string` to `contactEmail *string` (nil → fixture default, `&""` → pass empty through), or add a `contactEmailIsEmpty bool` flag. Then assert `payload.ContactEmail == ""` for that case. Alternatively drop the `""` iteration and its misleading comment; the two non-empty values already prove the value is not hardcoded.

### G-2 (minor, tautological + redundant) — `TestAuthHandlerHasNoLegacyTokenEntryPoint` cannot fail

**File:** `server/internal/handler/auth_test.go:857-874`.

**Problem:** the test constructs its **own** `http.ServeMux`, registers 5 handlers on it by hand, then asserts that an unregistered 6th path (`POST /auth/yandex`) returns 404. It asserts a property of the test's own mux, not of production routing. Re-adding `mux.Handle("/auth/yandex", ...)` to `routes.go` would not fail this test. Deleting `routes.go` entirely would not fail this test.

**Litmus:** fails the litmus test outright — there is no production line whose removal breaks it.

**Mitigation (why this is minor, not major):** the real assertion exists and is correct — `access_test.go:996`, `TestBuildRoutes_LegacyYandexTokenRouteIsGone`, goes through the production `BuildRoutes` and asserts 404. Coverage of US-17/Decision 7 is intact; this is a redundant test that provides false additional confidence.

**Fix:** delete `TestAuthHandlerHasNoLegacyTokenEntryPoint`. Its stated intent ("рассинхрон между удалённым методом и живой регистрацией ловит `go build ./...`") is already true and needs no test. If a handler-package-local assertion is wanted, replace it with a compile-time one in the style already used in this file, e.g. asserting the absence of the method is not expressible in Go — so deletion is the right call.

### G-3 (minor, indirection) — `RowsAffected` semantics are asserted on raw SQL, not through `CreateRequest`

**File:** `server/internal/repository/access_repo_test.go:759-802`, `TestAccessRepoContract_CreateRequestRowsAffectedSemantics`.

**Problem:** the test runs `createRequestSQL` via `db.Exec` directly. The chain "`RowsAffected == 0` ⇒ `ErrConflict`" inside `PostgresRepository.CreateRequest` (`postgres.go:762`) is therefore asserted only indirectly — a refactor that kept the SQL but broke the count→error mapping would be caught by `contractCreateRequestConflictWhilePending/postgres` (which does check `ErrConflict` against the live DB), so nothing is actually uncovered. But the *numbers* and the *mapping* are proved in two different tests, and a reader could reasonably conclude the mapping is unverified.

**Mitigation:** the test imports the production constant rather than copying the SQL, with an explicit comment about why — the worst version of this pattern (a drifting copy of the query) is already prevented.

**Fix (optional, cosmetic):** add one assertion to `contractCreateRequestAfterRejectedSucceeds`, or a short comment in `TestAccessRepoContract_CreateRequestRowsAffectedSemantics` pointing at `contractCreateRequestConflictWhilePending` as the test that closes the count→error mapping. No behavioural coverage is missing.

### G-4 (minor, weak assertion) — `18` is the default `LaborMinuteRate`, not an owner marker

**File:** `server/internal/handler/http_test.go:430-439`, `TestHandleUsers_WithAccess_200_OwnerScoped` → sub-test `"GET /settings 200"`.

**Problem:** this fixture is built **without** `seedSettings`; the preceding sub-test POSTs `service.DefaultUserSettings()`. The assertion `payload.PricingRules.LaborMinuteRate != 18` then checks against `costing.go:214`, where `DefaultUserSettings()` sets exactly `18`. The comment "ожидался 18 (настройки Петрова)" implies owner-discrimination, but `18` would also come back from any other owner's freshly-defaulted settings, or from a `GetSettings` that ignored the owner entirely and returned defaults on miss.

**Litmus:** if the handler resolved the owner from somewhere other than the session in this sub-test, the assertion would still pass (the fixture has only one user, and defaults are identical).

**Mitigation (why this is minor):** the owner-discriminating assertion **does** exist and is correct — `TestHandleUsers_ForeignOwnerUnreachable` seeds `18` for Petrov against `99` for Ivanov and asserts `99` is returned to Ivanov. US-15 coverage is intact. The weakness is confined to this one sub-test's claim about itself.

**Fix:** in the `"POST /settings 204"` sub-test, POST settings with a distinctive `LaborMinuteRate` (e.g. `37`) instead of raw `DefaultUserSettings()`, then assert `37` on the GET. That makes the round-trip assertion discriminating and removes the accidental coincidence with the default, at the cost of one line. Also correct the comment.

### G-5 (minor, coverage) — `POST /api/v1/access/requests` has no un-authenticated 401 case

**File:** `server/internal/handler/access_test.go`, `TestAccessRequests_*` group.

**Problem:** every `POST /api/v1/access/requests` test supplies `as: <login>`. The tech-spec API-contract table lists `401` as a valid response for this endpoint, but no test drives it without cookies. `GET /api/v1/access/me` has its 401 case (`TestAccessMe_NoCookies401`) and the route shares the `/api/v1/access/` `RequireAuth` prefix, so the branch is covered transitively — but the "request access without logging in" case is exactly the one a route-registration mistake (registering `/api/v1/access/requests` outside the guarded prefix) would break, and no test would notice.

Note this is **not** a tech-spec Testing-Strategy bullet — the bullet list only names `/access/me` for the 401 cases. It is a gap against the API contract table, not against the tested baseline.

**Fix:** add one case to `TestAccessRequests_*`:

```go
recorder := fixture.do(apiRequest{method: http.MethodPost, path: "/api/v1/access/requests"}) // no `as`
// assert 401, slug access_cookie_missing, and fixture.requestStatus("ivanov") == "" — no request row created
```

---

## 6. Summary table

| Load-bearing check | Verdict | Ran in this audit |
|---|---|---|
| 1. Admin-demotion regression (`EnsureUser`, Decision 11) | **PASS** — both fields asserted after read-back, both stores; extended to all 3 existing write paths | yes (memory + postgres) |
| 2. `SetAccess(granted=false)` DB-tested | **PASS** — separate test per direction, revoke preceded by a real grant | yes (postgres, **not** skipped) |
| 3. `CreateRequest` `RowsAffected` semantics | **PASS** — 1 / 0 / 2 all asserted by count on the production SQL constant; prior decision compared to captured values | yes (DB-only test ran) |
| 4. Origin matrix (Decision 8) | **PASS** — all 5 branches + 2 controls, every rejection re-checks the flag; login-CSRF variant asserts no session, no `EnsureUser`, no token exchange, no cookies | yes |
| 5. No error-text leak, all 8 branches | **PASS** — 400/403/404/409/429/500/503/504 each asserted for fixed slug + 7 leak fragments, plus a log-retention counterpart | yes |

| Exclusion | Verdict |
|---|---|
| No client tests | Correctly scoped — no runner in `client/package.json`, no test files exist, `tasks/7.md`/`8.md` use `code-reviewer` + `security-auditor`, not `test-reviewer` |
| `notifier_test.go` (Phase 2 / Task 16) | Correctly out of scope — not a gap |

**Gap count: 5, all minor.** No follow-up fix task is required. If one is opened opportunistically, G-1 (empty `CONTACT_EMAIL` untested) and G-2 (delete the tautological test) are the two worth doing; G-3, G-4 and G-5 are cosmetic or transitively covered.
