# Code Research: Доступ к панели по заявке с одобрением администратором

Feature path: `work/access-control`
Researched: 2026-08-05
Project root: `/home/makar/PoshivOn`

Everything below is what **exists today**. No design proposals — only facts and the constraints
they impose.

---

## 0. Corrections to the orchestrator's summary

| Claim | Verdict |
|---|---|
| Auth is Yandex OAuth, sessions in `oauth_sessions` keyed by sha256 of refresh cookie | **Correct** — `server/migrations/001_auth_sessions.up.sql:1-11`, `server/internal/auth/store.go:31-34` |
| Session row holds no user identity; identity fetched live from Yandex `/info` in `HandleMe` | **Correct** — `server/internal/auth/store.go:11-21` (no user column), `server/internal/handler/auth.go:197` |
| `validateSession` is the only session validation path, private method on `AuthHandler` | **Correct** — `server/internal/handler/auth.go:421-452`; called only from `HandleStatus` (auth.go:176) and `HandleMe` (auth.go:191). `HandleRefresh`/`HandleLogout` do their own cookie lookup (auth.go:213-224, 282-286) |
| `/api/v1/users/{userID}/*` takes userID from URL with no auth check | **Correct** — `server/internal/handler/http.go:26-70`. `APIHandler` has no reference to `auth.Store` or `AuthHandler` at all (http.go:13-20) |
| `users` table exists, id = Yandex login VARCHAR(255), rows created lazily by `upsertUser` | **Correct** — `server/migrations/002_costing_schema.up.sql:1-4`, `server/internal/repository/postgres.go:550-555` |
| Client routing pathname-based in `App.jsx`, `/panel` → `Panel.jsx`, `userID = profile?.login` | **Correct** — `client/src/App.jsx:34-35`, `client/src/pages/Panel.jsx:190` |
| **"React 19"** | **WRONG.** `client/package.json:12-13` declares `react ^18.3.1` / `react-dom ^18.3.1`; `package-lock.json` resolves both to **18.3.1**. Vite is 5.4.10 (`package.json:19`), Tailwind v4 via `@tailwindcss/vite` (`package.json:16-18`, `client/vite.config.js:6`) |
| **"MySQL/MariaDB"** | Ambiguous — it is **MariaDB 11.4 only**, and migration `003` already uses MariaDB-only syntax. See §2 |

Additional correction worth flagging: the dev `docker-compose.yml` does **not** set `APP_STORAGE`,
so `config.Load()` defaults it to `"memory"` (`server/internal/config/config.go:58`) and
`buildRepositories` returns `MemoryRepository` (`server/cmd/main.go:115-117`). **Dev `docker compose up`
runs the app against the in-memory repo while still connecting to and migrating MariaDB.**
Prod sets `APP_STORAGE=mysql` (`.github/workflows/deploy.yml:96`).

---

## 1. Migrations

### Mechanism

`server/migrations/migrate.go`, package `migrations`, single exported func `Run(db *sql.DB) error`.

- Files are embedded with `//go:embed *.up.sql` (migrate.go:13-14) — **compile-time**; a new file
  is picked up only by rebuilding the binary.
- Discovery: `fs.Glob(migrationsFS, "*.up.sql")` (migrate.go:21), then `sort.Strings(entries)` (migrate.go:25).
- Version key = filename minus `.up.sql` suffix: `extractVersion` (migrate.go:87-89). So version
  strings are literally `001_auth_sessions`, `002_costing_schema`, `003_pricing_and_chat_delete`.
- Ledger table `schema_migrations (version VARCHAR(255) PRIMARY KEY, applied_at TIMESTAMP)`,
  created with `CREATE TABLE IF NOT EXISTS` on every run (migrate.go:59-67).
- Each unapplied file is executed as **one** `db.Exec(string(sqlBytes))` (migrate.go:43) — the whole
  file in a single call. **Requires `multiStatements=true` in the DSN.**
- No transaction wraps a migration; no rollback on partial failure. `Run` returns an error and
  `main.go:32-34` calls `log.Fatalf`, so the container crash-loops until the DB state is fixed manually.
- **Down-migrations do not exist.** No `*.down.sql` file, no down path in `migrate.go`.
  Rolling back a migration is a manual DB operation.

### `0001_init.sql` is dead code

`server/migrations/0001_init.sql` does **not** match the glob `*.up.sql` (it ends in `_init.sql`),
so it is never embedded and never applied. Confirmed by inspection: its content is **PostgreSQL**
(`TIMESTAMPTZ`, `JSONB`, `BIGSERIAL`, `NOW()`, header comment `-- PostgreSQL 14+` at line 2) and
would fail against MariaDB if it ever ran. It is a leftover from an abandoned Postgres design.
Practical consequence: **`0001_init.sql` must be ignored entirely** — it is not the schema of record,
`002_costing_schema.up.sql` is.

### Naming convention for the next migration

Zero-padded 3-digit prefix + snake_case name + `.up.sql`:
**`server/migrations/004_<name>.up.sql`**

Ordering is plain lexicographic `sort.Strings` over the full filename, so 3-digit padding is
load-bearing: a file named `10_x.up.sql` would sort **before** `2_x.up.sql`. Staying at 3 digits
keeps ordering correct up to 999.

### DSN caveat that affects multi-statement migrations

`buildDSN` sets `multiStatements=true` (`server/internal/db/db.go:63`), but only when
`cfg.DatabaseURL` is empty (db.go:16-19). If anyone ever sets `DATABASE_URL`, that flag is lost and
every multi-statement migration file (i.e. all of them) fails. Prod passes `DB_HOST/DB_PORT/...`
and never `DATABASE_URL` (`docker-compose.prod.yml:22-26`, `.github/workflows/deploy.yml:253-257`),
so the fallback DSN path is what actually runs.

---

## 2. Database: which engine, which SQL dialect

**MariaDB 11.4 in both dev and prod.**

- dev: `docker-compose.yml:38` — `image: mariadb:11.4`, healthcheck uses `mariadb-admin` (line 50)
- prod: `docker-compose.prod.yml:56` — `image: mariadb:11.4`
- Driver: `github.com/go-sql-driver/mysql v1.8.1` (`server/go.mod:5`), GORM dialect
  `gorm.io/driver/mysql v1.6.0` (`go.mod:24`) — the MySQL wire protocol, which MariaDB speaks.
- `buildRepositories` accepts `"postgres" | "mysql" | "mariadb"` as aliases for the same GORM/MySQL
  path (`server/cmd/main.go:118-130`) — the string `postgres` there is a lie carried over from the
  abandoned design.

### What that means for DDL in a new migration

Verified from existing migrations (these constructs are known-good on this deployment):

| Construct | Status | Evidence |
|---|---|---|
| `VARCHAR(255) PRIMARY KEY` | works | `002:2` |
| `TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP` | works | `002:3` |
| `... ON UPDATE CURRENT_TIMESTAMP` | works | `002:11` |
| `JSON` column type | works | `002:8-10` |
| Composite `PRIMARY KEY (a, b)` | works | `002:22` |
| `FOREIGN KEY ... ON DELETE CASCADE` | works | `002:12-14, 23-24, 44-45` |
| `CONSTRAINT ... CHECK (...)` | works (MariaDB enforces CHECK since 10.2) | `002:46-53` |
| `BIGINT AUTO_INCREMENT PRIMARY KEY` | works | `002:28` |
| `BIGINT UNSIGNED ... AUTO_INCREMENT` | works | `001:2` |
| `CHAR(64) NOT NULL UNIQUE` | works | `001:3` |
| `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` | works — **MariaDB-only extension**, invalid on MySQL | `003:2-7` |
| `CREATE INDEX idx ON t(...)` (bare, no IF NOT EXISTS) | works, but **not idempotent** | `002:56-60` |

`ENUM('a','b')` is supported by MariaDB and is safe to use, though nothing in the codebase uses it
yet — status columns elsewhere are plain `VARCHAR` (`market_status VARCHAR(64)`, `003:16`).
Prefer `VARCHAR` + `CHECK` for consistency with the existing style.

**Constraints that will bite a "one pending request per user" rule:**

- MariaDB has **no partial / filtered indexes** (`UNIQUE ... WHERE status='pending'` is Postgres-only).
- MariaDB has **no functional indexes on expressions** (MySQL 8.0.13+ has them, MariaDB does not).
- The available workarounds are: (a) `UNIQUE KEY (user_id, status)` — which also forbids two
  historical rows with the same terminal status for one user; (b) a `VIRTUAL` generated column
  such as `IF(status='pending', user_id, NULL)` with a `UNIQUE` index on it — MariaDB does allow
  unique indexes on generated columns, but **this has not been verified against 11.4 on this
  project and should be tested before being specified**; (c) enforce uniqueness in the service layer
  with a `SELECT ... FOR UPDATE` inside a transaction. No MariaDB instance was available in this
  environment to verify (b) empirically.
- Note MariaDB spells stored generated columns `PERSISTENT` (it accepts `STORED` as a MySQL-compat
  synonym); `VIRTUAL` is the default.

**Idempotency:** migration `002` uses `CREATE TABLE IF NOT EXISTS` but bare `CREATE INDEX`
(002:56, 002:59). Since `Run` only applies a file once, this is fine in practice, but a re-run
against a DB whose `schema_migrations` row was lost would fail on the index.

---

## 3. Repository layer

### File map

- `server/internal/repository/postgres.go` (555 lines) — GORM-backed, **misnamed**: it targets
  MariaDB. Type `PostgresRepository` (postgres.go:89-95), constructor `NewPostgresRepository(db *gorm.DB)`.
- `server/internal/repository/memory.go` (242 lines) — `MemoryRepository`, `sync.RWMutex` + maps
  (memory.go:13-26). Mirrors every method of the GORM repo. Used when `APP_STORAGE=memory` (the dev default).
- `server/internal/repository/README.md` — states repositories must contain no business logic.

### Model conventions (postgres.go:15-87)

One unexported struct per table, all columns explicitly tagged, `TableName()` method on the value
receiver:

```go
type userModel struct {
    ID        string    `gorm:"column:id;primaryKey"`
    CreatedAt time.Time `gorm:"column:created_at"`
}
func (userModel) TableName() string { return "users" }
```

Nullable columns use `sql.NullString` (postgres.go:29-34) or `*time.Time` / `*string`
(postgres.go:48-49). Read-only projection structs (e.g. `chatListRow`, postgres.go:81-87) are
separate from write models.

### Interface compliance assertions

Both repos declare compile-time assertions right after the constructor:

- `postgres.go:97-99`: `var _ service.UserSettingsRepository = (*PostgresRepository)(nil)` (×3)
- `memory.go:28-30`: same three for `*MemoryRepository`

**Any new repository interface added to `service` must be implemented by both repos**, or
`buildRepositories` (`cmd/main.go:107-134`) cannot return it for the `memory` branch. This is the
single biggest structural cost of adding a users/access-requests repository.

### Interface shape in `service` (costing.go:168-183)

```go
type UserSettingsRepository interface {
    UpsertSettings(ctx context.Context, userID string, settings UserSettings) error
    GetSettings(ctx context.Context, userID string) (UserSettings, error)
}
type ChatRepository interface {
    CreateChat(ctx context.Context, chat Chat) (Chat, error)
    ListChats(ctx context.Context, userID string) ([]Chat, error)
    DeleteChat(ctx context.Context, userID, chatID, deletedBy string, hard bool) error
    RestoreChat(ctx context.Context, userID, chatID string) error
}
type ChatCalculationRepository interface {
    AppendCalculation(ctx context.Context, result CalculationResult) error
    ListCalculations(ctx context.Context, userID, chatID string) ([]CalculationResult, error)
}
```

House style: narrow, per-aggregate interfaces **defined in `service`, implemented in `repository`**
(consumer-side interfaces); `ctx context.Context` first; domain types (defined in `service`) in and
out; no GORM/`sql` types leak across the boundary.

### Upsert / conflict patterns

- Lazy user creation — `upsertUser` (postgres.go:550-555):
  ```go
  func upsertUser(tx *gorm.DB, userID string) error {
      return tx.Clauses(clause.OnConflict{
          Columns: []clause.Column{{Name: "id"}}, DoNothing: true,
      }).Create(&userModel{ID: userID}).Error
  }
  ```
  Called inside transactions at postgres.go:140, 233, 363. **`users` rows only appear when a user
  saves settings, creates a chat, or performs a calculation** — never at login. See §11.
- Update-upsert — `clause.OnConflict{Columns: ..., DoUpdates: clause.Assignments(map[string]any{...})}`
  (postgres.go:158-161, 374-378).
- Writes that touch more than one table run in `r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {...})`
  (postgres.go:139, 232, 362).
- Reads: `r.db.WithContext(ctx).Where(...).First(&record)`, mapping `gorm.ErrRecordNotFound` to the
  domain error (postgres.go:183-189).
- Errors are wrapped with `fmt.Errorf("query settings: %w", err)` style (postgres.go:188).
- Aggregate reads use the query builder with explicit `Table/Select/Joins/Group/Order/Scan`
  (postgres.go:252-264), not raw SQL.

### Where a new repository fits

`server/internal/repository/postgres.go` (add model structs + methods to `PostgresRepository`) and
`server/internal/repository/memory.go` (mirror on `MemoryRepository`), with new interfaces declared
in `server/internal/service/`. `buildRepositories` (`cmd/main.go:107-134`) returns a fixed 3-tuple
today and would need widening or restructuring to hand a fourth repo to `main`.

---

## 4. Handler layer

### Routing

`http.ServeMux` with **manual path splitting** — there is no router library and Go 1.22 method/wildcard
patterns are not used, despite `go 1.25.5` in `go.mod:3`.

- `APIHandler.Register(mux)` registers exactly one prefix: `mux.HandleFunc("/api/v1/users/", h.handleUsers)` (http.go:22-24).
- `handleUsers` trims the prefix, calls `splitPath` (http.go:218-224), then dispatches with a giant
  `switch { case resource == "..." && len(parts) == N && r.Method == ...: }` (http.go:26-70).
- Unmatched → `writeAPIError(w, http.StatusNotFound, "route not found")` (http.go:31, 67).
- Auth routes are registered flat, one `mux.HandleFunc` each, directly in `main` (`cmd/main.go:69-74`).
- Method checks are per-handler: `if r.Method != http.MethodGet { w.WriteHeader(405); return }`
  (auth.go:171-174, 186-189, 208-211, 277-280); some handlers also short-circuit `OPTIONS`
  (auth.go:43-46, 100-103).

### Two error helpers in the same package (real, and confusing)

| Helper | File:line | Body |
|---|---|---|
| `writeError(w, status, code string)` | `auth.go:581-587` | `{"error": code}` — used by all `auth.go` handlers, codes are machine-readable slugs (`access_cookie_missing`, `session_expired`) |
| `writeAPIError(w, status, message string)` | `http.go:247-249` | `{"error": message}` — used by `http.go`, messages are human/English prose |
| `writeAPIJSON(w, status, payload any)` | `http.go:241-245` | sets `Content-Type`, `WriteHeader`, `json.NewEncoder().Encode` |
| `writeAPIDomainError(w, err)` | `http.go:251-266` | domain-error → status mapping |

Both produce the identical wire shape `{"error": "..."}`; they differ only in the convention for
the string. **The client depends on the `auth.go` slug convention**: `shouldRefreshAuth` in
`client/src/utils/yandexAuth.js:14-16` matches exactly `access_cookie_missing`, `access_expired`,
`access_mismatch`. A new gated endpoint that returns 401 with a different code will **not** trigger
the client's refresh retry.

`writeAPIDomainError` mapping (http.go:251-266):
- `service.ErrInvalidArgument` → 400
- `service.ErrNotFound` → 404
- substring `rate_limit_exceeded` → 429, `service_unavailable` → 503, `timeout` → 504
- default → **500 with `err.Error()` echoed to the client** (http.go:264) — internal error text leaks.

There is **no `ErrForbidden` / `ErrConflict` / `ErrUnauthorized`** in `service` today
(`costing.go:15-18` defines only `ErrInvalidArgument` and `ErrNotFound`). 403 and 409 (both required
by the spec's verification table, rows 3/4 and 8) have no existing mapping.

### CORS wrapper — `server/internal/handler/cors.go`

`WithCORS(config CORSConfig, next http.Handler) http.Handler` (cors.go:12).

- **If `AllowedOrigins` is empty, it returns `next` unwrapped (cors.go:13-15)** — no CORS headers at
  all, and no global `OPTIONS` short-circuit. Dev `docker-compose.yml` sets no `CORS_ALLOWED_ORIGINS`,
  so this is the dev-compose behaviour.
- When active: exact-match origin allowlist, sets `Access-Control-Allow-Credentials: true` (cors.go:30),
  `Allow-Headers: Content-Type, Authorization` (cors.go:32),
  **`Allow-Methods: GET, POST, OPTIONS` (cors.go:33)** — `DELETE`, `PATCH`, `PUT` are **not**
  advertised, even though `handleDeleteChat` exists (http.go:51). Any cross-origin `PATCH`/`PUT`
  for toggling access would be blocked by preflight. In prod this is masked because nginx serves
  API and SPA on the same origin (`.github/workflows/deploy.yml:208-214`), and in local dev the
  Vite proxy does the same (`client/vite.config.js:8-20`).
- `OPTIONS` returns 204 before reaching the mux (cors.go:37-40).

### Metrics wrapper — `server/internal/handler/metrics.go`

`WithMetrics(next http.Handler) http.Handler` (metrics.go:48), records
`metrics.HTTPRequestsTotal{method,path,status}` and `HTTPRequestDuration{method,path}`
(metrics.go:59-60). `normalizePath` collapses path segments that "look like an ID" to `:id`
(metrics.go:23-45) — a segment is an ID if it parses as an int or if `len(s) > 8 && strings.Contains(s, "-")`
(metrics.go:38-44). **A Yandex login like `ivanov` (no dash, ≤8 chars) is not collapsed** and becomes
its own Prometheus label value; this is a pre-existing cardinality leak, not new to this feature.

### Composition order in `cmd/main.go`

```go
mux := http.NewServeMux()                                   // main.go:39
apiHandler.Register(mux)                                    // main.go:61
mux.HandleFunc("/health", ...)                              // main.go:63
mux.Handle("/metrics", promhttp.Handler())                  // main.go:68
mux.HandleFunc("/auth/...", authHandler.Handle...)          // main.go:69-74
handlerWithCORS := handler.WithCORS(cfg, handler.WithMetrics(mux))   // main.go:76-78
```

CORS is outermost, metrics inner, mux innermost. A new route group registers as either:
a) a method on `APIHandler` behind the existing `/api/v1/users/` prefix switch (http.go:38-69), or
b) a new `Register(mux)` on a new handler type, constructed in `main` after line 60 and given its
own `mux.HandleFunc("/api/v1/<prefix>/", ...)`.

**There is no middleware chain and no request-scoped context.** `AuthHandler.validateSession` is a
private method (auth.go:421) on a type that `APIHandler` does not hold a reference to
(http.go:13-20). Any server-side gating requires either exporting/relocating session validation or
constructing the new handler with the `*auth.Store` + `*config.Config` it needs.

### Prod nginx does not proxy every backend route

The prod nginx config is heredoc-generated inside the deploy workflow
(`.github/workflows/deploy.yml:130-233`) and proxies only these to `poshivon_app:8080`:
`= /auth/status`, `= /auth/me`, `= /auth/logout`, `= /auth/yandex`, `= /auth/yandex/code`,
`/health`, `/api/`. Same list (minus `/health` ordering) in `client/nginx.conf:13-67`.

**`/auth/refresh` is not in either list.** It falls through to `location /` → the SPA container →
`try_files ... /index.html` (`client/nginx.conf:9-11`) → HTTP 200 with HTML.
`refreshAuthSession` (`client/src/utils/yandexAuth.js:18-25`) returns `response.ok`, which is
therefore `true`, so `authFetch` retries the original request with the same stale cookie and
returns the same 401 (yandexAuth.js:43-51). **The 401-refresh-retry path is silently dead in prod.**
Consequence for this feature: new endpoints under `/api/` are proxied correctly, but anything placed
under a new top-level prefix requires editing the heredoc in `deploy.yml` **and** `client/nginx.conf`.

---

## 5. Yandex profile fetch and the requester's email

### What the code extracts today

`fetchYandexProfile(ctx, accessToken) (*yandexProfile, error)` — `server/internal/handler/auth.go:459-511`.

- GETs `cfg.YandexUserInfoURL`, default `https://login.yandex.ru/info`
  (`server/internal/config/config.go:75`), header `Authorization: OAuth <token>` (auth.go:468).
- Decodes into `map[string]interface{}` (auth.go:485-488) and reads only:
  `login`, `display_name`, `real_name`, `first_name`, `last_name` (auth.go:490-494).
- Returns a 2-field struct — **no email** (auth.go:454-457):
  ```go
  type yandexProfile struct {
      Name  string `json:"name"`
      Login string `json:"login"`
  }
  ```
- `HandleMe` JSON-encodes that struct directly (auth.go:203-204), so `/auth/me` responds
  `{"name": "...", "login": "..."}`. `Panel.jsx:190` reads `profile?.login`;
  `Panel.jsx:582` reads `profile?.name`.

### What `https://login.yandex.ru/info` additionally returns

The endpoint's response also carries `id`, `client_id`, `psuid`, `sex`, `birthday`,
`default_avatar_id`, `is_avatar_empty`, and — relevant here — **`default_email`** (string) and
**`emails`** (array of strings). Since the handler decodes into a generic map, adding
`getString(payload, "default_email")` (helper at auth.go:560-573) is a one-line extraction; the
struct and the `/auth/me` contract would need a new field.

**Gate on scope, not on code:** `default_email` / `emails` are only present when the OAuth app has
the `login:email` permission. `buildYandexAuthUrl` (`client/src/utils/yandexAuth.js:125-142`) sends
only `response_type`, `client_id`, `redirect_uri` — **no `scope` parameter** — so the granted
permissions are whatever is configured in the Yandex OAuth application console for
`VITE_YA_CLIENT_ID`. If `login:email` is not enabled there, `default_email` will be absent and the
notification email cannot carry the requester's address without a console change (and re-consent
from already-authorized users). This is an external dependency the tech-spec must call out.

### `server/api/openapi.yaml` is stale — do not treat it as the contract

- 379 lines, documents `/health` and five `/api/v1/users/...` paths only.
- **No `/auth/*` path is documented at all** — `/auth/me`'s response shape is undocumented.
- `UserSettings` schema (openapi.yaml:223-248) still describes the pre-`003` model
  (`base_prices`, `surcharge_percent`) — the live model is `pricing_rules`/`garments`/`operations`/
  `materials`/`urgency`/`market_bands` (`server/internal/service/costing.go:26-40`, `postgres.go:24-36`).
- `DELETE /chats/{id}` and `POST /chats/{id}/restore` (http.go:51-56) are missing from the spec.
- Nothing in the build or CI validates the spec against the code.

---

## 6. Tests

### What exists

**One test file in the entire repository:** `server/internal/service/costing_test.go` (293 lines).
No JS/JSX test files, no `*_test.go` anywhere else (verified by find over the repo excluding
`node_modules`).

Style — **stdlib `testing` only, zero test dependencies**:
- imports are `context`, `errors`, `testing` (costing_test.go:3-7). `go.mod` has no testify,
  no gomock, no dockertest, no testcontainers (`server/go.mod:1-27`).
- Hand-written stubs implementing the service interfaces:
  `settingsRepoStub` (costing_test.go:9-27) and `chatRepoStub` (costing_test.go:29-84), the latter
  implementing `ChatRepository` **and** `ChatCalculationRepository` and passed twice to the
  constructor (costing_test.go:91).
- `t.Parallel()` (costing_test.go:87), table-free arrange/act/assert, failures via
  `t.Fatalf("total = %d, want %d", ...)` (costing_test.go:151-153).
- Representative signatures:
  ```go
  func TestCostingService_CalculateInChat_UsesExpandedPricingModel(t *testing.T)   // :86
  func (s *settingsRepoStub) GetSettings(_ context.Context, userID string) (UserSettings, error)  // :21
  ```
- Assertions are on hard-coded expected numbers (`result.Total != 279955`, costing_test.go:151).

### What does not exist

- **No HTTP test harness.** No `httptest.NewServer`/`httptest.NewRecorder` anywhere; no test in
  `internal/handler` at all.
- **No DB fixtures, no test DB, no migration-under-test helper.** `MemoryRepository` is the only
  substitute for the DB in tests, and it is not used by the existing test (which uses stubs).
- **No CI test job.** `.github/workflows/deploy.yml` is the only workflow (verified: single file in
  `.github/workflows/`) and it triggers on tag push, builds two Docker images, and deploys —
  it never runs `go test`, `go vet`, or a client build check outside the image build.
- **No pre-commit hooks, no gitleaks.** `.git/hooks/` contains no non-sample hook;
  no `.pre-commit-config.yaml`.
- **Go toolchain is not installed on this machine** (`go: command not found`) and no MariaDB image
  is present locally (only `golang:1.25-alpine`). Running Go tests requires Docker.

### What integration tests for handlers would need built from scratch

1. A way to construct the handler under test with a session store — `auth.NewStore(db *sql.DB)`
   (`auth/store.go:27-29`) needs a real `*sql.DB`; there is no interface to fake it.
2. `httptest.NewRecorder` + manually-built `*http.Request` carrying both `ya_access` and `ya_refresh`
   cookies, since `validateSession` requires both (auth.go:422-430) and that the access cookie value
   equals `session.YandexAccessToken` (auth.go:447).
3. A stub for the Yandex `/info` call: `AuthHandler.httpClient` is set in `NewAuthHandler`
   (auth.go:36-38) and is **not injectable** — the only seam is `cfg.YandexUserInfoURL`
   (config.go:75), which can be pointed at an `httptest.NewServer`. Same for `YandexTokenURL`.
4. A DB: either MariaDB via testcontainers/docker-compose (new dependency + CI work), or
   `MemoryRepository` for the repo layer plus a hand-written stub for the session store.
5. `service.ErrForbidden`/`ErrConflict` do not exist, so 403/409 assertions have no existing mapping
   to test against (see §4).

---

## 7. Shared utilities

### Server (`server/internal/handler/auth.go`, package-private but package-wide)

| Symbol | Line | Purpose |
|---|---|---|
| `readCookie(r, name) (string, error)` | auth.go:524-530 | cookie read |
| `readJSON(body) (map[string]interface{}, error)` | auth.go:532-541 | loose JSON decode with `UseNumber()` |
| `getString(payload, key) string` | auth.go:560-573 | tolerant map field read (string or json.Number) |
| `parseExpiresIn(payload) int64` | auth.go:543-558 | number/string/float coercion |
| `generateToken() string` | auth.go:575-579 | 32 random bytes → hex; **ignores `rand.Read` error** (auth.go:577) |
| `parseSameSite(value) http.SameSite` | auth.go:513-522 | config string → enum, defaults to Lax |
| `writeError(w, status, code)` | auth.go:581-587 | see §4 |

### Server (`server/internal/handler/http.go`)

`splitPath` (http.go:218-224), `decodeJSON(r, dst) error` with `DisallowUnknownFields()` +
multiple-object rejection (http.go:226-239), `writeAPIJSON` / `writeAPIError` / `writeAPIDomainError`
(http.go:241-266).

### Server (`AuthHandler` methods, private)

`setAccessCookie` (auth.go:387), `setRefreshCookie` (auth.go:391), `writeCookie` (auth.go:395-406),
`clearCookie` (auth.go:408-419), `validateSession` (auth.go:421-452), `fetchYandexProfile` (auth.go:459-511).

### Server (`auth` package, exported)

`auth.HashRefreshToken(token) string` — sha256 hex (`store.go:31-34`);
`auth.Store` with `CreateSession` / `FindByRefreshHash` / `UpdateSessionTokens` / `RevokeByRefreshHash`
(store.go:36-141). `Session` struct at store.go:11-21.

### Server (`config`)

`envOrDefault`, `envBool`, `envInt` (config.go:123-155) plus `loadEnvFile` which walks
`.env`, `../.env`, `../../.env`, `../../../.env` and **only sets a var if it is currently empty**
(config.go:43-52, 117-119).

### Client

- `client/src/utils/yandexAuth.js` — `authFetch`, `refreshAuthSession`, `checkAuthStatus`,
  `fetchAuthProfile`, `logout`, `buildYandexAuthUrl`, `persistYandexToken`, `exchangeYandexCode`,
  `saveAuthReturnTo` / `consumeAuthReturnTo`, `getApiBase` (yandexAuth.js:3).
- `client/src/utils/panelApi.js` — the `request` wrapper (panelApi.js:3-35) + 8 endpoint functions.
- `client/src/pages/Panel.jsx` — local helpers: `formatMoney` (:1234), `formatPercent` (:1236),
  `mapPanelError` (:1300-1305), `normalizeSettings` (:1194), `mergeNamedMap` (:1211),
  `syncOrderForm` (:1221); presentational `SettingsSection` (:149), `SettingsField` (:161),
  `SettingsNumberInput` (:168).

---

## 8. Client: `Panel.jsx`

`client/src/pages/Panel.jsx`, 1335 lines, single default-exported `Panel` component (`:170-...`,
`export default Panel` at `:1335`) plus module-level helpers listed above.

### The `status` state machine

```js
const [status, setStatus] = useState("checking");   // :171
```
Only two values ever exist: `"checking"` (initial) and `"ready"` (set at `:216`). There is **no
error state** — every failure path calls `window.location.replace("/")` (`:199`, `:210`).

Render gate at `:547-555`:
```jsx
if (status !== "ready") {
  return (
    <div className={`page panel panel--${theme}`}>
      <main className="panel__content"><p>Проверяем доступ...</p></main>
    </div>
  );
}
```
Note the placeholder text is already literally "Проверяем доступ..." even though nothing checks access.

### The bootstrap effect — `:192-224`

```js
useEffect(() => {
  let isActive = true;
  const bootstrap = async () => {
    try {
      const ok = await checkAuthStatus();          // :197  → GET /auth/status
      if (!ok) { window.location.replace("/"); return; }   // :198-201
      const nextProfile = await fetchAuthProfile(); // :203  → GET /auth/me
      if (!isActive) return;
      setProfile(nextProfile);                     // :207
    } catch {
      if (isActive) window.location.replace("/");  // :209-211
      return;
    }
    if (isActive) setStatus("ready");              // :215-217
  };
  bootstrap();
  return () => { isActive = false; };              // :221-223
}, []);
```

**This is the insertion point for the gate.** It already does exactly two sequential network calls
and already owns the only transition to `"ready"`. Adding a third call here (or extending
`/auth/me`'s payload) keeps the gate on the single path that every panel render passes through.

### Data-loading effects that fire off `userID`

- `:230-278` — `loadSettings()` (`GET .../settings`, 404 tolerated at `:250`) and `loadChats()`
  run when `userID` becomes non-empty. `userID = profile?.login || ""` (`:190`).
- `:280-308` — history load, keyed on `[userID, activeChatID]`.

**These effects fire as soon as `profile` is set (`:207`), i.e. before/independently of `status`.**
So a user without access would still hit `/api/v1/users/{login}/settings` and `/chats` unless the
gate blocks `profile` from being set, or the effects gain a guard. Relevant to AC US-1.

### Nav / section structure

Nav — `:561-576`, two buttons only:
```jsx
<nav className="panel__nav">
  <button className={`panel__link ${activeSection === "workspace" ? "panel__link--active" : ""}`}
          onClick={() => setActiveSection("workspace")}>Чаты и расчёты</button>     // :562-568
  <button className={`panel__link ${activeSection === "settings" ? "panel__link--active" : ""}`}
          onClick={() => setActiveSection("settings")}>Настройки модели</button>    // :569-575
</nav>
```
`const [activeSection, setActiveSection] = useState("workspace")` (`:173`).

Body — a **binary ternary**, not a switch:
```jsx
{activeSection === "settings" ? (
  <section className="panel-settings ...">   // :617-618  … through :852
) : (
  <section className="panel-workspace">      // :853-854  … through the history block
)}
```
Adding a third section ("Пользователи") means converting this ternary into a chain/switch, adding a
third `<button className="panel__link">` in the nav (conditionally rendered for admins per US-12),
and — since the nav is not route-driven — nothing else. The always-visible `panel-summary` block
(`:599-615`, three chat-oriented cards) sits **above** the ternary and would render on the new
admin section too unless moved inside the workspace branch.

### Styling conventions — mixed, deliberately

Two coexisting systems:

1. **BEM-ish semantic classes in `client/src/App.css`** for panel chrome. Blocks: `.panel`
   (App.css:829), `.panel__sidebar` (:857), `.panel__brand` (:866), `.panel__nav` (:872),
   `.panel__link` / `.panel__link--active` (:878, :891-892), `.panel__content` (:897),
   `.panel__header` (:907), `.panel__card` (:932), `.panel__notice` (:1004), `.panel__empty` (:1011),
   `.panel__theme-toggle` (:961), `.panel__logout` (:923), `.panel__actions` (:992),
   `.panel__meta` (:998), plus element blocks `.panel-workspace` (:1018), `.panel-chat-list*`
   (:1029-1100), `.panel-form*` (:1105-1128), `.panel-history*` (:1135-1180), `.panel-summary*`.
   Theme is a modifier on the root: `.panel--light` (:837) / `.panel--dark` (:847), applied as
   `` className={`page panel panel--${theme}`} `` (Panel.jsx:549, 558); `theme` persists to
   `localStorage["panelTheme"]` (Panel.jsx:174, 226-228). Note App.css redefines `.panel`,
   `.panel--light`, `.panel--dark`, `.panel__sidebar`, `.panel__link` etc. a **second time** at
   :1214-1310 after a media query closes at :1211 — later rules win; be careful when editing.
2. **Tailwind v4 utility strings** for the settings section, hoisted into module constants
   (Panel.jsx:137-147: `settingsSectionClass`, `settingsInsetClass`, `settingsInputClass`,
   `settingsModeButtonBaseClass`) and referencing CSS custom properties defined in
   `client/src/index.css` (`@import "tailwindcss"` at index.css:1, `@theme { --color-* ... }` at
   index.css:3-20, keyframes `fade-rise` / `soft-pop` at index.css:22+).

A new admin section can follow either; the workspace branch (the closer analogue: list + rows +
notices) uses system 1 (`.panel__card`, `.panel-chat-list__item`, `.panel__notice`, `.panel__empty`).

`client/src/components/AuthModal.jsx` (48 lines) uses system 1 exclusively: `.modal-overlay`,
`.modal`, `.modal__close`, `.modal__action`, `.ya-btn`, `.ya-btn__icon`
(AuthModal.jsx:22, 30-38). It is landing-page-only (rendered from `App.jsx:50`) and is **not**
reachable from `/panel` — `App.jsx:34-35` returns `<Panel />` before the landing tree is built.
So the access-request plashka cannot reuse `AuthModal`; it must live inside `Panel.jsx`'s own tree.

---

## 9. Client API layer

### `client/src/utils/panelApi.js` (79 lines)

Every endpoint goes through one private `request(path, options)` (panelApi.js:3-35):
- delegates to `authFetch` with `Content-Type: application/json` merged in (`:4-10`);
- on `!response.ok`, tries to read `payload.error` and throws `new Error(message)` with
  `error.status = response.status` attached (`:12-28`);
- a bare 405 becomes the sentinel `"api_method_not_allowed"` (`:22-24`), which `mapPanelError`
  (Panel.jsx:1300-1305) turns into the "proxy for /api not applied" hint;
- `204` → `null` (`:30-32`), otherwise `response.json()` (`:34`).

Endpoint functions are one-liner arrow exports, e.g.
```js
export const getUserSettings = async (userID) => request(`/api/v1/users/${userID}/settings`);   // :37
export const deleteChat = async (userID, chatID) => { await request(`/api/v1/users/${userID}/chats/${chatID}`, { method: "DELETE" }); };  // :54-58
```
**New endpoints belong here**, as further `export const ... => request(...)`. `userID` is
interpolated raw into the path with no `encodeURIComponent` — a Yandex login can contain `.` and `-`
so this happens to be safe, but it is not defensive.

### `client/src/utils/yandexAuth.js` (176 lines)

- `getApiBase()` = `import.meta.env.VITE_API_URL || ""` (`:3`) — empty in every build config found,
  so all requests are same-origin relative paths.
- `authFetch(path, options = {}, retryOnAuth = true)` (`:27-52`): always `credentials: "include"`
  (`:31`, `:49`); reads the error code via `response.clone().json()` (`:6-12`); retries **once**,
  and only when `status === 401 && errorCode ∈ {access_cookie_missing, access_expired, access_mismatch}`
  (`shouldRefreshAuth`, `:14-16`); refresh via `POST /auth/refresh` (`:18-25`).
  **Important:** the retry decision reads the response body of *every* response
  (`parseErrorCode` at `:38`) — including successful ones — before returning it. Because it uses
  `.clone()`, the caller's body is still readable.
- `logout()` passes `retryOnAuth = false` explicitly (`:169-176`) — the only caller that does.
- `checkAuthStatus()` returns a bare boolean `response.ok` (`:152-157`), swallowing the reason.
  A 403-for-no-access on `/auth/status` would be indistinguishable from a 401 to `Panel.jsx:198`.
- See §4: `/auth/refresh` is not proxied in prod, so the retry branch never actually refreshes there.

---

## 10. Email / SMTP

**Zero SMTP infrastructure exists.** Verified by grep across `server/` for
`smtp|mail|email|admin|role`: the only hits are `Role` fields in the DeepSeek chat-message structs
(`server/internal/service/deepseek.go:102, 204, 208`). Nothing in `client/src` mentions email either.

- `server/go.mod` (27 lines) contains: `go-sql-driver/mysql`, `gorm.io/gorm`, `gorm.io/driver/mysql`,
  `prometheus/client_golang`, and transitive deps. **No mail library.**
- `net/smtp` is in the Go stdlib and needs no dependency. It is frozen (no new features) but fully
  functional for `smtp.SendMail(addr, auth, from, to, msg)` over STARTTLS/implicit-TLS via
  `smtp.PlainAuth`. Composing a MIME message with a UTF-8 Subject (the notification will contain
  Cyrillic) requires manual RFC 2047 encoded-word headers (`mime.QEncoding.Encode("utf-8", subj)`
  from stdlib `mime`) — doable, but a dependency such as `gomail`/`go-mail` would remove that
  hand-rolling. Nothing in the current dependency policy forbids either; the project has kept
  dependencies deliberately minimal (5 direct modules).
- The only outbound-HTTP integration to copy patterns from is `service.DeepSeekClient`
  (`server/internal/service/deepseek.go`, 631 lines), constructed in `main` from a config struct
  (`cmd/main.go:48-58`) with `Timeout`, `ConnectTimout` [sic], `MaxRetries` — that is the house
  pattern for "external service client configured from env".
- Config additions follow `envOrDefault` / `envInt` / `envBool` in `config.Load()`
  (`config.go:54-85`) plus a field on `Config` (`config.go:9-40`).

### Path new secrets must travel (all four hops are required)

1. GitHub repository secret.
2. `env:` block of the **Deploy** step — `.github/workflows/deploy.yml:93-120`.
3. The `envs:` **comma-separated allowlist** on the same step — `deploy.yml:126`. A variable absent
   from this list is silently dropped by `appleboy/ssh-action` even if it is in `env:`.
4. The `.env` heredoc written on the server — `deploy.yml:234-260` — which is what
   `docker compose --env-file` reads.
5. And `docker-compose.prod.yml:5-26` `environment:` mapping, or the container never sees it.

For dev, `docker-compose.yml:7-21` sets values inline (secrets like `DEEPSEEK_API_KEY` come from a
host `.env` via `${...}` at line 11); `config.loadEnvFile` also reads a local `.env` walking up four
levels (`config.go:43-52`). `.gitignore` covers `.env` (last line) — no gitleaks hook exists to
enforce it.

---

## 11. Deployment

- Single workflow: `.github/workflows/deploy.yml`, `on: push: tags: ["*"]` (`:3-6`). **No test/lint
  workflow exists.** Untagged pushes deploy nothing and validate nothing.
- Builds two images to GHCR: `poshivon-app` from `./server/Dockerfile` (`:36-42`) and `poshivon-web`
  from `./client/Dockerfile` (`:44-53`). The client image bakes `VITE_YA_CLIENT_ID` and
  `VITE_YA_REDIRECT_URI` as **build args** (`:51-53`, `client/Dockerfile:3-6`) — client-side env
  vars are compile-time and require an image rebuild to change.
- Deploys over SSH (`appleboy/ssh-action@v1.0.3`): writes `~/poshivon/nginx/default.conf` from a
  heredoc (`:130-233`), writes `~/poshivon/.env` from a heredoc (`:234-260`), `docker login ghcr.io`,
  then `pull` → `up -d db` → wait for health → recreate the DB user with `GRANT ALL` (`:270`) →
  `up -d node_exporter prometheus grafana` → `up -d app web proxy` (`:262-273`).
- **Migrations run automatically on every container start**: `main()` calls `db.Open` then
  `migrations.Run(database)` unconditionally before anything else (`cmd/main.go:26-34`), with
  `log.Fatalf` on failure. No separate migrate step, no manual gate. A bad migration = crash-looping
  `poshivon_app`.
- `Makefile` targets: `build`, `up`, `down`, `logs`, `restart` — dev compose only, no test target.
  `make down` runs `docker system prune -af --volumes` (Makefile:15-16), i.e. it nukes the DB volume.
- Prod runs behind an external shared proxy network `haclever_proxy`
  (`docker-compose.prod.yml:117-120`) with nginx-proxy/acme companion driven by `VIRTUAL_HOST` /
  `LETSENCRYPT_HOST` env on the `proxy` service (`docker-compose.prod.yml:41-44`).
- `deploy/nginx.conf` (73 lines) is a **third, unused** nginx config — it references service names
  `web`/`app`/`grafana` and terminates TLS itself, which does not match the current prod topology
  (the workflow heredoc uses `poshivon_web`/`poshivon_app`/`poshivon_grafana` and listens on 80
  behind the shared proxy). Editing it has no effect on production; edit `deploy.yml:130-233`.
- Server image is `alpine:3.20` running as non-root `appuser` (`server/Dockerfile:23-31`) — the
  binary has no shell tooling for ad-hoc SQL; DB inspection goes through `docker exec poshivon_db`.

### Env vars currently reaching the server container

`APP_STORAGE, APP_HOST, APP_PORT, LOG_LEVEL, DEEPSEEK_*, CORS_ALLOWED_ORIGINS, COOKIE_SECURE,
COOKIE_SAMESITE, COOKIE_DOMAIN, YANDEX_CLIENT_ID, YANDEX_CLIENT_SECRET, YANDEX_REDIRECT_URI,
DB_HOST, DB_PORT, DB_NAME, DB_USER, DB_PASSWORD` (`docker-compose.prod.yml:6-26`).

Not passed today although the config reads them: `COOKIE_PATH`, `REFRESH_TTL_HOURS`,
`YANDEX_TOKEN_URL`, `YANDEX_USERINFO_URL`, `DATABASE_URL` — all fall back to `config.go` defaults.
Any new var (SMTP host/port/user/password/from, admin recipient list, contact email) needs all five
hops from §10.

---

## 12. Risks and gotchas specific to this feature

1. **No `users` row exists for a user who has never saved settings / created a chat.**
   `upsertUser` (`postgres.go:550-555`) is only invoked from `UpsertSettings` (:140),
   `CreateChat` (:233), and `AppendCalculation` (:363). Login creates nothing. So an
   access-flag column on `users` will have **no row to read** for exactly the population this
   feature targets — first-time users. Any "list all users" admin view built on `SELECT * FROM users`
   shows only users who already did work, which is the opposite of what US-7 needs. Whatever design
   is chosen must decide where the row gets created for a login-only user.
   Additionally, `access_requests.user_id` cannot carry an FK to `users(id)` unless the row is
   guaranteed to exist first (the existing tables all do FK to `users`, `002:12-14, 23-24`).

2. **The gate needs the caller's login, and the only source is a live Yandex HTTP call.**
   `oauth_sessions` stores no identity (`store.go:11-21`), so per-request authorization means
   `fetchYandexProfile` → `https://login.yandex.ru/info` on every gated request: an extra external
   round-trip (10s client timeout, auth.go:36-38) on the hot path, and a hard dependency on Yandex
   uptime for authorization decisions. `HandleMe` already returns 502 `yandex_profile_failed` when
   that call fails (auth.go:198-201).

3. **`validateSession` is unreachable from `APIHandler`.** It is a private method
   (auth.go:421) on `AuthHandler`, and `APIHandler` holds only `*service.CostingService` and
   `*service.DeepSeekClient` (http.go:13-16). There is no middleware, no `context` value carrying an
   identity, and no shared interface. Server-side gating requires new plumbing, not a wrapper.

4. **`/auth/refresh` is not proxied in production** (`.github/workflows/deploy.yml:130-233`,
   `client/nginx.conf`), so the client's 401-refresh-retry (`yandexAuth.js:43-51`) resolves against
   the SPA's `index.html` (HTTP 200 → `response.ok === true`) and silently retries with the same
   stale cookie. If a gated endpoint returns 401 on an expired access token, the user sees a hard
   failure rather than a transparent refresh. Pre-existing, but this feature multiplies its blast radius.

5. **401 error codes are a contract with the client.** `shouldRefreshAuth`
   (`yandexAuth.js:14-16`) only reacts to `access_cookie_missing`, `access_expired`, `access_mismatch`.
   A new 401 with a code like `no_access` would not trigger a refresh — which is probably desirable,
   but it must be deliberate. Conversely, returning **403** (per the spec's verification table)
   bypasses the retry logic entirely, which is correct.

6. **`Panel.jsx`'s `status` machine has no failure state.** Both failure branches
   (`:198-201`, `:208-213`) hard-redirect to `/`. A "no access" outcome is neither
   `"checking"` nor `"ready"` nor a redirect, so the machine must gain a third value (and the
   `status !== "ready"` early return at `:547` must stop swallowing it). The bootstrap's
   `setStatus("ready")` at `:215-217` runs unconditionally after `setProfile`, so simply adding a
   check without restructuring will flash the full panel.

7. **Panel data effects fire on `profile`, not on `status`.** `:230-278` and `:280-308` key off
   `userID` (`:190`), which is set the moment `setProfile` runs (`:207`). A gated user would still
   issue `GET /settings` and `GET /chats` — visible in the network tab and in Prometheus, and
   (because `/api/v1/users/*` is unauthenticated) actually served. AC US-1 needs the effects guarded
   or `profile` withheld.

8. **CORS advertises only `GET, POST, OPTIONS`** (`cors.go:33`). A `PATCH`/`PUT` toggle endpoint
   works same-origin (prod nginx and the Vite dev proxy) but fails preflight for any cross-origin
   caller. Also, when `CORS_ALLOWED_ORIGINS` is unset the wrapper is bypassed entirely
   (`cors.go:13-15`) — which is the dev-compose configuration.

9. **Cookie settings in prod come entirely from secrets** — `COOKIE_SECURE`, `COOKIE_SAMESITE`,
   `COOKIE_DOMAIN` (`deploy.yml:106-108`, `docker-compose.prod.yml:16-18`), parsed by
   `envBool`/`envOrDefault` (`config.go:66-69`) with defaults `Secure=false`, `SameSite=Lax`,
   `Domain=""`. `envBool` **silently falls back** on an unparseable value (config.go:141).
   `parseSameSite` likewise defaults unknown strings to Lax (auth.go:513-522). If `COOKIE_SECURE`
   is misconfigured, cookies still work over HTTPS but are also accepted over HTTP.
   `ya_access` is `HttpOnly` (auth.go:388, 401) and holds the **raw Yandex access token** — the
   browser stores a live third-party API credential.

10. **`writeAPIDomainError` leaks internal error text on 500** (`http.go:264` echoes `err.Error()`),
    and repository errors are wrapped with SQL-flavoured context (`postgres.go:188`). Any new
    service errors that fall through the switch will surface DB details to the client.

11. **`service` has no `ErrForbidden`/`ErrConflict`.** `costing.go:15-18` defines only
    `ErrInvalidArgument` and `ErrNotFound`; `writeAPIDomainError` has no 403 or 409 branch
    (`http.go:251-266`). The spec's verification table requires both (rows 3/4 → 403, row 8 → 409).

12. **No CI safety net.** Nothing runs `go test`, `go vet`, or a client lint on push or PR
    (only `.github/workflows/deploy.yml`, tag-triggered). New integration tests will not gate
    anything unless a workflow is added. There is also no gitleaks/pre-commit hook, so the
    "SMTP creds must not reach the repo" mitigation in the user-spec is enforced by convention only.

13. **`server/api/openapi.yaml` is stale and unenforced** (§5) — treating it as the source of truth
    for existing contracts will produce wrong assumptions.

14. **Migration failure = crash loop, no rollback path.** `migrations.Run` has no down migrations
    (§1) and `main` calls `log.Fatalf` (`cmd/main.go:32-34`). A migration that half-applies (each
    file is one non-transactional multi-statement `Exec`, migrate.go:43) leaves the DB in a state
    that must be repaired by hand via `docker exec poshivon_db mariadb ...`.

15. **`client/.htpasswd` is committed** and copied into the web image
    (`client/Dockerfile:20`), though `client/nginx.conf` contains no `auth_basic` directive that
    uses it. Dead config carrying a credential file in git — worth noting while touching this area.
