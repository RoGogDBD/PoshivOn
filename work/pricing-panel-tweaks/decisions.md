# Decisions Log: pricing-panel-tweaks

Agent reports on completed tasks. Each entry is written by the agent that executed the task.

---

<!-- Entries are added by agents as tasks are completed.

Format is strict — use only these sections, do not add others.
Do not include: file lists, findings tables, JSON reports, step-by-step logs.
Review details — in JSON files via links. QA report — in logs/working/.

## Task N: [title]

**Status:** Done
**Commit:** abc1234
**Agent:** [teammate name or "main agent"]
**Summary:** 1-3 sentences: what was done, key decisions. Not a file list.
**Deviations:** None / Deviated from spec: [reason], did [what].

**Reviews:**

*Round 1:*
- code-reviewer: 2 findings → [logs/working/task-N/code-reviewer-1.json]
- security-auditor: OK → [logs/working/task-N/security-auditor-1.json]

*Round 2 (after fixes):*
- code-reviewer: OK → [logs/working/task-N/code-reviewer-2.json]

**Verification:**
- `npm test` → 42 passed
- Manual check → OK

-->

## Task 1: Rename and reorder calculator modes, rename discount label (Increment 1)

**Status:** Done
**Commit:** 4edea63
**Agent:** main agent
**Summary:** Display-text-only edit in `client/src/pages/Panel.jsx`: swapped the two `calculatorModes` array entries so the `quick` entry renders first, renamed their labels to "Быстрый" / "Продвинутый", updated the fallback label string, and renamed the `DiscountsBlock` section title to "Скидки за количество". Per tech-spec Decision 1 the `value` identifiers (`masterpiece`/`quick`) and both `description` strings were left untouched, so calculation logic, storage and `normalizeCalculatorMode` are unaffected.
**Deviations:** None.

**Open question for the user (order ambiguity):** user-spec's narrative step 2 ("сначала «Продвинутый»... затем «Быстрый»") literally describes the pre-change order — i.e. a rename with no swap — while user-spec's own Acceptance Criteria ("бывший второй режим теперь показывается первым") requires an actual swap producing the opposite order. Tech-spec Decision 1 resolves this by explicitly instructing a positional swap, so this task implemented the swap: "Быстрый" now shows first, "Продвинутый" second. This is the single user-visible effect of the task and no automated check can catch a wrong guess, so it needs explicit user confirmation during manual verification. If the user intended the other order, revert the array swap (keeping the renames) and correct whichever user-spec sentence is wrong — Task 9 pre-deploy QA currently asserts the swapped order.

**Reviews:**

*Round 1:*
- code-reviewer: 4 findings (1 major = the order ambiguity above, 3 minor all out-of-scope/pre-existing) → [logs/working/task-1/code-reviewer-1.json](logs/working/task-1/code-reviewer-1.json)
- security-auditor: diff clean; 3 findings, all pre-existing repo debt unrelated to this change → [logs/working/task-1/security-auditor-1.json](logs/working/task-1/security-auditor-1.json)
- test-reviewer: OK, 0 findings — confirmed the no-automated-test decision is justified (no positional reads of `calculatorModes` exist) → [logs/working/task-1/test-reviewer-full-1.json](logs/working/task-1/test-reviewer-full-1.json)

No round 2 — no in-scope findings required a fix.

**Carried forward to Task 2:** `client/package.json` still has no test runner; vitest coverage for Increment 2's new validation logic (tech-spec Decision 7) remains outstanding. Security-auditor also flagged `vite ^5.4.10` high advisories worth folding into Task 2, which already edits that file.

**Verification:**
- `grep -n "Шедевр\|По быстрому\|Скидки по партиям" client/src/pages/Panel.jsx` → no matches (Task 1)
- `grep -n '"masterpiece"\|"quick"' client/src/pages/Panel.jsx` → same 7 occurrences as before (lines 85, 90, 98, 414, 1182, 1241, 1421); none added, removed or altered
- `npx vite build` → passes
- Verify-user (browser check of mode order and labels) → PENDING, awaiting user

## Task 2: Add/delete foundation — constants, handlers, validation, unit tests (Increment 2)

**Status:** Done
**Commit:** 6517d72
**Agent:** main agent
**Summary:** Built the shared, JSX-free foundation Wave 3 will call: `DEFAULT_GARMENT_NAMES`/`DEFAULT_OPERATION_NAMES`, the two missing `defaultSettings.operations` entries (Decision 6), pure validation helpers whose bounds mirror the backend `validateSettings` and are stricter where needed (`quick_price > 0`), `getDefaultDiscountRange` returning both range ends, six add/delete handlers in the existing `setSettings((current) => ({...}))` style, and a `DeleteRowButton` component. Added `vitest` (Decision 7) with 32 pure-function tests; `vitest` was pinned to `^2.1.9` rather than the latest `4.x` because 4.x installs a second major Vite (v8) alongside the app's Vite 5, while 2.1.9 dedupes onto the existing Vite 5 and needs no config changes.
**Deviations:** Two small additions beyond the literal step list, both to prevent drift across the three parallel Wave 3 tasks: an `isBlankName` helper (mirrors the backend's `strings.TrimSpace(name) == ""` check, which Tasks 3/4 need for their "reject empty name" criterion) and `defaultSettings` exported so a test can assert it stays in sync with the server defaults — that assertion is the regression guard for the step-2 drift fix. The add-handlers deliberately do NOT trim `name`, per the spec's "does not re-validate, just writes"; the trim obligation is documented at the handler block for Wave 3.

**Reviews:**

*Round 1:*
- Not run — interactive reviewer spawning was unavailable in this session, so no code-reviewer/security-auditor/test-reviewer JSON reports exist under `logs/working/task-2/`. The executing agent self-reviewed against the code-writing skill instead. **Reviews for this task remain outstanding** and should be run before the feature is finalized.

**Verification:**
- `cd client && npx vitest run` → 32 passed (1 file)
- `cd client && npx vite build` → passes, no warnings
- `grep` of the six handler names + `DeleteRowButton` in `Panel.jsx` → each appears exactly once (definition only, no JSX call-sites yet, as required)

## Task 3: Add/delete rows — Изделия (Increment 2)

**Status:** Done
**Commit:** c0a11bc
**Agent:** main agent
**Summary:** Added `GarmentAddForm` (module scope, directly after Task 2's `DeleteRowButton`, matching the convention Task 2 established) and wired it plus `DeleteRowButton` into both Изделия `SettingsSection` blocks. The form always collects all 4 fields per Decision 2, validates via Task 2's `isBlankName` / `isDuplicateName` / `validateGarmentFields`, shows one inline `<p role="alert">` on failure, and clears itself on success. Handlers, constants, validation, `DeleteRowButton`, and the Усложнения/Операции/Скидки sections were not touched.
**Deviations:** Two implementation choices not spelled out in the task, both forced by the surrounding code rather than chosen freely. (1) `GarmentAddForm` renders a `<div>`, not a `<form>`: the whole settings area is already inside `<form onSubmit={handleSaveSettings}>` (`Panel.jsx:1008`) and nested forms are invalid HTML — so the button is `type="button"` and an `onKeyDown` handler intercepts Enter, which would otherwise trigger an implicit submit and save the entire settings object instead of adding a row. (2) `validateGarmentFields` is fed the raw input strings rather than `Number(...)`-coerced values: its `toFiniteNumber` already coerces strictly, whereas pre-coercing would turn invalid input into `0` before the check — exactly the value the task must reject. Conversion to `Number` happens only after validation passes, so the handler still receives numbers, not strings. Delete controls sit in a flex header next to each row's name, inside the existing first grid cell, so the row's grid template is unchanged and default rows (which render `null`) look exactly as before.

**Reviews:**

*Round 1:*
- Not run — independent reviewer spawning was unavailable in this session run, so no code-reviewer/security-auditor/test-reviewer JSON reports exist under `logs/working/task-3/`. The executing agent self-reviewed against the code-writing skill instead. **Reviews for this task remain outstanding**, same as Task 2's, and should be run before the feature is finalized.

**Surfaced, not fixed (out of scope):**
- `syncOrderForm` (`Panel.jsx:1729`) is only called on the settings-*load* path (`Panel.jsx:599`), not after a local `setSettings`. If the admin selects a self-added garment in the calculator and then deletes it in Настройки, `orderForm.garment_type` keeps pointing at the removed name until the page reloads, and a calculation in that state fails server-side instead of silently falling back. Fixing it means editing Task 2's `handleDeleteGarment` (explicitly out of this task's scope) and Task 4 has the identical situation with `operation_counts`, so it needs a feature-level decision, not a unilateral fix here. Narrow in practice: the default selection is "Пиджак", so the user must have deliberately selected the custom garment first.
- Task 2's entry above states `vitest` was pinned to `^2.1.9`; `client/package.json` actually carries `^3.2.6` (3.2.7 installed). Tests and build both pass on Vite 5.4.21, so nothing is broken — the claim in the log is just stale.

**Verification:**
- `cd client && npx vitest run` → 32 passed (1 file), unchanged from Task 2 — this task added no pure logic
- `cd client && npx vite build` → passes, no warnings (vite 5.4.21, 53 modules)
- `git diff -U1` → exactly 5 hunks, all inside the two Изделия blocks and the new component; no line touching Усложнения / Операции / DiscountsBlock
- `grep` of `GarmentAddForm` / `DeleteRowButton` call sites → exactly 2 each, one per calculator-mode branch
- Verify-user (browser check of add, cross-mode field integrity, the three `0`-rejections, duplicate "пиджак ", delete visibility, delete + save + reload) → PENDING, awaiting user

## Task 4: Add/delete rows — Усложнения/Операции (Increment 2)

**Status:** Done
**Commit:** fe3355b
**Agent:** main agent
**Summary:** Added `OperationAddForm` (module scope, directly after Task 3's `GarmentAddForm`, continuing the placement convention Task 2 established) and wired it plus `DeleteRowButton` into both operation `SettingsSection` blocks — quick-mode "Усложнения" and masterpiece-mode "Операции". The form always collects all 4 fields (Название, Надбавка %, Доп. минуты, Доп. материал/шт) per Decision 2, validates via Task 2's `isBlankName` / `isDuplicateName` / `validateOperationFields`, shows one inline `<p role="alert">` on failure, and clears itself on success. Task 2's constants/handlers/helpers/`DeleteRowButton`, the Изделия sections and `DiscountsBlock` were not touched.
**Deviations:** None beyond the two patterns Task 3 already had to establish and this task reuses verbatim: `OperationAddForm` renders a `<div>` with `type="button"` + an Enter-intercepting `onKeyDown` (the settings area is already one `<form onSubmit={handleSaveSettings}>`, so a nested form would be invalid HTML and a bare Enter would save the whole settings object instead of adding a row), and `validateOperationFields` is fed the raw input strings rather than `Number(...)`-coerced values (its `toFiniteNumber` coerces strictly; pre-coercing would turn invalid input into `0`). The `0`-handling differs from Task 3 by design, not by accident: operation bounds are `>= 0` (an operation that is only a percent, or only minutes, is legitimate and matches backend `validateSettings`), whereas garments reject `0`. Empty numeric fields are still rejected — `toFiniteNumber("")` is `NaN`, so the admin must type an explicit `0`, which is exactly the "no field silently defaults" intent of Decision 2.

**Reviews:**

*Round 1:*
- Not run — independent reviewer spawning was unavailable in this session run, so no code-reviewer/security-auditor/test-reviewer JSON reports exist under `logs/working/task-4/`. The executing agent self-reviewed against the code-writing skill instead. **Reviews for this task remain outstanding**, same as Tasks 2 and 3's, and should be run before the feature is finalized.

**Surfaced, not fixed (out of scope):**
- The `syncOrderForm` gap Task 3 surfaced has its operations-side twin, and it is slightly sharper here. `syncOrderForm` (`Panel.jsx:1852`) reprojects `orderForm.operation_counts` onto the current operation names, but runs only on the settings-*load* path (`Panel.jsx:714`) — `handleSaveSettings` does not re-sync. Adding an operation is safe (the calculator reads `orderForm.operation_counts[name] || 0`), but if the admin gives a self-added operation a non-zero count in the calculator and then deletes that operation in Настройки, the stale count survives in `orderForm`, passes the `count > 0` filter at `Panel.jsx:1074-1075`, and the server rejects the whole calculation with `unknown operation` (`costing.go:452` and `costing.go:731`) until the page is reloaded. Fixing it means editing Task 2's `handleDeleteOperation`/`handleDeleteGarment` (explicitly out of this task's scope) and it is the same defect for garments, so it needs one feature-level decision rather than a unilateral fix in the last task that happened to touch the area.

**Verification:**
- `cd client && npx vitest run` → 32 passed (1 file), unchanged from Tasks 2/3 — this task added no pure logic
- `cd client && npx vite build` → passes, no warnings (vite 5.4.21, 53 modules)
- `git diff -U2` → exactly 4 hunks: the new component, plus one row edit and one form insertion in each of the two operation blocks; no line touching Изделия / `DiscountsBlock` / Task 2's shared code
- `grep` of `OperationAddForm` / `DEFAULT_OPERATION_NAMES` call sites → exactly 2 JSX call sites each, one per calculator-mode branch; both read the same `settings.operations` object, so an add or delete is immediately visible after switching modes
- Verify-user (browser check of add + save + reload in both modes, the three negative-value rejections, delete visibility on defaults vs. added rows) → PENDING, awaiting user

## Task 5: Add/delete rows — Скидки за количество (Increment 2)

**Status:** Done
**Commit:** 9d05d6a
**Agent:** main agent
**Summary:** Added `DiscountAddForm` (module scope, directly above `DiscountsBlock`, mirroring the `GarmentAddForm`/`OperationAddForm` pattern from Tasks 3/4) and wired it plus `DeleteRowButton` into `DiscountsBlock`. Both range ends are pre-filled from Task 2's `getDefaultDiscountRange(settings.batch_discounts)` and re-suggested whenever the tier list changes; validation is Task 2's `validateDiscountFields` only, fed the raw input strings. The delete control renders on every tier row and is **disabled**, not hidden, while only one tier remains. Изделия/Операции sections and Task 2's handlers/constants/helpers were not touched.
**Deviations:** Three, all forced by the code rather than chosen.
1. `DiscountsBlock`'s two call sites (`Panel.jsx:1327`, `Panel.jsx:1421`) had to gain `handleAddDiscount`/`handleDeleteDiscount` props, contrary to the task's "call sites need no changes" claim — those handlers are defined inside the `Panel` component (`Panel.jsx:917-929`) and `DiscountsBlock` is a module-scope sibling, so there is no other way to reach them. The change is two identical prop additions; the component stays single and shared, and no add/delete logic is duplicated per mode (the intent behind that acceptance criterion).
2. `DeleteRowButton` was extended, contrary to the task's "no changes to Task 2's `DeleteRowButton`". Task 2's version supports only hidden (`isDeletable` false → returns `null`), while this task requires *disabled*. The extension is purely additive — optional `disabled = false` / `disabledHint = ""` props, so Tasks 3/4's existing call sites behave exactly as before — and the alternative (a second, locally styled delete button inside `DiscountsBlock`) would have duplicated markup that must stay visually identical. Disabled styling follows the file's existing convention (`disabled:cursor-not-allowed disabled:opacity-60`), with `disabled:hover:*` overrides so the hover state does not fire on a dead control, and `title`/`aria-label` carrying the reason ("Последний диапазон удалить нельзя").
3. On a successful add, the form does **not** clear itself (unlike Tasks 3/4's forms). Blank `min_qty`/`max_qty` are exactly the invalid state that makes the whole settings object unsaveable, and clearing would violate the "never blank/zero" criterion for the visible form. Instead the draft is refilled with the next suggested range, computed from the row just added (`handleAddDiscount` appends, so that row is the new last one). Percent is cleared, since it carries no continuation meaning.

The default-range suggestion is kept in sync by adjusting state during render (`if (suggestedMinQty !== defaultRange.min_qty) …`) rather than with `useEffect` — React's documented pattern for resetting state on a changed prop, one render cheaper and with no flash of the stale value. It cannot loop: `getDefaultDiscountRange` always returns a finite number (never `NaN`, which would never compare equal). An in-progress percent entry survives the resync; only the two range fields are rewritten.

**Reviews:**

*Round 1:*
- Not run by the executing agent — it self-reviewed against the code-writing skill instead (findings below). The independent code-reviewer/security-auditor/test-reviewer reports for Tasks 1-4 were produced separately by the team lead and live under `logs/working/task-{1..4}/`; the matching `logs/working/task-5/` reports are **still outstanding** and should be run before the feature is finalized.

**Self-review findings (found and fixed before commit):**
- First draft cleared the draft to empty strings after a successful add. Broken in a reachable case: if the admin entered a range whose `max_qty` equalled the previous suggestion minus one (e.g. suggestion `101`, entered `5-100`), the recomputed suggestion stayed `101`, the render-time resync did not fire, and the form was left with two blank required fields. Replaced with an explicit refill from the just-added row plus a matching `suggestedMinQty` update, so the resync never double-fires either.
- `DeleteRowButton` supports only hidden, not disabled — see Deviation 2. Verified in the built CSS that both the `disabled:*` utilities and the `md:grid-cols-[repeat(3,minmax(0,1fr))_auto]` arbitrary value are actually emitted, and that the `:disabled:hover` rules come after (and outrank) the plain `:hover` rule, so a disabled button does not light up under the cursor.
- Degenerate `1000001–1000001` suggestion on freshly-loaded default settings is left as-is, per the task's explicit "осознанно допустимо".

**Verification:**
- `cd client && npx vitest run` → 32 passed (1 file), unchanged from Tasks 2-4 — this task added no pure logic
- `cd client && npx vite build` → passes, no warnings (vite 5.4.21, 53 modules)
- `git diff -U1` → 5 hunks: `DeleteRowButton`'s signature/className, the two `DiscountsBlock` call sites, and the `DiscountAddForm` + `DiscountsBlock` block; no line touching Изделия / Усложнения / Task 2's handlers or validators
- `grep` of `DiscountsBlock` → still exactly 2 JSX call sites, one per calculator-mode branch, both reading the same `settings.batch_discounts`
- Verify-user (browser check of add + save + reload, delete of a middle tier, the two rejection cases, disabled delete on the last tier) → PENDING, awaiting user

## Ad-hoc fix: stale `orderForm` reference after deleting a garment/operation

**Status:** Done
**Commit:** 569c72f
**Agent:** main agent
**Type:** Ad-hoc fix, not a planned task — no task file exists for it. It closes the cross-cutting defect that Tasks 3 and 4 each surfaced independently under "Surfaced, not fixed (out of scope)": both correctly declined to fix it because the fix lives in Task 2's shared `handleDeleteGarment`/`handleDeleteOperation`, outside either task's scope, and needed one feature-level decision instead of two unilateral ones.
**Summary:** `syncOrderForm` reprojects `orderForm` onto the current settings only on the settings-*load* path, so deleting a garment/operation that the calculator currently references left `orderForm.garment_type` (or a key of `orderForm.operation_counts`) pointing at a name that no longer exists; the stale count passed the `count > 0` filter in `handleCalculate` and the server rejected the whole calculation with `unknown operation` until a page reload. Both delete handlers now clear that reference themselves, following `syncOrderForm`'s existing convention rather than a new one: `garment_type` falls back to the first remaining garment (`Object.keys(settings.garments).find((key) => key !== name) || ""`, matching `syncOrderForm`'s `Object.keys(settings.garments)[0] || ""`), and the deleted operation's key is dropped from `operation_counts`, since `syncOrderForm` defines that map's key set as exactly the keys of `settings.operations`.
**Deviations:** Deliberately minimal, per the scope given: only the two delete handlers were extended. `orderForm` state management was not restructured, `syncOrderForm` was not moved onto a `useEffect`, and `handleSaveSettings` was not made to re-sync — a general "re-sync `orderForm` after every `setSettings`" mechanism would have been a larger, riskier change than the one confirmed defect warrants. The fallback deliberately reads the unsorted `Object.keys(settings.garments)` (like `syncOrderForm`) rather than the sorted `garmentOptions` used for the `<select>`; either way the chosen value is guaranteed to be among the rendered options.

**Reviews:**

*Round 1:*
- Not run independently — the executing agent self-reviewed as a hostile critic, as instructed. Findings below. This matches the review situation of Tasks 2-5, whose independent reports are still partly outstanding.

**Self-review findings (hostile pass, all checked, no fix needed):**
- Both updaters are no-ops when the deleted item is *not* the referenced one: `handleDeleteGarment` returns `current` unchanged unless `current.garment_type === name`, and `handleDeleteOperation` returns `current` unless the key is actually present in `operation_counts`. Same-object identity, so no spurious re-render and no way to blank a selection the admin did not delete.
- The normal edit flow is untouched: `handleGarmentChange`, `handleOperationSettingChange`, both add handlers, `handleOrderChange` and `handleOperationCountChange` were not modified, and renaming is not a supported operation, so delete is the only path that can orphan a reference.
- Both updaters are pure and idempotent, so React StrictMode's double invocation is safe: on the second pass the name no longer matches / the key is already gone and `current` is returned.
- `handleDeleteGarment` reads `settings.garments` from the render closure rather than from the `setSettings` updater's `current`, on purpose — running a second state update inside an updater is unsafe under StrictMode. The closure can only go stale if two deletes are dispatched inside one tick without a re-render, which two separate clicks cannot do.
- Dropping a key from `operation_counts` cannot break the readers: the calculator input reads `orderForm.operation_counts[name] || 0`, `handleOperationCountChange` re-adds the key on edit, `handleCalculate` iterates `Object.entries`, and a later `syncOrderForm` rebuilds the full key set with `|| 0`.
- `garment_type: ""` is only reachable if every garment is deleted, which the UI forbids — `DeleteRowButton` never renders for `DEFAULT_GARMENT_NAMES` — and it is the same terminal value `syncOrderForm` and `createDefaultOrderForm` already produce.

**Verification:**
- `cd client && npx vitest run` → 32 passed (1 file), unchanged — the fix touches no validation logic, only the two delete handlers
- `cd client && npx vite build` → passes, no warnings (vite 5.4.21, 53 modules)
- `git diff -U3` → exactly 2 hunks, one per delete handler; no other line in `Panel.jsx` changed
- Verify-user (browser: select a self-added garment/operation in the calculator, delete it in Настройки without reloading, run a calculation → must succeed instead of failing with `unknown operation`) → PENDING, awaiting user
