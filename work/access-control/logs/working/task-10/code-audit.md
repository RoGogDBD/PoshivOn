# Task 10 — Code Audit (feature: access-control, Phase 1 MVP)

**Scope:** holistic, cross-component read of the merged codebase at `f300c07` (main). Every file
in tasks/10.md's Context Files list was read in full, in its current on-disk state, plus the
files the change depends on but the list omits (`server/internal/handler/routes.go`,
`cors.go`, `server/cmd/main_test.go`, `server/migrations/005_identity_collation.up.sql`,
`client/vite.config.js`).

**Method:** `code-reviewing` skill, 11 dimensions applied at feature level.

**Constraint:** no Go toolchain on this host (`go: command not found`, no `/usr/local/go`), so
findings are from reading, not from `go build` / `go vet` / `go test`. Task 6's own report
records a clean build and green tests at the previous commit; nothing found below is a
compile error.

**No code was modified by this task.**

---

## Summary of findings

| Severity | Count |
|---|---|
| critical | 4 |
| major | 8 |
| minor | 9 |

The invariants this audit was specifically asked to verify (single-instance shared resources,
middleware chain vs. the Architecture table, Decision 13's `userModel`, both compile-time
interface assertions, no `userID` left in any URL segment) are all **clean** — see
"Dimensions checked and found clean" at the end. The findings below are elsewhere: in the
seams between tasks, in the client, and in one decision that was explicitly deferred from
Task 4 to Task 6 and then dropped.

---

## CRITICAL

### C1. `RequireSameOrigin`'s same-origin fallback is incompatible with every proxy in this repo — mutating requests 403 in local dev, and prod correctness hangs on one un-defaulted secret

**Location:** `server/internal/handler/middleware.go:222-239` (`originAllowed`), together with
`client/vite.config.js:8-20`, `client/nginx.conf:12-19,21-27,…`, `docker-compose.yml:1-30`,
`.github/workflows/deploy.yml:111,253`.

**What is wrong.** When `CORS_ALLOWED_ORIGINS` is empty, `originAllowed` compares the browser's
`Origin` against `scheme + "://" + r.Host`, where `scheme` comes from `cfg.CookieSecure`. That
equality holds only if the Go process sees the *public* Host header verbatim, including port.
Every path by which a browser can reach this server in this repository breaks that assumption:

- `client/vite.config.js:11,15` sets `changeOrigin: true` on the `/api` and `/auth` proxies.
  Vite therefore rewrites Host to `127.0.0.1:8080`. Browser sends `Origin: http://localhost:5173`.
  `"http://localhost:5173" != "http://127.0.0.1:8080"` → **403 on every non-GET request**,
  including `POST /auth/yandex/code` (login itself) and `POST /api/v1/access/requests`.
- `client/nginx.conf` and the prod `nginx/default.conf` heredoc both use
  `proxy_set_header Host $host`. nginx's `$host` **strips the port**. In the docker dev stack
  (`docker-compose.yml`, web published on `5173:80`) Go sees `Host: localhost` while the browser
  sends `Origin: http://localhost:5173` → **403 again**.
- In production it works only by coincidence: the public port is 443 (implicit, so `$host`
  equals the Origin authority) **and** `COOKIE_SECURE` happens to be a truthy secret. That
  secret has no default anywhere — `config.go:70` is `envBool("COOKIE_SECURE", false)`, and
  `deploy.yml:111` passes it straight from `secrets.COOKIE_SECURE` with no fallback. If that
  secret is unset, empty, or misspelled, `envBool` silently returns `false`, the fallback
  builds `http://poshivon.ru`, the browser sends `https://poshivon.ru`, and **every mutating
  request on production returns 403 — including login**. Nothing in CI, the deploy script, or
  the post-deploy plan asserts it.

**Why this was invisible per-task.** Task 5's security-auditor raised the `r.Host` reliance and
it was accepted as "an infrastructure invariant, not this task's code"
(`decisions.md:186`) — but the infrastructure it was measured against was only
`docker-compose.prod.yml`. Nobody cross-checked `vite.config.js`, `client/nginx.conf`, or the
dev compose file, and those live in Task 7's and Task 9's diffs, not Task 5's.
`main_test.go:TestNewMux_ConfigValuesReachMiddleware` pins the *logic* (`req.Host` set by hand
to `api.example`) and cannot see that no real deployment delivers that Host.

**Impact.** The documented `Verify-user` steps for Task 7 ("открыть `localhost:5173/panel` …
нажать «Запросить доступ»") and the `Verify-smoke` for Task 5 cannot pass as written on a stock
checkout. In production, one unset secret turns the entire write surface — login, access
requests, granting access, saving settings, creating chats, calculating — into a blanket 403,
with a `forbidden` slug that gives no hint about the cause.

**Fix.**
1. Trust a proxy header for the authority instead of guessing: read `X-Forwarded-Host` /
   `X-Forwarded-Proto` behind a config flag (`TRUST_PROXY_HEADERS`), or
2. make `CORS_ALLOWED_ORIGINS` mandatory and fail fast at startup when it is empty
   (`log.Fatalf` in `config.Load`), removing the fallback entirely — the fallback exists to
   protect an unconfigured prod, but it is exactly the branch that misfires; or
3. at minimum: set `CORS_ALLOWED_ORIGINS` in `docker-compose.yml` for dev
   (`http://localhost:5173`), give `COOKIE_SECURE` a non-empty default in `deploy.yml`
   (`${{ secrets.COOKIE_SECURE || 'true' }}`), and add a startup log line stating which
   comparison mode `RequireSameOrigin` is in and against what value.

---

### C2. Decision 17 is violated at four call sites in `http.go` — internal parser text reaches the client, and those branches log nothing

**Location:** `server/internal/handler/http.go:99, 124, 167, 219`
(`writeAPIError(w, http.StatusBadRequest, err.Error())`), where `err` comes from
`decodeJSON` (`http.go:250-263`).

**What is wrong.** Decision 17 states that "каждая ветка … отдаёт фиксированный текст,
привязанный к категории ошибки … а исходная ошибка целиком пишется в лог", and tasks/10.md
asks for confirmation that "no `err.Error()` leaks anywhere in the touched files".
`writeAPIDomainError` itself is clean — all nine branches return fixed slugs and log
(`http.go:280-314`), and `http_test.go:20-60` proves it with an SQL-DSN-shaped payload. But
four handlers in the same file bypass that function entirely and hand the raw
`encoding/json` error to the client:

```go
if err := decodeJSON(r, &req); err != nil {
    writeAPIError(w, http.StatusBadRequest, err.Error())   // http.go:98-100
    return
}
```

`decodeJSON` wraps the decoder error as `invalid json: %w`, so the response body contains
strings like `invalid json: json: unknown field "user_id"` or
`invalid json: json: cannot unmarshal string into Go struct field PricingRules.pricing_rules.labor_minute_rate of type int64`
— Go package/struct/field names and the accepted field set, handed to any authenticated caller.
These four branches also write **no log line at all**, so the operator sees nothing.

**Why this was invisible per-task.** Task 4's own report explicitly parked this and handed it
forward: *"`writeAPIError` с `err.Error()` остался в трёх местах `http.go` … Если Task 6 будет
переписывать эти маршруты — стоит заодно свести и их к фиксированным кодам"*
(`decisions.md:152`). Task 6 rewrote exactly those handlers (removing the `{userID}` segment
changed every one of their signatures) and did not pick it up; its own carried-forward notes
do not mention it. Meanwhile Task 5 independently reached the **opposite** conclusion for the
identical error in `access.go:148-159`, with a comment saying so verbatim: *"Фиксированный слаг
вместо `err.Error()`: текст парсера называет неизвестное поле и форму тела, то есть
рассказывает про внутреннее устройство ровно так же, как ошибка репозитория, которую убрал
Decision 17."* Two contradictory conventions for the same error now live 90 lines apart in the
same package, and the later, better-reasoned one is the minority.

Task 4's justification for deferring — *"текст ошибки разбора JSON внутренних деталей не
содержит"* — is factually wrong for the `cannot unmarshal … into Go struct field X.Y.Z of type
T` form, which names internal Go types.

**Impact.** Information disclosure of internal type structure on four product endpoints; a
recorded architectural decision silently unenforced; no server-side record of malformed-body
400s.

**Fix.** Replace all four sites with the `access.go:150-154` pattern:

```go
if err := decodeJSON(r, &req); err != nil {
    log.Printf("api error: status=%d code=invalid_request err=%v", http.StatusBadRequest, err)
    writeAPIError(w, http.StatusBadRequest, "invalid_request")
    return
}
```

Extract it into a shared helper (`writeInvalidRequest(w, err)`) so a fifth call site cannot
diverge again, and add the assertion from `access_test.go:773-784`
(`containsAny(body, "unknown field", "invalid json", "json:", …)`) to `http_test.go` for each
of the four routes.

---

### C3. `generateToken()` swallows the `crypto/rand` error — a failure silently mints an all-zero session token

**Location:** `server/internal/handler/auth.go:702-706`.

```go
func generateToken() string {
    bytes := make([]byte, 32)
    _, _ = rand.Read(bytes)          // crypto/rand — error discarded
    return hex.EncodeToString(bytes)
}
```

**What is wrong.** This is the *only* generator for the refresh-cookie value, used both at
login (`auth.go:197`) and on every rotation (`auth.go:314`). If `crypto/rand.Read` ever fails,
`bytes` stays the zero slice and the function returns a constant, predictable
`"0000…0"` — 64 hex zeros — which is then hashed into `oauth_sessions.refresh_token_hash` and
set as the browser cookie. Every session created during such a window would collide on the same
hash and be mutually impersonatable. Per the code-reviewing severity anchors, a swallowed error
is `critical`; on a session-secret path it is unambiguously so.

Note the deliberate contrast: `service/costing.go:790-795` (`newChatID`) checks the identical
call and falls back — so the codebase knows the pattern; the auth path is the one that skips it.

**Impact.** Low probability, catastrophic blast radius, and completely silent. Also
inconsistent with the sibling function in `service`.

**Fix.** Return an error and fail the login/refresh closed:

```go
func generateToken() (string, error) {
    b := make([]byte, 32)
    if _, err := rand.Read(b); err != nil {
        return "", fmt.Errorf("generate session token: %w", err)
    }
    return hex.EncodeToString(b), nil
}
```

`newLoginSession` and `rotateSessionTokens` already return errors — propagate to the existing
`session_create_failed` / `session_update_failed` branches.

---

### C4. `Panel` is a ~955-line function inside a 1403-line file

**Location:** `client/src/pages/Panel.jsx:179-1134` (the `Panel` component), file total 1403 lines.

**What is wrong.** Severity anchor: *functions > 100 lines → critical*. `Panel` holds 18
`useState` hooks (`:182-200`), four `useEffect`s, eight `handle*` mutators, and the full JSX for
three mutually exclusive sections. The access-control feature made it materially worse: Task 7
added `status`/`access` state plus the bootstrap third call, and Task 8 added the admin nav item
and the `activeSection === "users"` branch (`:895-899`) — both landing in the same monolith.

The layering is inconsistent as a result: Task 8's admin section was correctly extracted into
`AdminUsersSection.jsx` with its own `useAdminUsers` hook, and Task 7's banner into
`AccessRequestBanner.jsx`, but the settings form (`:659-894`, ~235 lines of JSX) and the
workspace (`:900-1129`, ~230 lines) stayed inline. The file now demonstrates both conventions.

**Impact.** Any future change to the gate, the nav, or the section switch touches a file where
a reviewer cannot hold the state machine in their head; the `status`/`activeSection`/`access`
interaction (`:583-604`, `:627-635`, `:895-899`) is spread over 300 lines.

**Fix.** Extract along the seams the feature already created: `useePanelBootstrap()` (the
`status`/`profile`/`access` effect, `:204-250`), `usePanelData()` (the two guarded data
effects, `:260-338`), `<PanelSettingsSection>` (`:659-894`), `<PanelWorkspaceSection>`
(`:900-1129`). `Panel` then becomes the ~80-line shell it is trying to be, matching the shape
`AdminUsersSection.jsx` already uses.

---

## MAJOR

### M1. Panel bootstrap keeps `GET /auth/me`, which re-introduces the live Yandex round-trip Decision 1 exists to remove

**Location:** `client/src/pages/Panel.jsx:210-230`; `server/internal/handler/auth.go:250-270`
(`HandleMe` → `fetchYandexProfile`); `client/src/utils/yandexAuth.js:189-197`.

**What is wrong.** Decision 1's stated rationale: *"серверная проверка «кто вызывает» нужна на
каждом обращении, и она не должна зависеть от доступности Яндекса"*, replacing the live call to
`https://login.yandex.ru/info`. Identity now does live in `oauth_sessions` and
`GET /api/v1/access/me` returns `{login, display_name, email, role, has_access, …}` from the
database. Yet the panel bootstrap still runs three sequential calls:

```js
const ok = await checkAuthStatus();        // GET /auth/status   — DB only
const nextProfile = await fetchAuthProfile(); // GET /auth/me    — live HTTPS call to Yandex, 10s timeout
accessState = await fetchAccessState();    // GET /api/v1/access/me — DB only, returns login+display_name+email
```

`profile` is used for exactly two things: `profile?.login` → `userID` (`:202`) and
`profile?.name` in the greeting (`:642`). Both are already in `accessState` as `login` and
`display_name`. `/auth/me` is redundant, and it is the *only* remaining live Yandex dependency
on the panel's hot path.

Worse, the bootstrap wraps all three in one blanket `catch` that redirects to `/` (`:231-239`)
with **no logging of any kind**. So when Yandex is slow or down, `HandleMe` returns 502
`yandex_profile_failed`, and every user with a perfectly valid session and granted access is
silently bounced out of the panel to the landing page, with no message and nothing in the
console to diagnose it. The in-code comment at `:232-234` asserts "сюда попадает только 401 или
сетевой сбой" — that is wrong; 502 from `/auth/me` and 404/500 from `/api/v1/access/me` land
here too.

**Impact.** Third-party outage → app-wide logout with no diagnosis. Three sequential round-trips
where one suffices. Decision 1's benefit is realised on the API surface and thrown away on the
one page that matters.

**Fix.** Drop `checkAuthStatus()` and `fetchAuthProfile()` from the bootstrap; derive
`userID` from `accessState.login` and the greeting from `accessState.display_name`. A 401 from
`/api/v1/access/me` already means "no session", which is what `checkAuthStatus` was testing.
Separately, split the catch: redirect only on `error.status === 401`, and render an explicit
"сервис недоступен, попробуйте обновить" state for everything else, with `console.error(error)`
in both branches.

### M2. `normalizePath` still lets a per-chat identifier become a Prometheus label — Decision 18 closed the wrong half

**Location:** `server/internal/handler/metrics.go:23-67`; path shape from
`server/internal/handler/http.go:78-86`; ID format from `server/internal/service/costing.go:790-796`.

**What is wrong.** `looksLikeID` collapses a segment only if it parses as an integer, or is
longer than 8 chars **and contains a hyphen** (`metrics.go:57-67`). `newChatID` returns
`hex.EncodeToString` of 12 random bytes — 24 hex chars, no hyphen, not an integer. So
`looksLikeID` returns `false` and the chat ID is emitted verbatim as a Prometheus `path` label
for `/api/v1/users/chats/<24-hex>/calculate`, `/calculations`, `/restore`, and `DELETE`.

Decision 18 examined `normalizePath` in detail and declared the problem closed: *"Существующая
уязвимость `normalizePath` к коротким логинам без дефиса … не входит в объём: тот маршрут
дополнительно защищён Decision 6, а сегмент `userID` там после Decision 6 исчезает вовсе."*
That reasoning covers the `{userID}` segment and misses that Task 6's reshaping moved `{chatID}`
from index 5 to index 4 — into the same uncollapsed position, on four routes, with an identifier
that is by construction unique per chat.

**Impact.** Unbounded label cardinality growth in `HTTPRequestsTotal` and
`HTTPRequestDuration` — one new time series per chat per method per status. Authenticated-only
now (so not anonymously exploitable, unlike the admin-login vector Decision 18 did fix), but it
grows monotonically with normal product use and never shrinks; Prometheus memory is the failure
mode.

**Fix.** Extend the third rule the same way Decision 18 extended it for `/admin/users/`: collapse
index 4 when `parts[0..3] == ["api","v1","users","chats"]`, regardless of the value's shape.
`isAdminUserLogin` already has exactly the right shape to copy.

### M3. `MemoryRepository` and `PostgresRepository` diverge on validation, so contract tests pass on a store that cannot reject bad data

**Location:** `server/internal/repository/memory.go:293-338` vs.
`server/internal/repository/postgres.go:755-810`; constraint in
`server/migrations/004_access_control.up.sql:22-23` (`chk_access_requests_status`,
`fk_access_requests_user`).

**What is wrong.** The two implementations of `AccessRequestRepository` are behaviourally
identical only on the happy path. On the invalid path they differ:

- `MemoryRepository.DecideRequest(ctx, login, status, decidedBy)` (`memory.go:323-338`) writes
  whatever string it is handed. `PostgresRepository.DecideRequest` (`postgres.go:797-810`) hits
  `chk_access_requests_status CHECK (status IN ('pending','approved','rejected'))` and returns an
  error. A typo — `"aproved"` — compiles (the statuses cross the service/repository boundary as
  bare `string`, see `service/access.go:87` and `:221-224`), passes every contract test on
  Memory, and only fails in production on MariaDB.
- `MemoryRepository.CreateRequest` (`memory.go:293-307`) happily creates a request for a login
  with no user row; Postgres rejects it on `fk_access_requests_user`.
- `MemoryRepository` methods take `_ context.Context` throughout — cancellation and deadlines
  are silently ignored, where `PostgresRepository` threads them via `WithContext`.

The audit brief asked specifically whether the two implementations "diverge in method
signatures or leak storage-layer types". They do neither — signatures and domain types are
clean. They diverge in *contract strictness*, which is the more expensive kind, because
`access_repo_test.go` runs the same suite against both and `TEST_DB_DSN` is the only thing that
makes the strict half run at all.

**Impact.** The dev environment (which, per M4, is Memory-backed by default) accepts data the
production store rejects. A status-string regression is structurally undetectable on Memory.

**Fix.** Introduce a `service.RequestStatus` string type with the three constants exported (they
are unexported today at `service/access.go:22-26` and re-declared in `postgres.go:78`), and have
`DecideRequest` take it instead of `string`. Add explicit validation to
`MemoryRepository.DecideRequest` and a user-existence check to `MemoryRepository.CreateRequest`
so the in-memory store enforces the same invariants the schema does, and add negative contract
cases for both.

### M4. Dev `docker-compose.yml` runs the whole feature on `MemoryRepository` while applying migrations to MariaDB

**Location:** `docker-compose.yml:5-30` (no `APP_STORAGE`), against
`server/internal/config/config.go:59` (`envOrDefault("APP_STORAGE", "memory")`) and
`server/cmd/main.go:135-138`.

**What is wrong.** The dev stack connects to MariaDB, runs `migrations.Run` (which seeds
`RoGogDBD` and `irina2000aleshina` as admins into `users`), and then serves every request from
`MemoryRepository`, which knows nothing about that table. The tech-spec notes this in passing in
the AVP (*"Для проверок против БД нужно явно выставить `APP_STORAGE=mysql`"*) but the compose
file that ships was not changed.

Consequences specific to this feature: on every `docker compose up`, the admin list is empty,
both seeded administrators do not exist, `GET /api/v1/admin/users` returns `{"items":[]}` to
nobody (because nobody has `role=admin`), and every user is a fresh no-access user whose granted
access evaporates on restart. The dev stack also supplies none of `CORS_ALLOWED_ORIGINS`,
`COOKIE_SECURE`, `COOKIE_SAMESITE`, `YANDEX_CLIENT_ID/SECRET`, or `YANDEX_REDIRECT_URI`, so
login cannot complete there either (compounding C1).

**Impact.** The `Verify-smoke` and `Verify-user` steps written into Tasks 5, 7, and 8 do not
describe the environment the repo actually produces. Anyone reproducing this feature locally
will conclude the admin seeding is broken.

**Fix.** Set `APP_STORAGE: ${APP_STORAGE:-mysql}` in `docker-compose.yml` (the DB service is
already a healthcheck dependency of `app`), and add the auth/cors variables with dev-safe
defaults: `CORS_ALLOWED_ORIGINS: ${CORS_ALLOWED_ORIGINS:-http://localhost:5173}`,
`COOKIE_SECURE: "false"`.

### M5. `AccessRequestRepository.GetRequest` is dead production code carried by two implementations and two stubs

**Location:** declared `server/internal/service/access.go:83-84`; implemented
`postgres.go:772-792` and `memory.go:309-318`; stubbed in `service/access_test.go:159` and
`handler/middleware_test.go:227`; exercised only by `repository/access_repo_test.go`.

**What is wrong.** No production code path calls it. `AccessService` never touches
`requestRepo.GetRequest` — `GetAccessState` reads the status off the `UserRecord` LEFT JOIN
(`access.go:119-135`), and `access_test.go:515,551,594` actively asserts that it is *not* called.
It is a method on a narrow interface that exists to serve one caller and has none.

Task 3's own report flagged this and asked for a decision that was never made:
*"`AccessRequestRepository.GetRequest` may turn out genuinely unused … worth confirming they get
used or recording them as reserved"* (`decisions.md:65`). Nine tasks later it is still
unresolved, and it now costs four implementations plus a contract test written explicitly
"сверх списка TDD Anchor" to cover a branch nothing reaches (`access_repo_test.go:640-647`).

**Impact.** Every future repository implementation must implement a method with no consumer;
the narrow-interface principle the tech-spec cites (`costing.go:168-183` house style) is
weakened by an interface that is not, in fact, narrowed to its use.

**Fix.** Either delete it from the interface and both implementations (Phase 2's notifier does
not need it — `access.go` in `handler` never sees the request repo), or record it in
`decisions.md` as deliberately reserved for the Phase 2 admin request-detail view, with the
task number that will consume it.

### M6. CORS advertises `GET, POST, OPTIONS` while the client ships a `DELETE` — and Decision 16 was reasoned from that same stale premise

**Location:** `server/internal/handler/cors.go:33`; `client/src/utils/panelApi.js:57-61`;
route at `server/internal/handler/http.go:75-77`.

**What is wrong.** `WithCORS` sets
`Access-Control-Allow-Methods: "GET, POST, OPTIONS"`. `panelApi.deleteChat` issues
`DELETE /api/v1/users/chats/{chatID}`, and `handleUsers` routes it. In a same-origin deployment
this never surfaces (no preflight), which is why it has survived — but Decision 16 chose POST over
PATCH *specifically* on the grounds that *"CORS-обёртка объявляет только `GET, POST, OPTIONS`
… PATCH или PUT потребовали бы расширить список"*. That premise is already false in the shipped
code: the contract is broken for `DELETE` today.

A second, related gap: `WithCORS` short-circuits `OPTIONS` at `cors.go:37-40` **only when
`AllowedOrigins` is non-empty** — with an empty list it returns `next` unwrapped
(`cors.go:13-15`), so a preflight falls through to `RequireSameOrigin`, which treats `OPTIONS` as
mutating (`middleware.go:205`) and answers 403. Same-origin deployments never preflight, so this
is latent, but it means the "empty allowlist" configuration silently has no working CORS at all.

**Impact.** Any move to a split-origin deployment (`VITE_API_URL` pointing elsewhere) breaks
chat deletion and all preflights, in a way that will read as a client bug. And a recorded
decision rests on a stated fact about the code that the code contradicts.

**Fix.** Add `DELETE` to `Access-Control-Allow-Methods`, handle `OPTIONS` unconditionally in
`WithCORS` (move the short-circuit above the `len(config.AllowedOrigins) == 0` early return),
and add a line to Decision 16 noting that the method list was corrected.

### M7. nginx `/auth/*` routing is a copy-pasted block repeated fourteen times across two files, one of them a heredoc inside a deploy script

**Location:** `client/nginx.conf:21-67` (five identical blocks) and
`.github/workflows/deploy.yml:149-219` (eight, inside a `cat > … <<'EOF'`).

**What is wrong.** Each `/auth/*` route needs its own `location = /auth/x { proxy_pass …;
proxy_set_header Host $host; proxy_set_header X-Real-IP …; proxy_set_header X-Forwarded-For …;
proxy_set_header X-Forwarded-Proto …; }` block, duplicated verbatim. Task 9 added
`/auth/refresh` and had to write the same six lines into two files. The prod copy is embedded in
a YAML heredoc, so it has no syntax checking, no linting, and no diffability against the dev
copy — the two lists have already drifted (`client/nginx.conf` has no `/metrics` deny; the prod
one has `/auth` and `/auth/` SPA routes the dev one lacks).

**Impact.** Adding a `/auth/*` route (Phase 2 will not, but any future auth work will) requires
editing two files in different languages and redeploying, with nothing to catch an omission —
which is precisely the failure mode the tech-spec's own risk table records for `/auth/refresh`
("не проксируется … возвращает SPA с кодом 200 — клиентский retry молча не работает").

**Fix.** Collapse to one regex location per file —
`location ~ ^/auth/(status|me|logout|refresh|yandex/code)$ { … }` — and factor the five
`proxy_set_header` lines into an `include /etc/nginx/proxy_params;` snippet shipped alongside.
Move the prod config out of the heredoc into a real file next to `client/nginx.conf`, uploaded
by the existing `scp-action` step (the workflow already scp's `monitoring/`).

### M8. Bootstrap and section errors are caught and discarded without ever reaching the console

**Location:** `client/src/pages/Panel.jsx:231-239`;
`client/src/components/AdminUsersSection.jsx:99-105, 128-132`;
`client/src/components/AccessRequestBanner.jsx:25-34`.

**What is wrong.** Four `catch` blocks in the new client code bind nothing (`catch {`) or bind
and drop the error. They set user-facing state, so they are not *silent* to the user — but the
error object itself is destroyed, so there is no console output, no status code, and no way to
tell a 403 from a 500 from a network failure when a user reports "не удалось загрузить список
пользователей". The same codebase's `yandexAuth.js:90-92, 100-103, 130-132, 141-144` does log
(`console.log("…", error)`), so this is an inconsistency introduced by Tasks 7 and 8, not a house
style.

`Panel.jsx:231-239` is the worst of the four: it is the single funnel for three different
failures with three different meanings and it neither logs nor distinguishes them (see M1).

**Impact.** Production client failures in the newest, least-exercised code paths are
undiagnosable from a user's browser console.

**Fix.** `catch (error) { console.error("<context>", error); … }` in all four, and in
`AdminUsersSection.toggleAccess` include `error.status` in the notice when it is 403 vs 5xx —
the distinction matters to an admin (the former means their own role changed).

---

## MINOR

### m1. `nullableString` is defined twice, in two packages, with two different bodies
`server/internal/handler/auth.go:227-233` returns a zero `sql.NullString` for empty input;
`server/internal/repository/postgres.go:835-840` returns `{String: value, Valid: value != ""}`.
Observably identical today (both yield `Valid:false` for `""`), but they are two independent
statements of one rule ("empty means NULL") that the next edit can split. Extract to a shared
helper or, better, keep it in `repository` only and have `auth.go` build the `sql.NullString`
inline where the intent is local.

### m2. `panelApi.js` does not URL-encode path segments while `accessApi.js` does
`accessApi.js:74` uses `encodeURIComponent(login)`; `panelApi.js:58, 64, 70, 73` interpolate
`chatID` raw. Chat IDs are server-generated hex today so nothing breaks, but the two files state
opposite conventions for the same problem. Encode in both.

### m3. `panelApi.js` header merge is defeated by spread ordering
```js
authFetch(path, {
  headers: { "Content-Type": "application/json", ...(options.headers || {}) },
  ...options,      // panelApi.js:9 — re-overwrites `headers` wholesale
})
```
`...options` comes last, so any caller passing `headers` silently discards the merged
`Content-Type` rather than adding to it. No caller does today; the merge on line 5-8 is
therefore dead code that reads as if it works. Move `...options` above `headers`.

### m4. Two exported client functions have no callers
`panelApi.js:63-67` (`restoreChat`) and `:78-82` (`analyzeMarketWithAI`) are exported and
unreferenced anywhere in `client/src`. `restoreChat` corresponds to a live server route
(`http.go:78-80`) with no UI. Either wire them up or delete them; as-is they are two more paths
that Task 6's URL migration had to keep correct for no benefit.

### m5. `openapi.yaml` is incomplete for routes this feature touched
`server/api/openapi.yaml` documents 12 operations. Missing: `DELETE /api/v1/users/chats/{chatID}`
and `POST /api/v1/users/chats/{chatID}/restore` (both live, both re-shaped by Task 6, both
absent before too — so a pre-existing gap that Task 6's "документация расходится с кодом"
mandate did not close), and the entire `/auth/*` surface. Also `GET /api/v1/access/me`
(`:208-222`) documents only 200 and 401, while the handler can return 404 (`ErrNotFound` from
`GetAccessState` when the `users` row is absent) and 500 — and the client treats both as
"session dead, redirect to /".

### m6. `EnsureUser` overwrites a stored email with NULL when Yandex omits `default_email`
`postgres.go:615-635` unconditionally assigns `nullableString(email)` to the update set;
`memory.go:239-241` does the same with the raw string. Decision 12 anticipates a missing
`default_email` ("сохраняется пустой email") but not that a *later* login without the scope
erases an email captured earlier. Given Phase 2's notification depends on that column, guard the
update: only overwrite when the incoming value is non-empty (`IF(VALUES(email) = '', email,
VALUES(email))` or equivalent).

### m7. `deploy.yml` guards only `CONTACT_EMAIL` against being unset
`.github/workflows/deploy.yml:251` writes `CONTACT_EMAIL=${CONTACT_EMAIL:-}` while
`CORS_ALLOWED_ORIGINS`, `COOKIE_SECURE`, `COOKIE_SAMESITE`, `COOKIE_DOMAIN`, and `LOG_LEVEL` on
the surrounding lines have no `:-` default, under `set -euo pipefail`. Decision 8 explicitly
identifies `CORS_ALLOWED_ORIGINS` as an optional secret. Task 9 added the guard for its own
variable and did not generalise it. Apply `${VAR:-}` uniformly, or drop it from
`CONTACT_EMAIL` and rely on the action exporting empty strings — but pick one.

### m8. `docker login -p "${GHCR_TOKEN}"` exposes the token in the process table
`.github/workflows/deploy.yml:267`. Use
`echo "${GHCR_TOKEN}" | docker login ghcr.io -u "${GHCR_USERNAME}" --password-stdin`. Pre-existing,
not introduced by Task 9, but the file was in scope.

### m9. `handleMarketFeedback`'s 503 uses free text where every other error uses a category slug
`http.go:212-214` returns `"deepseek integration is not configured"` while
`classifyDomainError` (`:307-308`) maps the same status to the slug `"service_unavailable"`.
Two response vocabularies for one status code. Use the slug and log the detail.

### m10 (observation, no action needed). `MemoryRepository` ignores `context.Context` throughout
(`memory.go` uses `_ context.Context` on all eleven methods). Correct for an in-memory store and
consistent within the file; noted only because the audit brief asked about `ctx`-first
consistency — the *signature* convention is honoured everywhere; only the honouring of
cancellation differs, which is expected.

---

## Dimensions checked and found clean

These are the specific invariants tasks/10.md asked about. Each was verified by reading the
current code, not the spec.

**Shared resources — single instance (tech-spec Architecture table).** Clean.
`*sql.DB` is created exactly once at `cmd/main.go:25` (`db.Open`) and threaded to
`migrations.Run` (`:31`) and `auth.NewStore` (`:41`). `*gorm.DB` is created exactly once at
`:140` (`db.OpenGORM`), inside `buildRepositories`, which is itself called once at `:35`; the
returned `*PostgresRepository` is shared by all five repository interfaces (`:146`) — the
comment at `:123-125` states this intent explicitly and the code matches. `AccessService` is
constructed once at `:44` and passed to `NewAuthHandler` (`:45`), `NewAccessHandler` (`:61`),
and `newMux` → `RouteDeps.AccessService` (`:63`, `:101`) — one instance, three consumers,
exactly as the table specifies. `SMTPNotifier` is Phase 2 and correctly absent. No constructor
is called twice anywhere; `grep` for `db.Open`, `db.OpenGORM`, `NewAccessService`,
`NewMemoryRepository`, `NewPostgresRepository` across `cmd/` returns one production call site
each. `cmd/main_test.go:TestBuildRepositories_ProvidesAccessRepositories` pins it.

**Middleware chain vs. the routing table.** Clean, and better than the task brief assumed —
the wiring lives in `server/internal/handler/routes.go` (`BuildRoutes`), not `main.go`.
`routes.go:54-92` matches the Architecture table line for line: `/api/v1/users/` →
`sameOrigin → RequireAuth → RequireAccess` (`:56-58`); `/api/v1/admin/` →
`sameOrigin → RequireAuth → RequireAdmin` (`:62-64`); `/api/v1/access/` →
`sameOrigin → RequireAuth`, no `RequireAccess`, with the reason in a comment (`:66-71`);
`POST /auth/yandex/code`, `/auth/refresh`, `/auth/logout` → `sameOrigin` only (`:87-89`);
`/auth/status`, `/auth/me`, `/health`, `/metrics` unwrapped (`:73-78`, `:91-92`).
`POST /auth/yandex` is absent, with the reason recorded (`:80-82`). `main.go:63-67` keeps
`CORS → Metrics → mux` on the outside, which Decision 18 depends on. Both `main_test.go`
(against the real `newMux`) and the handler tests (against `BuildRoutes`) assert the chain
rather than a copy of it — the failure mode Task 4 hit and Task 5 fixed is genuinely closed.

**Decision 13 — `userModel` invariant.** Clean. `postgres.go:25-28` still has only `ID` and
`CreatedAt`; the invariant is stated as a nine-line comment at `:15-24` naming the constraint
(`chk_users_role`), the three write paths that would break (`UpsertSettings`, `CreateChat`,
`AppendCalculation`), and the live MariaDB error code. Access columns live on a separate
`userAccessModel` (`:38-44`) mapped to the same table, plus a read-side `userAccessRow`
(`:53-62`) for the JOIN. `upsertUser` (`:842-847`) still inserts only `userModel`.

**Decision 6 — no `userID` from any URL segment.** Clean. `grep` for `userID` in non-test
handler code returns only `http.go:58` (`userID := identity.Login`) and its propagation to the
nine sub-handlers as a parameter. `handleUsers` reads `parts[0]` as the *resource* name
(`:60`), and the legacy shape `/api/v1/users/ivanov/chats` matches no switch case and falls to
`404 route not found` (`:90-92`), as its comment claims. `AccessHandler.handleSetAccess` takes
`{login}` from the path (`access.go:145`) but that is the admin's *target*, not the caller —
the caller comes from `identity.Login` (`:140`, `:164`), which is the correct split.

**Compile-time interface assertions, both implementations.** Clean.
`postgres.go:153-157` and `memory.go:44-48` each carry all five:
`UserSettingsRepository`, `ChatRepository`, `ChatCalculationRepository`, `UserRepository`,
`AccessRequestRepository`. Neither interface was widened by Tasks 5/6 without both
implementations following — the method sets in `service/access.go:66-88` match both structs
exactly.

**House style of the new interfaces.** Clean. `UserRepository` and `AccessRequestRepository`
are declared in `service` (`access.go:66-88`), implemented in `repository`, take
`ctx context.Context` first on every method, and pass only domain types
(`service.UserRecord`, `service.AccessRequest`) across the boundary — no `gorm.DB`,
`sql.NullString`, or model struct escapes either implementation. `toUserRecord` in each file
(`postgres.go:822-833`, `memory.go:358-373`) is the conversion boundary. This mirrors
`costing.go:168-183` as the spec requires. (The one substantive divergence is behavioural, not
structural — see M3.)

**Decision 17 inside `writeAPIDomainError`.** Clean *within the function*.
`classifyDomainError` (`http.go:286-314`) returns a fixed slug on all nine branches —
`invalid_request` / `forbidden` / `not_found` / `conflict` / `rate_limited` /
`service_unavailable` / `timeout` / `internal_error` — plus a nil-guard first line; the raw
error goes to `log.Printf` at `:282` and nowhere else. `http_test.go:20-60` proves it with an
SQL-DSN-shaped payload across every branch. The violation is at four *call sites that bypass
this function* — see C2.

**Decision 10 in one place.** Clean. `hasEffectiveAccess` (`service/access.go:233-235`) is the
sole implementation of `has_access || role == "admin"`. `RequireAccess` explicitly delegates
rather than re-implementing (`middleware.go:116-131`), `ListUsers` deliberately does *not*
apply it and says why (`access.go:137-142`), and the client mirror is isolated in one function
with a comment acknowledging it is a mirror (`Panel.jsx:176-177`). No second copy exists.

**Migrations 004/005.** Read and consistent with the spec. `004` matches the Data Models block
verbatim including the two seeded admin logins; every statement is `IF NOT EXISTS`-guarded.
`005` (Decision 19, added after tasks/10.md was written) drops and restores all four FKs around
the collation change, uses the `FOREIGN KEY IF NOT EXISTS` form the comment warns about, and is
applied by `test.yml:78-85` in CI along with the rest.

**CI wiring.** Clean. `deploy.yml:13-17` uses the in-file `test:` job via
`uses: ./.github/workflows/test.yml` with `needs: test` on the deploy job — the mechanism the
tech-spec prescribes, correctly implemented. `test.yml` supplies a real `mariadb:11.4` service
container and a non-empty `TEST_DB_DSN` (`:95`), so the contract suite actually runs instead of
`t.Skip`-ing, and `-v` is on (`:96`) so a skip is visible in the log. Concurrency grouping
correctly excludes `workflow_call` from cancellation (`:22-24`), with the reasoning recorded.

---

## Cross-task drift ledger

Items that individual task reports promised, deferred, or asserted, and that the merged code
contradicts:

| Recorded in | Claim | Reality |
|---|---|---|
| `decisions.md:152` (Task 4) | "`err.Error()` остался в трёх местах `http.go` … если Task 6 будет переписывать эти маршруты — стоит свести их к фиксированным кодам" | Task 6 rewrote all of them; four sites still leak. Task 4's premise ("внутренних деталей не содержит") is false for `cannot unmarshal … Go struct field`. → **C2** |
| `decisions.md:65` (Task 3) | "`GetRequest` may turn out genuinely unused … worth confirming they get used or recording them as reserved" | Never resolved; still unused, still implemented four times. → **M5** |
| `decisions.md:186` (Task 5) | `r.Host` fallback "принято как есть, инвариант инфраструктуры" | Assessed against `docker-compose.prod.yml` only; `vite.config.js` (`changeOrigin: true`) and `nginx` (`$host` strips port) both break it. → **C1** |
| Decision 18 | "Существующая уязвимость `normalizePath` … не входит в объём: сегмент `userID` после Decision 6 исчезает вовсе" | True for `userID`; Task 6 moved `chatID` into the same uncollapsed index. → **M2** |
| Decision 16 | "CORS-обёртка объявляет только `GET, POST, OPTIONS`" (used as the reason to pick POST) | The shipped client already issues `DELETE` through that wrapper. → **M6** |
| Decision 1 | Identity must not depend on Yandex availability | `GET /auth/me` still makes the live call and the panel still calls it on every load. → **M1** |
| tech-spec AVP note | "`docker-compose.yml` не задаёт `APP_STORAGE` … для проверок против БД нужно явно выставить `APP_STORAGE=mysql`" | Noted but never fixed; dev ships Memory-backed with migration-seeded admins invisible. → **M4** |

---

## Files read

**Server (all read in full):** `migrations/004_access_control.up.sql`,
`migrations/005_identity_collation.up.sql`, `internal/service/access.go`,
`internal/service/costing.go` (interfaces, errors, ID generation), `internal/repository/postgres.go`,
`internal/repository/memory.go`, `internal/auth/store.go`, `internal/handler/auth.go`,
`internal/handler/middleware.go`, `internal/handler/http.go`, `internal/handler/access.go`,
`internal/handler/routes.go`, `internal/handler/metrics.go`, `internal/handler/cors.go`,
`internal/config/config.go`, `cmd/main.go`, `cmd/main_test.go`, `api/openapi.yaml`.
Test files (`service/access_test.go`, `repository/access_repo_test.go`, `auth/store_test.go`,
`handler/auth_test.go`, `handler/access_test.go`, `handler/middleware_test.go`,
`handler/http_test.go`) were read selectively for cross-checking production invariants only —
test quality is Task 12's scope.

**Client (all read in full):** `src/utils/accessApi.js`, `src/utils/yandexAuth.js`,
`src/utils/panelApi.js`, `src/pages/AuthCallback.jsx`, `src/pages/Panel.jsx`,
`src/components/AccessRequestBanner.jsx`, `src/components/AdminUsersSection.jsx`,
plus `vite.config.js` (not in the brief; required to assess C1).

**Deploy (all read in full):** `.github/workflows/deploy.yml`, `.github/workflows/test.yml`,
`docker-compose.prod.yml`, `docker-compose.yml`, `client/nginx.conf`.
