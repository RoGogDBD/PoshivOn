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
