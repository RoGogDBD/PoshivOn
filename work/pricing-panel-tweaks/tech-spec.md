---
created: 2026-08-15
status: draft
branch: dev
size: M
---

# Tech Spec: pricing-panel-tweaks

## Solution

Everything lives in one client file, `client/src/pages/Panel.jsx` (React/Vite,
no framework beyond hooks; no backend or schema changes anywhere in this
feature). The work splits into the two independently-deployable increments
user-spec defines:

**Increment 1 (rename/reorder/label)** — edit the `calculatorModes` array
(swap the two mode objects, edit only their `label` strings, leave `value`
untouched) and the `DiscountsBlock` title string. Three-line-level change,
zero behavioral risk since `value: "masterpiece"/"quick"` — the actual key
used everywhere in calculation and storage — is never touched.

**Increment 2 (add/delete rows)** — for each of the three sections (Изделия,
Усложнения/Операции, Скидки за количество):
- A new "Добавить" form component collecting **all** fields for that item
  type at once (no mode-conditional subsets, no hidden defaults — this is the
  user-spec decision that eliminates the round-1 `quick_price = 0` bug class),
  with client-side validation (required name + dedup check; numeric fields
  must satisfy the same bounds the backend's `validateSettings` already
  enforces, checked client-side *before* the row is even added).
- A new "Удалить" control per row, visible only when the row's name is not in
  a fixed list of the 4 default garment names / 8 default operation names
  (backend's `normalizeSettings` unconditionally re-injects those regardless
  of what the client sends — deleting them is not achievable without a
  backend change, which is explicitly out of scope). For Скидки за
  количество, "Удалить" is disabled on the last remaining tier instead (an
  empty array is likewise silently replaced by backend defaults).
- Both add and delete only mutate the existing `settings` React state — no
  new network calls, no new endpoints. Persistence rides the existing single
  `handleSaveSettings` → `POST /api/v1/users/settings` submit flow, exactly
  like every other field in this form already works.
- The new discount tier's default range is fully computed, both ends: `min_qty
  = last tier's max_qty + 1` **and** `max_qty = that same suggested min_qty`
  (matching user-spec's AC exactly) — a form that only pre-fills `min_qty` and
  leaves `max_qty` blank/zero would reintroduce the exact "invalid new row
  blocks the whole save" problem user-spec spent 3 revision rounds closing.

A small, targeted unit-test carve-out is added for the new pure validation
logic (name-dedup check, numeric bound checks, default-name lists, discount
default-range computation) — this project's client has no test infrastructure
today, but adding `vitest` is near-zero marginal cost here (it reuses the
existing Vite config) and directly guards the exact bug class user-spec's
round-1 review found and fixed. See Testing Strategy and Decision 7.

Because Изделия and Усложнения/Операции are each rendered twice in the JSX
(once per calculator mode, with different field subsets per row — pre-existing
structure, out of scope to unify), the add-form and delete-button pieces are
extracted as small standalone components and invoked identically at both
existing render sites, mirroring the one pattern this file already uses this
way (`DiscountsBlock`, called at both mode branches today).

## Architecture

### What we're building/modifying

- **`calculatorModes` array edit** (`client/src/pages/Panel.jsx:83-94`) — swap
  order, rename `label` fields only.
- **`DiscountsBlock` title edit** (`Panel.jsx:1258-1260`) — label rename.
- **`GarmentAddForm` (new component)** — collects Название, Мин. цена/шт,
  База/мин, Коэфф. сложности; validates client-side; calls `onAddGarment`.
  Rendered once inside each of the two existing Изделия `SettingsSection`
  blocks (quick mode and masterpiece mode).
- **`OperationAddForm` (new component)** — same shape for Усложнения/Операции
  (Название, Надбавка %, Доп. минуты, Доп. материал/шт). Rendered once inside
  each of the two existing Усложнения/Операции blocks.
- **Discount add-form (inline in `DiscountsBlock`)** — мин./макс. количество,
  процент; pre-filled via `getDefaultDiscountRange`. `DiscountsBlock` is
  already a single shared component called from both mode branches, so no
  extraction is needed here.
- **`DeleteRowButton` (new small component)** — renders "Удалить" or nothing,
  based on an `isDeletable` boolean the caller computes (name-not-in-defaults
  for garments/operations; `batch_discounts.length > 1` for discounts).
  Invoked from both existing per-row `.map(...)` blocks for Изделия and
  Усложнения/Операции, and once inside `DiscountsBlock`'s row markup.
- **New handlers** (`handleAddGarment`, `handleDeleteGarment`,
  `handleAddOperation`, `handleDeleteOperation`, `handleAddDiscount`,
  `handleDeleteDiscount`) — added alongside the existing
  `handleGarmentChange`/`handleOperationSettingChange`/`handleDiscountChange`
  block (`Panel.jsx:479-548`), following their exact `setSettings((current) =>
  ({...}))` immutable-update style. **All six handlers are written together in
  one task** (see Task 2) to avoid three parallel tasks editing the same
  contiguous line range — Wave 2 tasks only add JSX call-sites that invoke
  these already-defined handlers, not the handlers themselves.
- **`DEFAULT_GARMENT_NAMES` / `DEFAULT_OPERATION_NAMES` (new constants)** — the
  4 + 8 hardcoded default names, used only to compute delete-button
  visibility. Defined separately from the existing `defaultSettings` object
  (which is confirmed stale — missing 2 of the 8 default operation names —
  see Decision 6) rather than derived from it.
- **Validation helpers (new, local to `Panel.jsx`)** — name-dedup check
  (trimmed, case-insensitive against existing `Object.keys(...)`) and numeric
  bound checks (`> 0` / `>= 0` / range, per field) for the add-forms. No
  validation library exists in this project; these are plain functions.
- **`getDefaultDiscountRange(existingTiers)` (new helper)** — computes both
  ends of the suggested new tier: `min_qty = last tier's max_qty + 1`,
  `max_qty = that same min_qty` (not left blank/zero). Used to pre-fill the
  discount add-form.

### How it works

**Increment 1:** purely a data edit in a static array + one string literal —
no data flow to describe.

**Increment 2 (add flow):** admin fills `GarmentAddForm` (or
`OperationAddForm`, or the discount inline form) → client-side validation
runs on submit (name dedup, numeric bounds) → on pass, the form calls
`onAddGarment(name, fields)` (etc.) → handler does
`setSettings((current) => ({ ...current, garments: { ...current.garments,
[name]: fields } }))` → row appears immediately in both mode views (shared
`settings.garments`/`operations` object) → admin clicks the existing
"Сохранить изменения" → `handleSaveSettings` → `POST /api/v1/users/settings`
(whole-object) → backend `validateSettings` passes (client already enforced
equal-or-stricter bounds) → `UpsertSettings` persists → reload shows the new
row.

**Delete flow:** `DeleteRowButton` visible only if `isDeletable` → click →
`handleDeleteGarment(name)` (etc.) builds a new map/array omitting that
entry → `setSettings` → same existing Save button persists the removal via
the same whole-object POST.

### Shared resources

None — no new shared resources (no DB pools, no API clients, no singletons).
This feature only adds local React component state and handlers to an
existing page component.

## Decisions

### Decision 1: Rename touches only `label`, never `value`
**Decision:** Edit only the `label` strings in `calculatorModes` and swap the
two array entries' positions. The `value: "masterpiece"/"quick"` fields are
never touched.
**Rationale:** Supports user-spec Constraint "внутренний ключ режима... не
меняется" and AC "внутренние значения (masterpiece/quick) в коде, БД и
расчётах не изменены." `value` is the real identifier used in calculation
logic, storage, and the DB — confirmed via code research as the only
non-display-only reference.
**Alternatives considered:** None — this is the only approach consistent with
the constraint.

### Decision 2: Unified add-form, no mode-conditional fields or hidden defaults
**Decision:** `GarmentAddForm`/`OperationAddForm` always collect the full
field set for that item type, regardless of which calculator mode is
currently active.
**Rationale:** Directly implements user-spec's Technical Decision (itself the
outcome of a round-1 finding): a mode-conditional form with hidden defaults
for non-visible fields could silently produce `quick_price = 0` for a garment
added while in masterpiece mode, which passes save validation but crashes
price calculation later, in a different code path (`costing.go:718-720`).
Collecting all fields removes this entire bug class architecturally, not
just via validation.
**Alternatives considered:** Mode-conditional form with backend-satisfying
hidden defaults — rejected in user-spec round 1 for the reason above.

### Decision 3: Delete restricted to admin-added rows only
**Decision:** "Удалить" is shown for a garment/operation only if its name is
not one of the 4/8 hardcoded default names. Default rows never get a delete
control. Discount tiers can be deleted freely except the last remaining one.
**Rationale:** [TECHNICAL, driven by a backend constraint discovered during
user-spec adequacy validation] `normalizeSettings`
(`server/internal/service/costing.go:659-710`) unconditionally re-merges the
4 default garments and 8 default operations into whatever is saved — a
default-named row cannot be removed frontend-only, it silently reappears
after save. An empty `batch_discounts` array is likewise replaced by 4
default tiers. Implementing delete for default rows would require backend
changes (distinguishing "field absent" from "field intentionally emptied" in
`normalizeSettings`), which user-spec explicitly puts out of scope to avoid
touching `GetUserSettings`/`CalculateInChat` on a live production system for
this feature.
**Alternatives considered:** Fix `normalizeSettings` to support real
deletion — rejected in user-spec (scope/risk on live prod-serving backend
code); drop delete entirely — rejected, user explicitly wants to be able to
undo an add-mistake (typo).

### Decision 4: Add/delete ride the existing whole-object Save, no new endpoints
**Decision:** Add and delete only call `setSettings`; persistence is the
existing `handleSaveSettings` → single `POST /api/v1/users/settings` call.
**Rationale:** Supports user-spec Constraint "Никаких новых бэкенд-эндпоинтов
и изменений бэкенд-кода не требуется." Matches the file's existing
convention — every other field in this settings form already works this way
(local state mutation, one shared Save button, one whole-object POST). An
autosave-per-row pattern would be new and inconsistent with the rest of the
page.
**Alternatives considered:** Per-row autosave endpoint — rejected, not
requested, adds backend surface area and an inconsistent UX pattern for no
stated benefit.

### Decision 5: Add-form/delete-button extracted as small shared components, invoked twice
**Decision:** `GarmentAddForm`, `OperationAddForm`, and `DeleteRowButton` are
defined once each and rendered at both existing per-mode JSX sites (quick and
masterpiece branches), rather than trying to hoist the entire Изделия/
Усложнения sections above the `isQuickCalculator` branch.
**Rationale:** [TECHNICAL] The two mode branches render genuinely different
field subsets per existing row (pre-existing structure, out of this feature's
scope to unify) — only the add-form and delete-button are identical in both
branches and can be safely shared. This mirrors the one existing precedent in
the file, `DiscountsBlock`, which is already called from both branches
unchanged. Supports user-spec AC "поведение и набор полей формы одинаковы в
обоих режимах калькулятора" without an unrelated refactor of the
mode-branching structure.
**Alternatives considered:** Unify the whole Изделия/Усложнения rendering
above the mode branch — rejected, would require redesigning the existing
per-mode field-display logic, well outside this feature's scope and risk
budget on a live panel.

### Decision 6: Fix the stale `defaultSettings.operations` constant too [TECHNICAL]
**Decision:** In addition to adding the separate
`DEFAULT_GARMENT_NAMES`/`DEFAULT_OPERATION_NAMES` lookup used for the delete
check, also add the 2 missing operations ("Шлица", "Декоративная отстрочка")
to the existing `defaultSettings.operations` object
(`Panel.jsx:96-192`) so it matches the server's `DefaultUserSettings()`
exactly.
**Rationale:** Code research confirmed `defaultSettings` (the client's
pre-load fallback state) has drifted from the server's actual defaults —
only 6 of 8 default operations are listed. User-spec's own Technical
Decisions explicitly warns against using this stale constant as the
delete-check source, but doesn't mandate fixing it. Fixing it here is a
one-line-per-entry, zero-risk correction (it only affects the transient
pre-load render) that removes a latent inconsistency discovered as a direct
byproduct of this feature's own research, rather than leaving a known bug
in place.
**Alternatives considered:** Leave `defaultSettings.operations` as-is, only
add the separate name list — rejected as needless technical debt given the
fix is trivial and was already found.

### Decision 7: Targeted unit tests for the new validation logic, despite no test infra
**Decision:** Add `vitest` as a devDependency and write one small test file
covering the new pure functions only: name-dedup check, per-field numeric
bound checks, `DEFAULT_GARMENT_NAMES`/`DEFAULT_OPERATION_NAMES` exact-match
assertions, and `getDefaultDiscountRange`. No component/DOM testing, no
`@testing-library/react`, no broader test-infra buildout.
**Rationale:** Revises user-spec's original "no unit tests" Testing decision
— surfaced during tech-spec validation (test-reviewer) and confirmed with the
user. The original decision was made against a binary framing (stand up full
test infra vs. nothing); this is a narrower middle option that wasn't
previously on the table: `vitest` reuses this Vite project's existing config
with near-zero marginal setup cost, and the functions being tested are the
tech-spec's own top-named risk (`Risks` table row 3) — the exact bug class
(`quick_price = 0` reaching production) that took a full round of user-spec
revision to catch by inspection alone. A handful of pure-function assertions
converts an authorship-time-only human check into a permanent regression
guard, for a cost proportionate to the risk. → user-approved during tech-spec
validation (2026-08-15) — see User-Spec Deviations
**Alternatives considered:** No unit tests at all (user-spec's original
decision) — superseded per above; full component-level testing with
`@testing-library/react` — rejected as disproportionate, the risk lives in
plain validation logic, not in rendering behavior.

## Data Models

No DB schema changes. Existing shapes reused as-is (all already defined
server-side in `server/internal/service/costing.go` and mirrored client-side
in `Panel.jsx`'s `defaultSettings`):

```go
type GarmentConfig struct {
    BaseMinutes     int     `json:"base_minutes"`
    ComplexityCoeff float64 `json:"complexity_coeff"`
    QuickPrice      int64   `json:"quick_price"`
}
type OperationConfig struct {
    AdditionalMinutes         int     `json:"additional_minutes"`
    AdditionalMaterialPerUnit int64   `json:"additional_material_per_unit"`
    QuickPercent              float64 `json:"quick_percent"`
}
type BatchDiscount struct {
    MinQty  int     `json:"min_qty"`
    MaxQty  int     `json:"max_qty"`
    Percent float64 `json:"percent"`
}
```

`Garments`/`Operations` are name-keyed maps (`map[string]GarmentConfig` /
`map[string]OperationConfig`); adding a row is adding a map key, deleting is
removing one (where allowed — see Decision 3). `BatchDiscounts` is a plain
`[]BatchDiscount`; add is append, delete is filtering by index.

## Dependencies

### New packages
- `vitest` (devDependency only) — targeted unit tests for the new pure
  validation functions (Decision 7). Reuses the project's existing Vite
  config; no `@testing-library/react` or other component-testing tooling is
  added.

### Using existing (from project)
- `SettingsSection`, `SettingsField`, `SettingsNumberInput` (`Panel.jsx:219-238`) — reused for new form fields, matching existing visual/structural conventions.
- `settingsInputClass`, `settingsSectionClass` (`Panel.jsx:213-214` and nearby) — reused Tailwind class strings for the new text input and button styling.
- `handleSaveSettings` / `saveUserSettings` (`Panel.jsx:550-566`, `client/src/utils/panelApi.js:42-46`) — existing whole-object save path, unchanged, reused as-is for add/delete persistence.
- `mapPanelError` (`Panel.jsx:1489-1494`) — existing error-to-message mapping, reused unchanged for any save-time failure after an add/delete.

## Testing Strategy

**Feature size:** M

### Unit tests
Targeted, not comprehensive (Decision 7): one new test file covering only
the new pure validation logic —
- name-dedup check: exact match, case-insensitive match, whitespace-padded
  match all correctly flagged as duplicates; distinct names pass.
- numeric bound checks: each field's boundary (`0` and just-below/above)
  for garment (`quick_price`, `base_minutes`, `complexity_coeff`, all `> 0`),
  operation (all fields `>= 0`), and discount (`min_qty > 0`,
  `max_qty >= min_qty`, `0 <= percent <= 100`).
- `DEFAULT_GARMENT_NAMES` (4 entries) and `DEFAULT_OPERATION_NAMES` (8
  entries) exactly match the hardcoded list mirrored from
  `server/internal/service/costing.go:227-242` — a direct regression guard
  for the exact drift already found once in `defaultSettings.operations`.
- `getDefaultDiscountRange`: returns `min_qty = max_qty last tier + 1`,
  `max_qty = that same min_qty`.

No React component/DOM tests — `GarmentAddForm`/`OperationAddForm`/
`DeleteRowButton` rendering and wiring are covered by manual verification
only (see task `Verify-user` fields), not by this test file.

### Integration tests
None — no new backend endpoints, no schema change, no cross-service
interaction introduced. Existing backend validation (`validateSettings`,
`normalizeSettings`) is untouched and already has its own coverage in
`server/internal/service/costing_test.go`, which this feature does not modify.

### E2E tests
None — this is a frontend-only settings-panel change on top of an already
end-to-end-working save flow; manual verification against the live/dev panel
(per user-spec "How to Verify", extended per-task below) is proportionate for
size M without new architecture.

## Agent Verification Plan

**Source:** user-spec "How to Verify" section.

### Verification approach
No agent-executable verification (no MCP, no curl-checkable API) — user-spec
is explicit that `/panel` requires an authenticated admin session (Яндекс
login + granted access) the agent doesn't have. All acceptance verification
is manual, by the user, on the live/dev panel, per the checklist already in
user-spec's "How to Verify → User verifies". Per-task `Verify-user` fields in
Implementation Tasks point back to the relevant parts of that checklist.

### Tools required
None — no Playwright MCP, no Telegram MCP, no curl/bash checks apply to this
feature.

## Risks

| Risk | Mitigation |
|------|-----------|
| Delete button implemented against the stale `defaultSettings` constant (only 6 of 8 default operations) instead of the correct 4+8 name list, leaving "Шлица"/"Декоративная отстрочка" with a silently-broken delete button | Task explicitly defines `DEFAULT_GARMENT_NAMES`/`DEFAULT_OPERATION_NAMES` as new constants cross-checked against `server/internal/service/costing.go:227-242`, not derived from `defaultSettings`; Decision 6 also fixes `defaultSettings.operations` itself |
| Изделия/Усложнения add-form or delete-button implemented in only one of the two mode-branch render sites, working in one calculator mode but not the other | Decision 5 + task file-scoping explicitly call out both insertion points (quick-mode and masterpiece-mode branches) for each new component |
| Client-side validation drifts from backend `validateSettings` bounds (e.g. allows `quick_price = 0`), reintroducing the round-1 silent-price-break bug | Task Description and AC directly enumerate the exact bounds (`> 0` / `>= 0` / range) matching `costing.go`; code-reviewer + security-auditor review each add-form task; **also** covered by permanent regression-guarding unit tests (Decision 7), not just one-time authorship review |
| A bad new discount tier (or any invalid new row) blocks saving the *entire* settings object, including unrelated edits made in the same session | Client-side pre-submit validation on the add-form prevents ever submitting an out-of-bounds row; existing `mapPanelError` surfaces any backend-side rejection unchanged, no silent failure |
| Live production panel — a regression in this file could break the pricing panel for all current users | Small, additive, well-scoped changes to one file; existing edit/save behavior for all other fields is left untouched; manual verification checklist (user-spec) run before considering the feature done |
| Deleting a middle discount tier leaves a "hole" — quantities in the gap silently get 0% discount instead of an error | Accepted per user-spec Constraints (no continuity check is in scope); documented here so it reads as a conscious decision, not an overlooked bug, for future readers |
| Deleting a garment/operation referenced by a previously-saved chat calculation may cause its recalculation to fail | Accepted per user-spec Constraints (out of scope to prevent); existing saved calculations are unaffected, only re-calculation of an order referencing the deleted item is impacted |
| Garment/operation names typed via the new add-forms flow unescaped into an AI market-feedback prompt (`server/internal/deepseek.go:404-406`) — pre-existing backend behavior (names were never content-restricted), but this feature makes it reachable via a discoverable UI action instead of only a hand-crafted API call | No code change — backend trust boundary is unchanged and blast radius is self-scoped (affects only the same user's own AI feedback); noted here per security review so it's a documented, conscious acceptance rather than an unreviewed gap |

## User-Spec Deviations

**Added: fix `defaultSettings.operations` in `Panel.jsx` (2 missing entries)**
— not requested in user-spec, which only requires a *separate* default-name
list for the delete check and explicitly says not to derive that list from
`defaultSettings`. Tech-spec additionally corrects `defaultSettings` itself
(Decision 6), since the drift was discovered as a direct byproduct of this
feature's own code research and the fix is a one-line-per-entry, zero-risk
addition to a pre-load fallback object. → [PENDING USER APPROVAL]

**Changed: unit tests added (user-spec said "none")** — user-spec's Testing
section explicitly stated no unit tests, validated twice during user-spec
review against a "stand up full test infra vs. nothing" framing. Tech-spec
Decision 7 adds a narrow `vitest` carve-out (2-3 pure functions only, no
component testing) after this was raised during tech-spec validation and
confirmed directly with the user as a distinct, cheaper middle option not
previously on the table. → **user-approved during tech-spec validation
(2026-08-15)**

## Acceptance Criteria

Technical acceptance criteria (complement the user-facing ones from
user-spec's own Acceptance Criteria section, which remain authoritative for
this feature's behavior):

- [ ] `calculatorModes[*].value` is unchanged (`"masterpiece"`, `"quick"`) — grep confirms no occurrence of the Cyrillic labels anywhere outside `label`/`description` fields.
- [ ] No new HTTP endpoints exist on the backend after this feature (route table in `server/internal/handler/http.go` unchanged).
- [ ] `DEFAULT_GARMENT_NAMES` (4 entries) and `DEFAULT_OPERATION_NAMES` (8 entries) exactly match `DefaultUserSettings()` in `server/internal/service/costing.go:227-242`.
- [ ] Client-side validation bounds for the add-forms are equal to or stricter than backend `validateSettings` bounds (`costing.go`) for every field.
- [ ] `GarmentAddForm`/`OperationAddForm`/`DeleteRowButton` render correctly in both calculator mode branches (quick and masterpiece) — no duplicate/divergent behavior between the two call sites.
- [ ] `getDefaultDiscountRange` pre-fills both `min_qty` and `max_qty` (not just `min_qty`) — a freshly-opened discount add-form must never default to an invalid/blank range.
- [ ] `npx vitest run` passes for the new validation-logic test file (Decision 7).
- [ ] No regressions in existing settings fields (materials, urgency, market bands, existing garment/operation/discount edit inputs) — manual smoke check per user-spec "How to Verify".

## Implementation Tasks

<!-- Tasks are brief scope descriptions. AC, TDD, and detailed steps are created during task-decomposition.

     Verify-smoke: concrete executable checks the agent runs during implementation — no deployment needed.
     Types: command (curl, python -c, docker build), MCP tool (Playwright, Telegram),
     API call (OpenRouter, external services), local server check, agent with test prompt.
     Verify-user: agent asks user to verify something (UI, behavior, experience).
     Both fields optional — omit if task is internal logic fully covered by tests. -->

### Wave 1 (independent)

#### Task 1: Rename and reorder calculator modes, rename discount label (Increment 1)
- **Description:** Swap the order of the two entries in `calculatorModes` and rename their `label` fields ("Шедевр"→"Продвинутый", "По быстрому"→"Быстрый") without touching `value`. Rename the `DiscountsBlock` section title from "Скидки по партиям" to "Скидки за количество". This is the complete, independently-deployable Increment 1 from user-spec.
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-user:** open poshivon.ru/panel → «Настройки» (or local dev build) and check: plan order swapped, labels read "Продвинутый"/"Быстрый", discount section reads "Скидки за количество" — per user-spec "How to Verify → Инкремент 1".
- **Files to modify:** `client/src/pages/Panel.jsx` (lines ~83-94 `calculatorModes`, ~798 fallback string, ~1260 `DiscountsBlock` title)
- **Files to read:** none beyond the file being modified

### Wave 2 (depends on Wave 1 — sequenced, not parallel, purely to keep Task 2's edits at `Panel.jsx:96-192`/`479-548` from landing in the same diff pass as Task 1's adjacent `83-94`/`798` edits; there is no logical dependency between them)

#### Task 2: Add/delete foundation — constants, handlers, validation, unit tests
- **Description:** Add the default-name constants, validation helpers (name-dedup, numeric bounds, `getDefaultDiscountRange`), all six new handlers, and the `DeleteRowButton` component — the complete shared foundation for Wave 3. Also fix the 2 missing entries in `defaultSettings.operations` and add `vitest` with one test file covering the new validation logic. Concentrating all shared logic and handlers here means Wave 3's three tasks only add JSX call-sites, not shared code, which is what avoids parallel edits to the same lines.
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-smoke:** `npx vitest run` (new test file passes)
- **Files to modify:** `client/src/pages/Panel.jsx` (new constants/helpers/handlers/component near `defaultSettings` at ~96-192 and the existing handlers block at ~479-548), `client/package.json` (add `vitest` devDependency), new test file (e.g. `client/src/pages/Panel.validation.test.js`)
- **Files to read:** `server/internal/service/costing.go` (lines 227-242, 593-654 — exact default names and validation bounds to mirror)

### Wave 3 (depends on Wave 2)

#### Task 3: Add/delete rows — Изделия (Increment 2)
- **Description:** Build `GarmentAddForm` (Название, Мин. цена/шт, База/мин, Коэфф. сложности — all fields always) using Task 2's validation helpers and calling Task 2's `handleAddGarment`. Render the add-form once inside each of the two existing Изделия `SettingsSection` blocks (quick and masterpiece mode); render `DeleteRowButton` (calling `handleDeleteGarment`) in each existing per-row block, gated on the row's name not being in `DEFAULT_GARMENT_NAMES`.
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-user:** on the panel, add a new item via "Добавить" in Изделия, save, reload, confirm it persists and prices correctly in both calculator modes; try entering `0` in each of Мин. цена/шт, База/мин, Коэфф. сложности and confirm each is rejected; confirm no delete button on the 4 default items and a working one on the new item — per user-spec "How to Verify → Инкремент 2".
- **Files to modify:** `client/src/pages/Panel.jsx` (Изделия `SettingsSection` only, within the larger quick/masterpiece branches: quick mode ~847-864, masterpiece mode ~903-923 — not the Усложнения/Операции sub-ranges within the same branches, those belong to Task 4)
- **Files to read:** `client/src/pages/Panel.jsx` (Task 2's constants/helpers/handlers/`DeleteRowButton`)

#### Task 4: Add/delete rows — Усложнения/Операции (Increment 2)
- **Description:** Build `OperationAddForm` (Название, Надбавка %, Доп. минуты, Доп. материал/шт — all fields always) using Task 2's validation helpers and calling Task 2's `handleAddOperation`. Render the add-form once inside each of the two existing Усложнения/Операции `SettingsSection` blocks; render `DeleteRowButton` (calling `handleDeleteOperation`) in each existing per-row block, gated on the row's name not being in `DEFAULT_OPERATION_NAMES`.
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-user:** on the panel, add a new item via "Добавить" in Усложнения, save, reload, confirm it persists in both calculator modes; try entering a negative value in each of Надбавка %, Доп. минуты, Доп. материал/шт and confirm each is rejected; confirm no delete button on the 8 default items and a working one on the new item — per user-spec "How to Verify → Инкремент 2".
- **Files to modify:** `client/src/pages/Panel.jsx` (Усложнения/Операции blocks only: quick mode ~866-883, masterpiece mode ~925-945)
- **Files to read:** `client/src/pages/Panel.jsx` (Task 2's constants/helpers/handlers/`DeleteRowButton`)

#### Task 5: Add/delete rows — Скидки за количество (Increment 2)
- **Description:** Add an inline add-form to `DiscountsBlock` (мин./макс. количество, процент), pre-filled via Task 2's `getDefaultDiscountRange` and calling Task 2's `handleAddDiscount`. Render `DeleteRowButton` (calling `handleDeleteDiscount`) per existing discount row, disabled when it's the last remaining tier (`batch_discounts.length > 1` gate).
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-user:** add a discount tier with the suggested default range, save, reload, confirm it persists; add two more tiers, delete the *middle* one, save, reload, confirm exactly that tier is gone and the others are unchanged; try `max_qty` below `min_qty` and `percent` outside 0-100, confirm both rejected; confirm the delete button is disabled on the last remaining tier — per user-spec "How to Verify → Инкремент 2".
- **Files to modify:** `client/src/pages/Panel.jsx` (`DiscountsBlock` only, ~1258-1282)
- **Files to read:** `client/src/pages/Panel.jsx` (Task 2's constants/helpers/handlers/`DeleteRowButton`)

### Audit Wave

<!-- Full-feature audit: 3 auditors review all code in parallel. Always present. -->
<!-- Auditors read code and write reports. If issues found — lead spawns a fixer, auditors become reviewers. -->

#### Task N-2: Code Audit
- **Description:** Full-feature code quality audit. Read all source files created/modified in this feature (from decisions.md + tech-spec "Files to modify"). Review holistically for cross-component issues: duplicate resource initialization, shared resources compliance with Architecture decisions, architectural consistency. Write audit report.
- **Skill:** code-reviewing
- **Reviewers:** none

#### Task N-1: Security Audit
- **Description:** Full-feature security audit. Read all source files created/modified in this feature. Analyze for OWASP Top 10 across all components, cross-component auth/data flow. Write audit report.
- **Skill:** security-auditor
- **Reviewers:** none

#### Task N: Test Audit
- **Description:** Full-feature test quality audit. Read all files modified in this feature (`client/src/pages/Panel.jsx`, the new `vitest` test file). Verify the targeted unit tests (Decision 7) actually cover every documented validation bound and default-name list, that no assertion is trivial/tautological, and that the deliberately-untested surface (component rendering/wiring) is genuinely adequately covered by the manual verification checklist (user-spec "How to Verify", extended per Wave 3 `Verify-user` fields). Write audit report.
- **Skill:** test-master
- **Reviewers:** none

### Final Wave

<!-- QA is always present. No Deploy/Post-deploy task: this project's deploy.yml
     triggers only on a git tag push (a separate, deliberate release action outside
     this feature's scope), and user-spec's Agent Verification Plan explicitly rules
     out any MCP/live-environment check (no agent-accessible authenticated session). -->

#### Task N: Pre-deploy QA
- **Description:** Acceptance testing: run `npx vitest run` and verify all acceptance criteria from user-spec (both increments) and this tech-spec are met. Confirm the manual verification checklist in user-spec "How to Verify" (extended per Wave 3 `Verify-user` fields) has been walked through and reflects the actual implemented behavior.
- **Skill:** pre-deploy-qa
- **Reviewers:** none
