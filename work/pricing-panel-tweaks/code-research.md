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

## Implementation Research (tech-spec phase)

Deepened for implementation planning (file-level tasks). User-spec (approved,
`work/pricing-panel-tweaks/user-spec.md`) locks in: unified add-form (all fields at once,
no mode-conditional hidden defaults), delete restricted to admin-added rows only (name not in
the fixed default list), exact default name lists (4 garments / 8 operations), and reuse of
`mapPanelError` for save-time errors. All line numbers below are from
`client/src/pages/Panel.jsx` unless stated otherwise, current as of this research pass.

### 1. Existing UI component patterns to reuse

Three shared sub-components exist and are the only reusable UI primitives in the settings
tree — no button/form component library, everything is hand-rolled Tailwind-arbitrary-value
JSX:

- **`SettingsSection`** (`Panel.jsx:219-229`) — wraps a titled card:
  ```js
  const SettingsSection = ({ title, description, children }) => (
    <section className={settingsSectionClass}>
      <div className="mb-5 flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div className="max-w-3xl">
          <h3 className="text-lg font-semibold tracking-[-0.02em] text-[color:var(--settings-text)]">{title}</h3>
          {description ? <p className="mt-1 text-sm leading-6 text-[color:var(--settings-muted)]">{description}</p> : null}
        </div>
      </div>
      {children}
    </section>
  );
  ```
  Props: `title` (string), `description` (string, optional), `children`. `DiscountsBlock` is
  the existing example of a component that wraps its own `SettingsSection` call (`Panel.jsx:1258-1282`)
  — the same pattern (a small wrapper component around one `SettingsSection`) is the natural
  place to add the new add-row forms for Изделия/Усложнения if they're extracted as their own
  components rather than inlined.
- **`SettingsField`** (`Panel.jsx:231-236`) — labeled form-field wrapper:
  ```js
  const SettingsField = ({ label, children, className = "" }) => (
    <label className={`flex min-w-0 flex-col gap-2 ${className}`}>
      <span className="text-sm font-medium leading-5 text-[color:var(--settings-muted)]">{label}</span>
      {children}
    </label>
  );
  ```
  Props: `label` (string), `children` (the input), `className` (optional extra classes). Used
  for every single field in the settings form — a new add-form's Название/Мин. цена/База/Коэфф.
  fields should each be wrapped in `SettingsField` exactly like existing edit-fields are.
- **`SettingsNumberInput`** (`Panel.jsx:238`) — `const SettingsNumberInput = (props) => <input className={settingsInputClass} type="number" {...props} />` — thin styled wrapper over `<input type="number">`, spreads all props through (`min`, `max`, `step`, `value`, `onChange`). There is no equivalent `SettingsTextInput` — every existing input in the file is numeric; a new "Название" text field has no existing styled component to reuse and would need either a bare `<input className={settingsInputClass} type="text" .../>` (reusing the shared `settingsInputClass` string, `Panel.jsx:213-214`) or a new tiny wrapper component following the same one-line pattern as `SettingsNumberInput`.
- **Row container pattern** (repeated identically 4 times, e.g. `Panel.jsx:850-853`, `905-909`, `928-931`, `1265-1268`): each existing garment/operation/discount row is a `<div>` with the class string
  `"grid gap-4 rounded-[24px] border p-4 [background:color-mix(in_oklab,var(--settings-card-bg)_90%,transparent)] [border-color:var(--settings-card-border)] ..."`
  (grid-cols vary by section) and `key={name}` / `key={`${min}-${max}-${index}`}`. A new add-row
  or delete-button element should sit inside this same row-container styling to look native.
- **No existing "add" button pattern anywhere in the Tailwind/settings visual system.** The only
  existing add-like action in the whole file is `handleCreateChat`'s button (`Panel.jsx:1050-1052`,
  `<button type="button" onClick={handleCreateChat} disabled={isCreatingChat}>{isCreatingChat ? "Создаём..." : "Новый чат"}</button>`), but it belongs to the **legacy CSS system**
  (plain `panel__*` BEM classes from `App.css`, unstyled by Tailwind utility classes) used in the
  Workspace/chat-list section, not the settings section — it is a different visual language and
  should not be copy-pasted verbatim into the settings area. The submit button style at
  `Panel.jsx:1006-1012` (`inline-flex ... rounded-2xl border ... [background:var(--settings-accent)] ...`)
  is the closest existing example of a settings-area primary action button and is the best model
  for a new "Добавить" button's Tailwind class string (probably at a smaller/secondary visual
  weight, e.g. dropping the `[background:var(--settings-accent)]` fill for an outline style, to
  read as secondary vs. the page's one primary "Сохранить изменения" button).
- **Existing delete-button pattern** (different section, legacy CSS, but the only delete UI in
  the codebase to model behavior on) — `Panel.jsx:1065-1067`:
  ```js
  <button className="panel-chat-list__delete" type="button" onClick={() => handleDeleteChat(chat.id)} disabled={isDeletingChatID === chat.id}>
    {isDeletingChatID === chat.id ? "..." : "Удалить"}
  </button>
  ```
  styled by `client/src/App.css:1836-1847` (`.panel-chat-list__delete { border: 1px solid rgba(185, 74, 72, 0.18); background: rgba(185, 74, 72, 0.08); color: #8f2f2f; border-radius: 14px; ... }`, hover state at 1845-1847) — a fixed rgba red, **not** theme-token-based (no
  `--settings-danger` variable exists in `client/src/index.css`'s `.panel-settings` /
  `.panel--dark .panel-settings` token blocks, confirmed by grep — only `--settings-accent`,
  `--settings-text`, `--settings-muted`, `--settings-subtle`, `--settings-card-*`,
  `--settings-input-*`, `--settings-focus` are defined, `index.css:91-125`). A new delete button
  in the Tailwind-styled settings section has no existing theme-aware danger color to reuse —
  the tech-spec should either introduce a small inline Tailwind arbitrary-color (e.g. reusing the
  same rgba(185,74,72,...) literal inline) or accept a plain muted/outline style consistent with
  `settingsInputClass`'s border color. Note this existing delete button also does a **network
  call** (`deleteChat`) and a `window.confirm(...)` gate (`Panel.jsx:592`) before calling — the
  new garment/operation/discount delete is local-state-only (no network call per row, no
  confirm specified in user-spec), so only the *visual* class/disabled-state pattern transfers,
  not the async/confirm behavior.

### 2. State mutation patterns (`setSettings` immutable-update style)

All three existing per-field handlers follow the identical shape: `setSettings((current) => ({ ...current, <topLevelKey>: <new-value-for-that-key> }))`, i.e. spread the whole settings
object, then replace one top-level key with a freshly-built value (spread-and-override the
nested map/array). Exact signatures, `Panel.jsx:479-525`:

```js
const handleGarmentChange = (name, key, value) => {
  setSettings((current) => ({
    ...current,
    garments: {
      ...current.garments,
      [name]: { ...current.garments[name], [key]: Number(value) || 0 },
    },
  }));
};

const handleOperationSettingChange = (name, key, value) => {
  setSettings((current) => ({
    ...current,
    operations: {
      ...current.operations,
      [name]: { ...current.operations[name], [key]: Number(value) || 0 },
    },
  }));
};

const handleDiscountChange = (index, key, value) => {
  setSettings((current) => ({
    ...current,
    batch_discounts: current.batch_discounts.map((item, itemIndex) =>
      itemIndex === index ? { ...item, [key]: Number(value) || 0 } : item
    ),
  }));
};
```

`handleMaterialChange` (505-516) and `handleUrgencyChange` (527-535) follow the exact same
map-spread shape as `handleGarmentChange`/`handleOperationSettingChange`. All of them coerce
with `Number(value) || 0` (i.e. NaN/empty-string silently becomes `0` — this is edit-time
behavior for existing rows and, per user-spec Constraints, explicitly out of scope to change).

**Implication for new add/delete functions** — the natural, convention-matching signatures are:
- `handleAddGarment(name, fields)` → `setSettings((current) => ({ ...current, garments: { ...current.garments, [name]: fields } }))` (fields = `{ base_minutes, complexity_coeff, quick_price }`, all required/validated before calling, not defaulted to 0 inside the setter — matches the user-spec decision that the add-form always collects all fields, no hidden defaults).
- `handleDeleteGarment(name)` → build a new object omitting `name` (e.g. `Object.fromEntries(Object.entries(current.garments).filter(([key]) => key !== name))`, since there's no existing `delete`-based helper anywhere in the file to model against — this would be new code, not a copy of an existing pattern).
- `handleAddOperation(name, fields)` / `handleDeleteOperation(name)` — same shape as garments.
- `handleAddDiscount(fields)` → `setSettings((current) => ({ ...current, batch_discounts: [...current.batch_discounts, fields] }))`, mirroring how `batch_discounts` is already treated as a plain array in `handleDiscountChange`.
- `handleDeleteDiscount(index)` → `current.batch_discounts.filter((_, itemIndex) => itemIndex !== index)`, the array-analog of `handleDiscountChange`'s `.map` pattern.

All of these should be defined alongside the existing handlers (contiguous block,
`Panel.jsx:479-548` today) to match the file's existing convention of grouping all
`setSettings`-mutating handlers together before the JSX return.

### 3. Where the 3 "Добавить" buttons need to go (single-button-regardless-of-mode)

Confirmed structure: Изделия and Усложнения/Операции are each rendered **twice** in the JSX —
once inside the `isQuickCalculator ? (...) : (...)` true-branch (quick mode: `Panel.jsx:845-886`,
Изделия at 847-864, Усложнения at 866-883) and once inside the false-branch (masterpiece mode:
`Panel.jsx:888-1003`, Изделия at 903-923, Операции at 925-945) — two separate `SettingsSection`
JSX blocks per list, each iterating the same `settings.garments` / `settings.operations` object
but rendering different field subsets (quick mode shows only `quick_price`; masterpiece mode
shows `base_minutes` + `complexity_coeff`). `DiscountsBlock` (1258-1282) is the only one of the
three that is **already** a single shared component instance, called identically at both
`Panel.jsx:885` (inside quick branch) and `Panel.jsx:966` (inside masterpiece branch) — so it
already satisfies "one button regardless of mode" structurally; only Изделия and
Усложнения/Операции have the duplication problem.

**Proposed approach matching existing conventions** (mirrors what `DiscountsBlock` already
does for the discounts list, so it's consistent with the one existing precedent rather than
inventing a new pattern):
- Extract two new components at module scope (next to `DiscountsBlock`, e.g. adjacent in the
  file around line 1258): `GarmentAddForm` (or a `GarmentsBlock` that owns both the existing
  list-rendering loop *and* the add form, analogous to how `DiscountsBlock` owns both) and
  `OperationAddForm`/`OperationsBlock`.
  - **Caveat**: unlike `DiscountsBlock`, the *list rendering* itself cannot be trivially
    extracted into one shared component for Изделия/Операции, because the two modes render
    different field subsets per row (quick: `quick_price` only; masterpiece: `base_minutes` +
    `complexity_coeff`) — that part of the duplication is intentional and pre-existing, out of
    this feature's scope to unify. Only the **add-row form** (which per user-spec always shows
    all fields regardless of mode) is identical in both branches and can be extracted as one
    component, called once per branch with the same props/handler — same call-twice-same-
    component technique `DiscountsBlock` already uses at lines 885/966.
  - Concretely: define `GarmentAddForm({ settings, onAddGarment })` once, and add exactly one
    `<GarmentAddForm settings={settings} onAddGarment={handleAddGarment} />` call inside the
    Изделия `SettingsSection` in **each** branch (after the existing `.map(...)` loop, e.g. right
    before the closing `</div>`/`</SettingsSection>` at `Panel.jsx:863`/`Panel.jsx:864` for quick
    mode, and at `Panel.jsx:921`/`Panel.jsx:923` for masterpiece mode). Because it's the *same*
    component with the *same* handler prop in both places, "duplicated JSX call, shared
    component + shared handler" satisfies the AC ("одна форма ... независимо от того, какой
    режим калькулятора сейчас активен") without behavior actually differing between call sites —
    this is the cleanest fit with the file's existing style (plain duplicated call sites,
    e.g. `DiscountsBlock` itself is called twice already) rather than trying to hoist the whole
    Изделия/Операции sections above the `isQuickCalculator` branch (which would require
    unifying the differing field-display logic too, out of scope).
  - Same treatment for `OperationAddForm({ settings, onAddOperation })`, called once inside the
    Усложнения section (quick branch, insert near `Panel.jsx:881`/`883`) and once inside the
    Операции section (masterpiece branch, insert near `Panel.jsx:943`/`945`).
  - `DiscountsBlock` itself just needs its own add-form appended inside its existing single
    `SettingsSection` body (after the `.map(...)` at `Panel.jsx:1279`/`1280`, before the closing
    `</div>`) — no call-site duplication needed since it's already called once per branch with
    shared markup.
- Delete buttons live **inside** the existing per-row `.map(...)` blocks (Изделия row:
  `Panel.jsx:850-861` quick / `905-920` masterpiece; Усложнения row: `868-880` quick / `927-942`
  masterpiece; discount row: `1264-1278`) — conditionally rendered per-row based on the
  is-default-name check (§4) or, for discounts, based on `settings.batch_discounts.length > 1`.
  Since these row blocks are themselves duplicated per mode (Изделия/Операции), the delete
  button JSX + its is-default check would need to be added to **both** copies of the row markup
  (or the delete button extracted as its own tiny reusable component, e.g. `DeleteRowButton`,
  invoked identically from both copies — same "same component, two call sites" technique as the
  add-form).

### 4. Client-side default-name lists — `defaultSettings` is confirmed stale

`defaultSettings` (`Panel.jsx:96-192`) garments keys, exact as of this research (`Panel.jsx:112-117`):
```js
garments: {
  "Пиджак": { base_minutes: 260, complexity_coeff: 1.6, quick_price: 7000 },
  "Юбка": { base_minutes: 90, complexity_coeff: 1, quick_price: 3200 },
  "Рубашка": { base_minutes: 140, complexity_coeff: 1.15, quick_price: 4200 },
  "Платье": { base_minutes: 180, complexity_coeff: 1.3, quick_price: 5600 },
},
```
— 4 keys, **matches** the server's `DefaultUserSettings()` garments exactly (both name set and
values; `server/internal/service/costing.go:227-232`). No update needed for garments.

`defaultSettings` operations keys, exact (`Panel.jsx:118-125`):
```js
operations: {
  "Карман накладной": { additional_minutes: 15, additional_material_per_unit: 80, quick_percent: 8 },
  "Карман прорезной": { additional_minutes: 25, additional_material_per_unit: 120, quick_percent: 12 },
  Подклад: { additional_minutes: 35, additional_material_per_unit: 350, quick_percent: 15 },
  "Потайная молния": { additional_minutes: 12, additional_material_per_unit: 120, quick_percent: 6 },
  Воротник: { additional_minutes: 20, additional_material_per_unit: 90, quick_percent: 10 },
  Манжеты: { additional_minutes: 15, additional_material_per_unit: 70, quick_percent: 8 },
},
```
— **only 6 keys**. Confirmed missing vs. server's `DefaultUserSettings()` operations
(`server/internal/service/costing.go:233-242`, 8 keys):
```go
Operations: map[string]OperationConfig{
    "Карман накладной":       {AdditionalMinutes: 15, AdditionalMaterialPerUnit: 80, QuickPercent: 8},
    "Карман прорезной":       {AdditionalMinutes: 25, AdditionalMaterialPerUnit: 120, QuickPercent: 12},
    "Подклад":                {AdditionalMinutes: 35, AdditionalMaterialPerUnit: 350, QuickPercent: 15},
    "Потайная молния":        {AdditionalMinutes: 12, AdditionalMaterialPerUnit: 120, QuickPercent: 6},
    "Воротник":               {AdditionalMinutes: 20, AdditionalMaterialPerUnit: 90, QuickPercent: 10},
    "Манжеты":                {AdditionalMinutes: 15, AdditionalMaterialPerUnit: 70, QuickPercent: 8},
    "Шлица":                  {AdditionalMinutes: 18, AdditionalMaterialPerUnit: 50, QuickPercent: 7},
    "Декоративная отстрочка": {AdditionalMinutes: 18, AdditionalMaterialPerUnit: 0, QuickPercent: 5},
},
```
Missing from the client constant: **"Шлица"** (`{additional_minutes: 18, additional_material_per_unit: 50, quick_percent: 7}`) and **"Декоративная отстрочка"** (`{additional_minutes: 18, additional_material_per_unit: 0, quick_percent: 5}`).

**Tech-spec implication**: two independent things are needed, and they should not be conflated:
1. A `DEFAULT_GARMENT_NAMES` / `DEFAULT_OPERATION_NAMES` list (or a single combined lookup) used
   purely for the delete-button-visibility check (§ user-spec Decision) — 4 + 8 names, matching
   the server list above exactly. This can be a small new `const` (array or `Set`) placed near
   `defaultSettings`, independent of whether `defaultSettings.operations` itself gets fixed.
2. Whether `defaultSettings.operations` (the client's local-fallback-before-first-load object,
   used as `useState(defaultSettings)` initial value at `Panel.jsx:254`, i.e. only shown
   transiently before `GetSettings` returns from the server) also gets the 2 missing keys added
   is a separate decision — it's stale relative to the server default but since the server
   always returns its own `DefaultUserSettings()` merged result on load (`normalizeSettings`
   backend behavior per user-spec Constraints), the client's `defaultSettings` constant only
   matters for the brief pre-load render / not-yet-saved-anything state. The tech-spec should
   decide explicitly whether to fix `defaultSettings.operations` too (for consistency/correctness)
   or leave it and only add the separate delete-check name list — user-spec's own Technical
   Decisions section explicitly says not to use `defaultSettings` directly as the source for the
   delete-check list, but does not mandate fixing `defaultSettings` itself.

### 5. `calculatorModes` array — exact current content

`Panel.jsx:83-94`:
```js
const calculatorModes = [
  {
    value: "masterpiece",
    label: "Шедевр",
    description: "Полная калькуляция по минутам, материалам, срочности и рынку.",
  },
  {
    value: "quick",
    label: "По быстрому",
    description: "Простой расчет: изделие, усложнения и скидка от количества.",
  },
];
```
Confirms prior research: array order is `masterpiece` first, `quick` second — user-spec wants
`quick`'s renamed card ("Быстрый") to render **first** and `masterpiece`'s renamed card
("Продвинутый") **second**, i.e. the two object literals need to be swapped (or the array
reordered) in addition to editing `label`. `value` fields (`"masterpiece"`/`"quick"`) must stay
untouched per Constraints. `description` text is not mentioned in user-spec — leave as-is unless
told otherwise. Also re-confirms the two other read sites that key off this array and need no
separate edit: `Panel.jsx:798` (badge, `.find(...).label` with `"Шедевр"` JS-fallback string —
update the fallback string too for consistency, though functionally unreachable) and `828`
(mode-picker card title, `{mode.label}`, automatically picks up the reorder via `.map`).

### 6. Existing error display pattern (`mapPanelError`)

`Panel.jsx:1489-1494`:
```js
const mapPanelError = (error) => {
  if (error?.message === "api_method_not_allowed") {
    return "API недоступно для записи. Обычно это значит, что proxy для /api ещё не применён.";
  }
  return error?.message || "Операция не выполнена.";
};
```
Consumed only inside `catch` blocks of async handlers, always writing into a per-feature
"notice" state string that's rendered as a plain `<p>` near the relevant control. For settings
specifically: `handleSaveSettings` (`Panel.jsx:550-566`) — `setSettingsNotice(mapPanelError(error))`
in the `catch`, and `setSettingsNotice("Настройки сохранены.")` on success (`line 560`); rendered
at `Panel.jsx:1013`: `{settingsNotice ? <p className="text-sm leading-6 text-[color:var(--settings-muted)]">{settingsNotice}</p> : null}`, directly beside the submit button (1005-1014). There
is **no dedicated error-styling** (no red text, no alert box) — success and failure messages
share the exact same muted-gray `<p>` styling and the same `settingsNotice` state slot; the
message text itself (from `mapPanelError` or the hardcoded success string) is the only
differentiator. **Implication**: per user-spec Constraints, add/delete-triggered save failures
(e.g. backend rejects the whole settings object because of a bad row) need no new UI — they
naturally flow through the existing `handleSaveSettings` → `catch` → `setSettingsNotice(mapPanelError(error))` path unchanged, since add/delete only mutate `settings` state and
still go through the same single submit handler. No new error-display code is needed for
save-time errors; only the *new client-side pre-submit* validation (duplicate name, zero
values, discount range) needs its own message surface, which has no existing precedent to
reuse (see §7).

### 7. Existing client-side form validation helpers — none exist, must be written from scratch

- `client/src/utils/` contains exactly 3 files: `accessApi.js`, `panelApi.js`, `yandexAuth.js`
  — all thin fetch wrappers, none contain validation logic, form-state helpers, or any
  string/number sanitization utilities.
  `panelApi.js` exposes `saveUserSettings(settings)` as the entire client-side hook into
  persistence (POST to `/api/v1/users/settings`) — no per-field or per-form validate function
  is exported from it or anywhere else in `client/src`.
- No validation library in dependencies: `client/package.json` has no `yup`, `zod`,
  `react-hook-form`, or `formik`.
- The only existing ad hoc string-normalization in the whole file is `.trim()` used once, in
  `handleCreateChat` (`Panel.jsx:576`, `createChat(chatTitleDraft.trim())`) — trims before
  sending, no case-insensitivity/dedup logic anywhere. `.toLowerCase()` appears once in the file
  (`Panel.jsx:1431`) but inside an unrelated `switch` (market-status label formatting), not a
  validation context.
- Numeric coercion convention already used throughout (`handleGarmentChange` et al.):
  `Number(value) || 0` — this silently maps invalid/empty input to `0` and is explicitly *not*
  suitable for the new add-form's validation (per user-spec/AC, `0` must be rejected, not
  silently substituted) — the add-form's own validation needs distinct logic (e.g.
  `Number.isFinite(n) && n > 0` checks) rather than reusing this existing coercion helper style.

**Conclusion for tech-spec**: name-dedup (case/whitespace-insensitive compare against existing
`Object.keys(settings.garments)` / `Object.keys(settings.operations)`, trimmed +
`.toLowerCase()`'d on both sides) and numeric-range validation (>0 / >=0 / 0–100 per user-spec
§Acceptance-Criteria) are net-new client-side logic with no existing helper to call — they should
live as new local functions inside `Panel.jsx` (or a new small `client/src/utils/` module if the
tech-spec prefers extracting them, but there is no existing convention in this codebase of
splitting validation into its own utils file — everything colocates in `Panel.jsx` today).
