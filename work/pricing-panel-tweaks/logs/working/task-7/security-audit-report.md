# Security Audit — pricing-panel-tweaks (Task 7, full-feature)

**Date:** 2026-08-16
**Auditor:** security-auditor (Task 7, Audit Wave)
**Scope:** holistic full-feature OWASP Top 10 pass over the assembled final state of the
feature, not a diff-scoped per-task review.
**Commit range audited:** `4edea63^..HEAD` (`4edea63` … `19540c6`), i.e. Tasks 1-5 plus the
ad-hoc `orderForm` fix and the vitest CVE bump.

---

## Verdict

> **No Critical findings. No High findings.**
> Nothing in this feature blocks release from a security standpoint.

| Severity | Count |
|----------|-------|
| Critical | 0 |
| High     | 0 |
| Medium   | 1 (pre-existing, not introduced by this feature) |
| Low      | 2 (1 accepted per tech-spec Risks table, 1 pre-existing class) |

The `deepseek.go:404-406` accepted-risk item from tech-spec's Risks table **still holds** —
see the dedicated section below for the evidence.

---

## Scope of files reviewed

Complete set of files created or modified by the feature (`git diff --stat 4edea63^ HEAD`,
excluding `work/`):

| File | Change | Read in full |
|------|--------|--------------|
| `client/src/pages/Panel.jsx` | +676/-49 across 6 commits (Tasks 1-5 + ad-hoc fix) | yes — full diff plus the relevant final-state regions |
| `client/src/pages/Panel.validation.test.js` | new, 251 lines | yes |
| `client/package.json` | `vitest` devDependency + `test` script | yes |
| `client/package-lock.json` | vitest tree | reviewed via `npm audit` + pre/post lockfile comparison |

Read-only reference files (unmodified by this feature, used as trust-boundary comparators):

- `server/internal/service/deepseek.go` — prompt template, lines 395-410; `sortedKeysFrom*`
  helpers at 553-580.
- `server/internal/service/costing.go` — `validateSettings` (593), `normalizeSettings` (659),
  `DefaultUserSettings` (210).
- `server/internal/handler/http.go` — route table (23-100), `handleCalculate` (164),
  `handleMarketFeedback` (211).
- `client/src/utils/panelApi.js`, `client/src/utils/yandexAuth.js` — the existing save transport.

**Coverage note:** the per-task security reviews under `logs/working/task-{1..5}/` each
covered exactly one commit. The ad-hoc fix commit `569c72f` (stale `orderForm` reference)
landed *after* `task-5/security-auditor-1.json` was produced and was therefore never
security-reviewed. It is reviewed here for the first time — see "Ad-hoc fix" below. (Separately,
the `decisions.md` prose for Tasks 2-5 says reviews were "not run"; the JSON reports for all
four in fact exist, committed later in `5ef014d`, `c1eb100`, `c9ec79f`. That is a stale-prose
bookkeeping issue, not a security issue.)

---

## Verify-smoke output

Both checks from the task file, run verbatim, with actual output:

```
$ grep -n "dangerouslySetInnerHTML" client/src/pages/Panel.jsx
(exit code: 1  — 1 = no matches)

$ grep -rn "dangerouslySetInnerHTML\|innerHTML" client/src/
(exit code: 1 — 1 = no matches anywhere in client/src)

$ git log --oneline 4edea63^..HEAD -- server/internal/service/deepseek.go
(no output = file untouched by every commit of this feature)

$ git diff --stat 4edea63^ HEAD -- server/
(no output = no backend file changed at all)

$ git log -1 --format="%h %ad %s" --date=short -- server/internal/service/deepseek.go
5a777f9 2026-04-24 feat(monitoring): integrate Prometheus and Grafana for system and AI metrics
```

Supporting checks run alongside:

```
$ cd client && npx vitest run
 RUN  v3.2.7 /home/makar/PoshivOn/client
 ✓ src/pages/Panel.validation.test.js (32 tests) 6ms
 Test Files  1 passed (1)
      Tests  32 passed (32)

$ cd client && npx vite build
✓ 53 modules transformed.
✓ built in 529ms

$ git status --porcelain
(clean)
```

**Both smoke checks pass.** No unescaped-render sink exists anywhere in `client/src`, and
`deepseek.go`'s last modification (`5a777f9`, 2026-04-24) predates this feature entirely —
the accepted-risk trust boundary was not silently modified.

---

## Re-confirmation of the accepted `deepseek.go:404-406` risk

**Statement: the tech-spec Risks-table assessment STILL HOLDS. No change required, no
remediation proposed.**

Evidence, against the three sub-questions the task file asks:

**(a) Is the file genuinely untouched?** Yes. `git log 4edea63^..HEAD -- server/internal/service/deepseek.go`
returns nothing, and more strongly `git diff --stat 4edea63^ HEAD -- server/` returns nothing —
this feature changed zero backend files of any kind. The prompt-building code at
`deepseek.go:404-406` is byte-identical to its state before Task 1:

```go
strings.Join(sortedKeysFromGarments(settings.Garments), ", "),
strings.Join(sortedKeysFromMaterials(settings.Materials), ", "),
strings.Join(sortedKeysFromUrgency(settings.Urgency), ", "),
```

Garment/operation/material names are still string-interpolated into the `%s` slots of the
prompt template with no escaping, delimiting, or content restriction — exactly as before.

**(b) Is the blast radius still self-scoped?** Yes, and this was verified end-to-end rather
than assumed. Both call sites of `AnalyzeMarketFeedback` obtain their settings the same way:

- `http.go:178` (`handleCalculate`) → `h.costing.GetUserSettings(r.Context(), userID)`
- `http.go:225` (`handleMarketFeedback`) → `h.costing.GetUserSettings(r.Context(), userID)`

`userID` is `identity.Login`, taken from the session identity that `RequireAuth`/`RequireAccess`
put in the context (`http.go:53-56`); there is no user-controlled owner segment in the path or
body (`decodeJSON` uses `DisallowUnknownFields`, so a stray `user_id` is a 400). Therefore the
names that reach the prompt are **always the requesting user's own** names, and the model's
answer is returned only to that same user. There is no cross-user, cross-tenant, or
server-side-effect exposure: the AI result is advisory display data
(`result.AIFeedback`), attached *after* `CalculateInChat` has already computed the price, so a
successful prompt injection cannot alter a price, authorize anything, or trigger a tool call.
The worst realistic outcome remains "the admin gets misleading market advice they induced
themselves", which is the assessment in the Risks table.

**(c) Do the new add-forms introduce any additional unescaped sink?** No. The AI feedback
itself is rendered by `CalculationAIFeedback` (`Panel.jsx:1886+`) through ordinary JSX
interpolation with numeric coercion on the price fields, and the repo-wide grep above shows
zero `dangerouslySetInnerHTML`/`innerHTML` anywhere in `client/src` — so neither the injected
name nor the model's reply can become an XSS sink. The only new thing this feature contributes
is *discoverability*: a name that previously required a hand-crafted `POST /api/v1/users/settings`
can now be typed into a visible "Добавить" field. That is a usability change to an existing,
already-authenticated, already-self-scoped path — it does not move the trust boundary.

---

## Findings

### Medium

#### SEC-M1 — Build-toolchain dependency advisories (pre-existing, dev-only)

- **Where:** `client/package-lock.json` (transitive), surfaced by `npm audit` in `client/`.
- **What:** 4 open advisories: `rollup` <4.59.0 arbitrary file write via path traversal
  (npm: high, GHSA-mw96-cpmx-2vgc); `vite` <=6.4.2 path traversal in optimized-deps `.map`
  handling (npm: high, GHSA-4w7w-66w2-5vf9); `esbuild` <=0.24.2 dev-server request forgery
  (moderate, GHSA-67mh-4wv8-2f99); `@babel/core` <=7.29.0 arbitrary file read via
  `sourceMappingURL` (low, GHSA-4x5r-pxfx-6jf8). `npm audit` summary: *4 vulnerabilities
  (1 low, 1 moderate, 2 high)*.
- **Not introduced by this feature.** All four packages are present in the pre-feature
  lockfile (`git show 4edea63^:client/package-lock.json` contains `node_modules/rollup`,
  `node_modules/@babel/core`, `node_modules/esbuild`), and `vite: ^5.4.10` /
  `@vitejs/plugin-react: ^4.3.4` were already direct devDependencies before Task 1. The new
  `vitest ^3.2.6` (3.2.7 installed) adds **no** advisory of its own — no audit node points at
  `node_modules/vitest`. The one advisory vitest *did* introduce (GHSA-5xrq-8626-4rwp, the
  vitest 2.x UI server) was already remediated in follow-up commit `7a2aeee`, which this audit
  re-verified: installed version is 3.2.7 and it no longer appears in `npm audit`.
- **Severity rationale — Medium, not High:** every affected package is build-time/dev-server
  only. None ships in the production bundle (`vite build` output is `index.html` + one CSS +
  one JS chunk), and exploitation requires an attacker to reach a developer's local dev server
  or feed a malicious source map into a local build. Contextual severity for this repo is
  therefore Medium despite npm's High ratings.
- **Recommendation:** run `npm audit fix` in `client/` — `rollup` and `@babel/core` have
  non-breaking fixes available. `vite`/`esbuild` only clear via `npm audit fix --force`
  (vite 8.x, a major bump) — that is a separate maintenance decision, explicitly **out of
  scope for this feature**, and was already flagged as carried-forward debt in Task 1's
  security review and the Task 1 decisions entry.

### Low

#### SEC-L1 — Garment/operation names have no length or character-class bound (accepted)

- **Where:** `Panel.jsx` `GarmentAddForm` name input (no `maxLength`, no pattern),
  `OperationAddForm` name input (same); sink at `deepseek.go:404-406`; server-side
  `validateSettings` (`costing.go:611, 634`) only rejects `strings.TrimSpace(name) == ""`.
- **Attack vector:** an authenticated panel admin adds a garment/operation whose *name* is a
  multi-kilobyte string or contains instruction-shaped text ("ignore previous instructions,
  …"). It is saved verbatim as a map key, and on the next masterpiece-mode calculation it is
  joined into the DeepSeek prompt unescaped.
- **Impact:** self-inflicted only — inflated token spend on the operator's own DeepSeek
  budget, and misleading advisory output shown back to the same user. This is the same
  accepted item as the Risks-table row re-confirmed above, recorded here as a finding purely
  so the report's severity list is complete. **Accepted, not to be fixed in this feature**
  (fixing it means editing `deepseek.go` and/or `validateSettings`, which user-spec puts
  out of scope). If the operator ever wants belt-and-braces, the cheapest client-side
  mitigation is a `maxLength` on the two name inputs; the durable fix is delimiting the
  interpolated list server-side.

#### SEC-L2 — Non-integer numeric input passes client validation and makes the whole-object save fail

- **Where:** `validateGarmentFields` / `validateOperationFields` / `validateDiscountFields`
  (`Panel.jsx`, Task 2 helpers) check finiteness and bounds but not integrality; the add-forms
  then pass `Number(draft.base_minutes)` etc. straight through.
- **Attack vector / trigger:** an admin types `2.5` into «База, мин» (or «От»/«До», or
  «Доп. минуты»). Client validation passes (2.5 > 0). The row is added, the settings object is
  POSTed, and Go's `json.Unmarshal` rejects `2.5` for the `int` fields `BaseMinutes`,
  `AdditionalMinutes`, `MinQty`, `MaxQty` — so the **entire** settings save fails with a
  generic decode error, including unrelated edits made in the same session.
- **Impact:** availability/UX of the save path only — no data corruption, no bypass, no
  privilege issue; the backend correctly refuses the payload. Severity Low.
- **Pre-existing class, mildly extended:** the existing edit handlers
  (`handleGarmentChange:866`, `handleDiscountChange:976`, both `Number(value) || 0`) have had
  exactly the same hole since before this feature, so the add-forms do not create the class —
  they add three more entry points to it. Worth noting because it is adjacent to the
  "one bad row blocks the whole save" risk the feature explicitly set out to close.
- **Recommendation (optional, out of this feature's scope):** add
  `Number.isInteger(parsed)` to the four integer-typed field checks, or `step="1"` plus an
  explicit check on the inputs.

---

## OWASP Top 10 analysis — category by category

Scoped as the task file directs: this feature adds no backend endpoint, no schema change, and
no new auth surface, so the auth/access-control categories are a confirmation that nothing
moved, not a fresh audit of `/panel`.

### A03 — Injection / XSS (client-side): **no findings**

Explicitly confirmed, per acceptance criterion:

- `grep -rn "dangerouslySetInnerHTML\|innerHTML" client/src/` → **zero matches** (exit 1).
  No `eval(`, `new Function`, `document.write`, or `javascript:` URL construction exists in
  `Panel.jsx` either.
- Every place a user-typed name reaches the DOM is plain JSX interpolation, therefore
  auto-escaped by React. All four render sites were checked (both mode branches, per
  tech-spec Decision 5's twice-rendered pattern):
  - garment name, quick mode: `<strong …>{name}</strong>` inside the new flex header
  - garment name, masterpiece mode: identical `<strong …>{name}</strong>`
  - operation name, quick mode ("Усложнения"): identical
  - operation name, masterpiece mode ("Операции"): identical
  Discount tiers have no name field — only three numeric inputs — so they contribute no
  string sink at all.
- New components audited individually: `GarmentAddForm`, `OperationAddForm`, `DiscountAddForm`,
  `DeleteRowButton`. All render through JSX; the only attribute carrying dynamic text is
  `DeleteRowButton`'s `title`/`aria-label`, fed from the hardcoded literal
  `"Последний диапазон удалить нельзя"`, never from user input. Inline validation errors
  render as `<p role="alert">{error}</p>` where `error` is always one of the fixed message
  strings from the Task 2 validators — no user input is echoed back into the error text.
- Values flow into inputs as the `value` prop (never `defaultValue` with markup, never an
  attribute template), so React's DOM property assignment applies.
- No SQL, no shell, no template engine, no dynamic `import()` is touched anywhere in the diff;
  no backend file was modified at all.

**Prototype pollution (explicitly checked, no finding).** User-typed names become object keys
in four places: `{ ...current.garments, [name]: fields }`, `{ ...current.operations, [name]: fields }`,
`Object.fromEntries(Object.entries(...).filter(...))` in both delete handlers, and
`Object.fromEntries` in `syncOrderForm`/`createDefaultOrderForm`. A name of `__proto__`,
`constructor` or `prototype` is *not* exploitable here, verified empirically in this repo's
Node runtime:

```
computed key own prop? true | proto polluted? false
Object.keys: [ 'a', '__proto__' ]
fromEntries own? true value: 0
entries roundtrip own? true
global proto intact: true
JSON roundtrip own? true
```

Computed keys in an object literal use `CreateDataPropertyOrThrow` (define, not `[[Set]]`), and
`Object.fromEntries` and `JSON.parse` likewise define own properties — so `__proto__` becomes a
normal own key and `Object.prototype` is never touched. `handleDeleteOperation`'s
`name in (current.operation_counts || {})` guard is inherited-property-sensitive in principle,
but `operation_counts` is always built by `Object.fromEntries`, so the key is genuinely own and
the subsequent rest-destructuring removes it correctly.

### A03 — Injection (server-side, LLM prompt): **1 accepted item, re-confirmed**

See the dedicated re-confirmation section above and SEC-L1. Status: still holds, no change.

### A01 — Broken access control: **no findings, nothing changed**

No new endpoint exists (`server/internal/handler/http.go` route table is byte-identical —
`git diff --stat 4edea63^ HEAD -- server/` is empty, satisfying the tech-spec AC directly).
Persistence still rides the single pre-existing `handleSaveSettings` →
`saveUserSettings` → `POST /api/v1/users/settings` whole-object flow. The owner of the
written data is still `identity.Login` from the session context, never a path segment or body
field. The client-side `hasPanelAccess` mirror is unchanged by this feature and remains
cosmetic — real authorization stays server-side in `RequireAuth`/`RequireAccess`.

**Client-side-only gates are correctly non-security.** Two new controls gate behavior in the
UI only, and in both cases the server independently enforces the real rule, so bypassing them
via devtools yields nothing:
- `DeleteRowButton isDeletable={!DEFAULT_GARMENT_NAMES.includes(name)}` — a forged delete of a
  default row is undone by `normalizeSettings` (`costing.go:659+`), which unconditionally
  re-merges the 4 default garments and 8 default operations over whatever the client sent.
- the last-tier `disabled` gate — an emptied `batch_discounts` is likewise silently replaced by
  the 4 default tiers.

### A04 — Insecure design: **no findings**

Decision 2 (collect all fields always, no hidden defaults) removes the `quick_price = 0`
bug class architecturally rather than by validation alone; this audit confirms the implemented
forms actually do collect all four fields in all three add-forms, in both mode branches. The
add handlers deliberately do not re-validate, and every call site validates before calling —
verified for all three (`GarmentAddForm.handleAdd`, `OperationAddForm.handleAdd`,
`DiscountAddForm.handleAdd`); no path reaches a handler unvalidated.

### A05 — Security misconfiguration: **no findings**

`client/package.json`'s only changes are `"test": "vitest run"` and
`"vitest": "^3.2.6"` in `devDependencies` — no new runtime dependency, no postinstall script,
no registry override, no unpinned `*` range. The added test file imports only `vitest` and the
local `./Panel.jsx`; it performs no network, filesystem, or `eval` access.

### A07 — Identification & authentication failures: **no findings, nothing changed**

The transport (`authFetch`, `credentials: "include"`, cookie session with
`SameSite` from `COOKIE_SAMESITE`, default `Lax`) is untouched by this feature and adds no new
request. CSRF posture is unchanged: the save is a `Content-Type: application/json` POST behind
a CORS allowlist (`handler/cors.go`) and a `Lax`-or-stricter session cookie.

### A08 — Software & data integrity: **no findings**

No deserialization of untrusted data was added; no CI/CD file was touched
(`git diff --stat` shows only the four client files). Lockfile changes are confined to the
vitest subtree.

### A09 — Logging & monitoring: **no findings**

This feature adds no logging and removes none. No secret, token, or PII is written to the
console anywhere in the diff.

### Hardcoded secrets: **no findings**

Explicitly checked, per acceptance criterion. `grep -n
"process.env|import.meta.env|api_key|apiKey|token|secret|password|Bearer"` over
`Panel.jsx` returns only three unrelated, pre-existing lines (`localStorage` get/set of
`"panelTheme"`, and `window.location.replace("/")` redirects). The same grep over
`Panel.validation.test.js` returns nothing — the test file contains only Cyrillic fixture names
and numeric bounds, no credentials, no URLs, no environment access. Zero secrets, tokens,
credentials, or connection strings were introduced anywhere in the diff.

---

## Input-validation consistency: client vs. Task 2 helpers vs. backend

Acceptance-criterion table. Rule applied per the task file's edge-case note: **client stricter
than backend is fine; only client *looser* than backend is a finding.**

| Field | Backend `validateSettings` (`costing.go`) | Task 2 shared helper | Add-form actually used | Verdict |
|---|---|---|---|---|
| garment name | `TrimSpace(name) != ""` (611) | `isBlankName` (exact mirror) + `isDuplicateName` (no backend equivalent — map keys silently overwrite) | `GarmentAddForm.handleAdd`: trim → `isBlankName` → `isDuplicateName(name, settings.garments)` | **stricter** (dedup is client-only, by design) |
| `base_minutes` | `> 0` (614) | `isPositiveNumber` | same helper, raw string in | **equal** |
| `complexity_coeff` | `> 0` (617) | `isPositiveNumber` | same helper | **equal** |
| `quick_price` | `>= 0` (620) | `isPositiveNumber` (`> 0`) | same helper | **stricter — deliberately.** `costing.go:718-721` divides by/relies on a positive quick price, so `0` saves fine but breaks quick-mode calculation later. This is the round-1 bug class the feature exists to close. |
| operation name | `TrimSpace(name) != ""` (634) | `isBlankName` + `isDuplicateName` | `OperationAddForm.handleAdd` | **stricter** |
| `additional_minutes` | `>= 0` (637) | `isNonNegativeNumber` | same helper | **equal** |
| `additional_material_per_unit` | `>= 0` (637) | `isNonNegativeNumber` | same helper | **equal** |
| `quick_percent` | `>= 0` (640) | `isNonNegativeNumber` | same helper | **equal** |
| `min_qty` | `> 0` (646) | `isPositiveNumber` | `DiscountAddForm.handleAdd` | **equal** |
| `max_qty` | `>= min_qty` (649) | `isPositiveNumber` **and** `>= min_qty` | same helper | **stricter** (also requires `> 0` independently) |
| `percent` | `0 <= p <= 100` (652) | finite ∧ `>= 0` ∧ `<= 100` | same helper | **equal** |

**No drift in the dangerous direction anywhere.** Every bound is equal or stricter; no
add-form is looser than the backend on any field.

**No bypass and no duplicated-with-drift logic.** All three add-forms call the Task 2 helpers
exclusively — `grep` confirms `isBlankName`, `isDuplicateName`, `validateGarmentFields`,
`validateOperationFields`, `validateDiscountFields` are each defined exactly once and no form
reimplements a bound inline. Two implementation details were checked and are correct rather
than suspicious:
- Validators receive the **raw input strings**, not `Number(...)`-coerced values. This is
  right: `toFiniteNumber` maps `""`, `"   "`, `"abc"`, `null` and `undefined` to `NaN`, whereas
  pre-coercing with the file's usual `Number(v) || 0` idiom would silently turn invalid input
  into `0` — the exact value that must be rejected for garments. Confirmed covered by tests
  ("rejects empty, blank and non-numeric input instead of coercing it to 0").
- `Number(...)` is applied only *after* validation passes, so handlers receive numbers, not
  strings, and `batch_discounts` never receives string members.

The 32-assertion test file is a genuine regression guard for these bounds (including the
`DEFAULT_*_NAMES` ↔ server-defaults drift check), not tautological — verified by reading it in
full. Independent of the client, the backend still re-validates every field on save
(defense in depth intact) and `normalizeSettings` still re-injects defaults, so a devtools
bypass of any client check changes nothing security-relevant.

---

## Ad-hoc fix `569c72f` (stale `orderForm` reference) — first security review

The lead specifically asked for this commit to be examined for implications not previously
considered. It was never covered by a per-task security review (it landed after
`task-5/security-auditor-1.json`). Reviewed here in full: **no findings, no security
implications.**

What it does: `handleDeleteGarment` now also does `setOrderForm(current => current.garment_type === name ? {...current, garment_type: Object.keys(settings.garments).find(k => k !== name) || ""} : current)`,
and `handleDeleteOperation` drops the deleted key from `orderForm.operation_counts`.

Security-relevant checks performed:

- **No new trust boundary, no new data flow.** Both updaters mutate local React state only.
  No network call, no storage write, no new field on any request body. The payload shape
  posted to `/api/v1/calculate` is unchanged — the fix removes a *stale* value, it never adds
  one.
- **No injection surface.** `name` is used only for equality comparison (`=== name`,
  `key !== name`, `name in obj`) and as a rest-destructuring key. It is never interpolated
  into markup, a URL, a query, or a template.
- **Prototype-pollution on the delete path:** `const { [name]: removedCount, ...restCounts }`
  with `name === "__proto__"` — verified safe. `operation_counts` is always built via
  `Object.fromEntries`, so `__proto__` is an own property, the guard is accurate, and rest-spread
  produces a fresh plain object.
- **Cannot be used to blank an unrelated selection.** Both updaters return `current` unchanged
  unless the deleted item is the referenced one, so there is no way for one admin action to
  clear state the admin did not target. Both are idempotent, so React StrictMode's double
  invocation is harmless.
- **`garment_type: ""` terminal state** is only reachable if every garment is deleted, which
  the UI forbids (defaults never render a delete button) *and* the backend forbids
  (`normalizeSettings` re-injects the 4 defaults). Even if reached, it is the identical value
  `createDefaultOrderForm` and `syncOrderForm` already produce, and the server rejects an empty
  `garment_type` with a normal validation error — fail-closed.
- **Net security effect is mildly positive:** it removes a state in which the client posted a
  reference to a non-existent item, which previously produced a confusing server-side
  `unknown operation` rejection. Fewer spurious 400s, no new capability.

---

## Explicitly checked, explicitly not findings

Recorded so a future reader can see these were considered rather than missed:

- **Out-of-scope by task instruction, not re-litigated:** Decision 3 (default rows are never
  deletable), the discount-tier-gap risk, and the deleted-item-in-saved-calculation risk. All
  three are conscious, user-approved acceptances in tech-spec's Risks table.
- **Both mode-branch render sites checked** for every new component, per the task's edge-case
  note — `GarmentAddForm` ×2, `OperationAddForm` ×2, `DeleteRowButton` at 4 row sites plus the
  shared `DiscountsBlock`. No divergence between quick and masterpiece branches; both read the
  same `settings.garments`/`settings.operations` object.
- **Nested-form avoidance** (`<div>` + `type="button"` + Enter interception) is a correctness
  fix, not a security control, and does not weaken anything: without it, Enter would submit the
  outer settings form. Every new button is `type="button"`, verified.
- **`DiscountAddForm`'s render-time `setState`** is a supported React reset pattern and cannot
  loop (`getDefaultDiscountRange` always returns a finite number). No DoS.
- **`defaultSettings` is now exported** (for the drift test). It contains only non-secret
  default pricing constants that the server already returns to any authenticated panel user;
  exporting it changes no security posture.

---

## Recommendations

1. **None blocking.** No Critical or High findings; the feature is releasable as-is from a
   security standpoint.
2. `npm audit fix` in `client/` at a convenient moment (SEC-M1) — non-breaking for `rollup`
   and `@babel/core`. The `vite` major bump is separate maintenance debt, already tracked from
   Task 1 onward.
3. Optional, out of this feature's scope: `Number.isInteger` checks (SEC-L2) and a `maxLength`
   on the two name inputs (SEC-L1).
