# Task 9 — Pre-deploy QA: pricing-panel-tweaks

**QA agent:** pre-deploy-qa (Task 9, Final Wave)
**Date:** 2026-08-16
**Scope:** whole feature, commits `4edea63^..HEAD` (Tasks 1-8 + 2 ad-hoc fixes)
**Verdict:** **PASS at source level — 19/19 acceptance criteria verified true against the final code. Feature is deploy-ready pending the user's manual browser pass (§3), which is the only way the remaining behavioral confirmation can be obtained (no agent-executable live check exists for this feature).**

---

## 1. Automated results

### 1.1 Test suite

```
$ cd /home/makar/PoshivOn/client && npx vitest run

 RUN  v3.2.7 /home/makar/PoshivOn/client
 ✓ src/pages/Panel.validation.test.js (41 tests) 8ms

 Test Files  1 passed (1)
      Tests  41 passed (41)
   Duration  497ms
```

**41 passed, 0 failed, 0 skipped** — 32 from Task 2 + 9 added by the Audit-Wave
integer-validation fix (`f8216ca`). Matches the expected count exactly.

### 1.2 Build

```
$ cd /home/makar/PoshivOn/client && npx vite build
vite v5.4.21 building for production...
✓ 53 modules transformed.
✓ built in 567ms
```

Passes, no warnings.

### 1.3 Coverage of the feature's scope

`client/src/pages/Panel.jsx` is the only source file this feature modifies, and
its new **pure** logic (7 exported helpers/constants) is covered by
`Panel.validation.test.js`. Per tech-spec Decision 7 the component/JSX surface
is deliberately untested and delegated to manual verification — Task 8's audit
confirmed 22/22 enumerated bounds covered with zero tautological assertions.
No coverage tooling is configured in this project, so no threshold check applies.

---

## 2. Acceptance-criteria verification against final source

Every criterion below was re-verified by reading the current
`client/src/pages/Panel.jsx` and `server/internal/service/costing.go`, not taken
from prior task reports.

### 2.1 user-spec — Инкремент 1 (3 criteria)

| # | Criterion | Status | Evidence |
|---|---|---|---|
| U1.1 | Mode order changed — the former second mode now shows first | **passed** | Pre-change (`git show 4edea63^`) `calculatorModes` = `[masterpiece "Шедевр", quick "По быстрому"]`; current `Panel.jsx:83-94` = `[quick "Быстрый", masterpiece "Продвинутый"]`. The former second entry (`quick`) is now index 0, and `Panel.jsx:1302` renders by `.map` in array order. **Note:** satisfies the AC as literally written; user-spec's narrative step 2 says the opposite — see §4, needs user sign-off. |
| U1.2 | Labels are "Продвинутый"/"Быстрый"; `masterpiece`/`quick` unchanged in code, DB, calculations | **passed** | `Panel.jsx:85-92` — `value: "quick"` / `value: "masterpiece"` untouched. `grep -rn "Шедевр\|По быстрому" client/src server/` → 0 matches. `git diff 4edea63^ HEAD -- server/` → empty (no backend/DB change at all). Fallback label at `Panel.jsx:1286` reads "Продвинутый". |
| U1.3 | "Скидки по партиям" → "Скидки за количество" everywhere (shared component, both modes) | **passed** | `Panel.jsx:1886` — `title="Скидки за количество"` inside `DiscountsBlock`, which is the single component rendered from both mode branches (`Panel.jsx:1381`, `1475`). `grep "Скидки по партиям"` → 0 matches repo-wide. |

### 2.2 user-spec — Инкремент 2 (8 criteria)

| # | Criterion | Status | Evidence |
|---|---|---|---|
| U2.1 | 3 "Добавить" buttons visible at all times, identical form/behavior in both calculator modes | **passed** | Quick branch: `GarmentAddForm` `:1355`, `OperationAddForm` `:1378`, `DiscountsBlock`→`DiscountAddForm` `:1381`/`:1915`. Masterpiece branch: `:1427`, `:1453`, `:1475`/`:1915`. Each form is one module-scope component invoked with identical props at both sites — no per-mode variant exists, so field set and behavior cannot diverge. |
| U2.2 | Add-form shows all fields at once, no hidden/fake defaults for unseen fields | **passed** | `GarmentAddForm` draft = `{name, quick_price, base_minutes, complexity_coeff}` (`:428`), all four rendered unconditionally (`:499-516`); `OperationAddForm` = all four (`:539`, `:614-631`). No `isQuickCalculator` reference inside either component. Empty numeric fields are rejected, not defaulted: `toFiniteNumber("")` → `NaN` (`:227-235`), so nothing silently becomes 0. |
| U2.3 | Add-form rejects garment `quick_price`/`base_minutes`/`complexity_coeff` ≤ 0, and discount `min_qty` ≤ 0 / `max` < `min` / `percent` outside [0,100] | **passed** | `validateGarmentFields` (`:279-297`) — all three via `isPositiveNumber`. `validateDiscountFields` (`:318-341`) — `min_qty` positive, `max_qty` positive and `>= min_qty`, `percent` finite and in [0,100]. Unit-covered by tests #2-#5 in each validator's describe block. |
| U2.4 | An added row + "Сохранить изменения" survives reload and prices correctly in both modes | **passed (source-level); behavior deferred to manual** | Add handlers write into the same `settings` state the existing `handleSaveSettings` POSTs whole-object (`:925-991`); no separate persistence path exists. Because the form always collects all 4 fields (U2.2), a saved garment carries valid `quick_price`, `base_minutes`, `complexity_coeff` for both mode calculations — the `quick_price = 0` failure class is architecturally excluded. End-to-end save/reload/price is **manual item A4/B1** below. |
| U2.5 | Discount add-form pre-fills a valid full range continuing the last tier (`min = last max + 1`, `max = that min`) | **passed** | `getDefaultDiscountRange` (`:346-351`) returns `{min_qty: nextMinQty, max_qty: nextMinQty}` — both ends, never blank/zero. Wired at `DiscountAddForm:1781-1783` and re-synced whenever the tier list changes (`:1791-1794`), refilled after each add (`:1824-1826`). Unit tests assert both keys in all 4 cases plus a cross-function test that the suggestion passes `validateDiscountFields`. |
| U2.6 | Duplicate name (case-insensitive, trimmed) blocked client-side with "Такое название уже есть" | **passed** | `isDuplicateName` (`:267-273`) trims + lowercases **both sides**. `GarmentAddForm:453-456` and `OperationAddForm:568-571` both emit exactly `"Такое название уже есть"` and `return` before calling the add handler — no silent overwrite possible. |
| U2.7 | "Удалить" only on admin-added Изделия/Усложнения rows, absent on the 4/8 defaults; removal persists after reload | **passed** | `DeleteRowButton` returns `null` when `!isDeletable` (`:409-412`). Gates: `!DEFAULT_GARMENT_NAMES.includes(name)` at `:1345` and `:1414`; `!DEFAULT_OPERATION_NAMES.includes(name)` at `:1368` and `:1440` — all four call sites, both modes. Constants at `:101-112` verified char-for-char against `DefaultUserSettings()` (see T3 below), so "Шлица"/"Декоративная отстрочка" are correctly protected. Persistence rides the same whole-object save. |
| U2.8 | Discount "Удалить" works on any row except the last remaining one (disabled there) | **passed** | `DiscountsBlock:1882` — `canDeleteRow = settings.batch_discounts.length > 1`; the button renders with `isDeletable` always true and `disabled={!canDeleteRow}` (`:1905-1910`), i.e. **visible but disabled**, with `disabledHint="Последний диапазон удалить нельзя"` surfaced via `title`/`aria-label` (`:419-420`). |

### 2.3 tech-spec — technical criteria (8)

| # | Criterion | Status | Evidence |
|---|---|---|---|
| T1 | `calculatorModes[*].value` unchanged; Cyrillic labels appear only in `label`/`description` | **passed** | `:85`, `:90` — `"quick"` / `"masterpiece"`. `grep -rn "Шедевр\|По быстрому"` → 0 hits anywhere in `client/src` or `server/`. |
| T2 | No new backend HTTP endpoints | **passed** | `git diff --stat 4edea63^ HEAD -- server/` → **empty output**. No server file was touched at all, so `handler/http.go`'s route table is byte-identical. |
| T3 | `DEFAULT_GARMENT_NAMES` (4) / `DEFAULT_OPERATION_NAMES` (8) exactly match `DefaultUserSettings()` | **passed** | Read `costing.go:227-242` directly this pass. Garments: Пиджак, Юбка, Рубашка, Платье → identical to `Panel.jsx:101`. Operations: Карман накладной, Карман прорезной, Подклад, Потайная молния, Воротник, Манжеты, **Шлица**, **Декоративная отстрочка** → identical to `Panel.jsx:103-112`, 8/8. `defaultSettings.operations` (`:136-145`) also now carries all 8 (Decision 6 drift fixed). |
| T4 | Client validation bounds equal to or stricter than backend `validateSettings` for every field | **passed** | Backend (`costing.go:593-657`) vs client (`Panel.jsx:279-341`): garment `BaseMinutes>0` = `>0`; `ComplexityCoeff>0` = `>0`; `QuickPrice>=0` vs client **`>0` (stricter, deliberate — `costing.go:720` breaks on 0)**; operation all three `>=0` = `>=0`; tier `MinQty>0`, `MaxQty>=MinQty`, `Percent∈[0,100]` all equal. Client is additionally **stricter** on the 6 int/int64 fields via `isIntegerValue` (`:260`), guarding a JSON-unmarshal 400 the backend validator never reaches. No field is looser. |
| T5 | Add-forms / delete button render in both mode branches with no divergence | **passed** | Call-site census by grep: `GarmentAddForm` ×2 (`:1355`, `:1427`), `OperationAddForm` ×2 (`:1378`, `:1453`), `DiscountsBlock` ×2 (`:1381`, `:1475`), `DeleteRowButton` ×5 (garment ×2, operation ×2, discount ×1-in-shared-component). Props are identical strings at each paired site; all read the same `settings` object, so an add/delete is immediately visible after switching modes. |
| T6 | `getDefaultDiscountRange` pre-fills both `min_qty` and `max_qty` | **passed** | `:350` returns both keys from the same `nextMinQty`; a fresh form can never hold a blank/invalid range. Also covered by 5 unit tests including the suggestion→validator invariant. |
| T7 | `npx vitest run` passes | **passed** | 41/41, §1.1. |
| T8 | No regressions in existing settings fields (materials, urgency, market bands, existing edit inputs) | **passed** | `git diff 4edea63^ HEAD -- client/src/pages/Panel.jsx` shows **no modification to any existing handler definition** (`handleRuleChange`, `handleGarmentChange`, `handleOperationSettingChange`, `handleDiscountChange`, `handleMaterialChange`, `handleUrgencyChange`, `handleMarketChange`, `handleSaveSettings`, `syncOrderForm`). The only edits to pre-existing lines are: the `calculatorModes` reorder/relabel, the `DiscountsBlock` title, two `DiscountsBlock` call sites gaining 2 props each, and per-row name cells being wrapped in a flex header to host the delete button (grid template unchanged). Materials/Urgency/Market sections are untouched. `vite build` clean. Final smoke of the rendered panel is **manual item E1**. |

**Totals: 19 criteria checked — 19 passed, 0 failed, 0 not_verifiable at source level.**
U2.4 and T8 are source-verified but have a residual behavioral component that only
the browser pass can close (items A4/B1 and E1).

---

## 3. Manual verification checklist — definitive, consolidated, ready to execute

Merged and deduplicated from three sources: user-spec "How to Verify → User
verifies"; the `Verify-user` fields of Tasks 1, 3, 4, 5; and gaps M1-M8 from
Task 8's test audit. Run on poshivon.ru/panel (or a local dev build) logged in as
an admin. Section F lists things that look wrong but are **not** bugs — read it
before filing anything.

### A. Инкремент 1 — labels and order

- [ ] **A1.** Open /panel → «Настройки». The «Режим расчёта» block shows exactly two
      cards, labelled **«Быстрый»** and **«Продвинутый»** — neither «Шедевр» nor
      «По быстрому» appears anywhere.
- [ ] **A2. (needs your explicit decision, not just pass/fail)** The card order is
      currently **«Быстрый» first, «Продвинутый» second**. This is a deliberate swap
      of the previous order. user-spec's acceptance criterion ("the former second
      mode now shows first") requires exactly this, but user-spec's narrative
      sentence describes the opposite. **Confirm which order you actually want.** If
      you want «Продвинутый» first, say so — it is a one-line revert of the array
      swap, keeping the renames.
- [ ] **A3.** Switching the active mode updates the «Активный режим» box on the right
      to the matching label, and the price calculator keeps working in both modes
      (the internal mode keys were not renamed).
- [ ] **A4.** The discount section is titled **«Скидки за количество»** (not «Скидки
      по партиям») — check in **both** calculator modes.

### B. Инкремент 2 — Изделия (garments)

- [ ] **B1.** In Быстрый mode, click «Добавить» under Изделия. The form «Новое
      изделие» shows **all four** fields at once: Название, Мин. цена / шт, База мин,
      Коэфф. сложности. Fill all four (e.g. Пальто / 9000 / 300 / 1.7) → «Добавить».
      The row appears in the list.
- [ ] **B2.** Switch to Продвинутый mode without saving — the new row is there too,
      with База мин and Коэфф. showing exactly what you typed (not 0). Switch back —
      Мин. цена / шт is still what you typed.
- [ ] **B3.** Click «Сохранить изменения», then **reload the page**. The new изделие
      is still present with all values intact, in both modes.
- [ ] **B4.** Run a price calculation with the new изделие in **Быстрый** mode → a
      price is produced, no error, no NaN/undefined. Repeat in **Продвинутый** mode →
      same. *(This is the AC that most directly proves the "all fields at once"
      design works.)*
- [ ] **B5. Rejections — one field at a time.** In the add-form, enter `0` in
      **Мин. цена / шт** (other fields valid) → «Добавить» must be refused with an
      inline error. Repeat with `0` in **База, мин**. Repeat with `0` in **Коэфф.
      сложности**. All three must be rejected.
- [ ] **B6. Fractional rejection (added by the audit fix).** Enter `10.5` in
      **База, мин** → rejected with a "должно быть целым числом" message. Same for
      `7000.5` in **Мин. цена / шт**. But `1.65` in **Коэфф. сложности** must be
      **accepted** (it is a genuine decimal field).
- [ ] **B7. Duplicate name.** Try adding a изделие named `пиджак ` (lowercase, with a
      trailing space) → must be refused with exactly **«Такое название уже есть»**,
      and no row is added or overwritten.
- [ ] **B8. Blank name (M7).** Try adding with an empty Название, then with a name of
      only spaces → both refused.
- [ ] **B9. Delete visibility.** Confirm **no** «Удалить» button next to **Пиджак,
      Юбка, Рубашка, Платье** — check in **both** modes. Confirm the row you added
      **does** have one.
- [ ] **B10.** Click «Удалить» on your added изделие → the row disappears. Save,
      reload → it stays gone and does not come back.

### C. Инкремент 2 — Усложнения / Операции

- [ ] **C1.** In Быстрый mode, «Добавить» under Усложнения. The form «Новая операция»
      shows all four fields: Название, Надбавка %, Доп. минуты, Доп. материал / шт.
      Add one (e.g. Косая бейка / 6 / 10 / 40).
- [ ] **C2.** Switch to Продвинутый mode («Операции») — the new row is present with
      Минуты and Материалы / шт exactly as typed. Save, reload → still there in both
      modes.
- [ ] **C3. Rejections.** Enter a negative value in **Надбавка %** → refused. Repeat
      for **Доп. минуты**, then **Доп. материал / шт** → each refused.
- [ ] **C4. Zero must be ACCEPTED (M2 — the most confusable rule in this feature).**
      Add an operation with `0` in **Надбавка %** (other fields valid) → must be
      **accepted**, not rejected. Then add another with `0` in **all three** numeric
      fields → also accepted. *(Garments reject 0; operations accept it. If an
      operation with 0 is refused, that is a real bug — report it.)*
- [ ] **C5. Fractional rejection.** `10.5` in **Доп. минуты** → rejected as
      non-integer; `40.5` in **Доп. материал / шт** → rejected. But `7.5` in
      **Надбавка %** must be **accepted**.
- [ ] **C6. Blank numeric fields.** Leave **Доп. минуты** empty (not `0`, actually
      blank) → refused. You must type an explicit `0`; nothing silently defaults.
- [ ] **C7. Duplicate name (M6).** Try adding an operation named `  шлица ` (lowercase
      + padding) → refused with **«Такое название уже есть»**.
- [ ] **C8. Delete visibility — name the two risky ones explicitly (M4).** Confirm no
      «Удалить» on any of the 8 defaults, and specifically check **«Шлица»** and
      **«Декоративная отстрочка»** — these two were missing from an outdated client
      list and are the feature's #1 named risk. Neither may show a delete button, in
      either mode.
- [ ] **C9.** Delete your added operation → gone; save, reload → stays gone.

### D. Инкремент 2 — Скидки за количество

- [ ] **D1.** Click «Добавить» under Скидки за количество. The «Новый диапазон» form
      is **pre-filled on both ends** — «От» and «До» both carry a number, neither is
      blank or 0.
- [ ] **D2.** Add a tier using the suggested default range (fill in a percent) →
      accepted. Save → **no save error**.
- [ ] **D3. Refill, not clear (M8).** With a suggestion of e.g. `101`, edit the form
      to `5`–`100` and add it. Then look at the form again: «От» and «До» must be
      **pre-filled again**, never left blank. *(A blank range here is what would block
      saving the whole settings object.)*
- [ ] **D4. Rejections.** «До» below «От» → refused. Percent `-1` → refused. Percent
      `101` → refused. «От» `0` → refused. «До» left blank → refused.
- [ ] **D5. Boundary values that must be ACCEPTED (M5).** Add a tier with
      **percent = 0** → accepted. Add one with **percent = 100** → accepted. Add one
      where **«До» equals «От»** (e.g. 200–200) → accepted. All three are legal; a
      refusal here is a real bug.
- [ ] **D6. Fractional.** `10.5` in «От» or «До» → rejected as non-integer. But a
      fractional **percent** (e.g. `7.5`) must be **accepted**.
- [ ] **D7. Middle-tier delete.** With at least 3 tiers, delete the **middle** one →
      save → reload. Exactly that tier is gone; the other two are unchanged.
- [ ] **D8. Last tier.** Delete tiers until only **one** remains. Its «Удалить» button
      must still be **visible but greyed out / disabled** (not missing), and hovering
      it shows «Последний диапазон удалить нельзя».

### E. Cross-cutting — the two ad-hoc fixes and general smoke

- [ ] **E1. General regression smoke.** In Продвинутый mode, confirm the untouched
      sections still render and edit normally: Общие правила, Материалы, Срочность,
      Рыночные диапазоны, and the existing per-row edit inputs on Изделия/Операции/
      Скидки. Change one value, save, reload → it persisted.

- [ ] **E2. Stale-reference fix, garment half (M1 — highest-priority item; this code
      has ZERO automated coverage and appears in no other checklist).**
      1. Add a new изделие via «Добавить» and save.
      2. Go to the calculator and **select that self-added изделие** in the Изделие
         dropdown.
      3. Return to «Настройки» and **delete that same изделие** — *without reloading
         the page*.
      4. Go back to the calculator. The Изделие selector must have **fallen back to a
         remaining изделие**, not be stuck on the deleted name or blank.
      5. Run a calculation → it must **succeed**. Before this fix it failed
         server-side.

- [ ] **E3. Stale-reference fix, operation half (M1, second half).**
      1. Add a new операция via «Добавить» and save.
      2. In the calculator, give that self-added операция a **non-zero count**
         (e.g. 2).
      3. Return to «Настройки» and **delete that same операция** — *without
         reloading*.
      4. Go back to the calculator and run a calculation → it must **succeed**, and
         must **not** fail with an `unknown operation` / generic save-or-calc error.

- [ ] **E4. Enter key must not save the whole settings object (M3 — deliberate
      non-obvious workaround, no automated coverage).** In **each** of the three
      add-forms (Новое изделие, Новая операция, Новый диапазон), type valid values
      then press **Enter** inside a text/number field. Expected: the **row is added**
      and the page does **not** perform a full settings save (no "saved" notice, no
      network save). Repeat for all three forms.

### F. Known and accepted — do NOT report these as bugs

These are documented, deliberate decisions in user-spec Constraints / tech-spec
Risks. Seeing them means the implementation is correct, not broken.

1. **Degenerate `1000001 – 1000001` suggestion.** On a freshly-loaded default
   settings object the server's last tier ends at `1000000`, so the add-form
   suggests `1000001–1000001`. That is a *valid starting range you are expected to
   edit*, not a recommendation. Accepted per user-spec Constraints.
2. **A "hole" between discount tiers after deleting a middle one.** If you had
   1–10 / 11–50 / 51–100 and delete 11–50, quantities 11-50 fall into no tier and
   silently get **0 %** discount rather than an error. The panel deliberately does
   not check tier continuity. Accepted per user-spec Constraints and tech-spec Risks.
3. **Deleting a изделие/операция does not rewrite already-saved chat
   calculations.** Old saved results stay as they are; **re-calculating** an old
   order that references a deleted item may fail. Accepted per user-spec Constraints.
4. **Default rows can never be deleted.** There is no «Удалить» on the 4 default
   изделия or the 8 default усложнения by design — the backend re-injects them on
   every save, so a delete button there would silently do nothing. Absence of the
   button is the correct behavior.
5. **Settings are per-user.** A row you add appears only in your own account's
   settings, not as a site-wide price list. Expected, not a bug.

---

## 4. Open item requiring the user's decision

**Calculator mode order (carried forward from Task 1, unresolved).** user-spec
contains two mutually exclusive statements: narrative step 2 describes
«Продвинутый» first, while the acceptance criterion requires "the former second
mode now shows first" — which, given the original order (`Шедевр`, `По быстрому`),
means **«Быстрый» first**. The implementation follows tech-spec Decision 1 and the
acceptance criterion: **«Быстрый» is currently first**. QA marks U1.1 passed against
the AC as written, but this is a coin-flip no automated check can adjudicate — it
needs the user's explicit sign-off at checklist item **A2**. If the other order is
wanted, the fix is reverting the array swap only (labels stay) plus correcting
whichever user-spec sentence is wrong.

---

## 5. Findings

**None.** Zero critical, zero major, zero minor findings against the final code.
Task 6's one major finding (F1, fractional values reaching an `int` backend field)
and Task 8's U1 (untested `max_qty` branch) were both closed by the ad-hoc fix in
`f8216ca`, verified this pass: `isIntegerValue` (`Panel.jsx:260`) is applied to
exactly the 6 int/int64 fields and to none of the 3 genuine float fields, and the
test suite grew 32 → 41 accordingly.

**Process note, not a defect:** the ad-hoc `orderForm` fix (`569c72f`) carries only
a self-review plus Task 7's security pass, and the integer-validation fix
(`f8216ca`) has its reviewer round recorded as run in `100bcfe`. Both are covered
behaviorally by checklist items **E2/E3** and **B6/C5/D6** respectively — which is
precisely why those items are mandatory rather than optional in §3.

---

## 6. Verdict

| Gate | Result |
|---|---|
| `npx vitest run` | **PASS** — 41/41 |
| `npx vite build` | **PASS** |
| user-spec AC, Инкремент 1 (3) | **3/3 passed** |
| user-spec AC, Инкремент 2 (8) | **8/8 passed** |
| tech-spec AC (8) | **8/8 passed** |
| Manual browser checklist | **NOT YET WALKED — handed to the user in §3** |

**Source-level QA: PASS.** No acceptance criterion failed, no finding was raised,
and the two Audit-Wave defects are confirmed fixed. The feature is technically
deploy-ready.

**Not yet confirmed:** the §3 manual checklist has not been executed — this agent
has no authenticated admin session and this feature's Agent Verification Plan
explicitly provides no MCP or curl-checkable surface. Per Task 9's own AC ("User
confirms the manual checklist was walked through"), the feature must not be
declared fully accepted until the user reports back on §3, and in particular on
**A2** (mode-order sign-off), **E2/E3** (the untested `orderForm` fix), **E4**
(Enter interception), and **C4/D5** (boundary-accept cases) — the items with no
automated coverage anywhere.
