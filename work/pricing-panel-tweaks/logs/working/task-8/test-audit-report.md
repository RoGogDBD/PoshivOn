# Task 8 — Test Audit: pricing-panel-tweaks

**Auditor:** test-master (Task 8, Audit Wave)
**Date:** 2026-08-16
**Verdict:** **PASS with 8 minor findings** — none blocking. Ship-ready provided
Task 9 (Pre-deploy QA) picks up the consolidated manual-gap list in §4.

---

## 0. Automated suite run

Run by this auditor, not taken from prior reports:

```
$ cd /home/makar/PoshivOn/client && npx vitest run

 RUN  v3.2.7 /home/makar/PoshivOn/client

 ✓ src/pages/Panel.validation.test.js (32 tests) 6ms

 Test Files  1 passed (1)
      Tests  32 passed (32)
   Start at  10:37:38
   Duration  494ms (transform 130ms, setup 0ms, collect 142ms, tests 6ms,
                   environment 0ms, prepare 94ms)
```

Green. Test file confirmed at `client/src/pages/Panel.validation.test.js` (the
tech-spec's working name turned out to be exact). `it(` count = 32, matching the
reported total — no skipped/`.only` tests, no empty describes.

**Sources of truth re-read directly, not trusted from prior reports:**
- `server/internal/service/costing.go` — `DefaultUserSettings()` garment/operation
  maps, and `validateSettings` (garment `BaseMinutes>0`, `ComplexityCoeff>0`,
  `QuickPrice>=0`; operation `AdditionalMinutes>=0`, `AdditionalMaterialPerUnit>=0`,
  `QuickPercent>=0`; tier `MinQty>0`, `MaxQty>=MinQty`, `Percent in [0,100]`).
- `client/src/pages/Panel.jsx:96-317` — the constants, `toFiniteNumber`,
  `isPositiveNumber`/`isNonNegativeNumber`, `isBlankName`, `isDuplicateName`,
  the three validators, `getDefaultDiscountRange`.
- `client/src/pages/Panel.jsx:891-960` — the six handlers incl. the ad-hoc
  `orderForm`-clearing fix; `DeleteRowButton`, `GarmentAddForm`,
  `OperationAddForm`, `DiscountAddForm`, `DiscountsBlock` and all 10 JSX call sites.

---

## 1. Coverage matrix — tech-spec Testing Strategy bounds

Every bound the Testing Strategy enumerates, checked against the *implementation*
and against `costing.go`, not against the test file's own comments.

| # | Bound (source of truth) | Covered | By which test |
|---|---|---|---|
| 1 | name-dedup: exact match → duplicate | ✅ | `isDuplicateName` #1 (`"Пиджак"`) |
| 2 | name-dedup: case-insensitive → duplicate | ✅ | `isDuplicateName` #1 (`"пиджак"`, `"ПИДЖАК"`) |
| 3 | name-dedup: whitespace-padded → duplicate | ✅ | `isDuplicateName` #1 (`"  Пиджак  "`, `" пиджак "`) + #2 (padding on the *existing key* side) |
| 4 | name-dedup: distinct name passes | ✅ | `isDuplicateName` #3 — incl. `"Пиджак-жилет"`, a prefix-collision probe |
| 5 | garment `base_minutes > 0` — reject at `0` | ✅ | `validateGarmentFields` #3 (`0`, `-0.01`, `-30`) |
| 6 | garment `complexity_coeff > 0` — reject at `0` | ✅ | `validateGarmentFields` #4 (`0`, `-0.01`, `-1.6`) |
| 7 | garment `quick_price > 0` — reject at `0` (client stricter than backend `>= 0`; the round-1 bug) | ✅ | `validateGarmentFields` #2 (`0`, `-0.01`, `-1`) |
| 8 | garment — passing case just above `0` | ✅ | `validateGarmentFields` #5 (`0.01` on all three fields simultaneously) |
| 9 | operation `additional_minutes >= 0` — **`0` must PASS** | ✅ | `validateOperationFields` #2 (asserts full `{valid:true, errors:{}}`) |
| 10 | operation `additional_material_per_unit >= 0` — `0` passes | ✅ | `validateOperationFields` #2 |
| 11 | operation `quick_percent >= 0` — `0` passes | ✅ | `validateOperationFields` #2 |
| 12 | operation — reject below `0`, per field | ✅ | `validateOperationFields` #3 — loops all 3 fields × `[-0.01, -1]` |
| 13 | discount `min_qty > 0` — reject at `0` | ✅ | `validateDiscountFields` #2 (`0`, `-1`) |
| 14 | discount `max_qty >= min_qty` — reject `<` | ✅ | `validateDiscountFields` #3 (`11`/`10`) |
| 15 | discount `max_qty === min_qty` — **must PASS** | ✅ | `validateDiscountFields` #3 (`11`/`11`) |
| 16 | discount `percent >= 0` / `<= 100` — reject outside | ✅ | `validateDiscountFields` #4 (`-0.01`, `-1`, `100.01`, `101`) |
| 17 | discount `percent === 0` — **must PASS** | ✅ | `validateDiscountFields` #4 |
| 18 | discount `percent === 100` — **must PASS** | ✅ | `validateDiscountFields` #4 |
| 19 | `DEFAULT_GARMENT_NAMES` — exactly 4, matching `DefaultUserSettings()` | ✅ | `default name constants` #1 |
| 20 | `DEFAULT_OPERATION_NAMES` — exactly 8 incl. "Шлица", "Декоративная отстрочка" | ✅ | `default name constants` #2 |
| 21 | `getDefaultDiscountRange` → `min_qty = last max_qty + 1` | ✅ | `getDefaultDiscountRange` #1 (`101`) |
| 22 | `getDefaultDiscountRange` → `max_qty` equals that same `min_qty` (not blank/zero) | ✅ | `getDefaultDiscountRange` #1, #2, #3, #4 — every case asserts both keys |

**22 / 22 bounds covered.** No gap against the Testing Strategy's own list.

### Verified against `costing.go` by hand (AC3)

`DEFAULT_GARMENT_NAMES` = `["Пиджак", "Юбка", "Рубашка", "Платье"]` — matches
`DefaultUserSettings().Garments` exactly, 4/4.

`DEFAULT_OPERATION_NAMES` = `["Карман накладной", "Карман прорезной", "Подклад",
"Потайная молния", "Воротник", "Манжеты", "Шлица", "Декоративная отстрочка"]` —
matches `DefaultUserSettings().Operations` exactly, 8/8, **including both
Decision-6 names**. `defaultSettings.operations` in `Panel.jsx:136-145` now also
carries all 8 (the drift is fixed, and tests #3/#4 in `default name constants`
are its permanent regression guard).

### Coverage beyond the Testing Strategy's list (credit where due)

These are not required by the Testing Strategy but genuinely earn their place:

- **Blank/non-numeric coercion** (`validateGarmentFields` #6, `validateOperationFields` #4,
  `validateDiscountFields` #5): assert `""`, `"   "`, `"abc"`, `null`, `undefined`,
  `NaN`, `Infinity` are *rejected, not coerced to 0*. This is the highest-value
  test in the file — it guards the `toFiniteNumber`-vs-`Number(x)||0` decision,
  and `Number(x)||0` is the pattern used everywhere else in `Panel.jsx`. A future
  refactor "simplifying" `toFiniteNumber` back to the house pattern turns blank
  input into `0` and reintroduces exactly the round-1 bug class. Caught here.
- **Numeric strings accepted** (`validateGarmentFields` #7): the forms feed raw
  input strings (Tasks 3/4/5 deviation), so this asserts the real call contract.
- **All-fields-fail-at-once** (`validateGarmentFields` #8): asserts the exact error
  key set, guarding the "highlight every bad field" design, not just the first.
- **`getDefaultDiscountRange` #2** — "uses the last tier in array order, not the
  largest `max_qty`". A genuine differential test: the two candidate
  implementations disagree (`11` vs `101`) and the test pins the intended one.
- **`getDefaultDiscountRange` #5** — feeds the suggestion straight into
  `validateDiscountFields` and asserts it passes. A cross-function invariant
  ("the form never pre-fills a value its own validator would reject") that neither
  function's own tests could catch. Strongest test in the file.
- **`defaultSettings` sync tests** — the Decision-6 drift guard.

---

## 2. Trivial / tautological assertions

**None found.** Applying the task's strict definition (a test is trivial only if
it cannot fail given a wrong implementation):

- Every test imports the **real exports** from `./Panel.jsx` and invokes them.
  No test asserts a literal against itself.
- The two default-name tests compare `DEFAULT_GARMENT_NAMES`/`DEFAULT_OPERATION_NAMES`
  against `SERVER_GARMENT_NAMES`/`SERVER_OPERATION_NAMES` — **locally transcribed
  copies of `costing.go`, deliberately duplicated** (see the file's header comment
  at lines 15-17). This is an independent oracle, not a tautology: the expectation
  is not derived from the constant under test, so any drift in `Panel.jsx` fails
  the assertion. The tautological version would have been
  `expect(DEFAULT_OPERATION_NAMES).toHaveLength(8)` or deriving the expected list
  from `defaultSettings` — neither is what was written.
- The boundary assertions that *look* trivial (`expect(...valid).toBe(false)` on a
  single `0`) are legitimate boundary-value tests per the task's own carve-out, and
  each is paired with a passing case on the other side of the bound.
- `expect(result).toEqual({valid: true, errors: {}})` is used for the positive
  cases rather than the weaker `.valid).toBe(true)` — it also pins that no spurious
  error keys leak in. Stronger than required.

**Caveat, not a finding:** the independent-oracle property of tests #19/#20 depends
on a human transcription that a future editor *could* update in the same commit as
the drift it is meant to catch. That is inherent to any cross-language constant
mirror without codegen, the test file's header comment explicitly warns against it,
and the alternative (parsing Go from JS) is disproportionate for 12 strings.
Accepted as-is.

---

## 3. Gaps found in the unit test file

All minor. None affects a bound the Testing Strategy actually enumerates.

**U1 (Moderate-low) — `max_qty`'s own `> 0` branch is never independently exercised.**
`validateDiscountFields` (`Panel.jsx:297-301`) has two `max_qty` branches: an `if
(!isPositiveNumber(max_qty))` and an `else if (max_qty < min_qty)`. Every test hits
only the second. Concretely: **blank / non-numeric `max_qty` is untested** —
`validateDiscountFields` #5 loops bad values through `min_qty` and `percent` but
not `max_qty`. Delete the first branch and the suite still passes, yet
`{min_qty: 11, max_qty: "", percent: 5}` would then be accepted (`NaN < 11` is
`false`), producing a tier the backend rejects and making the *entire settings
object* unsaveable. That is precisely the failure mode Task 5's Deviation 3 was
written to prevent. The implementation is correct today; only the guard is missing.
*Fix cost: one line — add `max_qty` to the bad-value loop in `validateDiscountFields` #5.*

**U2 (Low) — the blank/non-numeric loops only run against one field per validator.**
Garments probe `quick_price` only; operations probe `quick_percent` only. The other
four numeric fields route through the same `isPositiveNumber`/`isNonNegativeNumber`
helpers, so the risk is a per-field wiring mistake, not a logic bug — and the
"reports every failing field at once" test partially covers the wiring. Acceptable.

**U3 (Low) — `isDuplicateName` is only ever exercised against `defaultSettings.garments`.**
The operations collection is never passed in. Same function, so no logic risk;
noted because operation-name dedup is also thin on the manual side (see M6).

None of U1-U3 blocks. U1 is worth fixing opportunistically if anyone touches the
file again; it is a test-only change with zero product risk.

---

## 4. Manual-verification adequacy — the deliberately untested surface

Scope assessed: `GarmentAddForm`, `OperationAddForm`, `DiscountAddForm`,
`DeleteRowButton`, all 10 JSX call sites, and the ad-hoc `orderForm`-clearing fix.
Sources: user-spec "How to Verify → User verifies" (aggregate), plus `Verify-user`
on Tasks 1/3/4/5, plus the ad-hoc fix's pending-verification line in `decisions.md`.

**Assessment: adequate in aggregate for the *planned* scope, with real holes on the
*unplanned* additions and on three boundary-accept cases.**

### 4a. What the checklists genuinely do cover — confirmed

| Requirement | Covered by |
|---|---|
| Both render sites, Изделия | Task 3 — add in Быстрый, switch to Продвинутый, values intact (the strongest per-mode check in the feature) |
| Both render sites, Усложнения | Task 4 (persists in both modes) + user-spec ("значения верны в обоих режимах") |
| Delete visible on added garment, absent on all 4 defaults, **both modes** | Task 3 |
| Delete absent on all 8 default operations | Task 4 + user-spec |
| Discount delete **disabled** (not hidden) on last remaining tier | Task 5 + user-spec |
| Garment `0` rejected in each of 3 fields, one at a time | Task 3 |
| Operation negative rejected in each of 3 fields | Task 4 |
| Discount `max_qty < min_qty` and `percent` outside 0-100 rejected | Task 5 |
| Name dedup incl. case + padding (`"пиджак "` vs `"Пиджак"`) | Task 3 + user-spec |
| Add + save + reload persistence, all 3 blocks | user-spec; Tasks 4, 5 |
| Delete + save + reload persistence | user-spec; Task 3 (garment); Task 5 (middle tier) |
| Middle-tier delete leaves the other two untouched | Task 5 |
| Increment 1 renames + the order ambiguity | Task 1 (explicitly flagged for the user) |

### 4b. Adjudicating the two gaps prior reviewers flagged

**"AC8 recalculation-in-both-modes never manually exercised" (Task 3's reviewer) —
DOWNGRADE. Not actually a gap.** user-spec's How to Verify does contain it: *"цена
по новому изделию считается без ошибок в обоих режимах"*. The reviewer was right at
the *task* level (no Task 3/4/5 `Verify-user` mentions calculation at all) but the
aggregate checklist covers it. **Not consequential; must not be dropped from Task 9.**

**"`min_qty <= 0` rejection never manually exercised" (Task 5's reviewer) — CONFIRMED
but NOT consequential.** Triple-defended: unit-tested (`validateDiscountFields` #2),
the input carries a browser-level `min="1"`, and backend `validateSettings` rejects
it independently. A one-line addition to Task 9 is cheap, but this does not block.

### 4c. Manual gaps this audit found — the consolidated list for Task 9

Ordered by consequence. **M1-M3 are the ones that would be genuine escapes if Task 9
skips them** — each is new behavior with *zero* automated coverage *and* zero
presence in any existing checklist.

**M1 (Highest) — the ad-hoc `orderForm`-clearing fix has no checklist entry anywhere.**
It postdates user-spec, has no task file, and therefore appears in no `Verify-user`
field — its only verification line lives buried in a `decisions.md` prose entry that
Task 9 has no reason to treat as a checklist. It is brand-new state-mutation logic
(`Panel.jsx:901-940`) with no unit test, and its failure mode is a hard server
rejection of the whole calculation. Task 9 must run both halves:
- Select a self-added **garment** in the calculator → delete it in Настройки
  *without reloading* → confirm the calculator's Изделие selector falls back to a
  remaining garment and a calculation succeeds.
- Give a self-added **operation** a non-zero count in the calculator → delete that
  operation in Настройки *without reloading* → run a calculation → must succeed,
  **not** fail with `unknown operation`.

**M2 (Moderate) — operation field `0` being *accepted* is never manually exercised.**
The garment-rejects-`0` / operation-accepts-`0` asymmetry is this feature's single
most confusable rule, and every manual checklist only tests the *rejecting* side
(Task 4: "negative value ... rejected"). If `OperationAddForm` were mis-wired to
`validateGarmentFields`, the unit suite would still be green (it tests the
functions, not the wiring) and every manual step would still pass. Task 9: add an
operation with `0` in Надбавка % (and ideally `0` in all three) → must be accepted.

**M3 (Moderate) — Enter-key handling in the three add-forms is untested everywhere.**
Tasks 3/4/5 all had to intercept Enter (`Panel.jsx:458`, `573`, `1806`) because the
add-forms are `<div>`s nested inside the page-wide `<form onSubmit={handleSaveSettings}>`;
an unintercepted Enter submits the *whole settings object* instead of adding a row.
This is a deliberate, non-obvious workaround with no unit test and no checklist line.
Task 9: in each of the three add-forms, press Enter in a text field → the row is
added, the page does **not** perform a full settings save.

**M4 (Low-moderate) — "Шлица" / "Декоративная отстрочка" are never named explicitly.**
This is the feature's #1 named risk (tech-spec Risks row 1): the stale
`defaultSettings.operations` had only 6 of 8, so a delete button implemented against
it would silently no-op on exactly these two. The unit suite covers the *constant*;
the manual side only says "8 стандартных усложнений", which a tester might satisfy by
spot-checking. Task 9: name the two explicitly — confirm neither shows a Удалить button.

**M5 (Low) — discount boundary-*accept* cases unexercised at UI level.**
`percent = 0`, `percent = 100`, and `max_qty === min_qty` are all unit-tested as
*passing*, but every manual discount step tests only rejections. A too-strict UI
(e.g. `min="1"` accidentally applied to percent) would pass the whole suite and the
whole checklist. Task 9: add a tier with `percent = 0`, one with `percent = 100`,
and one with `max_qty === min_qty` — all three must be accepted.

**M6 (Low) — operation-name duplicate rejection is thin.**
Task 4's `Verify-user` omits it entirely; only user-spec's generic
"изделие/усложнение с именем, которое уже есть" covers it, and only Task 3 gives a
concrete case-and-padding example. Task 9: repeat the `"пиджак "`-style probe on an
operation name (e.g. `"  шлица "`).

**M7 (Low) — blank-name rejection is unexercised manually.**
`isBlankName` is unit-tested but no checklist step tries submitting an empty or
whitespace-only name in any of the three forms.

**M8 (Low) — the discount add-form's refill-not-clear behavior.**
Task 5's Deviation 3 replaced "clear on success" with "refill from the just-added
row" precisely because a reachable case left both range fields blank (suggestion
`101`, admin enters `5-100` → old suggestion unchanged → resync never fired → blank
form). Self-review caught and fixed it; nothing verifies it stays fixed. Task 9:
add a tier whose `max_qty` equals the current suggestion minus one, then confirm
От/До are pre-filled, not blank.

### 4d. Known-and-accepted — Task 9 should NOT file these as bugs

- Degenerate `1000001–1000001` default suggestion on a freshly-loaded default
  settings object (server defaults end at `MaxQty: 1000000`) — explicitly accepted
  in user-spec Constraints and Task 5's edge cases.
- Deleting a middle discount tier leaves a coverage "hole" — accepted, tech-spec Risks.
- Deleting a garment/operation referenced by a previously-saved chat calculation may
  break *its* recalculation — accepted, user-spec Constraints.
- The Task 1 mode-order ambiguity is an **open question for the user**, not a defect —
  Task 9 must surface it for explicit confirmation (Task 1's `Verify-user` and the
  `decisions.md` Task 1 entry both flag it), not silently assert the current order.

---

## 5. Overall verdict

**PASS with 8 minor findings (3 unit-level U1-U3, 5+3 manual-level M1-M8). Nothing
blocks the Final Wave.**

**On job 1 — does the unit suite deliver on Decision 7's promise?** Yes, verifiably.
All 22 enumerated bounds are covered, all assertions invoke real exports, zero
tautologies, and several tests (the coercion loops, the cross-function suggestion/
validator invariant, the `defaultSettings` sync guards) exceed the mandate in exactly
the places the feature's top risk lives. The named risk — *client validation drifting
from backend `validateSettings`, letting `quick_price = 0` through* — is genuinely
converted from a one-time authorship check into a permanent regression guard. The
one real hole (U1, blank `max_qty`) is in a branch adjacent to that risk and is worth
a one-line fix, but it is a test-only gap with no product defect behind it today.

**On job 2 — is the deliberately untested surface adequately covered manually?**
For the *planned* Wave 3 scope: yes. The aggregate checklist genuinely walks both
render sites per component, delete presence/absence for default vs. added rows in
both sections, the disabled-last-tier state, and add/delete persistence across
reload — the four things the task named. For the *unplanned* additions it is not:
the ad-hoc `orderForm` fix (M1) and the Enter-interception workaround (M3) were both
introduced after user-spec was written and never made it into any checklist, and the
boundary-*accept* cases (M2, M5) are systematically absent because every checklist
was written from the "confirm it's rejected" angle. These are cheap to add and Task 9
is the right place — they do not warrant a new implementation task.

**Recommendation:** proceed to Task 9, with M1-M8 appended to its manual pass and
the 4d list handed over as known-accepted so QA doesn't re-litigate them. Fix U1
opportunistically. Note separately that independent code-reviewer/security-auditor/
test-reviewer rounds remain outstanding for Tasks 2-5 and the ad-hoc fix per
`decisions.md` — that is a review-process gap, outside this audit's scope, but Task 9
should know the code it is accepting has been largely self-reviewed.
