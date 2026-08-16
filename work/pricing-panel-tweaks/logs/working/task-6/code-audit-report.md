# Code Audit — pricing-panel-tweaks (Task 6, Audit Wave)

**Date:** 2026-08-16
**Auditor:** Task 6 agent, `code-reviewing` skill (11 review dimensions, applied at feature level)
**Scope:** holistic, cross-component pass over the assembled post-Wave-3 state — Tasks 1-5 plus
the ad-hoc `orderForm` fix, all merged.
**Code state audited:** `HEAD` = `19540c6`; feature diff = `4edea63~1..HEAD`
(`Panel.jsx` +676/-61, new `Panel.validation.test.js` 251 lines, `package.json` +6/-1).

## Verdict

**PASS with findings — the Audit Wave is not blocked.**

0 critical, 1 major, 9 minor. The major finding (F1) is a real, reachable
save-blocking defect but it is a *missing* validation rule, not a broken one: nothing is
mis-saved, no data is corrupted, and the existing behaviour of every pre-feature field is
untouched. It should be fixed before the feature is called done, but it does not invalidate
Wave 3 or require rework of the architecture.

The three independently-built add-forms **converged**, not diverged. Handler naming, prop
naming, trim behaviour, raw-string-to-validator discipline, error markup, and shared-helper
reuse are identical across all three. There is **no duplicated validation logic** anywhere.
Decisions 1, 2, 3, 4, 5 and 6 are all correctly realised in the shipped code. The residual
findings are consistency/polish issues plus one genuine bug class the per-task reviewers could
not see, because seeing it requires reading a form defined in one task against a row-renderer
owned by a different task in the *other* calculator mode.

## Files read (current, post-Wave-3 state)

| File | Read | Notes |
|---|---|---|
| `client/src/pages/Panel.jsx` | full, 2127 lines | all of Tasks 1-5 + ad-hoc fix |
| `client/src/pages/Panel.validation.test.js` | full, 251 lines | Task 2's actual path (matches tech-spec's example name) |
| `client/package.json` | full | `vitest ^3.2.6`, `"test": "vitest run"` script |
| `client/src/utils/panelApi.js` | full | save path + client error surface |
| `client/src/main.jsx` | full | confirms `React.StrictMode` is on |
| `server/internal/service/costing.go` | ranges 220-250, 590-745 | `DefaultUserSettings`, `validateSettings`, `normalizeSettings`, `calculateQuickInChat` |
| `server/internal/handler/http.go` | ranges 60-115, 250-300 | `handleUpsertSettings`, `decodeJSON`, error mapping |
| existing per-task reviews | `logs/working/task-{1..5}/*.json` | to avoid re-reporting what round-1 already covered |

`npx vitest run` on the merged codebase → **32/32 passed** (Task 6 Verification Step satisfied;
Tasks 3/4/5's JSX edits did not break Task 2's pure-logic suite).

---

## Findings

### F1 — major — Add-forms accept fractional values for backend integer fields; the resulting row makes the *entire* settings object unsavable with an opaque error

**Dimension:** 10 (cross-file consistency) / 4 (error handling)
**Location:** `client/src/pages/Panel.jsx:264-276` (`validateGarmentFields`),
`278-290` (`validateOperationFields`), `292-307` (`validateDiscountFields`), and the three
consuming forms at `404-503`, `519-618`, `1746-1842`.

Six of the fields the new add-forms collect are **integers** on the server, not floats:

| Field | Go type (`costing.go` Data Models) | Client check today |
|---|---|---|
| `base_minutes` | `int` | `isPositiveNumber` — accepts `10.5` |
| `quick_price` | `int64` | `isPositiveNumber` — accepts `7000.5` |
| `additional_minutes` | `int` | `isNonNegativeNumber` — accepts `15.5` |
| `additional_material_per_unit` | `int64` | `isNonNegativeNumber` — accepts `80.5` |
| `min_qty` / `max_qty` | `int` | `isPositiveNumber` — accepts `10.5` |
| `complexity_coeff`, `quick_percent`, `percent` | `float64` | correct as-is |

`validateSettings` never sees these values: `handleUpsertSettings`
(`server/internal/handler/http.go:96-103`) decodes into `service.UserSettings` **first**, and
Go's `encoding/json` returns `cannot unmarshal number 10.5 into Go struct field ... of type int`.
`writeAPIDecodeError` (`http.go:284-287`) turns that into HTTP 400 `invalid_request`, which
`panelApi.js:12-27` re-throws verbatim and `mapPanelError` (`Panel.jsx:2091-2096`) passes
straight through. The admin sees the settings notice **"invalid_request"** with no indication
of which row or field is at fault.

**Why no per-task reviewer could see it.** The `Добавить` button is `type="button"` (correct,
per Tasks 3/4/5's nested-form reasoning), so it deliberately bypasses native HTML constraint
validation — including the implicit `step=1` that a bare `type="number"` carries. The
*row-editor* input for the same field would have caught it on submit — but the row editors
render **different field subsets per calculator mode** (pre-existing structure, Decision 5),
so the offending field is simply not on screen in the mode where the add happened:

- add a garment in **quick** mode with `База, мин = 10.5` → the quick-mode garment row
  (`Panel.jsx:1301-1320`) renders only `Мин. цена / шт`; `base_minutes` is nowhere in the DOM
  → Save submits → 400.
- add a garment in **masterpiece** mode with `Мин. цена / шт = 7000.5` → the masterpiece row
  (`1370-1391`) renders only `База, мин` and `Коэфф.` → Save submits → 400.
- add an operation in **quick** mode with a fractional `Доп. минуты` or `Доп. материал / шт`
  → the quick row (`1324-1343`) renders only `Надбавка, %` → Save submits → 400.
- discounts are the mild case: `От`/`До` are rendered in both modes with `min="1"` and default
  `step`, so the browser blocks the submit with a generic tooltip instead of a 400 — still a
  dead-end Save with the bad row already committed to state.

**Why it matters beyond the error text.** This is precisely the failure mode tech-spec's Risks
table row 4 was written to prevent ("a bad new discount tier (or **any invalid new row**) blocks
saving the *entire* settings object, including unrelated edits made in the same session") and
the bug class user-spec spent three revision rounds closing. The stated mitigation — "client-side
pre-submit validation on the add-form prevents ever submitting an out-of-bounds row" — holds for
*bounds* but not for *integrality*. Tech-spec Acceptance Criterion "client-side validation bounds
for the add-forms are equal to or stricter than backend `validateSettings` bounds" is satisfied
on a literal reading (`validateSettings` has no integrality check) but not against the effective
backend contract, which includes the JSON→`int` decode that runs before it.

**Suggested fix** (not applied — audit task writes no code):
add two helpers next to `isPositiveNumber`/`isNonNegativeNumber` —

```js
const isPositiveInteger = (value) => { const n = toFiniteNumber(value); return Number.isInteger(n) && n > 0; };
const isNonNegativeInteger = (value) => { const n = toFiniteNumber(value); return Number.isInteger(n) && n >= 0; };
```

— and swap them in for the six fields above, leaving `complexity_coeff`, `quick_percent` and
`percent` on the float checks. Also add `step="1"` to the corresponding `SettingsNumberInput`s
in the three forms so the affordance matches the rule. This is a pure-function change, so it
extends Task 2's existing test file naturally (`base_minutes: 10.5` → invalid) rather than
needing new infrastructure. Consider separately whether `mapPanelError` should map
`invalid_request` to something an admin can act on — out of this feature's scope, but this
finding is the first path that makes it reachable from a normal UI action.

---

### F2 — minor — Add-form field labels disagree with the row-editor labels for the same field

**Dimension:** 3 (readability/consistency)
**Location:** `Panel.jsx:592` vs `1410`; `595` vs `1413`; `480` vs `1387`.

| Field | Add-form label | Row-editor label |
|---|---|---|
| `additional_minutes` | `Доп. минуты` | `Минуты` |
| `additional_material_per_unit` | `Доп. материал / шт` | `Материалы / шт` |
| `complexity_coeff` | `Коэфф. сложности` | `Коэфф.` |
| `base_minutes` | `База, мин` | `База, мин` ✓ |
| `quick_price` | `Мин. цена / шт` | `Мин. цена / шт` ✓ |
| `quick_percent` | `Надбавка, %` | `Надбавка, %` ✓ |
| discount `От`/`До`/`Скидка, %` | — | identical ✓ |

The admin fills in "Доп. минуты: 15", the row appears immediately below labelled "Минуты: 15".
Not a defect, but the two vocabularies now sit five lines apart on the same screen. Task 3 and
Task 4 each picked reasonable labels in isolation; nobody compared them to the row renderers
they were being inserted next to. Pick one label per field (the add-form's are arguably clearer
and could be pushed down into the rows).

---

### F3 — minor — The three add-forms duplicate ~35 lines of chrome each, and inline three new Tailwind class strings instead of hoisting them, against this file's own established convention

**Dimension:** 3 (DRY) / 1 (architectural consistency)
**Location:** `Panel.jsx:408-411` ≡ `523-526` ≡ `1762-1765` (`updateField`);
`448-453` ≡ `563-568` ≡ `1796-1801` (`handleKeyDown`);
`457` ≡ `572` ≡ `1805` (dashed-card wrapper className);
`484-491` ≡ `599-606` ≡ `1823-1830` (`<p role="alert">` block, byte-identical);
`493-499` ≡ `608-614` ≡ `1832-1838` (`Добавить` button className, byte-identical).

The duplication is *correct* — that is the good news, and it is the single strongest evidence
that the three tasks converged rather than drifted. But it is also the standing drift risk: the
next person who restyles one error banner or one Add button will change one of three copies,
and nothing in the file or the tests will notice.

This file's own convention is explicitly to hoist repeated Tailwind strings into module
constants — `settingsSectionClass` (332), `settingsInsetClass` (335), `settingsInputClass` (338),
`settingsModeButtonBaseClass` (341). Tech-spec Dependencies → "Using existing" named
`settingsInputClass`/`settingsSectionClass` as the strings to reuse "for the new text input and
button styling". The forms do reuse `settingsInputClass` for their text inputs (correct), but
then invent three *new* long class strings each and leave them inline — so the feature added
nine inline copies of three strings where the file's pattern would have produced three constants.

Suggested fix: hoist `addFormCardClass`, `addFormErrorClass`, `addFormSubmitButtonClass` as
module constants next to the existing four; optionally extract a shared `AddRowCard` shell
component taking `{ title, hint, error, onAdd, children }`, which would also absorb the
triplicated `handleKeyDown`. Non-blocking; strictly a maintainability investment.

---

### F4 — minor — `DiscountAddForm` sits ~1130 lines from its two sibling add-forms, on the far side of the `Panel` component

**Dimension:** 3 (code organisation)
**Location:** `DeleteRowButton` 375, `GarmentAddForm` 404, `OperationAddForm` 519 — all above
`Panel` (626-1736). `DiscountAddForm` 1746 — below `Panel`, immediately above `DiscountsBlock`.

Task 5's own placement reasoning is locally sound (`DiscountsBlock` already lived below `Panel`
pre-feature, so its form went with it), but the assembled result is that two of three
structurally identical sibling components are in one place and the third is elsewhere, and a
reader looking for "the add-forms" finds two of them. Relatedly, `emptyGarmentDraft` (394) and
`emptyOperationDraft` (505) are hoisted module constants while `DiscountAddForm` builds its
initial draft inline (necessarily so — it is computed from `getDefaultDiscountRange` — so this
half is not fixable, only the placement is).

Note also that `decisions.md`'s Task 5 entry states `DiscountAddForm` was added "mirroring the
`GarmentAddForm`/`OperationAddForm` pattern from Tasks 3/4"; on placement, the assembled file
does not bear that out (see F10).

---

### F5 — minor — `DeleteRowButton`'s two gating props are semantically overloaded after Task 5's extension

**Dimension:** 3 (naming) / 1 (component API design)
**Location:** `Panel.jsx:375-392` (definition); call sites 1311, 1334, 1380, 1406, 1871.

After Task 5, `isDeletable` means "render at all" and `disabled` means "render, but inert". The
names no longer say that: `isDeletable={true} disabled={true}` reads as a contradiction. The
discount call site (1871-1876) has to pass a bare literal `isDeletable` purely to clear the
visibility gate, while `disabled={!canDeleteRow}` carries the actual business rule — so the
prop that *sounds* like the delete-eligibility rule is the one that carries no information
there, and vice versa.

Behaviour is correct at all five call sites (verified: the four garment/operation sites pass
only `isDeletable`/`onDelete`; the new props default to `false`/`""`, `title`/`aria-label`
evaluate to `undefined`, and React omits undefined attributes — so Tasks 3/4's rendered markup
is genuinely unchanged). This is a readability finding only. Suggest `isVisible` + `isDisabled`,
or a single `state: "hidden" | "disabled" | "enabled"` prop.

---

### F6 — minor — The per-field `errors` map that `buildValidationResult` was designed to produce is discarded by all three consumers; error text is flattened into one sentence and never cleared on input

**Dimension:** 3 (readability) / 4 (error handling)
**Location:** `Panel.jsx:260-262` (design intent), `433`, `548`, `1773` (consumers).

`buildValidationResult`'s own comment states the map exists "чтобы форма могла подсветить
каждое [поле], а не только первое". No form uses it: all three do
`setError(Object.values(errors).join(". "))`, producing one long run-on string
("База/мин должна быть больше 0. Коэффициент сложности должен быть больше 0. Мин. цена / шт
должна быть больше 0.") with no field visually marked. The designed capability is unrealised
everywhere, which means either the forms should use it or the helper is over-built.

Secondary, same locations: `error` is cleared only on a *successful* add. A stale error banner
stays on screen while the admin corrects the field and only disappears on the next successful
click. Clearing in `updateField` would be a two-line fix.

Both behaviours are **identical across all three forms**, so this is not drift — it is one
shared shortcoming, which is why no per-task reviewer flagged it as inconsistent.

---

### F7 — minor — Accessibility: multiple identical "Удалить" buttons with no per-row context

**Dimension:** 3 (readability, a11y)
**Location:** `Panel.jsx:375-392`.

`DeleteRowButton` sets `aria-label` **only when disabled**. In the enabled case the accessible
name is the literal text "Удалить" — so a screen reader user traversing the Изделия list hears
"Удалить, кнопка" repeatedly with no indication of which garment, and in `DiscountsBlock` every
tier row produces an identical one. Suggest always supplying a label:
`aria-label={disabled && disabledHint ? \`Удалить — ${disabledHint}\` : \`Удалить ${rowLabel}\`}`
with a new `rowLabel` prop (`name` for garments/operations, `Диапазон ${index + 1}` for tiers).

---

### F8 — minor / informational — Locally-appended discount tiers are unsorted while the server sorts on save; `getDefaultDiscountRange` reads the last array element, so it can suggest a range that overlaps existing tiers, and rows visibly reorder after reload

**Dimension:** 10 (cross-file consistency)
**Location:** `Panel.jsx:945-950` (`handleAddDiscount`), `312-317` (`getDefaultDiscountRange`),
`1790` (`DiscountAddForm`'s post-add refill) vs `costing.go:694-699` (`normalizeSettings` sorts
`BatchDiscounts` by `MinQty`, then `MaxQty`).

Two consequences, both accepted-by-spec but neither previously written down:

1. Add `200–300`, then add `5–10`: the local array is `[…, 200–300, 5–10]`, so the next
   suggestion is `11–11` — which overlaps whatever tier already covers 11. Range overlap and
   gap-checking are explicitly out of scope per user-spec Constraints and the Risks table
   ("Deleting a middle discount tier leaves a hole — accepted"), so this is correct behaviour,
   not a defect; it is recorded here because the *suggestion* now actively proposes an
   overlapping range rather than merely permitting one.
2. After Save + reload the tier list comes back sorted by `min_qty`, so rows the admin added out
   of order jump position. Pre-feature this was invisible (the list was effectively append-only
   from defaults); it is newly reachable now that adding is a UI action. Harmless, but a likely
   "is this a bug?" question during manual verification — worth a line in the QA checklist.

Also in this area: `handleDiscountChange` uses the file's pre-existing `Number(value) || 0`, so
clearing an existing last row's `До` sets it to `0`, which drops `getDefaultDiscountRange` to
`{1, 1}` and — via the render-time resync at `1757-1760` — silently rewrites the add-form's two
range fields. Recoverable in one keystroke; noted for completeness.

---

### F9 — minor / pre-existing, newly reachable — `DiscountsBlock` row `key` embeds both the row's own values and its array index

**Dimension:** 9 (performance / React correctness)
**Location:** `Panel.jsx:1859` — `key={\`${discount.min_qty}-${discount.max_qty}-${index}\`}`
(unchanged by this feature; confirmed pre-existing via `git diff 4edea63~1..HEAD`).

Because the key contains the row's own edited values, **typing in `От` or `До` changes the key
on every keystroke**, so React unmounts and remounts the row and the input loses focus after
each character. And because it also contains the index, deleting a middle tier shifts every
later row's key and remounts them too — a path that did not exist before Task 5 added the
delete control.

Out of this feature's scope (Task 5 correctly did not touch the key), but the audit reads the
assembled block and the second half of the problem is newly reachable. Suggested fix is one
line: key on `index` alone, or attach a stable client-side id when a tier is created.

---

### F10 — minor — `decisions.md` has drifted from the shipped code in four places

**Dimension:** 3 (documentation accuracy)
**Location:** `work/pricing-panel-tweaks/decisions.md`.

1. **Task 2 entry (line 68)** claims `vitest` was "pinned to `^2.1.9` rather than the latest
   `4.x`". `client/package.json` carries `^3.2.6` and 3.2.7 is installed. Task 3's entry already
   surfaced this ("the claim in the log is just stale") and it was never corrected in place, so
   the wrong statement is still the first one a reader hits.
2. **Tasks 2, 3, 4, 5 and the ad-hoc entry** all state that independent reviews were "not run"
   and "remain outstanding". They exist:
   `logs/working/task-{1..5}/{code-reviewer,security-auditor,test-reviewer}-1.json`, added by the
   lead in commits `c1eb100` and `c9ec79f`. Anyone reading `decisions.md` today would conclude
   the feature shipped unreviewed, which is the opposite of the truth — and the reports are
   substantive (Task 5's, for instance, independently verifies the `DeleteRowButton` extension
   is backward-compatible at all four pre-existing call sites).
3. **Task 5 entry (line 133)** — placement claim, see F4.
4. **Task 2 entry** does not mention the `"test": "vitest run"` npm script it added to
   `package.json`, which is the script anyone would reach for first.

Fix is documentation-only: correct the version, replace the "reviews outstanding" paragraphs
with links to the JSON reports that exist, and soften the placement sentence.

---

## Dimensions checked with no findings

### Decision 1 — `calculatorModes[*].value` untouched
`grep -n 'masterpiece\|"quick"' Panel.jsx` → 7 occurrences: lines 85, 90, 116, 794, 1662, 1721,
2023 — the identical set Task 1's own verification recorded pre-feature (85, 90, 98, 414, 1182,
1241, 1421, renumbered by later insertions). Both `value` fields are the original strings; the
Cyrillic labels appear only in `label`/`description`. Nothing in Tasks 2-5 introduced,
removed or reordered a `value` reference. `normalizeCalculatorMode` (2023) is untouched.
**Tech-spec AC satisfied.**

### Decision 6 — default-name constants and the stale `defaultSettings.operations` fix
`DEFAULT_GARMENT_NAMES` (101) and `DEFAULT_OPERATION_NAMES` (103-112) were compared
element-by-element against `DefaultUserSettings()` (`costing.go:227-242`): 4 garments and all 8
operations match exactly, including "Шлица" and "Декоративная отстрочка". Both previously
missing `defaultSettings.operations` entries landed (143-144) **with server-matching values**
(`18/50/7` and `18/0/5`) — the half-done failure mode the task file warned about did not occur.
No code anywhere derives delete-eligibility from `defaultSettings`: the only two consumers of
delete-eligibility are `!DEFAULT_GARMENT_NAMES.includes(name)` (1311, 1380) and
`!DEFAULT_OPERATION_NAMES.includes(name)` (1334, 1406). Test file lines 198-216 assert both
constants against an independently-transcribed server list rather than against `Panel.jsx`, so
the regression guard is real and not tautological. **Risks table row 1 mitigated as designed.**

### Decision 5 — both mode-branch render sites present, identical, and non-duplicated
Every new component appears at exactly the expected call sites, with byte-identical props
between the two branches:

| Component | Quick branch | Masterpiece branch | Props identical |
|---|---|---|---|
| `GarmentAddForm` | 1321 | 1393 | yes — `settings`, `onAddGarment` |
| `OperationAddForm` | 1344 | 1419 | yes — `settings`, `onAddOperation` |
| `DeleteRowButton` (garment) | 1311 | 1380 | yes — `!DEFAULT_GARMENT_NAMES.includes(name)` |
| `DeleteRowButton` (operation) | 1334 | 1406 | yes — `!DEFAULT_OPERATION_NAMES.includes(name)` |
| `DiscountsBlock` | 1347 | 1441 | yes — 4 props each |
| `DiscountAddForm` | (inside `DiscountsBlock`, 1881) | same | single instance |
| `DeleteRowButton` (discount) | (inside `DiscountsBlock`, 1871) | same | single instance |

`DiscountsBlock` was **not** duplicated and contains **no** mode-conditional logic — it does not
read `calculatorMode`, `isQuickCalculator`, or `pricing_rules` at all. Both call sites gained
the same two handler props for the same reason (the handlers live inside `Panel`; the block is a
module-scope sibling), which is the minimum change and preserves the "single shared component"
intent. **Risks table row 2 mitigated; tech-spec AC "no duplicate/divergent behavior between the
two call sites" satisfied.**

One behavioural note, not a finding: because the two mode branches are distinct JSX subtrees, an
in-progress (unsubmitted) add-form draft is discarded when the admin switches calculator mode.
This is identical for all three forms, so Decision 5's "поведение и набор полей формы одинаковы
в обоих режимах" still holds; worth one line in the manual QA checklist so it is not mistaken
for a bug.

### No duplicate logic — every shared helper defined once, called from the expected sites
Verified by grepping every feature symbol. Each of `isBlankName`, `isDuplicateName`,
`validateGarmentFields`, `validateOperationFields`, `validateDiscountFields`,
`getDefaultDiscountRange`, `toFiniteNumber`, `isPositiveNumber`, `isNonNegativeNumber`,
`buildValidationResult` has exactly **one** definition, all in Task 2's block (227-317), and no
form reimplements or inlines any of them:

- `GarmentAddForm` → `isBlankName` (415), `isDuplicateName(name, settings.garments)` (419),
  `validateGarmentFields` (427).
- `OperationAddForm` → `isBlankName` (530), `isDuplicateName(name, settings.operations)` (534),
  `validateOperationFields` (542).
- `DiscountAddForm` → `validateDiscountFields(draft)` (1771), `getDefaultDiscountRange` (1747,
  1790). Correctly does **not** call the name helpers — discount tiers have no name.

No task copy-pasted a dedup check, a bound check, or a range computation. **This was the audit's
single largest risk and it came back clean.**

### Convergence of the three independently-built forms
Compared side by side across ten axes; all three agree except where a divergence is documented
and justified:

| Axis | Garment | Operation | Discount |
|---|---|---|---|
| Component prop shape | `{ settings, onAddGarment }` | `{ settings, onAddOperation }` | `{ settings, onAddDiscount }` |
| Handler naming | `handleAddGarment`/`handleDeleteGarment` | `handleAddOperation`/`handleDeleteOperation` | `handleAddDiscount`/`handleDeleteDiscount` |
| Renders `<div>`, not `<form>` | yes | yes | yes |
| `type="button"` + `onKeyDown` Enter intercept | yes | yes | yes |
| Raw strings passed to validator, `Number()` only after passing | yes | yes | yes |
| `name.trim()` before dedup **and** before calling the handler | yes | yes | n/a |
| Local `draft` + `error` state, `updateField(key)` curried setter | yes | yes | yes |
| Error surface: single `<p role="alert">`, identical className | yes | yes | yes |
| Uses `SettingsField` + `SettingsNumberInput` + `settingsInputClass` | yes | yes | yes |
| Post-success behaviour | clears draft | clears draft | **refills with next suggested range** |

The one divergence (post-success behaviour) is deliberate and correct: clearing the discount
form would leave `min_qty`/`max_qty` blank, which is exactly the invalid state that blocks the
whole save — the bug user-spec spent three rounds closing. It is documented in Task 5's
`decisions.md` deviation 3 and its refill logic is provably convergent (see below).

Likewise the `0`-handling difference between garments (`> 0`) and operations (`>= 0`) is not
drift: it mirrors `validateSettings` exactly (`garment.BaseMinutes <= 0` /
`ComplexityCoeff <= 0` reject zero; `op.AdditionalMinutes < 0` / `AdditionalMaterialPerUnit < 0`
/ `QuickPercent < 0` permit it), and both forms still reject *empty*, since `toFiniteNumber("")`
is `NaN` — so "no field silently defaults" (Decision 2) holds in both.

### Validation bounds vs backend — cross-checked field by field
Excluding integrality (F1), every bound is equal to or stricter than the server:

| Field | `validateSettings` (`costing.go:593-654`) | Client (`Panel.jsx:264-307`) | Verdict |
|---|---|---|---|
| garment name | `TrimSpace(name) == ""` rejected | `isBlankName` + `isDuplicateName` | stricter ✓ |
| `base_minutes` | `<= 0` rejected | `isPositiveNumber` | equal ✓ |
| `complexity_coeff` | `<= 0` rejected | `isPositiveNumber` | equal ✓ |
| `quick_price` | `< 0` rejected (i.e. `0` allowed) | `isPositiveNumber` (`> 0`) | **stricter ✓ — correct** |
| operation name | `TrimSpace(name) == ""` rejected | `isBlankName` + `isDuplicateName` | stricter ✓ |
| `additional_minutes` | `< 0` rejected | `isNonNegativeNumber` | equal ✓ |
| `additional_material_per_unit` | `< 0` rejected | `isNonNegativeNumber` | equal ✓ |
| `quick_percent` | `< 0` rejected | `isNonNegativeNumber` | equal ✓ |
| `min_qty` | `<= 0` rejected | `isPositiveNumber` | equal ✓ |
| `max_qty` | `< min_qty` rejected | `isPositiveNumber` **and** `>= min_qty` | stricter ✓ |
| `percent` | outside `[0, 100]` rejected | `< 0 \|\| > 100` rejected, non-finite rejected | stricter ✓ |

The deliberately-stricter `quick_price > 0` is independently justified and correct:
`calculateQuickInChat` (`costing.go:718-721`) rejects `QuickPrice <= 0` at *calculation* time,
so a `quick_price = 0` garment saves cleanly and then breaks pricing later, in a different code
path — exactly the round-1 bug class Decision 2 was written to eliminate. The comment at
`Panel.jsx:216-223` records this reasoning at the source. **Risks table row 3 mitigated.**

### `getDefaultDiscountRange` pre-fills both ends
`312-317` returns `{ min_qty: nextMinQty, max_qty: nextMinQty }` — never blank, never zero,
never `NaN` (`Math.floor(lastMaxQty) + 1`, falling back to `1` for an empty/malformed list).
`DiscountAddForm` seeds both inputs from it at mount (`1748`) and refills both after a
successful add (`1790-1791`). Test line 247-250 asserts the invariant directly: any suggestion
the helper produces passes `validateDiscountFields`. **Tech-spec AC satisfied; the
"invalid new row blocks the whole save" regression is closed.**

The render-time state adjustment at `1757-1760` was checked specifically for the two ways this
pattern goes wrong and is safe on both: it cannot loop (the guard compares against a value that
is always a finite number, and the branch assigns exactly that value, so the second render's
condition is false), and it is `React.StrictMode`-safe (`main.jsx` confirms StrictMode is on;
the double-invoked render body queues identical updates, which converge). The post-add refill
also correctly pre-empts the resync by setting `suggestedMinQty` to the same value the next
render will compute, so the fields are not rewritten twice.

### `setSettings` immutable-update style in every new handler
All six new handlers (`891`, `901`, `917`, `927`, `945`, `952`) use
`setSettings((current) => ({ ...current, … }))`, matching `handleGarmentChange` (859),
`handleOperationSettingChange` (872) and `handleDiscountChange` (972) exactly. Deletions build
new containers rather than mutating: `Object.fromEntries(Object.entries(current.garments).filter(…))`
(904, 930) and `current.batch_discounts.filter(…)` (955). **No direct mutation of `current`
anywhere in the feature diff** — checked by reading every new updater. The two `setOrderForm`
updaters added by the ad-hoc fix (910, 936) follow the same discipline and additionally return
`current` unchanged when nothing applies, so they cannot cause a spurious re-render or blank a
selection the admin did not delete.

### Architectural consistency with existing `Panel.jsx` patterns
Every new form field uses `SettingsField` + `SettingsNumberInput` (or a bare `<input>` carrying
`settingsInputClass` for the two text fields, since no `SettingsTextInput` exists). No new
one-off input markup was introduced. `SettingsSection` was reused unchanged; the add-forms are
rendered *inside* the existing sections rather than as new sections. `DiscountsBlock` remained a
single component. No new state was hoisted into `Panel` beyond the six handlers — each form owns
its own draft, which is the right boundary. No new imports, no new dependencies in the render
path.

### The ad-hoc `orderForm` fix, read against the assembled file
`handleDeleteGarment` (901-915) and `handleDeleteOperation` (927-943) now clear the stale
reference. Checked against every reader of the data they touch:
`syncOrderForm` (2012-2021) rebuilds `operation_counts` with `|| 0` and falls back
`Object.keys(settings.garments)[0] || ""`, so the fix's fallbacks match the existing convention;
the calculator input reads `orderForm.operation_counts[name] || 0` (1634) so a dropped key
renders as 0; `handleOperationCountChange` (1073) re-adds it on edit; `handleCalculate` (1102)
iterates `Object.entries` so a missing key is simply absent. The `garment_type` fallback reads
the unsorted `Object.keys(settings.garments)` while the `<select>` renders the sorted
`garmentOptions` (787) — different order, but the chosen value is guaranteed to be one of the
rendered options either way, so no invalid selection is possible. The deliberate closure read of
`settings.garments` (rather than a nested state update) is correct under StrictMode. **No
follow-on defect found; the fix is complete for both delete paths and correctly does nothing for
`handleDeleteDiscount`, since `orderForm` holds no discount reference.**

### Dependencies
`vitest ^3.2.6` added as a devDependency only, plus a `"test": "vitest run"` script. No
`@testing-library/react`, no runtime dependency added — matches Decision 7's stated boundary.
Installed 3.2.7 dedupes onto the project's existing Vite 5.4.21 (`npx vitest run` and
`npx vite build` both pass per Tasks 2-5's verification, and the test run was re-confirmed here).
The version differs from what `decisions.md` records — see F10. Note that
`vite ^5.4.10`'s advisories, which Task 1's security-auditor suggested folding into Task 2,
were not addressed; that is Task 7's territory, flagged here only for handoff.

### Security (light pass — Task 7 owns this dimension)
Nothing new introduced by this feature: no `dangerouslySetInnerHTML`, no `eval`, no new network
call, no new endpoint, no credential or secret in the diff, no `console.log`. All new user input
flows into React text nodes and `value` attributes, which React escapes. Authorization is
unchanged and remains server-side (`RequireAdmin`, session-derived `userID`). The one known
acceptance — garment/operation names reaching the DeepSeek prompt unescaped
(`server/internal/deepseek.go:404-406`) — is already recorded in tech-spec's Risks table as a
conscious, self-scoped acceptance; this feature makes it reachable via UI rather than API only,
which is exactly what that row says. Deferring in full to Task 7.

### Testing (light pass — Task 8 owns this dimension)
32/32 pass on the merged codebase. Assertions are non-trivial: boundary values on both sides of
every bound, `NaN`/`""`/`null`/`Infinity` coercion cases, and the default-name lists asserted
against an independently transcribed server list rather than against `Panel.jsx` itself. The
deliberately untested surface (component rendering/wiring) matches Decision 7's scope. One gap
relevant to F1: no test asserts integrality for any field, because no such rule exists yet —
if F1 is fixed, the fix is directly testable in this file with no new infrastructure. Deferring
in full to Task 8.

---

## Recommended disposition

| # | Severity | Recommendation |
|---|---|---|
| F1 | major | Fix before the feature is called done. Small, well-contained, test-covered by construction. |
| F2 | minor | Fix — one-line label edits, removes a live inconsistency on screen. |
| F3 | minor | Fix the class-string hoisting (cheap, matches file convention); the shared-shell extraction is optional. |
| F4 | minor | Optional — move `DiscountAddForm` up beside its siblings, or leave and correct the `decisions.md` claim. |
| F5 | minor | Optional rename; behaviour is already correct. |
| F6 | minor | Optional. Clearing `error` on input is worth the two lines; per-field highlighting is a judgement call. |
| F7 | minor | Optional a11y improvement. |
| F8 | minor | No code change — add a line to the manual QA checklist so the reorder-after-save isn't mistaken for a bug. |
| F9 | minor | Pre-existing; fix opportunistically (one-line key change) or file separately. |
| F10 | minor | Documentation-only correction to `decisions.md`. |

**Audit Wave gate: not blocked.** Nothing found requires reverting or restructuring any of
Tasks 1-5. F1 should be scheduled as a follow-up fix before Pre-deploy QA signs off, since it
re-opens a bug class the specs treat as closed.
