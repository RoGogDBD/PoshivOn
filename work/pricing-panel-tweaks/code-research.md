# Code Research: pricing-panel-tweaks

Repo root: `/home/makar/PoshivOn`. Stack: React (Vite) client in `client/`, Go backend
(`net/http` + GORM/MySQL) in `server/`. No existing `code-research.md` prior to this file
(user-spec.md is still the unfilled scaffold; an interview.yml with an initial free-form
task description already exists at `work/pricing-panel-tweaks/logs/userspec/interview.yml`
and explicitly defers "is Шедевр/По быстрому a key?" and "exact add-row fields" to this
research).

## 1. Where /panel is implemented

- **Route entry**: `client/src/App.jsx` — path-based routing without a router library.
  Line 30: `if (pathname.startsWith("/auth") || pathname.startsWith("/panel") || pathname.startsWith("/privacy"))`,
  line 41: `else if (window.location.pathname.startsWith("/panel"))` renders the `Panel` page.
- **Page component**: `client/src/pages/Panel.jsx` (1525 lines) — single large component,
  no sub-routing inside `/panel`; section switching is done via local React state
  (`activeSection`: `"workspace" | "settings" | "users"`), not URL segments.
- **Stack**: React functional components + hooks (`useState`/`useEffect`/`useMemo`/`useRef`),
  Tailwind-style utility classes mixed with a legacy `panel__*`/`panel-*` BEM-ish class set
  (see `client/src/App.css`). No component library, no state management library (Redux/Zustand) —
  all state is local to `Panel`. API calls go through `client/src/utils/panelApi.js` (thin
  fetch wrapper) and `client/src/utils/accessApi.js`.
- **Backend**: Go, stdlib `net/http` router built by hand in `server/internal/handler/routes.go`
  and `server/internal/handler/http.go` (manual path-segment switch, not a framework like
  gin/chi). ORM: GORM (`gorm.io/gorm`) over MySQL (`server/internal/db/db.go` uses
  `github.com/go-sql-driver/mysql` — despite the repository file being named `postgres.go`,
  the actual driver/DSN is MySQL: `sql.Open("mysql", dsn)`).

## 2. Are "Шедевр" / "По быстрому" plain strings or keys/IDs?

**They are plain display strings only, confined to `client/src/pages/Panel.jsx`. They are
NOT used as keys/IDs/enum values anywhere** — the real identifier for the two calculator
modes is the separate, already-English internal key `calculator_mode` with values
`"masterpiece"` / `"quick"`. Renaming/reordering the Cyrillic labels is safe with respect to
stored data and calculation logic, **as long as the `value: "masterpiece"/"quick"` fields in
the `calculatorModes` array (below) are left untouched** and only `label` is edited.

Evidence:

- `client/src/pages/Panel.jsx:83-94` — the only place the two Cyrillic strings exist as UI copy:
  ```js
  const calculatorModes = [
    { value: "masterpiece", label: "Шедевр", description: "..." },
    { value: "quick", label: "По быстрому", description: "..." },
  ];
  ```
  `value` (`"masterpiece"`/`"quick"`) is what's sent to/from the backend and used in all
  conditional logic (`isQuickCalculator = calculatorMode === "quick"` at line 414); `label`
  is pure display text rendered at lines 798, 828, and inside the mode-picker buttons
  (813-843). Swapping the two objects' order in this array reorders the mode-picker cards;
  changing only `label` renames them without touching `value`.
- `client/src/pages/Panel.jsx:798` — `{calculatorModes.find((mode) => mode.value === calculatorMode)?.label || "Шедевр"}`
  (the "Активный режим" badge). The `"Шедевр"` here is only a JS fallback default string, not
  a stored value.
- Backend: `server/internal/service/costing.go:194-195` — `calculatorModeMasterpiece = "masterpiece"`,
  `calculatorModeQuick = "quick"` — the Go constants are the English words, never the Cyrillic
  labels. `PricingRules.CalculatorMode` (`costing.go:29`) stores `"masterpiece"`/`"quick"` in
  the `pricing_rules` JSON column (see §3). `CalculationResult.CalculationMode` (`costing.go:118`)
  likewise stores `"masterpiece"`/`"quick"` per saved calculation row — confirmed by
  `server/internal/service/costing_test.go:281-283` (`result.CalculationMode != calculatorModeQuick`).
- Marketing/landing pages reference the **English** words `quick`/`masterpiece` as prose, not
  the Cyrillic labels, and are unrelated to this rename: `client/src/sections/FeaturesSection.jsx:19`
  (`"Режимы quick и masterpiece"`) and `client/src/sections/FAQSection.jsx:17`
  (`"Чем отличаются режимы quick и masterpiece?"`). These do not need to change for this feature.
- No occurrence of `"Шедевр"` or `"По быстрому"` in migrations, Go code, or any other file —
  confirmed via `grep -rn "Шедевр\|По быстрому"` across the repo (excluding `node_modules`/`dist`);
  the only hits are `client/src/pages/Panel.jsx:86,91,798` and the feature's own planning files
  under `work/pricing-panel-tweaks/`.

## 3. Data shape for Изделия / Усложнения / Скидки по партиям

All three sections are part of one **per-user** `UserSettings` object, persisted as a single
row per `user_id` in MySQL table `user_settings`, with each top-level field stored in its own
JSON column (not normalized rows/tables, no per-item primary keys). "Per-user" matters: this
is not one global site-wide pricing config — every panel user (admin or access-granted) has
their own independent settings row (`user_id VARCHAR(255) PRIMARY KEY` in
`server/migrations/002_costing_schema.up.sql:6`).

**Go domain types** — `server/internal/service/costing.go:22-87`:
```go
type BatchDiscount struct {
    MinQty  int     `json:"min_qty"`
    MaxQty  int     `json:"max_qty"`
    Percent float64 `json:"percent"`
}
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
type UserSettings struct {
    PricingRules   PricingRules
    Garments       map[string]GarmentConfig    // keyed by garment NAME, e.g. "Пиджак"
    Operations     map[string]OperationConfig  // keyed by operation NAME, e.g. "Карман накладной"
    Materials      map[string]MaterialConfig
    BatchDiscounts []BatchDiscount             // plain array, no id/name — index only
    Urgency        map[string]UrgencyRule
    MarketBands    map[string]MarketBand
}
```
Frontend mirrors this exactly as a plain JS object — `client/src/pages/Panel.jsx:96-192`
(`defaultSettings`), e.g.:
```js
garments: {
  "Пиджак": { base_minutes: 260, complexity_coeff: 1.6, quick_price: 7000 },
  ...
},
operations: {
  "Карман накладной": { additional_minutes: 15, additional_material_per_unit: 80, quick_percent: 8 },
  ...
},
batch_discounts: [
  { min_qty: 1, max_qty: 10, percent: 0 },
  { min_qty: 11, max_qty: 50, percent: 5 },
  { min_qty: 51, max_qty: 100, percent: 10 },
],
```

**Key implication for "add a row"**: `garments` and `operations` are maps keyed by the
human-readable Cyrillic **name** — there is no separate id field. Adding a row means adding a
new key to the map (the name the admin types in the add-form IS the identifier, and must be
non-empty/unique per `validateSettings`, see §7 problems). `batch_discounts` is a plain array
appended to — order/index is the only identity, no dedupe check exists beyond the
min/max/percent range validation.

**DB schema** — `server/migrations/002_costing_schema.up.sql:1-14` (base table, MySQL,
originally only 3 JSON columns) and `003_pricing_and_chat_delete.up.sql:1-7` (adds the rest):
```sql
CREATE TABLE IF NOT EXISTS user_settings (
    user_id VARCHAR(255) PRIMARY KEY,
    base_prices JSON NOT NULL,        -- legacy, always written as "{}" now (postgres.go:160)
    surcharge_percent JSON NOT NULL,  -- legacy, always written as "{}" now (postgres.go:164)
    batch_discounts JSON NOT NULL,
    updated_at TIMESTAMP ...,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
ALTER TABLE user_settings
    ADD COLUMN pricing_rules JSON NULL,
    ADD COLUMN garments JSON NULL,
    ADD COLUMN operations JSON NULL,
    ADD COLUMN materials JSON NULL,
    ADD COLUMN urgency JSON NULL,
    ADD COLUMN market_bands JSON NULL;
```
Read/write is whole-object JSON marshal/unmarshal per column, no per-row SQL —
`server/internal/repository/postgres.go:159-237` (`UpsertSettings`) marshals each Go field to
its own JSON column and does a GORM `OnConflict` upsert on `user_id`; `GetSettings` (lines
239-287) unmarshals each column back into `service.UserSettings`, defaulting via
`service.DefaultUserSettings()` (`costing.go:210+`) for any null column.

**Rendering** — `client/src/pages/Panel.jsx`:
- Изделия (quick mode fields only — `quick_price`): lines 847-864, field label "Мин. цена / шт" at line 858.
- Изделия (masterpiece mode fields — `base_minutes`, `complexity_coeff`): lines 903-923 (different
  section instance, same underlying `settings.garments` map, different fields shown).
- Усложнения (quick mode, `quick_percent`): lines 866-883.
- Операции (masterpiece mode, `additional_minutes` + `additional_material_per_unit`,
  **note: labelled "Операции" not "Усложнения" in this mode**): lines 925-945.
- Скидки по партиям (`DiscountsBlock` component, shared by both modes): lines 1258-1282,
  rendered at line 885 (inside quick branch) and line 966 (inside masterpiece branch) — same
  component instance both times, so a single "Добавить" implementation there covers both modes.

**Important scoping nuance**: the task's screenshot/description ("Изделия" with "Мин. цена / шт",
"Усложнения" as % surcharges) matches the **quick-mode** view specifically. The masterpiece-mode
view shows the same underlying `garments`/`operations` data through different field sets and a
different section title ("Операции" instead of "Усложнения"). Both views edit the *same*
`settings.garments` / `settings.operations` objects, so any row added in one mode is visible
(with its non-edited fields blank/zero) in the other mode too — see §7.

## 4. Existing "add new row" / CRUD capability

**None exists anywhere in the panel or backend.** Confirmed by:
- No endpoint other than `GET/POST /api/v1/users/settings` touches garments/operations/
  batch_discounts — full route table at `server/internal/handler/http.go:63-90`:
  `settings` (GET/POST, whole-object), `chats` (POST/GET/DELETE/restore/calculate/calculations),
  `market-feedback` (POST). No `POST /garments`, `/operations`, `/batch-discounts`, no per-item
  PUT/PATCH/DELETE.
- `grep -rn "handleAdd\|onAdd\|addRow\|newRow\|Добавить" client/src` returns **no matches** —
  no dynamic add-row UI pattern exists anywhere in the client, including the one other list-like
  admin UI (`AdminUsersSection.jsx`), which only supports *toggling* an existing user's access
  checkbox (`onToggle` in `client/src/components/AdminUsersSection.jsx:48-53`) — it has no
  "create new row" flow to model after.
- The three target sections (`garments`, `operations`, `batch_discounts`) currently only support
  *editing* existing keys/indices in place (`handleGarmentChange`, `handleOperationSettingChange`,
  `handleDiscountChange` — `Panel.jsx:479-525`) and are seeded entirely from
  `defaultSettings`/`DefaultUserSettings()` plus whatever was previously saved; there is no
  client-side "add key to map" or "push to array" helper anywhere in the file.

**Conclusion**: all 3 add-row forms + the row-creation logic need to be built from scratch on
the client. On the backend, no new endpoint is strictly required — see §5 — but the client
needs new state-mutation functions (add-a-map-key / push-to-array) that don't exist yet.

## 5. Current save behavior

**Whole-panel-form submit via a single Save button — no autosave, no per-row save.**
- All settings edits (`Мин. цена / шт`, base minutes, batch discount %, everything in the
  "Настройки модели" section) live in one `<form className="space-y-5" onSubmit={handleSaveSettings}>`
  wrapper — `client/src/pages/Panel.jsx:808`. Every input inside it (`handleGarmentChange`,
  `handleOperationSettingChange`, `handleMaterialChange`, `handleDiscountChange`,
  `handleUrgencyChange`, `handleMarketBandChange`, `handleRuleChange`) only mutates local React
  state (`setSettings`) — no network call fires on individual field change.
- The single submit button (`Panel.jsx:1006-1012`, text "Сохранить изменения" / "Сохраняем...")
  triggers `handleSaveSettings` (`Panel.jsx:550-566`), which does exactly one network call:
  `saveUserSettings(settings)` → `POST /api/v1/users/settings` with the **entire** `settings`
  object as JSON body (`client/src/utils/panelApi.js:41-46`; handler
  `server/internal/handler/http.go:96-109` → `service.CostingService.SaveUserSettings` →
  `validateSettings` (§7) → `repo.UpsertSettings` full-column overwrite, see §3).
- **Implication for "add + persist"**: the simplest, most consistent-with-existing-code approach
  is to make "Добавить" only add a new row to local `settings` state (no network call of its
  own), and let it ride the existing Save button / `handleSaveSettings` flow to persist — this
  requires no new backend endpoint. Auto-saving on add (its own POST) would be a **new** pattern
  not used anywhere else in this panel and would diverge from every other field's behavior.

## 6. Auth / access control on /panel

Present and already wired (not something this feature needs to add), for context:
- Client-side gate: `Panel.jsx:271-317` bootstrap effect — calls `checkAuthStatus()` →
  `fetchAuthProfile()` → `fetchAccessState()` (from `client/src/utils/yandexAuth.js` and
  `client/src/utils/accessApi.js`); redirects to `/` on any auth failure
  (`window.location.replace("/")`, lines 280, 303); shows a distinct "no access" screen
  (`AccessRequestBanner`) when authenticated but `has_access` is false (`hasPanelAccess` helper,
  `Panel.jsx:243-244`).
- Server-side: `server/internal/handler/routes.go:30-64` — route table comment documents
  `/api/v1/users/ → RequireSameOrigin → RequireAuth → RequireAccess` and
  `/api/v1/admin/ → RequireSameOrigin → RequireAuth → RequireAdmin`; middleware implementations
  in `server/internal/handler/middleware.go` (`RequireAuth` line 66, `RequireAdmin` line 136).
  The settings endpoints used by this feature sit behind `RequireAuth` + `RequireAccess` (not
  admin-only) — any user with granted access, not just role `admin`, can edit these sections.
  A separate "Пользователи" admin-only section exists (`isAdmin` check, `Panel.jsx:1017-1021`)
  but is unrelated to pricing data.

## 7. Exact locations of the labels to rename

- **"Скидки по партиям"** (note: plural "Скидки", not singular "Скидка" as in the task
  wording) — appears **exactly once** in the codebase, as the `SettingsSection` title prop:
  `client/src/pages/Panel.jsx:1260`:
  ```js
  const DiscountsBlock = ({ settings, handleDiscountChange }) => (
    <SettingsSection
      title="Скидки по партиям"
      description="Диапазоны количества и процент скидки для автоматического уменьшения цены на крупные заказы."
    >
  ```
  Since `DiscountsBlock` is a single shared component rendered in both quick mode (line 885) and
  masterpiece mode (line 966), editing this one line renames the label everywhere it's shown.
- **"Шедевр"** — `client/src/pages/Panel.jsx:86` (`label` in `calculatorModes[0]`) and line 798
  (JS fallback string in the "Активный режим" badge, only used if `calculatorModes.find(...)`
  returns undefined, which cannot happen given the array always contains both modes — effectively
  dead/defensive code, but worth updating for consistency).
- **"По быстрому"** — `client/src/pages/Panel.jsx:91` (`label` in `calculatorModes[1]`). No
  fallback-string occurrence for this one.
- Both labels are also rendered indirectly wherever `mode.label` / `calculatorModes.find(...).label`
  is read: the mode-picker card title (`Panel.jsx:828`) and the "Активный режим" badge
  (`Panel.jsx:798`) — both derive from the single source array at lines 83-94, so no additional
  edits are needed beyond that array.
- **"Мин. цена / шт"** (field label inside Изделия, quick mode) — single occurrence,
  `client/src/pages/Panel.jsx:858`, inside `SettingsField label="Мин. цена / шт"`.

## Potential Problems (relevant to this feature)

1. **Adding a garment via the quick-mode "Изделия" form would fail server-side validation
   unless base_minutes/complexity_coeff get sane defaults.** `validateSettings`
   (`server/internal/service/costing.go:593-651`) requires, for **every** garment regardless of
   `calculator_mode`: `BaseMinutes > 0` (line 613) and `ComplexityCoeff > 0` (line 616,
   error `"garment coefficient should be positive"`). The quick-mode add-row UI, per the task
   description, would only collect a name + `quick_price`. If the client sends
   `base_minutes: 0, complexity_coeff: 0` for a garment created that way, the subsequent
   `handleSaveSettings` → `POST /settings` call will be rejected with a 4xx
   (`ErrInvalidArgument`) and **nothing will persist** — silently breaking the "must survive
   reload" acceptance criterion for that section specifically. `costing_test.go:256-258` shows
   the project's own test data explicitly sets `BaseMinutes: 1, ComplexityCoeff: 1` even for a
   quick-mode-only garment, confirming this constraint is intentional and unconditional. The
   add-garment form (or its submit handler) needs to supply non-zero placeholder values for
   these two fields even though they're not shown in quick mode.
   Operations do **not** have this problem: `AdditionalMinutes`/`AdditionalMaterialPerUnit` only
   require `>= 0` (costing.go:634), so `0` (the natural default when only `quick_percent` is
   collected) passes validation.
2. **Garment/operation names are the identity key, with only an empty-string check.**
   `validateSettings` rejects an empty/whitespace name (`costing.go:611,629`) but does **not**
   check for duplicates — adding a row with a name that already exists in `settings.garments`
   will silently overwrite/merge into the existing entry (since it's a JS/Go map keyed by name)
   rather than erroring or creating a second row. The add-row form should probably guard against
   this client-side (existing project code has no such guard to model after).
3. **`batch_discounts` has no such name uniqueness issue** (plain array, no key) but has range
   validation to satisfy: `MinQty > 0`, `MaxQty >= MinQty`, `0 <= Percent <= 100`
   (`costing.go:640-649`) — an add-row form needs to pre-fill valid defaults (e.g. `min_qty`
   one above the current last tier's `max_qty`) or the Save button will fail validation for the
   whole settings object, not just the new row (this is a whole-object POST, see §5 — a bad new
   row blocks saving *everything*, including unrelated edits made in the same session).
4. **Settings are per-user, not a single global config.** Each authenticated panel user (not
   just `role=admin`) has an independent `user_settings` row keyed by `user_id`
   (`server/migrations/002_costing_schema.up.sql:6`, `RequireAccess` not `RequireAdmin` gates
   `/api/v1/users/settings`). Worth clarifying with the requester whether "the pricing panel"
   they mean is genuinely single-admin-only in practice, or whether multiple accounts each have
   their own separate garments/operations/discounts — renaming/reordering the plan labels is a
   pure frontend constant so it's global regardless, but the "add row" feature adds a row only
   to the acting user's own settings, not a shared/site-wide list.
