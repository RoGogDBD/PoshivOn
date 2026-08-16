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
**Summary:** Built the shared, JSX-free foundation Wave 3 will call: `DEFAULT_GARMENT_NAMES`/`DEFAULT_OPERATION_NAMES`, the two missing `defaultSettings.operations` entries (Decision 6), pure validation helpers whose bounds mirror the backend `validateSettings` and are stricter where needed (`quick_price > 0`), `getDefaultDiscountRange` returning both range ends, six add/delete handlers in the existing `setSettings((current) => ({...}))` style, and a `DeleteRowButton` component. Added `vitest` (Decision 7) with 32 pure-function tests; `vitest` was pinned to `^3.2.6` rather than the latest `4.x` because 4.x installs a second major Vite (v8) alongside the app's Vite 5, while the 3.x line dedupes onto the existing Vite 5 and needs no config changes. (This entry originally recorded the pin as `^2.1.9`, which was the value at the time of writing; `7a2aeee` bumped it to `^3.2.6` to clear a vitest CVE, and `client/package.json` has carried `^3.2.6` since.)
**Deviations:** Two small additions beyond the literal step list, both to prevent drift across the three parallel Wave 3 tasks: an `isBlankName` helper (mirrors the backend's `strings.TrimSpace(name) == ""` check, which Tasks 3/4 need for their "reject empty name" criterion) and `defaultSettings` exported so a test can assert it stays in sync with the server defaults — that assertion is the regression guard for the step-2 drift fix. The add-handlers deliberately do NOT trim `name`, per the spec's "does not re-validate, just writes"; the trim obligation is documented at the handler block for Wave 3.

**Reviews:**

*Round 1:*
- Not run at the time this entry was written — interactive reviewer spawning was unavailable in that session, so the executing agent self-reviewed against the code-writing skill instead.
- **Run later by the team lead.** All three reports exist and were committed in `5ef014d` ("chore: review reports for task 2"): [logs/working/task-2/code-reviewer-1.json](logs/working/task-2/code-reviewer-1.json), [logs/working/task-2/security-auditor-1.json](logs/working/task-2/security-auditor-1.json), [logs/working/task-2/test-reviewer-1.json](logs/working/task-2/test-reviewer-1.json). Nothing about this task remains outstanding.

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
- Not run at the time this entry was written — independent reviewer spawning was unavailable in that session run, so the executing agent self-reviewed against the code-writing skill instead.
- **Run later by the team lead.** All three reports exist: [logs/working/task-3/code-reviewer-1.json](logs/working/task-3/code-reviewer-1.json) (committed in `c1eb100`, "chore: review reports for tasks 3 and 4"), plus [logs/working/task-3/security-auditor-1.json](logs/working/task-3/security-auditor-1.json) and [logs/working/task-3/test-reviewer-1.json](logs/working/task-3/test-reviewer-1.json) (committed in `8f20c45`). Nothing about this task remains outstanding.

**Surfaced, not fixed (out of scope):**
- `syncOrderForm` (`Panel.jsx:1729`) is only called on the settings-*load* path (`Panel.jsx:599`), not after a local `setSettings`. If the admin selects a self-added garment in the calculator and then deletes it in Настройки, `orderForm.garment_type` keeps pointing at the removed name until the page reloads, and a calculation in that state fails server-side instead of silently falling back. Fixing it means editing Task 2's `handleDeleteGarment` (explicitly out of this task's scope) and Task 4 has the identical situation with `operation_counts`, so it needs a feature-level decision, not a unilateral fix here. Narrow in practice: the default selection is "Пиджак", so the user must have deliberately selected the custom garment first.
- Task 2's entry above stated `vitest` was pinned to `^2.1.9`, while `client/package.json` actually carries `^3.2.6` (3.2.7 installed) after the CVE bump in `7a2aeee`. Tests and build both pass on Vite 5.4.21, so nothing was broken — the claim in the log was just stale, and it has since been corrected in place (audit finding F10).

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
- Not run at the time this entry was written — independent reviewer spawning was unavailable in that session run, so the executing agent self-reviewed against the code-writing skill instead.
- **Run later by the team lead.** All three reports exist and were committed in `c1eb100` ("chore: review reports for tasks 3 and 4"): [logs/working/task-4/code-reviewer-1.json](logs/working/task-4/code-reviewer-1.json), [logs/working/task-4/security-auditor-1.json](logs/working/task-4/security-auditor-1.json), [logs/working/task-4/test-reviewer-1.json](logs/working/task-4/test-reviewer-1.json). Nothing about this task remains outstanding.

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
- Not run by the executing agent — it self-reviewed against the code-writing skill instead (findings below). The independent reports for Tasks 1-4 were produced separately by the team lead and live under `logs/working/task-{1..4}/`.
- **Run later by the team lead.** All three Task 5 reports exist and were committed in `c9ec79f` ("chore: review reports for task 5"): [logs/working/task-5/code-reviewer-1.json](logs/working/task-5/code-reviewer-1.json), [logs/working/task-5/security-auditor-1.json](logs/working/task-5/security-auditor-1.json), [logs/working/task-5/test-reviewer-1.json](logs/working/task-5/test-reviewer-1.json). Nothing about this task remains outstanding.

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
- Not run independently — the executing agent self-reviewed as a hostile critic, as instructed. Findings below. (This entry originally said the same held for Tasks 2-5; that is no longer true — their independent reports were all run later and live under `logs/working/task-{2..5}/`. This ad-hoc fix is the one item still carrying only a self-review; the Task 7 security audit did cover it separately.)

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

## Task 6: Code Audit (Audit Wave)

**Status:** Done
**Commit:** (this entry)
**Agent:** main agent
**Summary:** Holistic cross-component pass over the assembled feature (Tasks 1-5 + the ad-hoc `orderForm` fix) using the `code-reviewing` skill's 11 dimensions at feature level. Verdict **PASS with findings — Audit Wave not blocked**: 0 critical, 1 major, 9 minor. The three independently-built add-forms converged rather than drifted (identical handler/prop naming, trim and coercion discipline, error markup, shared-helper reuse), no validation logic is duplicated anywhere, and Decisions 1-6 are all correctly realised — both mode-branch render sites present with identical props, `DEFAULT_*_NAMES` exact against `costing.go:227-242`, `defaultSettings.operations` fully fixed, `getDefaultDiscountRange` pre-filling both ends. The one major finding is a gap no per-task reviewer could see: the add-forms accept fractional values for six fields that are `int`/`int64` server-side, and because the row editors show different field subsets per calculator mode, the offending field is invisible in the mode where the add happened — the whole settings save then fails with an opaque `invalid_request`, re-opening the "one bad new row blocks the entire save" bug class the specs treat as closed. Full findings, severities and suggested fixes → [logs/working/task-6/code-audit-report.md](logs/working/task-6/code-audit-report.md).
**Deviations:** None. No code was changed, per the task's explicit "report only, do not fix" instruction — including for findings that are one-line fixes.

**Reviews:**

None — audit task. Per the project's skills-and-reviewers catalog the audit itself is the review; no `logs/working/task-6/{reviewer}-{round}.json` reports are produced.

**Verification:**
- `cd client && npx vitest run` → 32 passed (1 file) on the merged codebase — confirms Tasks 3/4/5's JSX edits did not break Task 2's validation suite
- Every file in the Tasks 1-5 union read in its current post-Wave-3 state (`Panel.jsx` 2127 lines, `Panel.validation.test.js`, `package.json`), plus `panelApi.js`, `main.jsx` and the server-side references `costing.go` / `handler/http.go` used to cross-check bounds, defaults and the save-time error path
- No code changes: `git status` clean apart from this entry and the report file

## Task 7: Security Audit (Audit Wave)

**Status:** Done
**Commit:** (report only — no code changes)
**Agent:** main agent
**Summary:** Holistic full-feature OWASP Top 10 pass over the assembled final state of `Panel.jsx` (commits `4edea63^..HEAD`), covering ground no diff-scoped per-task review saw: cross-component data flow of admin-typed names from the three add-forms all the way to the backend AI prompt, plus the ad-hoc `569c72f` `orderForm` fix that landed after Task 5's review and had never been security-reviewed. Verdict: **0 Critical, 0 High, 1 Medium, 2 Low** — nothing blocking. Full report: [logs/working/task-7/security-audit-report.md](logs/working/task-7/security-audit-report.md).
**Deviations:** None.

**Findings accepted as-is rather than fixed:**
- The `deepseek.go:404-406` prompt item is **re-confirmed as still holding** and is noted, not remediated, exactly as tech-spec's Risks table intends. Verified three ways: the file is byte-identical (no backend file changed at all in this feature), the blast radius is still self-scoped (both `AnalyzeMarketFeedback` call sites take settings from `GetUserSettings(ctx, userID)` with `userID` from the session identity, and the AI result is display-only, attached after the price is computed), and no new unescaped sink exists. This feature changes discoverability only, not the trust boundary — fixing it would require editing backend code that user-spec puts out of scope.
- The Medium finding (build-toolchain `npm audit` advisories) is pre-existing dev-only debt already carried forward from Task 1, not introduced here — `vitest` itself adds no advisory, and its own CVE was already fixed in `7a2aeee`. The `vite` major bump remains separate maintenance work.
- Both Low findings are optional hardening outside this feature's scope (no `maxLength` on the name inputs; no integrality check on integer-typed numeric fields — the latter a pre-existing class shared with the existing edit handlers).

**Reviews:**

None — this task is itself the security review for the feature (`reviewers` intentionally empty); findings live in the report, not in a reviewer JSON cycle.

**Bookkeeping note for the lead:** the Tasks 2-5 entries above state their reviews were "not run", but `logs/working/task-{2..5}/security-auditor-1.json` (and the code/test reviewer reports) do exist, committed later in `5ef014d`, `c1eb100`, `c9ec79f`. Stale prose only — their findings were read and are consistent with this audit.

**Verification:**
- Verify-smoke 1: `grep -n "dangerouslySetInnerHTML" client/src/pages/Panel.jsx` → no matches (exit 1); widened to `grep -rn "dangerouslySetInnerHTML\|innerHTML" client/src/` → also no matches
- Verify-smoke 2: `git log --oneline 4edea63^..HEAD -- server/internal/service/deepseek.go` → empty, and `git diff --stat 4edea63^ HEAD -- server/` → empty (no backend file touched at all); `deepseek.go`'s last commit is `5a777f9` (2026-04-24), long before this feature
- `cd client && npx vitest run` → 32 passed (1 file); `npx vite build` → passes; `git status --porcelain` → clean

## Task 8: Test Audit (Audit Wave)

**Status:** Done
**Agent:** main agent
**Summary:** Holistic test-quality audit of the whole feature — verdict **PASS with 8 minor findings, none blocking**. All 22 bounds enumerated in tech-spec's Testing Strategy are genuinely covered by the 32-test vitest suite, every assertion invokes real exports from `Panel.jsx` (zero tautologies), and both default-name lists were re-verified by hand against `DefaultUserSettings()` in `costing.go` — 4/4 garments and 8/8 operations including "Шлица" and "Декоративная отстрочка". Decision 7's promise holds: the `quick_price = 0` drift risk is now a permanent regression guard, not a one-time authorship check. Full report: [logs/working/task-8/test-audit-report.md](logs/working/task-8/test-audit-report.md).
**Deviations:** None. Read-only audit — no code, tests or specs were modified.

**Findings and disposition (nothing blocks the Final Wave):**
- One unit-level gap worth an opportunistic one-line fix: `validateDiscountFields` never exercises `max_qty`'s own `> 0` branch, so blank/non-numeric `max_qty` is unguarded by the suite (the implementation is correct today; only the test is missing). Two further unit gaps are low-risk shared-helper duplication.
- The manual checklist is adequate in aggregate for the *planned* Wave 3 scope — it does walk both render sites per component, delete presence/absence for default vs. added rows, the disabled-last-tier state, and add/delete persistence across reload. It is **not** adequate for the two *unplanned* additions: the ad-hoc `orderForm`-clearing fix and the Enter-interception workaround in the three add-forms both postdate user-spec and appear in no `Verify-user` field. Every checklist was also written from the "confirm it's rejected" angle, so the boundary-*accept* cases (operation field `0`, `percent` 0/100, `max_qty === min_qty`) are systematically absent.
- Both gaps previously raised by per-task test-reviewers were adjudicated as **not consequential**: "AC8 recalculation-in-both-modes" is in fact covered by user-spec's How to Verify (it was only missing from task-level fields), and "`min_qty <= 0` rejection" is triple-defended by the unit test, the input's `min="1"`, and backend `validateSettings`.
- All 8 manual gaps (M1-M8) are handed to Task 9 rather than warranting a new implementation task, together with a "known-and-accepted, do not file as bugs" list (degenerate `1000001–1000001` suggestion, middle-tier discount hole, deleted-item recalculation of saved chats) and a reminder that Task 1's mode-order ambiguity needs explicit user confirmation, not a silent assertion.
- Outside this audit's scope but relevant to whoever accepts the feature: the independent code-reviewer/security-auditor/test-reviewer rounds for Tasks 2-5 did in fact run (reports under `logs/working/task-{2..5}/`, committed in `5ef014d`, `c1eb100`, `c9ec79f`, `8f20c45`) — this line was written from the then-stale prose in the Tasks 2-5 entries, corrected under audit finding F10. The `orderForm` ad-hoc fix remains the one change carrying only a self-review plus the Task 7 security pass.

**Verification:**
- `cd client && npx vitest run` → 32 passed (1 file), run by the auditor, not taken from prior reports
- `it(` count in `Panel.validation.test.js` → 32, matching the reported total; no skipped or `.only` tests
- `DEFAULT_GARMENT_NAMES` / `DEFAULT_OPERATION_NAMES` cross-checked line-by-line against `server/internal/service/costing.go` `DefaultUserSettings()` → exact match; `defaultSettings.operations` now carries all 8 entries (Decision 6 drift confirmed fixed)

## Ad-hoc fix: reject fractional values for integer backend fields (Audit findings F1, U1)

**Status:** Done — reviews pending
**Commit:** f8216ca (code + tests), d49d08a (decisions.md F10 doc fix)
**Agent:** main agent
**Type:** Ad-hoc fix, not a planned task — no task file exists for it. It closes code-audit finding **F1** (major, [logs/working/task-6/code-audit-report.md](logs/working/task-6/code-audit-report.md)) and test-audit finding **U1** (moderate-low, [logs/working/task-8/test-audit-report.md](logs/working/task-8/test-audit-report.md)), plus the documentation drift of finding **F10** in a separate commit.
**Summary:** Task 2's three validators checked only sign and range, never integrality, so a fractional value in a field the Go backend declares as `int`/`int64` passed client validation and reached the server. Six such fields exist: `base_minutes` and `quick_price` (`costing.go:45-47`), `additional_minutes` and `additional_material_per_unit` (`costing.go:51-52`), `min_qty` and `max_qty` (`costing.go:23-24`). A value like `10.5` in any of them fails `encoding/json` unmarshal *before* the server's own `validateSettings` runs, so the whole settings POST returns 400 — one bad new row again blocking the save of the entire settings object, the exact failure class this feature exists to prevent. The failure is also invisible: `База, мин` is not rendered in quick mode's garment row, so nothing on screen looks wrong until Save fails. Fix: a shared `isIntegerValue` helper (`Number.isInteger(toFiniteNumber(value))`, so non-numeric input is rejected too) applied as an `else if` after each field's existing finite/sign check, with a field-specific "должно быть целым числом" message. `step="1"` was added to the six matching `SettingsNumberInput` elements as a browser hint only — the "Добавить" buttons are `type="button"` and never trigger native form validation, so enforcement lives entirely in the validators.
**Deviations:** None. The three genuinely-float server fields were deliberately left alone and are now marked as such in the code: `complexity_coeff` (`float64`, `costing.go:46`), `quick_percent` (`float64`, `costing.go:53`) and `percent` (`float64`, `costing.go:25`) still accept fractions, which the new tests assert explicitly so a future "make everything integer" sweep breaks loudly.

**Ordering decision (discount `max_qty`):** the integer check is placed *before* the existing `max_qty >= min_qty` comparison. Without that order, `max_qty: 10.5` with `min_qty: 1` would pass the range comparison and reach the server. Both branches reject, so the ordering only decides which message the admin sees; the more specific cause wins. U1's premise was verified rather than assumed: `{min_qty: 11, max_qty: ""}` is indeed already rejected today, but by `isPositiveNumber(NaN)` on `max_qty`'s own branch, not by the `NaN >= 11` comparison the audit hypothesized — the comparison sits in an `else if` that a blank `max_qty` never reaches. That branch is now covered directly, so it cannot silently lose its guard if the comparison is ever rewritten.

**Test coverage added (9 new cases, suite 32 → 41):**
- `validateGarmentFields`: fractional `base_minutes` / `quick_price` rejected; fractional `complexity_coeff` still accepted.
- `validateOperationFields`: fractional `additional_minutes` / `additional_material_per_unit` rejected while `0` stays valid; fractional `quick_percent` still accepted.
- `validateDiscountFields` (U1): blank/non-numeric `max_qty` on its own branch, `max_qty <= 0` with a valid `min_qty`, fractional `min_qty`/`max_qty`, and — asserted separately — that a *whole* `max_qty` below `min_qty` still reports the range message rather than the integer one.
- One pre-existing test was amended, not deleted: "accepts the smallest positive values" used `base_minutes: 0.01, quick_price: 0.01`, which the new rule correctly rejects; the smallest valid integer is `1`, and `complexity_coeff` keeps its `0.01`.

**Reviews:**

*Round 1:* Pending — code-reviewer and test-reviewer (the two auditors who raised F1 and U1) to be spawned by the team lead.

**Verification:**
- `cd client && npx vitest run` → 41 passed (1 file), vitest 3.2.7
- `cd client && npx vite build` → passes, no warnings (vite 5.4.21, 53 modules)
- Field-by-field cross-check of every validated field against its Go struct tag in `costing.go` → exactly 6 int/int64 fields guarded, exactly 3 float64 fields left free

## Task 9: Pre-deploy QA (Final Wave)

**Status:** Done
**Commit:** (this entry)
**Agent:** main agent
**Summary:** Source-level QA of the assembled feature: `npx vitest run` → 41/41 green (32 from Task 2 + 9 from the integer-validation fix), `npx vite build` → clean, and all **19 acceptance criteria verified true by reading the final `Panel.jsx` and `costing.go` directly** rather than trusting prior task reports — 3 Инкремент 1 + 8 Инкремент 2 (user-spec) + 8 technical (tech-spec). **0 findings.** Both Audit-Wave defects (F1 fractional values, U1 untested `max_qty` branch) confirmed closed. Full report: [logs/working/task-9/qa-report.md](logs/working/task-9/qa-report.md) (+ machine-readable [qa-report.json](logs/working/task-9/qa-report.json)).
**Deviations:** None. Read-only task — no code, tests or specs modified.

**Manual checklist status:** **NOT walked — handed to the user, not confirmed.** This feature's Agent Verification Plan has no agent-executable live check (`/panel` needs an authenticated admin session, no MCP/curl surface), so Task 9's job was to *produce* the definitive checklist, not execute it. §3 of the QA report merges user-spec's "How to Verify", the `Verify-user` fields of Tasks 1/3/4/5, and Task 8's gaps M1-M8 into one deduplicated 35-item list organised by area, plus a "known-accepted, do not file as bugs" section (degenerate `1000001–1000001` suggestion, mid-list discount hole, deleted-item recalculation of saved chats, undeletable defaults, per-user settings). Per Task 9's own AC the feature is not fully accepted until the user reports back.

**Highest-priority manual items — zero automated coverage anywhere:**
- **E2/E3** — the ad-hoc `orderForm` stale-reference fix (`569c72f`), both halves: delete a self-added garment that is currently selected in the calculator, and delete a self-added operation that currently has a non-zero count, each without reloading; a calculation must still succeed.
- **E4** — Enter inside each of the 3 add-forms must add the row and **not** trigger a full settings save (the deliberate nested-form workaround).
- **C4 / D5** — boundary-*accept* cases every prior checklist omitted: operation field `= 0`, discount `percent = 0`, `percent = 100`, `max_qty === min_qty`.
- **A2** — Task 1's mode-order ambiguity, still open: user-spec's narrative and its own AC contradict each other; the code follows the AC (**«Быстрый» first**). Needs explicit user sign-off, not a silent pass. Reverting is a one-line array swap with the renames kept.

**Reviews:**

None — QA task; the QA pass is itself the verification for this feature (`reviewers: []`).

**Verification:**
- `cd client && npx vitest run` → 41 passed (1 file), vitest 3.2.7 — run by this agent, not taken from prior reports
- `cd client && npx vite build` → passes, 53 modules, no warnings
- `git diff --stat 4edea63^ HEAD -- server/` → empty (AC T2: no backend change, route table untouched)
- `DEFAULT_GARMENT_NAMES` / `DEFAULT_OPERATION_NAMES` re-checked line-by-line against `DefaultUserSettings()` (`costing.go:227-242`) → 4/4 and 8/8 exact
- Client vs backend bounds cross-checked field-by-field (`Panel.jsx:279-341` vs `costing.go:593-657`) → every bound equal or stricter, none looser
- Call-site census by grep → `GarmentAddForm` ×2, `OperationAddForm` ×2, `DiscountsBlock` ×2, `DeleteRowButton` ×5, one per mode branch as specified
- Verify-user (the §3 browser checklist) → PENDING, awaiting user

## Post-deploy follow-up: quick-tariff add-form simplification, default mode, header removal

**Status:** Done
**Commit:** 289b257 (client), and one backend commit immediately before it
**Agent:** main agent
**Summary:** Three user-requested changes after the feature was already deployed (tag `0.2.1.17`): (1) `GarmentAddForm` now hides "База, мин"/"Коэфф. сложности" when adding from Быстрый mode, substituting hidden defaults (`base_minutes: 1, complexity_coeff: 1`) — a deliberate reversal of Decision 2's "always show all fields" design for the quick-mode call site only (Продвинутый-mode call site unchanged, still shows all 4 fields); (2) default `calculator_mode` changed from `masterpiece` to `quick` on both client (`defaultSettings`) and backend (`DefaultUserSettings()`), closing the mismatch Task 1's security-auditor flagged (mode-picker showed Быстрый first but new users still defaulted to masterpiece); (3) removed the "Настройка модели / Модель расчёта" header block and "Активный режим" status card above the settings form.
**Deviations:** Point (1) is a conscious, user-approved reintroduction of the hidden-default risk Decision 2 was built to avoid — documented inline in the code comment above `HIDDEN_GARMENT_DEFAULTS`. A garment added from Быстрый mode will compute a technically-valid but fabricated price if later used in Продвинутый mode, until an admin corrects База/Коэфф. manually.

**Reviews:**

None — implemented directly by the main agent (small, well-scoped follow-up), not routed through the task/reviewer pipeline. Self-reviewed only.

**Verification:**
- `cd client && npx vitest run` → 41 passed, no regression
- `cd client && npx vite build` → passes, 53 modules
- `go build ./...` and `go vet ./...` (via `golang:1.25.5-alpine` Docker image, matching `server/Dockerfile`'s `GO_VERSION`) → clean
- `go test ./...` → **initially failed** (`TestCostingService_CalculateInChat_RespectsMinimumMarginFloor`, implicitly relied on the old default mode) — fixed by explicitly setting `CalculatorMode = calculatorModeMasterpiece` in that test (it exercises masterpiece-only margin-floor logic); full suite green after the fix

## Post-deploy hotfix: AI feedback stopped after default-mode change

**Status:** Done
**Commit:** 0d0f007
**Agent:** main agent
**Summary:** User reported "DeepSeek stopped answering" after tag `0.2.1.18`. Root cause: `handleCalculate` only requested AI feedback when `result.CalculationMode == "masterpiece"` — harmless while masterpiece was the default tariff, but the prior commit switched the default to `quick`, so any user without explicitly saved settings silently got zero AI feedback regardless of UI. Fixed by dropping the mode restriction; `buildMarketFeedbackInputFromCalculation` already degrades gracefully for quick-mode's empty `MaterialType`/`Urgency`/`MarketSegment` fields, so the restriction was never load-bearing.
**Deviations:** None.

**Reviews:**

None — small, well-understood bugfix, self-reviewed, not routed through the task/reviewer pipeline.

**Verification:**
- `go build ./...`, `go vet ./...`, `go test ./...` (via `golang:1.25.5-alpine`, matching `server/Dockerfile`) → all clean, no test relied on the removed mode check

## Post-deploy hotfix (2): AI feedback still not shown after backend fix

**Status:** Done
**Commit:** d18277c
**Agent:** main agent
**Summary:** After the backend hotfix (`0d0f007`, tag `0.2.1.19`) started returning `ai_feedback` for quick-mode calculations, the user reported the block still wasn't showing on poshivon.ru/panel. Found a second, independent gate: the client's calculation-history render branch for `itemMode === "quick"` never called `<CalculationAIFeedback>` at all (only the masterpiece branch did, plus a redundant dead `itemMode === "quick" ? null : ...` check inside a branch that only renders when it's already not quick). Added the component to the quick branch and removed the dead guard.
**Deviations:** None.

**Reviews:**

None — small, well-understood follow-up to the previous hotfix, self-reviewed.

**Verification:**
- `cd client && npx vitest run` → 41 passed, no regression
- `cd client && npx vite build` → passes, 53 modules
- Read `CalculationAIFeedback`'s helpers (`formatMarketPosition`, `marketStatusLabel`, `buildAIVerdict`) — all have safe switch defaults, confirmed no crash risk from quick-mode items lacking `market_status`/`market_segment`

## Post-deploy follow-up: simplify quick-tariff operation add-form

**Status:** Done
**Commit:** 6a53b0f
**Agent:** main agent
**Summary:** Same treatment as the earlier `GarmentAddForm` change, applied to `OperationAddForm`: hides "Доп. минуты"/"Доп. материал / шт" when adding from Быстрый mode, substituting hidden defaults of `0` for both. Lower-risk than the garment case — these two fields genuinely allow `0` (`>= 0` bound in `validateOperationFields`/backend), so the hidden value is real and valid, not a fabricated placeholder like garments' `base_minutes: 1`/`complexity_coeff: 1`. Продвинутый-mode call site unchanged.
**Deviations:** None — user explicitly requested this, consistent with the earlier garment decision.

**Reviews:**

None — small, well-scoped, mirrors an already-reviewed-in-principle pattern.

**Verification:**
- `cd client && npx vitest run` → 41 passed, no regression
- `cd client && npx vite build` → passes, 53 modules
