import { useEffect, useMemo, useRef, useState } from "react";
import { checkAuthStatus, fetchAuthProfile, logout } from "../utils/yandexAuth.js";
import AccessRequestBanner from "../components/AccessRequestBanner.jsx";
import AdminUsersSection from "../components/AdminUsersSection.jsx";
import { fetchAccessState } from "../utils/accessApi.js";
import {
  calculateInChat,
  createChat,
  deleteChat,
  getUserSettings,
  listChatCalculations,
  listChats,
  saveUserSettings,
} from "../utils/panelApi.js";

// Иконки навигации сайдбара — инлайн SVG, без внешней библиотеки (в client/package.json её
// нет, а трёх-четырёх глифов ради одной панели не стоит заводить новую зависимость).
// currentColor — чтобы цвет наследовался от .panel__link/.panel__sidebar-toggle без
// отдельных цветовых пропсов.
const NavWorkspaceIcon = () => (
  <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
    <path d="M21 11.5a8.38 8.38 0 0 1-4.2 7.26 8.5 8.5 0 0 1-8.9-.3L3 21l1.9-4.9a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 8.5-8.5h.4a8.48 8.48 0 0 1 8 8v.7z" />
  </svg>
);

const NavSettingsIcon = () => (
  <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
    <line x1="4" y1="7" x2="20" y2="7" />
    <circle cx="9" cy="7" r="2" />
    <line x1="4" y1="14" x2="20" y2="14" />
    <circle cx="16" cy="14" r="2" />
    <line x1="4" y1="19" x2="20" y2="19" />
    <circle cx="10" cy="19" r="2" />
  </svg>
);

const NavUsersIcon = () => (
  <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
    <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
    <circle cx="9" cy="7" r="4" />
    <path d="M22 21v-2a4 4 0 0 0-3-3.87" />
    <path d="M16 3.13a4 4 0 0 1 0 7.75" />
  </svg>
);

const SunIcon = () => (
  <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
    <circle cx="12" cy="12" r="4" />
    <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41" />
  </svg>
);

const MoonIcon = () => (
  <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
    <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
  </svg>
);

const ChevronUpDownIcon = () => (
  <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
    <path d="M8 9l4-4 4 4M8 15l4 4 4-4" />
  </svg>
);

// initialsFrom — инициалы для кружка-аватара: первые буквы первых двух слов имени, иначе
// первая буква логина. У профиля из Яндекса фотографии нет, а без неё лучше буквы, чем
// пустой круг.
const initialsFrom = (name, login) => {
  const source = (name || "").trim();
  if (source) {
    const letters = source
      .split(/\s+/)
      .slice(0, 2)
      .map((part) => part[0])
      .join("");
    if (letters) {
      return letters.toUpperCase();
    }
  }
  return (login || "?").slice(0, 1).toUpperCase();
};

const calculatorModes = [
  {
    value: "quick",
    label: "Быстрый",
    description: "Простой расчет: изделие, усложнения и скидка от количества.",
  },
  {
    value: "masterpiece",
    label: "Продвинутый",
    description: "Полная калькуляция по минутам, материалам, срочности и рынку.",
  },
];

// DEFAULT_GARMENT_NAMES / DEFAULT_OPERATION_NAMES — имена изделий и операций из серверного
// DefaultUserSettings() (server/internal/service/costing.go:227-242). Нужны только для одного:
// решить, можно ли удалять строку (дефолтные удалять нельзя). Список задан отдельно, а не выведен
// из defaultSettings ниже, именно потому что defaultSettings уже один раз разъехался с сервером
// (в operations не хватало двух записей) — отдельный явный список ловит такой дрейф тестом.
export const DEFAULT_GARMENT_NAMES = ["Пиджак", "Юбка", "Рубашка", "Платье"];

export const DEFAULT_OPERATION_NAMES = [
  "Карман накладной",
  "Карман прорезной",
  "Подклад",
  "Потайная молния",
  "Воротник",
  "Манжеты",
  "Шлица",
  "Декоративная отстрочка",
];

export const defaultSettings = {
  pricing_rules: {
    calculator_mode: "quick",
    labor_minute_rate: 18,
    payroll_taxes_percent: 30,
    overhead_percent: 18,
    logistics_cost_per_unit: 120,
    margin_percent: 25,
    min_margin_percent: 12,
    included_fittings: 1,
    extra_fitting_minutes: 20,
    custom_figure_coefficient: 1.1,
    child_coefficient: 0.75,
    default_risk_percent: 3,
    default_consumables_per_unit: 90,
  },
  garments: {
    "Пиджак": { base_minutes: 260, complexity_coeff: 1.6, quick_price: 7000 },
    "Юбка": { base_minutes: 90, complexity_coeff: 1, quick_price: 3200 },
    "Рубашка": { base_minutes: 140, complexity_coeff: 1.15, quick_price: 4200 },
    "Платье": { base_minutes: 180, complexity_coeff: 1.3, quick_price: 5600 },
  },
  operations: {
    "Карман накладной": { additional_minutes: 15, additional_material_per_unit: 80, quick_percent: 8 },
    "Карман прорезной": { additional_minutes: 25, additional_material_per_unit: 120, quick_percent: 12 },
    Подклад: { additional_minutes: 35, additional_material_per_unit: 350, quick_percent: 15 },
    "Потайная молния": { additional_minutes: 12, additional_material_per_unit: 120, quick_percent: 6 },
    Воротник: { additional_minutes: 20, additional_material_per_unit: 90, quick_percent: 10 },
    Манжеты: { additional_minutes: 15, additional_material_per_unit: 70, quick_percent: 8 },
    Шлица: { additional_minutes: 18, additional_material_per_unit: 50, quick_percent: 7 },
    "Декоративная отстрочка": { additional_minutes: 18, additional_material_per_unit: 0, quick_percent: 5 },
  },
  materials: {
    Хлопок: {
      coefficient: 1,
      fabric_cost_per_unit: 650,
      lining_cost_per_unit: 0,
      interfacing_cost_per_unit: 60,
      thread_cost_per_unit: 35,
      hardware_cost_per_unit: 50,
      decor_cost_per_unit: 0,
      packaging_cost_per_unit: 20,
      consumables_cost_per_unit: 30,
      risk_percent: 2,
    },
    "Костюмная ткань": {
      coefficient: 1.05,
      fabric_cost_per_unit: 1200,
      lining_cost_per_unit: 320,
      interfacing_cost_per_unit: 120,
      thread_cost_per_unit: 45,
      hardware_cost_per_unit: 90,
      decor_cost_per_unit: 0,
      packaging_cost_per_unit: 25,
      consumables_cost_per_unit: 40,
      risk_percent: 3,
    },
    Лён: {
      coefficient: 1.1,
      fabric_cost_per_unit: 980,
      lining_cost_per_unit: 0,
      interfacing_cost_per_unit: 70,
      thread_cost_per_unit: 40,
      hardware_cost_per_unit: 60,
      decor_cost_per_unit: 0,
      packaging_cost_per_unit: 20,
      consumables_cost_per_unit: 35,
      risk_percent: 4,
    },
    Шёлк: {
      coefficient: 1.3,
      fabric_cost_per_unit: 1750,
      lining_cost_per_unit: 450,
      interfacing_cost_per_unit: 90,
      thread_cost_per_unit: 55,
      hardware_cost_per_unit: 70,
      decor_cost_per_unit: 30,
      packaging_cost_per_unit: 30,
      consumables_cost_per_unit: 50,
      risk_percent: 7,
    },
  },
  batch_discounts: [
    { min_qty: 1, max_qty: 10, percent: 0 },
    { min_qty: 11, max_qty: 50, percent: 5 },
    { min_qty: 51, max_qty: 100, percent: 10 },
  ],
  urgency: {
    Стандарт: { percent: 0 },
    "Срочно 3-5 дней": { percent: 15 },
    "Срочно 1-2 дня": { percent: 30 },
    "В день заказа": { percent: 50 },
  },
  market_bands: {
    Массмаркет: { min_price_per_unit: 2500, average_price_per_unit: 4500, max_price_per_unit: 7000 },
    Средний: { min_price_per_unit: 5000, average_price_per_unit: 9000, max_price_per_unit: 15000 },
    Премиум: { min_price_per_unit: 9000, average_price_per_unit: 16000, max_price_per_unit: 26000 },
  },
};

// --- Валидация форм добавления строк (Изделия / Усложнения / Скидки) --------------------
//
// Библиотеки валидации в проекте нет, поэтому это обычные чистые функции. Границы зеркалят
// серверный validateSettings (server/internal/service/costing.go:593-657) и там, где нужно,
// строже него: quick_price сервер при сохранении пускает от 0, но быстрый расчёт
// (costing.go:720-721) на quick_price <= 0 падает — значит клиент не должен давать создать
// такое изделие вообще.
//
// Отдельно от привычного в этом файле `Number(value) || 0`: тот паттерн молча превращает
// пустую строку и мусор в 0, а 0 — как раз одно из значений, которые здесь надо отклонять.

// toFiniteNumber — строгое приведение значения из input к числу: пустая строка, пробелы,
// null/undefined и нечисловой текст дают NaN, а не 0.
const toFiniteNumber = (value) => {
  if (typeof value === "number") {
    return value;
  }
  if (typeof value === "string" && value.trim() !== "") {
    return Number(value);
  }
  return NaN;
};

const isPositiveNumber = (value) => {
  const parsed = toFiniteNumber(value);
  return Number.isFinite(parsed) && parsed > 0;
};

const isNonNegativeNumber = (value) => {
  const parsed = toFiniteNumber(value);
  return Number.isFinite(parsed) && parsed >= 0;
};

// isIntegerValue — для полей, объявленных на сервере как int/int64: base_minutes и quick_price
// у изделия (costing.go:45-47), additional_minutes и additional_material_per_unit у операции
// (costing.go:51-52), min_qty и max_qty у диапазона скидки (costing.go:23-24).
//
// Дробное значение в таком поле до серверного validateSettings даже не доходит: encoding/json
// роняет разбор тела запроса, и весь POST настроек возвращает 400 — то есть одна новая строка
// снова блокирует сохранение всего объекта настроек, ровно тот класс отказа, ради которого
// делалась эта фича. Хуже того, отказ невидим: в быстром режиме, например, «База, мин» в строке
// изделия не показывается, и на экране ничего не выглядит сломанным до самого сохранения.
//
// Атрибут step="1" на инпутах — только подсказка браузера: кнопка «Добавить» это type="button",
// нативную валидацию формы она не запускает, поэтому настоящая проверка живёт здесь.
// Number.isInteger(NaN) === false, так что нечисловой ввод тоже не пройдёт.
const isIntegerValue = (value) => Number.isInteger(toFiniteNumber(value));

// isBlankName — зеркало серверной проверки strings.TrimSpace(name) == "".
export const isBlankName = (name) => String(name ?? "").trim() === "";

// isDuplicateName — есть ли уже такой ключ в settings.garments / settings.operations.
// Сравнение по trim + lowerCase с обеих сторон: «пиджак», « Пиджак » и «Пиджак» — одно и то же.
export const isDuplicateName = (name, existingEntries) => {
  const candidate = String(name ?? "").trim().toLowerCase();
  if (candidate === "") {
    return false;
  }
  return Object.keys(existingEntries || {}).some((key) => key.trim().toLowerCase() === candidate);
};

// Валидаторы возвращают { valid, errors }, где errors — карта «поле → сообщение».
// Проверяются все поля сразу, чтобы форма могла подсветить каждое, а не только первое.
const buildValidationResult = (errors) => ({ valid: Object.keys(errors).length === 0, errors });

export const validateGarmentFields = (fields = {}) => {
  const errors = {};
  if (!isPositiveNumber(fields.base_minutes)) {
    errors.base_minutes = "База/мин должна быть больше 0";
  } else if (!isIntegerValue(fields.base_minutes)) {
    errors.base_minutes = "База/мин должна быть целым числом";
  }
  // complexity_coeff намеренно без проверки на целое: на сервере это float64 (costing.go:46),
  // дробный коэффициент сложности — штатное значение (1.15, 1.6 в дефолтах).
  if (!isPositiveNumber(fields.complexity_coeff)) {
    errors.complexity_coeff = "Коэффициент сложности должен быть больше 0";
  }
  if (!isPositiveNumber(fields.quick_price)) {
    errors.quick_price = "Мин. цена / шт должна быть больше 0";
  } else if (!isIntegerValue(fields.quick_price)) {
    errors.quick_price = "Мин. цена / шт должна быть целым числом";
  }
  return buildValidationResult(errors);
};

export const validateOperationFields = (fields = {}) => {
  const errors = {};
  if (!isNonNegativeNumber(fields.additional_minutes)) {
    errors.additional_minutes = "Доп. минуты не могут быть меньше 0";
  } else if (!isIntegerValue(fields.additional_minutes)) {
    errors.additional_minutes = "Доп. минуты должны быть целым числом";
  }
  if (!isNonNegativeNumber(fields.additional_material_per_unit)) {
    errors.additional_material_per_unit = "Доп. материал / шт не может быть меньше 0";
  } else if (!isIntegerValue(fields.additional_material_per_unit)) {
    errors.additional_material_per_unit = "Доп. материал / шт должен быть целым числом";
  }
  // quick_percent намеренно без проверки на целое: на сервере это float64 (costing.go:53).
  if (!isNonNegativeNumber(fields.quick_percent)) {
    errors.quick_percent = "Надбавка, % не может быть меньше 0";
  }
  return buildValidationResult(errors);
};

export const validateDiscountFields = (fields = {}) => {
  const errors = {};
  if (!isPositiveNumber(fields.min_qty)) {
    errors.min_qty = "«От» должно быть больше 0";
  } else if (!isIntegerValue(fields.min_qty)) {
    errors.min_qty = "«От» должно быть целым числом";
  }
  // Порядок веток важен: проверка на целое идёт перед сравнением с «От», иначе 10.5 при «От» = 1
  // прошло бы дальше и уехало на сервер. Обе ветки всё равно отклоняют строку, разница только
  // в тексте ошибки — показываем самую конкретную причину.
  if (!isPositiveNumber(fields.max_qty)) {
    errors.max_qty = "«До» должно быть больше 0";
  } else if (!isIntegerValue(fields.max_qty)) {
    errors.max_qty = "«До» должно быть целым числом";
  } else if (isPositiveNumber(fields.min_qty) && toFiniteNumber(fields.max_qty) < toFiniteNumber(fields.min_qty)) {
    errors.max_qty = "«До» не может быть меньше «От»";
  }
  // percent намеренно без проверки на целое: на сервере это float64 (costing.go:25).
  const percent = toFiniteNumber(fields.percent);
  if (!Number.isFinite(percent) || percent < 0 || percent > 100) {
    errors.percent = "Скидка должна быть от 0 до 100 %";
  }
  return buildValidationResult(errors);
};

// getDefaultDiscountRange — подсказка для формы добавления скидки: следующий диапазон
// начинается сразу за последним существующим. Возвращает оба конца: пустое или нулевое
// max_qty сделало бы всю форму настроек несохраняемой (сервер требует max_qty >= min_qty).
export const getDefaultDiscountRange = (existingTiers) => {
  const tiers = Array.isArray(existingTiers) ? existingTiers : [];
  const lastMaxQty = toFiniteNumber(tiers[tiers.length - 1]?.max_qty);
  const nextMinQty = Number.isFinite(lastMaxQty) && lastMaxQty >= 0 ? Math.floor(lastMaxQty) + 1 : 1;
  return { min_qty: nextMinQty, max_qty: nextMinQty };
};

const createDefaultOrderForm = (settings = defaultSettings) => ({
  garment_type: Object.keys(settings.garments)[0] || "",
  material_type: Object.keys(settings.materials)[0] || "",
  urgency: Object.keys(settings.urgency)[0] || "Стандарт",
  market_segment: Object.keys(settings.market_bands)[1] || Object.keys(settings.market_bands)[0] || "",
  quantity: 15,
  fittings: 1,
  is_custom_figure: false,
  is_child: false,
  comment: "",
  operation_counts: Object.fromEntries(Object.keys(settings.operations).map((name) => [name, 0])),
});

const settingsSectionClass =
  "rounded-[28px] border p-5 shadow-[0_20px_55px_var(--settings-card-shadow)] backdrop-blur-xl [background:var(--settings-card-bg)] [border-color:var(--settings-card-border)] sm:p-6";

const settingsInsetClass =
  "rounded-[24px] border p-4 shadow-[0_16px_40px_var(--settings-card-shadow)] backdrop-blur-xl [background:color-mix(in_oklab,var(--settings-card-bg)_86%,transparent)] [border-color:var(--settings-card-border)]";

const settingsInputClass =
  "h-11 w-full rounded-2xl border px-4 text-sm font-medium text-[color:var(--settings-text)] outline-none transition [background:var(--settings-input-bg)] [border-color:var(--settings-input-border)] shadow-[inset_0_1px_0_var(--settings-input-shadow)] placeholder:text-[color:var(--settings-subtle)] focus:border-[color:var(--settings-accent)] focus:ring-4 focus:ring-[color:var(--settings-focus)]";

const settingsModeButtonBaseClass =
  "group flex h-full flex-col gap-2 rounded-[24px] border p-5 text-left motion-safe:animate-soft-pop motion-safe:transition-transform motion-safe:duration-300 motion-safe:ease-[var(--ease-soft-spring)] [border-color:var(--settings-card-border)] [background:color-mix(in_oklab,var(--settings-card-bg)_92%,transparent)] motion-safe:hover:-translate-y-1 motion-safe:hover:[border-color:color-mix(in_oklab,var(--settings-accent)_18%,var(--settings-card-border))] motion-safe:hover:shadow-[0_18px_40px_var(--settings-card-shadow)]";

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

const SettingsField = ({ label, children, className = "" }) => (
  <label className={`flex min-w-0 flex-col gap-2 ${className}`}>
    <span className="text-sm font-medium leading-5 text-[color:var(--settings-muted)]">{label}</span>
    {children}
  </label>
);

const SettingsNumberInput = (props) => <input className={settingsInputClass} type="number" {...props} />;

// DeleteRowButton — кнопка удаления строки настроек. Показывается только когда isDeletable:
// дефолтные изделия/операции удалять нельзя, и решает это вызывающая сторона.
// type="button" обязателен — кнопка живёт внутри формы настроек,
// иначе клик отправил бы handleSaveSettings. Своего цвета «опасного действия» в теме нет
// (--settings-danger в index.css отсутствует), поэтому стиль приглушённый, на токенах инпута.
//
// disabled — отдельный от isDeletable случай: кнопка видна, но неактивна. Нужен для последнего
// диапазона скидок (Task 5): удалить его нельзя, но админ должен видеть почему, а не искать
// пропавший элемент управления. disabledHint уходит в title/aria-label, чтобы причина читалась
// и мышью, и скринридером.
const DeleteRowButton = ({ isDeletable, onDelete, disabled = false, disabledHint = "" }) => {
  if (!isDeletable) {
    return null;
  }

  return (
    <button
      type="button"
      onClick={onDelete}
      disabled={disabled}
      title={disabled && disabledHint ? disabledHint : undefined}
      aria-label={disabled && disabledHint ? `Удалить — ${disabledHint}` : undefined}
      className="h-11 rounded-2xl border px-4 text-sm font-medium text-[color:var(--settings-muted)] transition [border-color:var(--settings-input-border)] [background:var(--settings-input-bg)] hover:text-[color:var(--settings-text)] hover:[border-color:var(--settings-accent)] focus:outline-none focus:ring-4 focus:ring-[color:var(--settings-focus)] disabled:cursor-not-allowed disabled:opacity-60 disabled:hover:text-[color:var(--settings-muted)] disabled:hover:[border-color:var(--settings-input-border)]"
    >
      Удалить
    </button>
  );
};

const emptyGarmentDraft = { name: "", quick_price: "", base_minutes: "", complexity_coeff: "" };

// Скрытые значения для полей, которые форма не показывает в быстром режиме (см. isQuickMode
// ниже). Раньше (Decision 2) форма всегда собирала все четыре поля именно чтобы не подставлять
// такие дефолты — они дают технически «рабочее», но фиктивное значение для продвинутого режима
// (база/коэффициент = 1 — не реальная норма времени/сложность, а просто валидное число, чтобы
// прошла проверка). Сознательно принятый компромисс по просьбе пользователя: изделие,
// добавленное из быстрого тарифа, будет с виду нормально считаться в продвинутом режиме,
// но по абсурдно заниженным нормам, пока админ не поправит База/Коэфф. вручную.
const HIDDEN_GARMENT_DEFAULTS = { base_minutes: 1, complexity_coeff: 1 };

// GarmentAddForm — форма добавления изделия. В продвинутом режиме показывает все четыре поля
// сразу (Decision 2). В быстром режиме (isQuickMode) — только Название и Мин. цена/шт; База и
// Коэфф. подставляются скрытыми дефолтами (HIDDEN_GARMENT_DEFAULTS), см. комментарий выше.
//
// Это НЕ <form>: вся секция настроек уже обёрнута в <form onSubmit={handleSaveSettings}>,
// а вложенные формы HTML запрещает. Поэтому кнопка type="button" + onClick, а Enter внутри
// полей перехватывается вручную — иначе он отправил бы внешнюю форму и сохранил настройки
// вместо добавления строки.
const GarmentAddForm = ({ settings, onAddGarment, isQuickMode = false }) => {
  const [draft, setDraft] = useState(emptyGarmentDraft);
  const [error, setError] = useState("");

  const updateField = (key) => (event) => {
    const { value } = event.target;
    setDraft((current) => ({ ...current, [key]: value }));
  };

  const handleAdd = () => {
    const name = draft.name.trim();
    if (isBlankName(name)) {
      setError("Введите название изделия");
      return;
    }
    if (isDuplicateName(name, settings.garments)) {
      setError("Такое название уже есть");
      return;
    }

    // Валидатору отдаются сырые строки из input: он сам приводит их к числу строго
    // (пустая строка и мусор дают NaN, а не 0). Приводить через Number() заранее нельзя —
    // это подменило бы невалидный ввод нулём ещё до проверки. Поля, скрытые в быстром режиме,
    // валидируются по своим скрытым дефолтам — они всегда корректны (1 > 0), но валидатор всё
    // равно вызывается на полном наборе, чтобы не разойтись с бэкендом при будущих правках bounds.
    const { valid, errors } = validateGarmentFields({
      base_minutes: isQuickMode ? HIDDEN_GARMENT_DEFAULTS.base_minutes : draft.base_minutes,
      complexity_coeff: isQuickMode ? HIDDEN_GARMENT_DEFAULTS.complexity_coeff : draft.complexity_coeff,
      quick_price: draft.quick_price,
    });
    if (!valid) {
      setError(Object.values(errors).join(". "));
      return;
    }

    // Обработчик добавления имя не обрезает — обрезаем здесь, иначе ключом изделия
    // станет строка с пробелами по краям.
    onAddGarment(name, {
      base_minutes: isQuickMode ? HIDDEN_GARMENT_DEFAULTS.base_minutes : Number(draft.base_minutes),
      complexity_coeff: isQuickMode ? HIDDEN_GARMENT_DEFAULTS.complexity_coeff : Number(draft.complexity_coeff),
      quick_price: Number(draft.quick_price),
    });
    setDraft(emptyGarmentDraft);
    setError("");
  };

  const handleKeyDown = (event) => {
    if (event.key === "Enter") {
      event.preventDefault();
      handleAdd();
    }
  };

  return (
    <div
      className="mt-4 grid gap-4 rounded-[24px] border border-dashed p-4 [background:color-mix(in_oklab,var(--settings-card-bg)_90%,transparent)] [border-color:var(--settings-card-border)]"
      onKeyDown={handleKeyDown}
    >
      <div>
        <strong className="text-base font-semibold tracking-[-0.02em] text-[color:var(--settings-text)]">Новое изделие</strong>
        <p className="mt-1 text-sm text-[color:var(--settings-muted)]">
          {isQuickMode
            ? "База и коэффициент сложности подставляются автоматически — поправьте их в продвинутом режиме при необходимости."
            : "Значения заполняются сразу для обоих режимов расчёта."}
        </p>
      </div>
      <div className={`grid gap-4 md:grid-cols-2 ${isQuickMode ? "" : "xl:grid-cols-4"}`}>
        <SettingsField label="Название">
          <input
            className={settingsInputClass}
            type="text"
            value={draft.name}
            onChange={updateField("name")}
            placeholder="Например, Пальто"
          />
        </SettingsField>
        <SettingsField label="Мин. цена / шт">
          <SettingsNumberInput step="1" min="0" value={draft.quick_price} onChange={updateField("quick_price")} placeholder="7000" />
        </SettingsField>
        {isQuickMode ? null : (
          <>
            <SettingsField label="База, мин">
              <SettingsNumberInput step="1" min="0" value={draft.base_minutes} onChange={updateField("base_minutes")} placeholder="260" />
            </SettingsField>
            <SettingsField label="Коэфф. сложности">
              <SettingsNumberInput step="0.01" min="0" value={draft.complexity_coeff} onChange={updateField("complexity_coeff")} placeholder="1.6" />
            </SettingsField>
          </>
        )}
      </div>
      {error ? (
        <p
          role="alert"
          className="rounded-2xl border px-4 py-2 text-sm font-medium text-[color:var(--settings-text)] [background:var(--settings-accent-soft)] [border-color:var(--settings-accent)]"
        >
          {error}
        </p>
      ) : null}
      <div>
        <button
          type="button"
          onClick={handleAdd}
          className="h-11 rounded-2xl border px-5 text-sm font-semibold text-[color:var(--settings-text)] transition [background:var(--settings-input-bg)] [border-color:var(--settings-accent)] hover:[background:var(--settings-accent-soft)] focus:outline-none focus:ring-4 focus:ring-[color:var(--settings-focus)]"
        >
          Добавить
        </button>
      </div>
    </div>
  );
};

const emptyOperationDraft = { name: "", quick_percent: "", additional_minutes: "", additional_material_per_unit: "" };

// OperationAddForm — форма добавления усложнения/операции. Как и GarmentAddForm, собирает все
// четыре поля всегда, независимо от активного режима калькулятора (Decision 2): операция,
// добавленная в быстром режиме без additional_minutes, сохранилась бы с нулём и тихо выпала бы
// из расчёта в продвинутом режиме — и наоборот с quick_percent.
//
// Границы полей у операций слабее, чем у изделий: все три числа допускают 0 (>= 0, зеркало
// серверного validateSettings, costing.go:634-644) — операция «только проценты» или
// «только минуты» это нормальный случай, в отличие от изделия с нулевой базой.
//
// Это НЕ <form> — секция настроек уже обёрнута в <form onSubmit={handleSaveSettings}>,
// вложенные формы HTML запрещает. Отсюда type="button" + onClick и ручной перехват Enter:
// иначе Enter отправил бы внешнюю форму и сохранил настройки вместо добавления строки.
const OperationAddForm = ({ settings, onAddOperation }) => {
  const [draft, setDraft] = useState(emptyOperationDraft);
  const [error, setError] = useState("");

  const updateField = (key) => (event) => {
    const { value } = event.target;
    setDraft((current) => ({ ...current, [key]: value }));
  };

  const handleAdd = () => {
    const name = draft.name.trim();
    if (isBlankName(name)) {
      setError("Введите название операции");
      return;
    }
    if (isDuplicateName(name, settings.operations)) {
      setError("Такое название уже есть");
      return;
    }

    // Валидатору отдаются сырые строки из input: он сам приводит их к числу строго
    // (пустая строка и мусор дают NaN, а не 0). Приводить через Number() заранее нельзя —
    // это подменило бы невалидный ввод нулём ещё до проверки, а 0 здесь валиден.
    const { valid, errors } = validateOperationFields({
      additional_material_per_unit: draft.additional_material_per_unit,
      additional_minutes: draft.additional_minutes,
      quick_percent: draft.quick_percent,
    });
    if (!valid) {
      setError(Object.values(errors).join(". "));
      return;
    }

    // Обработчик добавления имя не обрезает — обрезаем здесь, иначе ключом операции
    // станет строка с пробелами по краям.
    onAddOperation(name, {
      additional_material_per_unit: Number(draft.additional_material_per_unit),
      additional_minutes: Number(draft.additional_minutes),
      quick_percent: Number(draft.quick_percent),
    });
    setDraft(emptyOperationDraft);
    setError("");
  };

  const handleKeyDown = (event) => {
    if (event.key === "Enter") {
      event.preventDefault();
      handleAdd();
    }
  };

  return (
    <div
      className="mt-4 grid gap-4 rounded-[24px] border border-dashed p-4 [background:color-mix(in_oklab,var(--settings-card-bg)_90%,transparent)] [border-color:var(--settings-card-border)]"
      onKeyDown={handleKeyDown}
    >
      <div>
        <strong className="text-base font-semibold tracking-[-0.02em] text-[color:var(--settings-text)]">Новая операция</strong>
        <p className="mt-1 text-sm text-[color:var(--settings-muted)]">Значения заполняются сразу для обоих режимов расчёта.</p>
      </div>
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <SettingsField label="Название">
          <input
            className={settingsInputClass}
            type="text"
            value={draft.name}
            onChange={updateField("name")}
            placeholder="Например, Косая бейка"
          />
        </SettingsField>
        <SettingsField label="Надбавка, %">
          <SettingsNumberInput step="0.01" min="0" value={draft.quick_percent} onChange={updateField("quick_percent")} placeholder="8" />
        </SettingsField>
        <SettingsField label="Доп. минуты">
          <SettingsNumberInput step="1" min="0" value={draft.additional_minutes} onChange={updateField("additional_minutes")} placeholder="15" />
        </SettingsField>
        <SettingsField label="Доп. материал / шт">
          <SettingsNumberInput step="1" min="0" value={draft.additional_material_per_unit} onChange={updateField("additional_material_per_unit")} placeholder="80" />
        </SettingsField>
      </div>
      {error ? (
        <p
          role="alert"
          className="rounded-2xl border px-4 py-2 text-sm font-medium text-[color:var(--settings-text)] [background:var(--settings-accent-soft)] [border-color:var(--settings-accent)]"
        >
          {error}
        </p>
      ) : null}
      <div>
        <button
          type="button"
          onClick={handleAdd}
          className="h-11 rounded-2xl border px-5 text-sm font-semibold text-[color:var(--settings-text)] transition [background:var(--settings-input-bg)] [border-color:var(--settings-accent)] hover:[background:var(--settings-accent-soft)] focus:outline-none focus:ring-4 focus:ring-[color:var(--settings-focus)]"
        >
          Добавить
        </button>
      </div>
    </div>
  );
};

// hasPanelAccess — клиентское зеркало серверного правила has_access || role == admin
// (Decision 10). Держится в одном месте: настоящую авторизацию всё равно делает сервер,
// здесь условие нужно только чтобы выбрать экран.
const hasPanelAccess = (accessState) =>
  Boolean(accessState?.has_access) || accessState?.role === "admin";

const Panel = () => {
  // status: "checking" -> "ready" | "no-access". "no-access" — не ошибка загрузки, а
  // штатный экран: пользователь аутентифицирован, но доступ ему ещё не выдан.
  const [status, setStatus] = useState("checking");
  const [profile, setProfile] = useState(null);
  const [access, setAccess] = useState(null);
  const [activeSection, setActiveSection] = useState("workspace");
  const [theme, setTheme] = useState(() => localStorage.getItem("panelTheme") || "light");
  const [settings, setSettings] = useState(defaultSettings);
  const [settingsNotice, setSettingsNotice] = useState("");
  const [chats, setChats] = useState([]);
  const [activeChatID, setActiveChatID] = useState("");
  const [chatTitleDraft, setChatTitleDraft] = useState("");
  const [chatNotice, setChatNotice] = useState("");
  const [history, setHistory] = useState([]);
  const [historyStatus, setHistoryStatus] = useState("idle");
  const [orderForm, setOrderForm] = useState(createDefaultOrderForm(defaultSettings));
  const [calcNotice, setCalcNotice] = useState("");
  const [isSavingSettings, setIsSavingSettings] = useState(false);
  const [isCreatingChat, setIsCreatingChat] = useState(false);
  const [isCalculating, setIsCalculating] = useState(false);
  const [isDeletingChatID, setIsDeletingChatID] = useState("");

  const userID = profile?.login || "";

  useEffect(() => {
    let isActive = true;

    const bootstrap = async () => {
      let accessState = null;

      try {
        const ok = await checkAuthStatus();
        if (!ok) {
          window.location.replace("/");
          return;
        }

        const nextProfile = await fetchAuthProfile();
        if (!isActive) {
          return;
        }
        setProfile(nextProfile);

        // Третий вызов bootstrap-эффекта: это единственная точка, через которую проходит
        // каждый рендер /panel (Decision 15), поэтому состояние доступа берётся здесь и
        // больше нигде — плашка и (в Task 8) раздел администратора читают уже готовое.
        accessState = await fetchAccessState();
        if (!isActive) {
          return;
        }
        setAccess(accessState);
      } catch {
        // Провал любого из трёх вызовов трактуется одинаково — как отсутствие рабочей
        // сессии: у /api/v1/access/me ветки 403 нет вовсе, «доступа нет» приходит как
        // 200 с has_access: false, поэтому сюда попадает только 401 или сетевой сбой.
        if (isActive) {
          window.location.replace("/");
        }
        return;
      }

      if (isActive) {
        setStatus(hasPanelAccess(accessState) ? "ready" : "no-access");
      }
    };

    bootstrap();
    return () => {
      isActive = false;
    };
  }, []);

  useEffect(() => {
    localStorage.setItem("panelTheme", theme);
  }, [theme]);

  // Оба эффекта загрузки данных ждут status === "ready", а не только userID: userID
  // становится истинным вместе с profile, то есть раньше, чем известен результат проверки
  // доступа. Без этой охраны пользователь без доступа успел бы отправить GET /settings и
  // GET /chats до того, как на экране появится плашка.
  useEffect(() => {
    if (status !== "ready" || !userID) {
      return;
    }

    let isActive = true;

    const loadSettings = async () => {
      try {
        const loaded = await getUserSettings();
        if (!isActive) {
          return;
        }
        const nextSettings = normalizeSettings(loaded);
        setSettings(nextSettings);
        setOrderForm((current) => syncOrderForm(current, nextSettings));
      } catch (error) {
        if (!isActive) {
          return;
        }
        if (error?.status !== 404) {
          setSettingsNotice("Не удалось загрузить настройки.");
        }
      }
    };

    const loadChats = async () => {
      try {
        const payload = await listChats();
        if (!isActive) {
          return;
        }
        const nextChats = payload.items || [];
        setChats(nextChats);
        setActiveChatID((current) => current || nextChats[0]?.id || "");
      } catch {
        if (isActive) {
          setChatNotice("Не удалось загрузить чаты.");
        }
      }
    };

    loadSettings();
    loadChats();

    return () => {
      isActive = false;
    };
  }, [status, userID]);

  useEffect(() => {
    if (status !== "ready" || !userID || !activeChatID) {
      setHistory([]);
      return;
    }

    let isActive = true;
    setHistoryStatus("loading");

    listChatCalculations(activeChatID)
      .then((payload) => {
        if (!isActive) {
          return;
        }
        setHistory(payload.items || []);
        setHistoryStatus("ready");
      })
      .catch(() => {
        if (!isActive) {
          return;
        }
        setHistory([]);
        setHistoryStatus("error");
      });

    return () => {
      isActive = false;
    };
  }, [status, userID, activeChatID]);

  const garmentOptions = useMemo(() => Object.keys(settings.garments).sort((a, b) => a.localeCompare(b)), [settings.garments]);
  const materialOptions = useMemo(() => Object.keys(settings.materials).sort((a, b) => a.localeCompare(b)), [settings.materials]);
  const urgencyOptions = useMemo(() => Object.keys(settings.urgency).sort((a, b) => a.localeCompare(b)), [settings.urgency]);
  const operationOptions = useMemo(() => Object.keys(settings.operations).sort((a, b) => a.localeCompare(b)), [settings.operations]);
  const marketOptions = useMemo(() => Object.keys(settings.market_bands).sort((a, b) => a.localeCompare(b)), [settings.market_bands]);
  const activeChat = chats.find((chat) => chat.id === activeChatID) || null;
  const calculatorMode = normalizeCalculatorMode(settings.pricing_rules?.calculator_mode);
  const isQuickCalculator = calculatorMode === "quick";
  const totalHistoryAmount = useMemo(
    () => history.reduce((sum, item) => sum + (Number(item.total) || 0), 0),
    [history]
  );
  // Роль берётся из уже полученного bootstrap-эффектом ответа /api/v1/access/me — отдельного
  // запроса за identity здесь нет. Флаг решает только, что рендерить: доступ к самим
  // админским маршрутам сервер проверяет сам (RequireAdmin).
  const isAdmin = access?.role === "admin";

  // Меню профиля — единственный пункт в нём (выход) и так был отдельной кнопкой; шеврон
  // просто даёт ему то же место, что в референсе, а не вводит новую функциональность.
  const [profileMenuOpen, setProfileMenuOpen] = useState(false);
  const profileMenuRef = useRef(null);

  useEffect(() => {
    if (!profileMenuOpen) {
      return undefined;
    }
    const closeOnOutsideClick = (event) => {
      if (profileMenuRef.current && !profileMenuRef.current.contains(event.target)) {
        setProfileMenuOpen(false);
      }
    };
    const closeOnEscape = (event) => {
      if (event.key === "Escape") {
        setProfileMenuOpen(false);
      }
    };
    document.addEventListener("mousedown", closeOnOutsideClick);
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("mousedown", closeOnOutsideClick);
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [profileMenuOpen]);

  const handleLogout = () => {
    logout()
      .catch(() => {})
      .finally(() => {
        window.location.replace("/");
      });
  };

  const handleRuleChange = (key, value) => {
    setSettings((current) => ({
      ...current,
      pricing_rules: {
        ...current.pricing_rules,
        [key]: Number(value) || 0,
      },
    }));
  };

  const handleCalculatorModeChange = (value) => {
    setSettings((current) => ({
      ...current,
      pricing_rules: {
        ...current.pricing_rules,
        calculator_mode: normalizeCalculatorMode(value),
      },
    }));
  };

  const handleGarmentChange = (name, key, value) => {
    setSettings((current) => ({
      ...current,
      garments: {
        ...current.garments,
        [name]: {
          ...current.garments[name],
          [key]: Number(value) || 0,
        },
      },
    }));
  };

  const handleOperationSettingChange = (name, key, value) => {
    setSettings((current) => ({
      ...current,
      operations: {
        ...current.operations,
        [name]: {
          ...current.operations[name],
          [key]: Number(value) || 0,
        },
      },
    }));
  };

  // Добавление и удаление строк настроек. Валидацию делают формы добавления (Wave 3) через
  // isBlankName / isDuplicateName / validateGarmentFields / validateOperationFields /
  // validateDiscountFields — сюда значения приходят уже проверенными, обработчик только пишет
  // в состояние. Контракт для вызывающей стороны: name передавать уже обрезанным (name.trim()),
  // иначе ключом изделия/операции станет строка с пробелами по краям. Сохранение на сервер
  // по-прежнему делает общая кнопка «Сохранить изменения».
  const handleAddGarment = (name, fields) => {
    setSettings((current) => ({
      ...current,
      garments: {
        ...current.garments,
        [name]: fields,
      },
    }));
  };

  const handleDeleteGarment = (name) => {
    setSettings((current) => ({
      ...current,
      garments: Object.fromEntries(Object.entries(current.garments).filter(([key]) => key !== name)),
    }));
    // syncOrderForm чинит такие расхождения только на загрузке настроек, поэтому удаление
    // внутри сессии обязано подчистить ссылку само — иначе в калькуляторе останется имя,
    // которого больше нет, и расчёт упадёт на сервере. Фолбэк тот же, что в syncOrderForm:
    // первое оставшееся изделие (удаляемое исключено), пустая строка — если их не осталось.
    setOrderForm((current) =>
      current.garment_type === name
        ? { ...current, garment_type: Object.keys(settings.garments).find((key) => key !== name) || "" }
        : current
    );
  };

  const handleAddOperation = (name, fields) => {
    setSettings((current) => ({
      ...current,
      operations: {
        ...current.operations,
        [name]: fields,
      },
    }));
  };

  const handleDeleteOperation = (name) => {
    setSettings((current) => ({
      ...current,
      operations: Object.fromEntries(Object.entries(current.operations).filter(([key]) => key !== name)),
    }));
    // То же, что в handleDeleteGarment: ключи operation_counts в syncOrderForm — ровно ключи
    // settings.operations, поэтому удалённая операция просто выбрасывается. Иначе её счётчик
    // пережил бы удаление, прошёл фильтр count > 0 в handleCalculate и вернулся бы с сервера
    // ошибкой unknown operation.
    setOrderForm((current) => {
      if (!(name in (current.operation_counts || {}))) {
        return current;
      }
      const { [name]: removedCount, ...restCounts } = current.operation_counts;
      return { ...current, operation_counts: restCounts };
    });
  };

  const handleAddDiscount = (fields) => {
    setSettings((current) => ({
      ...current,
      batch_discounts: [...current.batch_discounts, fields],
    }));
  };

  const handleDeleteDiscount = (index) => {
    setSettings((current) => ({
      ...current,
      batch_discounts: current.batch_discounts.filter((_, itemIndex) => itemIndex !== index),
    }));
  };

  const handleMaterialChange = (name, key, value) => {
    setSettings((current) => ({
      ...current,
      materials: {
        ...current.materials,
        [name]: {
          ...current.materials[name],
          [key]: Number(value) || 0,
        },
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

  const handleUrgencyChange = (name, value) => {
    setSettings((current) => ({
      ...current,
      urgency: {
        ...current.urgency,
        [name]: { percent: Number(value) || 0 },
      },
    }));
  };

  const handleMarketBandChange = (name, key, value) => {
    setSettings((current) => ({
      ...current,
      market_bands: {
        ...current.market_bands,
        [name]: {
          ...current.market_bands[name],
          [key]: Number(value) || 0,
        },
      },
    }));
  };

  const handleSaveSettings = async (event) => {
    event.preventDefault();
    if (!userID) {
      return;
    }

    setIsSavingSettings(true);
    setSettingsNotice("");
    try {
      await saveUserSettings(settings);
      setSettingsNotice("Настройки сохранены.");
    } catch (error) {
      setSettingsNotice(mapPanelError(error));
    } finally {
      setIsSavingSettings(false);
    }
  };

  const handleCreateChat = async () => {
    if (!userID) {
      return;
    }

    setIsCreatingChat(true);
    setChatNotice("");
    try {
      const chat = await createChat(chatTitleDraft.trim());
      setChats((current) => [chat, ...current]);
      setActiveChatID(chat.id);
      setChatTitleDraft("");
      setChatNotice("Чат создан.");
    } catch (error) {
      setChatNotice(mapPanelError(error));
    } finally {
      setIsCreatingChat(false);
    }
  };

  const handleDeleteChat = async (chatID) => {
    if (!userID) {
      return;
    }
    const ok = window.confirm("Удалить чат? История будет скрыта из списка.");
    if (!ok) {
      return;
    }

    setIsDeletingChatID(chatID);
    setChatNotice("");
    try {
      await deleteChat(chatID);
      const nextChats = chats.filter((chat) => chat.id !== chatID);
      setChats(nextChats);
      if (activeChatID === chatID) {
        setHistory([]);
        setActiveChatID(nextChats[0]?.id || "");
      }
      setChatNotice("Чат удалён.");
    } catch (error) {
      setChatNotice(mapPanelError(error));
    } finally {
      setIsDeletingChatID("");
    }
  };

  const handleOrderChange = (key, value) => {
    setOrderForm((current) => ({ ...current, [key]: value }));
  };

  const handleOperationCountChange = (name, value) => {
    setOrderForm((current) => ({
      ...current,
      operation_counts: {
        ...current.operation_counts,
        [name]: Math.max(0, Number(value) || 0),
      },
    }));
  };

  const handleCalculate = async (event) => {
    event.preventDefault();
    if (!userID || !activeChatID) {
      return;
    }

    setIsCalculating(true);
    setCalcNotice("");
    try {
      const payload = {
        garment_type: orderForm.garment_type,
        material_type: isQuickCalculator ? "" : orderForm.material_type,
        urgency: isQuickCalculator ? "" : orderForm.urgency,
        market_segment: isQuickCalculator ? "" : orderForm.market_segment,
        quantity: Number(orderForm.quantity) || 0,
        fittings: isQuickCalculator ? 0 : Number(orderForm.fittings) || 0,
        is_custom_figure: isQuickCalculator ? false : Boolean(orderForm.is_custom_figure),
        is_child: isQuickCalculator ? false : Boolean(orderForm.is_child),
        comment: orderForm.comment,
        operation_counts: Object.fromEntries(
          Object.entries(orderForm.operation_counts).filter(([, count]) => Number(count) > 0)
        ),
      };
      const result = await calculateInChat(activeChatID, payload);
      setHistory((current) => [...current, result]);
      setChats((current) =>
        current
          .map((chat) =>
            chat.id === activeChatID
              ? {
                  ...chat,
                  updated_at: result.created_at,
                  calculations_count: (chat.calculations_count || 0) + 1,
                }
              : chat
          )
          .sort((a, b) => new Date(b.updated_at) - new Date(a.updated_at))
      );
      setCalcNotice(`Расчёт выполнен. Итог: ${formatMoney(result.total)} ₽`);
    } catch (error) {
      setCalcNotice(mapPanelError(error));
    } finally {
      setIsCalculating(false);
    }
  };

  // Гейт стоит до всего рабочего интерфейса: пользователь без доступа не должен видеть ни
  // одного его элемента, а не просто получать 403 на фоновых запросах.
  if (status === "no-access") {
    return (
      <div className={`page panel panel--${theme}`}>
        <main className="panel__content">
          <AccessRequestBanner
            contactEmail={access?.contact_email || ""}
            requestStatus={access?.request_status || ""}
          />
        </main>
      </div>
    );
  }

  if (status !== "ready") {
    return (
      <div className={`page panel panel--${theme}`}>
        <main className="panel__content">
          <p>Проверяем доступ...</p>
        </main>
      </div>
    );
  }

  return (
    <div className={`page panel panel--${theme}`}>
      <aside className="panel__sidebar">
        <nav className="panel__nav">
          <button
            className={`panel__link ${activeSection === "workspace" ? "panel__link--active" : ""}`}
            type="button"
            onClick={() => setActiveSection("workspace")}
          >
            <span className="panel__link-icon"><NavWorkspaceIcon /></span>
            Чаты и расчёты
          </button>
          <button
            className={`panel__link ${activeSection === "settings" ? "panel__link--active" : ""}`}
            type="button"
            onClick={() => setActiveSection("settings")}
          >
            <span className="panel__link-icon"><NavSettingsIcon /></span>
            Настройки модели
          </button>
          {/* Не-админу пункт не рендерится вовсе, а не прячется стилями: сам факт наличия
              раздела ему знать незачем, а запросы за списком закрыты RequireAdmin. */}
          {isAdmin ? (
            <button
              className={`panel__link ${activeSection === "users" ? "panel__link--active" : ""}`}
              type="button"
              onClick={() => setActiveSection("users")}
            >
              <span className="panel__link-icon"><NavUsersIcon /></span>
              Пользователи
            </button>
          ) : null}
        </nav>
        <div className="panel__sidebar-bottom">
          <button
            className="panel__sidebar-toggle"
            type="button"
            onClick={() => setTheme(theme === "light" ? "dark" : "light")}
          >
            <span className="panel__sidebar-toggle-icon">
              {theme === "light" ? <MoonIcon /> : <SunIcon />}
            </span>
            {theme === "light" ? "Тёмная тема" : "Светлая тема"}
          </button>
          <div className="panel__profile" ref={profileMenuRef}>
            {profileMenuOpen ? (
              <div className="panel__profile-menu" role="menu">
                <button
                  type="button"
                  className="panel__profile-menu-item"
                  role="menuitem"
                  onClick={handleLogout}
                >
                  Выход из аккаунта
                </button>
              </div>
            ) : null}
            <button
              type="button"
              className="panel__profile-trigger"
              onClick={() => setProfileMenuOpen((open) => !open)}
              aria-haspopup="menu"
              aria-expanded={profileMenuOpen}
            >
              <span className="panel__profile-avatar" aria-hidden="true">
                {initialsFrom(profile?.name, userID)}
              </span>
              <span className="panel__profile-info">
                <span className="panel__profile-name">{profile?.name || userID}</span>
                <span className="panel__profile-login">{userID}</span>
              </span>
              <span className="panel__profile-chevron"><ChevronUpDownIcon /></span>
            </button>
          </div>
        </div>
      </aside>
      <main className="panel__content">
        <div className="panel__header">
          <p className="panel__eyebrow">Рабочая панель</p>
        </div>

        {activeSection === "settings" ? (
          <section className="panel-settings rounded-[32px] border p-5 shadow-[0_28px_80px_var(--settings-shell-shadow)] backdrop-blur-xl motion-safe:animate-fade-rise [background:var(--settings-shell-bg)] [border-color:var(--settings-shell-border)] sm:p-7">
            <form className="space-y-5" onSubmit={handleSaveSettings}>
              <SettingsSection
                title="Режим расчёта"
                description="Выберите логику калькулятора. Карточки переключения собраны как отдельные поверхности, чтобы активное состояние читалось без резкого контраста."
              >
                <div className="grid gap-4 lg:grid-cols-2">
                  {calculatorModes.map((mode) => {
                    const isActive = calculatorMode === mode.value;
                    return (
                      <button
                        key={mode.value}
                        className={`${settingsModeButtonBaseClass} ${
                          isActive
                            ? "translate-y-[-1px] [background:color-mix(in_oklab,var(--settings-accent)_10%,var(--settings-card-bg))] [border-color:color-mix(in_oklab,var(--settings-accent)_22%,var(--settings-card-border))] shadow-[0_22px_44px_var(--settings-card-shadow)]"
                            : ""
                        }`}
                        type="button"
                        onClick={() => handleCalculatorModeChange(mode.value)}
                      >
                        <div className="flex items-center justify-between gap-3">
                          <strong className="text-base font-semibold tracking-[-0.02em] text-[color:var(--settings-text)]">{mode.label}</strong>
                          <span
                            aria-hidden="true"
                            className={`size-3 rounded-full border ${
                              isActive
                                ? "[background:var(--settings-accent)] [border-color:var(--settings-accent)]"
                                : "[background:transparent] [border-color:var(--settings-input-border)]"
                            }`}
                          />
                        </div>
                        <span className="text-sm leading-6 text-[color:var(--settings-muted)]">{mode.description}</span>
                      </button>
                    );
                  })}
                </div>
              </SettingsSection>

              {isQuickCalculator ? (
                <>
                  <SettingsSection title="Изделия" description="Базовая стоимость за единицу для быстрого расчёта.">
                    <div className="grid gap-4">
                      {Object.entries(settings.garments).map(([name, item]) => (
                        <div
                          className="grid gap-4 rounded-[24px] border p-4 [background:color-mix(in_oklab,var(--settings-card-bg)_90%,transparent)] [border-color:var(--settings-card-border)] md:grid-cols-[minmax(0,1fr)_minmax(220px,280px)] md:items-end"
                          key={name}
                        >
                          <div>
                            <div className="flex items-center justify-between gap-3">
                              <strong className="text-base font-semibold tracking-[-0.02em] text-[color:var(--settings-text)]">{name}</strong>
                              <DeleteRowButton isDeletable={!DEFAULT_GARMENT_NAMES.includes(name)} onDelete={() => handleDeleteGarment(name)} />
                            </div>
                            <p className="mt-1 text-sm text-[color:var(--settings-muted)]">Фиксированная минимальная цена на единицу изделия.</p>
                          </div>
                          <SettingsField label="Мин. цена / шт">
                            <SettingsNumberInput min="0" value={item.quick_price} onChange={(event) => handleGarmentChange(name, "quick_price", event.target.value)} />
                          </SettingsField>
                        </div>
                      ))}
                    </div>
                    <GarmentAddForm settings={settings} onAddGarment={handleAddGarment} isQuickMode />
                  </SettingsSection>

                  <SettingsSection title="Усложнения" description="Процентные надбавки, которые добавляются к базовой цене в быстром режиме.">
                    <div className="grid gap-4">
                      {Object.entries(settings.operations).map(([name, item]) => (
                        <div
                          className="grid gap-4 rounded-[24px] border p-4 [background:color-mix(in_oklab,var(--settings-card-bg)_90%,transparent)] [border-color:var(--settings-card-border)] md:grid-cols-[minmax(0,1fr)_minmax(220px,280px)] md:items-end"
                          key={name}
                        >
                          <div>
                            <div className="flex items-center justify-between gap-3">
                              <strong className="text-base font-semibold tracking-[-0.02em] text-[color:var(--settings-text)]">{name}</strong>
                              <DeleteRowButton isDeletable={!DEFAULT_OPERATION_NAMES.includes(name)} onDelete={() => handleDeleteOperation(name)} />
                            </div>
                            <p className="mt-1 text-sm text-[color:var(--settings-muted)]">Добавка к цене за дополнительную сложность.</p>
                          </div>
                          <SettingsField label="Надбавка, %">
                            <SettingsNumberInput step="0.01" min="0" value={item.quick_percent} onChange={(event) => handleOperationSettingChange(name, "quick_percent", event.target.value)} />
                          </SettingsField>
                        </div>
                      ))}
                    </div>
                    <OperationAddForm settings={settings} onAddOperation={handleAddOperation} />
                  </SettingsSection>

                  <DiscountsBlock
                    settings={settings}
                    handleDiscountChange={handleDiscountChange}
                    handleAddDiscount={handleAddDiscount}
                    handleDeleteDiscount={handleDeleteDiscount}
                  />
                </>
              ) : (
                <>
                  <SettingsSection title="Общие правила" description="Базовые коэффициенты и ставки, влияющие на расчёт стоимости и цены.">
                    <div className="grid gap-4 md:grid-cols-2 2xl:grid-cols-3">
                      {Object.entries(settings.pricing_rules)
                        .filter(([key]) => key !== "calculator_mode")
                        .map(([key, value]) => (
                          <div key={key} className={settingsInsetClass}>
                            <SettingsField label={ruleLabels[key] || key}>
                              <SettingsNumberInput step="0.01" min="0" value={value} onChange={(event) => handleRuleChange(key, event.target.value)} />
                            </SettingsField>
                          </div>
                        ))}
                    </div>
                  </SettingsSection>

                  <SettingsSection title="Изделия" description="База в минутах и коэффициент сложности по каждому виду изделия.">
                    <div className="grid gap-4">
                      {Object.entries(settings.garments).map(([name, item]) => (
                        <div
                          className="grid gap-4 rounded-[24px] border p-4 [background:color-mix(in_oklab,var(--settings-card-bg)_90%,transparent)] [border-color:var(--settings-card-border)] xl:grid-cols-[minmax(0,1fr)_minmax(180px,220px)_minmax(180px,220px)] xl:items-end"
                          key={name}
                        >
                          <div>
                            <div className="flex items-center justify-between gap-3">
                              <strong className="text-base font-semibold tracking-[-0.02em] text-[color:var(--settings-text)]">{name}</strong>
                              <DeleteRowButton isDeletable={!DEFAULT_GARMENT_NAMES.includes(name)} onDelete={() => handleDeleteGarment(name)} />
                            </div>
                            <p className="mt-1 text-sm text-[color:var(--settings-muted)]">Параметры расчёта для этого типа изделия.</p>
                          </div>
                          <SettingsField label="База, мин">
                            <SettingsNumberInput min="0" value={item.base_minutes} onChange={(event) => handleGarmentChange(name, "base_minutes", event.target.value)} />
                          </SettingsField>
                          <SettingsField label="Коэфф.">
                            <SettingsNumberInput step="0.01" min="0" value={item.complexity_coeff} onChange={(event) => handleGarmentChange(name, "complexity_coeff", event.target.value)} />
                          </SettingsField>
                        </div>
                      ))}
                    </div>
                    <GarmentAddForm settings={settings} onAddGarment={handleAddGarment} />
                  </SettingsSection>

                  <SettingsSection title="Операции" description="Дополнительные минуты и материалы, увеличивающие стоимость единицы.">
                    <div className="grid gap-4">
                      {Object.entries(settings.operations).map(([name, item]) => (
                        <div
                          className="grid gap-4 rounded-[24px] border p-4 [background:color-mix(in_oklab,var(--settings-card-bg)_90%,transparent)] [border-color:var(--settings-card-border)] xl:grid-cols-[minmax(0,1fr)_minmax(180px,220px)_minmax(180px,220px)] xl:items-end"
                          key={name}
                        >
                          <div>
                            <div className="flex items-center justify-between gap-3">
                              <strong className="text-base font-semibold tracking-[-0.02em] text-[color:var(--settings-text)]">{name}</strong>
                              <DeleteRowButton isDeletable={!DEFAULT_OPERATION_NAMES.includes(name)} onDelete={() => handleDeleteOperation(name)} />
                            </div>
                            <p className="mt-1 text-sm text-[color:var(--settings-muted)]">Норма времени и материалов на одну дополнительную операцию.</p>
                          </div>
                          <SettingsField label="Минуты">
                            <SettingsNumberInput min="0" value={item.additional_minutes} onChange={(event) => handleOperationSettingChange(name, "additional_minutes", event.target.value)} />
                          </SettingsField>
                          <SettingsField label="Материалы / шт">
                            <SettingsNumberInput min="0" value={item.additional_material_per_unit} onChange={(event) => handleOperationSettingChange(name, "additional_material_per_unit", event.target.value)} />
                          </SettingsField>
                        </div>
                      ))}
                    </div>
                    <OperationAddForm settings={settings} onAddOperation={handleAddOperation} />
                  </SettingsSection>

                  <SettingsSection title="Материалы" description="Токены затрат по тканям и комплектующим с разбиением по каждой категории.">
                    <div className="grid gap-4 xl:grid-cols-2">
                      {Object.entries(settings.materials).map(([name, item]) => (
                        <div className={settingsSectionClass} key={name}>
                          <div className="mb-4">
                            <strong className="text-base font-semibold tracking-[-0.02em] text-[color:var(--settings-text)]">{name}</strong>
                          </div>
                          <div className="grid gap-4 md:grid-cols-2">
                            {Object.entries(item).map(([key, value]) => (
                              <SettingsField key={key} label={materialLabels[key] || key}>
                                <SettingsNumberInput step="0.01" min="0" value={value} onChange={(event) => handleMaterialChange(name, key, event.target.value)} />
                              </SettingsField>
                            ))}
                          </div>
                        </div>
                      ))}
                    </div>
                  </SettingsSection>

                  <DiscountsBlock
                    settings={settings}
                    handleDiscountChange={handleDiscountChange}
                    handleAddDiscount={handleAddDiscount}
                    handleDeleteDiscount={handleDeleteDiscount}
                  />

                  <SettingsSection title="Срочность" description="Процентная надбавка к цене в зависимости от срока выполнения.">
                    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
                      {Object.entries(settings.urgency).map(([name, item]) => (
                        <div key={name} className={settingsInsetClass}>
                          <SettingsField label={name}>
                            <SettingsNumberInput step="0.01" min="0" value={item.percent} onChange={(event) => handleUrgencyChange(name, event.target.value)} />
                          </SettingsField>
                        </div>
                      ))}
                    </div>
                  </SettingsSection>

                  <SettingsSection title="Рынок" description="Нижняя, средняя и верхняя граница цены для проверки попадания в сегмент.">
                    <div className="grid gap-4 xl:grid-cols-3">
                      {Object.entries(settings.market_bands).map(([name, item]) => (
                        <div className={settingsSectionClass} key={name}>
                          <div className="mb-4">
                            <strong className="text-base font-semibold tracking-[-0.02em] text-[color:var(--settings-text)]">{name}</strong>
                          </div>
                          <div className="grid gap-4">
                            <SettingsField label="Мин">
                              <SettingsNumberInput min="0" value={item.min_price_per_unit} onChange={(event) => handleMarketBandChange(name, "min_price_per_unit", event.target.value)} />
                            </SettingsField>
                            <SettingsField label="Средняя">
                              <SettingsNumberInput min="0" value={item.average_price_per_unit} onChange={(event) => handleMarketBandChange(name, "average_price_per_unit", event.target.value)} />
                            </SettingsField>
                            <SettingsField label="Макс">
                              <SettingsNumberInput min="0" value={item.max_price_per_unit} onChange={(event) => handleMarketBandChange(name, "max_price_per_unit", event.target.value)} />
                            </SettingsField>
                          </div>
                        </div>
                      ))}
                    </div>
                  </SettingsSection>
                </>
              )}

              <div className="flex flex-col gap-3 pt-2 sm:flex-row sm:items-center sm:justify-between">
                <button
                  className="inline-flex min-h-12 items-center justify-center rounded-2xl border px-5 text-sm font-semibold text-white motion-safe:transition-all motion-safe:duration-200 motion-safe:ease-[var(--ease-soft-spring)] motion-safe:hover:-translate-y-0.5 motion-safe:hover:opacity-95 motion-safe:active:scale-[0.985] disabled:cursor-not-allowed disabled:opacity-60 [background:var(--settings-accent)] [border-color:color-mix(in_oklab,var(--settings-accent)_90%,black)] shadow-[0_16px_30px_var(--settings-card-shadow)]"
                  type="submit"
                  disabled={isSavingSettings}
                >
                  {isSavingSettings ? "Сохраняем..." : "Сохранить изменения"}
                </button>
                {settingsNotice ? <p className="text-sm leading-6 text-[color:var(--settings-muted)]">{settingsNotice}</p> : null}
              </div>
            </form>
          </section>
        ) : activeSection === "users" && isAdmin ? (
          // isAdmin здесь дублирует условие пункта меню намеренно: без него состояние
          // "users", оставшееся от прошлого рендера, отправило бы не-админа в раздел,
          // который тут же упрётся в 403. Не-админ попадает в рабочую ветку по умолчанию.
          <AdminUsersSection />
        ) : (
          <>
            <section className="panel-summary">
              <article className="panel-summary__card motion-safe:animate-fade-rise motion-safe:transition-transform motion-safe:duration-300 motion-safe:ease-[var(--ease-soft-spring)] motion-safe:hover:-translate-y-1">
                <span className="panel-summary__label">Активный чат</span>
                <strong>{activeChat?.title || "Не выбран"}</strong>
                <p>{activeChat ? `${activeChat.calculations_count || 0} расчётов сохранено` : "Создайте чат и начните расчёт."}</p>
              </article>
              <article className="panel-summary__card motion-safe:animate-fade-rise motion-safe:transition-transform motion-safe:duration-300 motion-safe:ease-[var(--ease-soft-spring)] motion-safe:hover:-translate-y-1 [animation-delay:80ms]">
                <span className="panel-summary__label">Всего чатов</span>
                <strong>{chats.length}</strong>
                <p>Чаты изолированы по пользователю и имеют собственную историю расчётов.</p>
              </article>
              <article className="panel-summary__card motion-safe:animate-fade-rise motion-safe:transition-transform motion-safe:duration-300 motion-safe:ease-[var(--ease-soft-spring)] motion-safe:hover:-translate-y-1 [animation-delay:140ms]">
                <span className="panel-summary__label">Сумма по чату</span>
                <strong>{formatMoney(totalHistoryAmount)} ₽</strong>
                <p>Сумма сохранённых расчётов в выбранном чате.</p>
              </article>
            </section>

            <section className="panel-workspace">
              <div className="panel__card panel-chat-list">
                <div className="panel-chat-list__header">
                  <h2>Чаты</h2>
                  <p>Удаление по умолчанию мягкое: чат пропадает из списка, история остаётся в базе.</p>
                </div>
                <div className="panel-chat-list__create">
                  <input type="text" placeholder="Название чата" value={chatTitleDraft} onChange={(event) => setChatTitleDraft(event.target.value)} />
                  <button type="button" onClick={handleCreateChat} disabled={isCreatingChat}>
                    {isCreatingChat ? "Создаём..." : "Новый чат"}
                  </button>
                </div>
                {chatNotice ? <p className="panel__notice">{chatNotice}</p> : null}
                <div className="panel-chat-list__items">
                  {chats.length === 0 ? (
                    <p className="panel__empty">Чатов пока нет. Создайте первый.</p>
                  ) : (
                    chats.map((chat) => (
                      <div className={`panel-chat-list__item-wrap ${chat.id === activeChatID ? "panel-chat-list__item-wrap--active" : ""}`} key={chat.id}>
                        <button className="panel-chat-list__item" type="button" onClick={() => setActiveChatID(chat.id)}>
                          <strong>{chat.title}</strong>
                          <span>{chat.calculations_count || 0} расчётов</span>
                        </button>
                        <button className="panel-chat-list__delete" type="button" onClick={() => handleDeleteChat(chat.id)} disabled={isDeletingChatID === chat.id}>
                          {isDeletingChatID === chat.id ? "..." : "Удалить"}
                        </button>
                      </div>
                    ))
                  )}
                </div>
              </div>

              <div className="panel-workspace__main">
                <div className="panel__card">
                  <h2>{activeChat ? activeChat.title : "Выберите чат"}</h2>
                  {activeChat ? (
                    <form className="panel-form" onSubmit={handleCalculate}>
                      <div className="panel-form__grid panel-form__grid--compact">
                        <label className="panel-form__row">
                          <span>Изделие</span>
                          <select value={orderForm.garment_type} onChange={(event) => handleOrderChange("garment_type", event.target.value)}>
                            {garmentOptions.map((name) => (
                              <option key={name} value={name}>{name}</option>
                            ))}
                          </select>
                        </label>
                        {isQuickCalculator ? null : (
                          <label className="panel-form__row">
                            <span>Материал</span>
                            <select value={orderForm.material_type} onChange={(event) => handleOrderChange("material_type", event.target.value)}>
                              {materialOptions.map((name) => (
                                <option key={name} value={name}>{name}</option>
                              ))}
                            </select>
                          </label>
                        )}
                        {isQuickCalculator ? null : (
                          <label className="panel-form__row">
                            <span>Срочность</span>
                            <select value={orderForm.urgency} onChange={(event) => handleOrderChange("urgency", event.target.value)}>
                              {urgencyOptions.map((name) => (
                                <option key={name} value={name}>{name}</option>
                              ))}
                            </select>
                          </label>
                        )}
                        {isQuickCalculator ? null : (
                          <label className="panel-form__row">
                            <span>Сегмент рынка</span>
                            <select value={orderForm.market_segment} onChange={(event) => handleOrderChange("market_segment", event.target.value)}>
                              {marketOptions.map((name) => (
                                <option key={name} value={name}>{name}</option>
                              ))}
                            </select>
                          </label>
                        )}
                        <label className="panel-form__row">
                          <span>Размер партии</span>
                          <input type="number" min="1" value={orderForm.quantity} onChange={(event) => handleOrderChange("quantity", Number(event.target.value) || 0)} />
                        </label>
                        {isQuickCalculator ? null : (
                          <label className="panel-form__row">
                            <span>Примерки</span>
                            <input type="number" min="0" value={orderForm.fittings} onChange={(event) => handleOrderChange("fittings", Number(event.target.value) || 0)} />
                          </label>
                        )}
                      </div>

                      {isQuickCalculator ? null : (
                        <div className="panel-form__grid panel-form__grid--compact panel-form__grid--toggles">
                          <label className="panel-form__toggle">
                            <input type="checkbox" checked={orderForm.is_custom_figure} onChange={(event) => handleOrderChange("is_custom_figure", event.target.checked)} />
                            <span>Нестандартная фигура</span>
                          </label>
                          <label className="panel-form__toggle">
                            <input type="checkbox" checked={orderForm.is_child} onChange={(event) => handleOrderChange("is_child", event.target.checked)} />
                            <span>Детское изделие</span>
                          </label>
                        </div>
                      )}

                      <label className="panel-form__row panel-form__row--stacked">
                        <span>Комментарий</span>
                        <textarea value={orderForm.comment} onChange={(event) => handleOrderChange("comment", event.target.value)} rows="4" />
                      </label>

                      <div className="panel-form__block">
                        <h3>{isQuickCalculator ? "Усложнения" : "Усложняющие операции"}</h3>
                        <div className="panel-form__grid panel-form__grid--compact">
                          {operationOptions.map((name) => (
                            <label className="panel-form__row" key={name}>
                              <span>{isQuickCalculator ? `${name} (${formatPercent(settings.operations[name]?.quick_percent)}%)` : name}</span>
                              <input type="number" min="0" value={orderForm.operation_counts[name] || 0} onChange={(event) => handleOperationCountChange(name, event.target.value)} />
                            </label>
                          ))}
                        </div>
                      </div>

                      <div className="panel-form__footer">
                        <button className="panel__theme-toggle motion-safe:transition-all motion-safe:duration-200 motion-safe:ease-[var(--ease-soft-spring)] motion-safe:hover:-translate-y-0.5 motion-safe:hover:shadow-[0_14px_28px_rgba(36,94,255,0.18)] motion-safe:active:scale-[0.985]" type="submit" disabled={isCalculating}>
                          {isCalculating ? "Считаем..." : "Рассчитать"}
                        </button>
                        {calcNotice ? <p className="panel__notice">{calcNotice}</p> : null}
                      </div>
                    </form>
                  ) : (
                    <p className="panel__empty">Сначала выберите чат слева.</p>
                  )}
                </div>

                <div className="panel__card">
                  <h2>История расчётов</h2>
                  {historyStatus === "loading" ? <p>Загружаем историю...</p> : null}
                  {historyStatus === "error" ? <p className="panel__notice">Не удалось загрузить историю расчётов.</p> : null}
                  {historyStatus !== "loading" && history.length === 0 ? <p className="panel__empty">В этом чате пока нет расчётов.</p> : null}
                  <div className="panel-history">
                    {history.map((item, index) => {
                      const itemMode = normalizeCalculatorMode(item.calculation_mode || calculatorMode);
                      return (
                        <article className="panel-history__item motion-safe:animate-fade-rise motion-safe:transition-transform motion-safe:duration-300 motion-safe:ease-[var(--ease-soft-spring)] motion-safe:hover:-translate-y-0.5" key={`${item.created_at}-${index}`}>
                          {itemMode === "quick" ? (
                            <>
                              <div className="panel-history__head">
                                <div>
                                  <strong>{item.garment_type}</strong>
                                  <span>Быстрый расчет</span>
                                </div>
                                <span>{new Date(item.created_at).toLocaleString("ru-RU")}</span>
                              </div>
                              <div className="panel-history__stats">
                                <span>Партия: {item.quantity}</span>
                                <span>База: {formatMoney(item.min_allowed_price_per_unit)} ₽</span>
                                <span>За единицу: {formatMoney(item.price_per_unit)} ₽</span>
                                <span>Итого: {formatMoney(item.total)} ₽</span>
                              </div>
                              <ul className="panel-history__list">
                                {item.applied_operations?.length > 0 ? item.applied_operations.map((operation) => (
                                  <li key={`${item.created_at}-${operation.name}`}>
                                    {operation.name} × {operation.count}: +{formatMoney(operation.additional_material_cost)} ₽
                                  </li>
                                )) : <li>Усложнений нет.</li>}
                                <li>До скидки: {formatMoney(item.price_before_discount_per_unit)} ₽ за единицу</li>
                                <li>Скидка: {item.discount_percent}% ({formatMoney(item.discount_amount)} ₽)</li>
                              </ul>
                            </>
                          ) : (
                            <>
                              <div className="panel-history__head">
                                <div>
                                  <strong>{item.garment_type}</strong>
                                  <span>{item.material_type} · {item.urgency}</span>
                                </div>
                                <span>{new Date(item.created_at).toLocaleString("ru-RU")}</span>
                              </div>
                              <div className="panel-history__stats">
                                <span>Партия: {item.quantity}</span>
                                <span>За единицу: {formatMoney(item.price_per_unit)} ₽</span>
                                <span>Итого: {formatMoney(item.total)} ₽</span>
                                <span className={`panel-history__badge panel-history__badge--${item.market_status || "unknown"}`}>
                                  {marketStatusLabel(item.market_status)}
                                </span>
                              </div>
                              <div className="panel-history__breakdown">
                                <span>Труд: {formatMoney(item.labor_cost_per_unit)} ₽</span>
                                <span>Материалы: {formatMoney(item.materials_cost_per_unit)} ₽</span>
                                <span>Расходники: {formatMoney(item.consumables_cost_per_unit)} ₽</span>
                                <span>Накладные: {formatMoney(item.overhead_cost_per_unit)} ₽</span>
                                <span>Риск: {formatMoney(item.risk_reserve_per_unit)} ₽</span>
                                <span>Себестоимость: {formatMoney(item.cost_price_per_unit)} ₽</span>
                              </div>
                              <ul className="panel-history__list">
                                {item.applied_operations?.length > 0 ? item.applied_operations.map((operation) => (
                                  <li key={`${item.created_at}-${operation.name}`}>
                                    {operation.name} × {operation.count}: +{operation.additional_minutes} мин, +{formatMoney(operation.additional_material_cost)} ₽
                                  </li>
                                )) : <li>Дополнительных операций нет.</li>}
                                <li>Скидка: {item.discount_percent}% ({formatMoney(item.discount_amount)} ₽)</li>
                                <li>Минуты: база {item.base_minutes_per_unit}, операции {item.operation_minutes_per_unit}, примерки {item.fitting_minutes_per_unit}, итог {item.adjusted_minutes_per_unit}</li>
                              </ul>
                              {itemMode === "quick" ? null : <CalculationAIFeedback calculation={item} feedback={item.ai_feedback} />}
                            </>
                          )}
                        </article>
                      );
                    })}
                  </div>
                </div>
              </div>
            </section>
          </>
        )}
      </main>
    </div>
  );
};

// DiscountAddForm — форма добавления диапазона скидки. Оба конца диапазона предзаполнены
// getDefaultDiscountRange: пустое или нулевое «До» не прошло бы серверную валидацию
// (max_qty >= min_qty) и заблокировало бы сохранение всех остальных правок разом — настройки
// уходят на сервер одним POST целого объекта.
//
// Это НЕ <form> — секция настроек уже обёрнута в <form onSubmit={handleSaveSettings}>,
// вложенные формы HTML запрещает. Отсюда type="button" + onClick и ручной перехват Enter:
// иначе Enter отправил бы внешнюю форму и сохранил настройки вместо добавления строки.
const DiscountAddForm = ({ settings, onAddDiscount }) => {
  const defaultRange = getDefaultDiscountRange(settings.batch_discounts);
  const [draft, setDraft] = useState({ min_qty: String(defaultRange.min_qty), max_qty: String(defaultRange.max_qty), percent: "" });
  const [suggestedMinQty, setSuggestedMinQty] = useState(defaultRange.min_qty);
  const [error, setError] = useState("");

  // Список диапазонов изменился (добавили, удалили или поправили «До» последней строки) —
  // подсказка пересчитывается, чтобы форма всегда предлагала продолжить с конца последнего
  // диапазона. Правка состояния прямо в рендере — штатный приём React для сброса стейта по
  // изменившемуся пропу: дешевле useEffect (без лишнего коммита) и без мигания старым значением.
  // Уже введённый процент не трогаем — он от диапазона не зависит.
  if (suggestedMinQty !== defaultRange.min_qty) {
    setSuggestedMinQty(defaultRange.min_qty);
    setDraft((current) => ({ ...current, min_qty: String(defaultRange.min_qty), max_qty: String(defaultRange.max_qty) }));
  }

  const updateField = (key) => (event) => {
    const { value } = event.target;
    setDraft((current) => ({ ...current, [key]: value }));
  };

  const handleAdd = () => {
    // Валидатору отдаются сырые строки из input: он сам приводит их к числу строго
    // (пустая строка и мусор дают NaN, а не 0). Приводить через Number() заранее нельзя —
    // это подменило бы невалидный ввод нулём ещё до проверки.
    const { valid, errors } = validateDiscountFields(draft);
    if (!valid) {
      setError(Object.values(errors).join(". "));
      return;
    }

    // Обработчик добавления числа не приводит — пишет в состояние как есть, поэтому
    // Number() здесь обязателен, иначе в batch_discounts уедут строки.
    const added = {
      min_qty: Number(draft.min_qty),
      max_qty: Number(draft.max_qty),
      percent: Number(draft.percent),
    };
    onAddDiscount(added);

    // Форма сразу предлагает следующий диапазон, а не очищается: пустые поля — это как раз
    // тот невалидный ввод, из-за которого ломается сохранение всех настроек. Подсказку считаем
    // от только что добавленной строки (handleAddDiscount дописывает её в конец списка), и тем
    // же значением обновляем suggestedMinQty, чтобы синхронизация выше не сбросила поля повторно.
    const nextRange = getDefaultDiscountRange([added]);
    setDraft({ min_qty: String(nextRange.min_qty), max_qty: String(nextRange.max_qty), percent: "" });
    setSuggestedMinQty(nextRange.min_qty);
    setError("");
  };

  const handleKeyDown = (event) => {
    if (event.key === "Enter") {
      event.preventDefault();
      handleAdd();
    }
  };

  return (
    <div
      className="mt-4 grid gap-4 rounded-[24px] border border-dashed p-4 [background:color-mix(in_oklab,var(--settings-card-bg)_90%,transparent)] [border-color:var(--settings-card-border)]"
      onKeyDown={handleKeyDown}
    >
      <div>
        <strong className="text-base font-semibold tracking-[-0.02em] text-[color:var(--settings-text)]">Новый диапазон</strong>
        <p className="mt-1 text-sm text-[color:var(--settings-muted)]">Диапазон предзаполнен сразу за последним существующим — поправьте его под свою партию.</p>
      </div>
      <div className="grid gap-4 md:grid-cols-3">
        <SettingsField label="От">
          <SettingsNumberInput step="1" min="1" value={draft.min_qty} onChange={updateField("min_qty")} placeholder={String(defaultRange.min_qty)} />
        </SettingsField>
        <SettingsField label="До">
          <SettingsNumberInput step="1" min="1" value={draft.max_qty} onChange={updateField("max_qty")} placeholder={String(defaultRange.max_qty)} />
        </SettingsField>
        <SettingsField label="Скидка, %">
          <SettingsNumberInput step="0.01" min="0" max="100" value={draft.percent} onChange={updateField("percent")} placeholder="5" />
        </SettingsField>
      </div>
      {error ? (
        <p
          role="alert"
          className="rounded-2xl border px-4 py-2 text-sm font-medium text-[color:var(--settings-text)] [background:var(--settings-accent-soft)] [border-color:var(--settings-accent)]"
        >
          {error}
        </p>
      ) : null}
      <div>
        <button
          type="button"
          onClick={handleAdd}
          className="h-11 rounded-2xl border px-5 text-sm font-semibold text-[color:var(--settings-text)] transition [background:var(--settings-input-bg)] [border-color:var(--settings-accent)] hover:[background:var(--settings-accent-soft)] focus:outline-none focus:ring-4 focus:ring-[color:var(--settings-focus)]"
        >
          Добавить
        </button>
      </div>
    </div>
  );
};

const DiscountsBlock = ({ settings, handleDiscountChange, handleAddDiscount, handleDeleteDiscount }) => {
  // Последний диапазон удалять нельзя: серверный normalizeSettings молча подменяет пустой
  // batch_discounts четырьмя дефолтными диапазонами (costing.go), то есть «удалил всё и
  // сохранил» вернуло бы чужие скидки вместо пустого списка.
  const canDeleteRow = settings.batch_discounts.length > 1;

  return (
    <SettingsSection
      title="Скидки за количество"
      description="Диапазоны количества и процент скидки для автоматического уменьшения цены на крупные заказы."
    >
      <div className="grid gap-4">
        {settings.batch_discounts.map((discount, index) => (
          <div
            className="grid gap-4 rounded-[24px] border p-4 [background:color-mix(in_oklab,var(--settings-card-bg)_90%,transparent)] [border-color:var(--settings-card-border)] md:grid-cols-[repeat(3,minmax(0,1fr))_auto]"
            key={`${discount.min_qty}-${discount.max_qty}-${index}`}
          >
            <SettingsField label="От">
              <SettingsNumberInput min="1" value={discount.min_qty} onChange={(event) => handleDiscountChange(index, "min_qty", event.target.value)} />
            </SettingsField>
            <SettingsField label="До">
              <SettingsNumberInput min="1" value={discount.max_qty} onChange={(event) => handleDiscountChange(index, "max_qty", event.target.value)} />
            </SettingsField>
            <SettingsField label="Скидка, %">
              <SettingsNumberInput step="0.01" min="0" max="100" value={discount.percent} onChange={(event) => handleDiscountChange(index, "percent", event.target.value)} />
            </SettingsField>
            <div className="flex items-end">
              <DeleteRowButton
                isDeletable
                disabled={!canDeleteRow}
                disabledHint="Последний диапазон удалить нельзя"
                onDelete={() => handleDeleteDiscount(index)}
              />
            </div>
          </div>
        ))}
      </div>
      <DiscountAddForm settings={settings} onAddDiscount={handleAddDiscount} />
    </SettingsSection>
  );
};

const CalculationAIFeedback = ({ calculation, feedback }) => {
  if (!calculation) {
    return null;
  }

  if (!feedback) {
    return null;
  }

  const finalPricePerUnit = Number(calculation.price_per_unit) || 0;
  const aiMidPrice = Number(feedback.estimated_unit_price_mid_rub) || 0;
  const aiMinPrice = Number(feedback.estimated_unit_price_min_rub) || 0;
  const aiMaxPrice = Number(feedback.estimated_unit_price_max_rub) || 0;
  const priceDelta = finalPricePerUnit - aiMidPrice;
  const priceDeltaPercent = aiMidPrice > 0 ? Math.round((priceDelta / aiMidPrice) * 100) : 0;
  const actualMarketPosition = formatMarketPosition(calculation.market_status);
  const actualSegmentLabel = calculation.market_status === "in_market"
    ? calculation.market_segment || feedback.suggested_market_segment || "выбранного сегмента"
    : feedback.suggested_market_segment || calculation.market_segment || "другого сегмента";
  const verdict = buildAIVerdict({
    finalPricePerUnit,
    aiMidPrice,
    marketStatus: calculation.market_status,
    targetSegment: calculation.market_segment,
    suggestedSegment: feedback.suggested_market_segment,
  });

  return (
    <div className="mt-4 rounded-[22px] border p-4 motion-safe:animate-fade-rise [background:color-mix(in_oklab,var(--panel-card)_90%,white)] [border-color:color-mix(in_oklab,var(--panel-accent)_16%,var(--panel-border))]">
      <div className="mb-3 flex items-center justify-between gap-3">
        <div>
          <strong className="block text-base font-semibold text-[color:var(--panel-text)]">Оценка DeepSeek</strong>
          <p className="mt-1 text-sm leading-6 text-[color:color-mix(in_oklab,var(--panel-text)_62%,white)]">
            Финальный расчет проверен на попадание в рынок и адекватность цены.
          </p>
        </div>
        <span className="rounded-full border px-3 py-1.5 text-xs font-semibold uppercase tracking-[0.16em] [background:color-mix(in_oklab,var(--panel-accent)_8%,white)] [border-color:color-mix(in_oklab,var(--panel-accent)_18%,var(--panel-border))] text-[color:var(--panel-accent)]">
          {formatConfidence(feedback.confidence)}
        </span>
      </div>

      <div className="grid gap-3 sm:grid-cols-3">
        <div className="rounded-[18px] border p-3.5 motion-safe:transition-transform motion-safe:duration-300 motion-safe:ease-[var(--ease-soft-spring)] motion-safe:hover:-translate-y-0.5 [background:color-mix(in_oklab,var(--panel-card)_94%,white)] [border-color:var(--panel-border)]">
          <span className="block text-xs font-semibold uppercase tracking-[0.16em] text-[color:color-mix(in_oklab,var(--panel-text)_55%,white)]">Ваш расчет</span>
          <strong className="mt-2 block text-xl text-[color:var(--panel-text)]">{formatMoney(finalPricePerUnit)} ₽</strong>
          <p className="mt-1 text-sm leading-6 text-[color:color-mix(in_oklab,var(--panel-text)_62%,white)]">
            {actualMarketPosition} относительно {actualSegmentLabel}
          </p>
        </div>
        <div className="rounded-[18px] border p-3.5 motion-safe:transition-transform motion-safe:duration-300 motion-safe:ease-[var(--ease-soft-spring)] motion-safe:hover:-translate-y-0.5 [background:color-mix(in_oklab,var(--panel-card)_94%,white)] [border-color:var(--panel-border)]">
          <span className="block text-xs font-semibold uppercase tracking-[0.16em] text-[color:color-mix(in_oklab,var(--panel-text)_55%,white)]">Ориентир AI</span>
          <strong className="mt-2 block text-xl text-[color:var(--panel-text)]">{formatMoney(aiMidPrice)} ₽</strong>
          <p className="mt-1 text-sm leading-6 text-[color:color-mix(in_oklab,var(--panel-text)_62%,white)]">
            {formatMoney(aiMinPrice)} - {formatMoney(aiMaxPrice)} ₽ за единицу
          </p>
        </div>
        <div className="rounded-[18px] border p-3.5 motion-safe:transition-transform motion-safe:duration-300 motion-safe:ease-[var(--ease-soft-spring)] motion-safe:hover:-translate-y-0.5 [background:color-mix(in_oklab,var(--panel-card)_94%,white)] [border-color:var(--panel-border)]">
          <span className="block text-xs font-semibold uppercase tracking-[0.16em] text-[color:color-mix(in_oklab,var(--panel-text)_55%,white)]">Отклонение</span>
          <strong className="mt-2 block text-xl text-[color:var(--panel-text)]">
            {priceDelta >= 0 ? "+" : ""}{formatMoney(priceDelta)} ₽
          </strong>
          <p className="mt-1 text-sm leading-6 text-[color:color-mix(in_oklab,var(--panel-text)_62%,white)]">
            {aiMidPrice > 0 ? `${priceDelta >= 0 ? "+" : ""}${priceDeltaPercent}% к AI-ориентиру` : "Без сравнения"}
          </p>
        </div>
      </div>

      <div className="mt-3 rounded-[18px] border p-3.5 motion-safe:animate-soft-pop [background:color-mix(in_oklab,var(--panel-card)_94%,white)] [border-color:var(--panel-border)]">
        <strong className="block text-[15px] font-semibold leading-6 text-[color:var(--panel-text)]">{verdict}</strong>
        <p className="mt-2 text-[15px] leading-7 text-[color:color-mix(in_oklab,var(--panel-text)_68%,white)]">
          {feedback.scenario_summary}
        </p>
        <p className="mt-2 text-[15px] leading-7 text-[color:color-mix(in_oklab,var(--panel-text)_68%,white)]">{feedback.reasoning}</p>
      </div>

      {(feedback.key_drivers?.length || feedback.recommendations?.length) ? (
        <div className="mt-3 grid gap-3 lg:grid-cols-2">
          <div className="rounded-[18px] border p-3.5 motion-safe:transition-transform motion-safe:duration-300 motion-safe:ease-[var(--ease-soft-spring)] motion-safe:hover:-translate-y-0.5 [background:color-mix(in_oklab,var(--panel-card)_94%,white)] [border-color:var(--panel-border)]">
            <strong className="block text-[15px] font-semibold text-[color:var(--panel-text)]">Что влияет на цену</strong>
            <ul className="mt-2 grid gap-2 text-[15px] leading-7 text-[color:color-mix(in_oklab,var(--panel-text)_68%,white)]">
              {(feedback.key_drivers || []).slice(0, 3).map((item, index) => (
                <li key={`${item}-${index}`}>• {item}</li>
              ))}
            </ul>
          </div>
          <div className="rounded-[18px] border p-3.5 motion-safe:transition-transform motion-safe:duration-300 motion-safe:ease-[var(--ease-soft-spring)] motion-safe:hover:-translate-y-0.5 [background:color-mix(in_oklab,var(--panel-card)_94%,white)] [border-color:var(--panel-border)]">
            <strong className="block text-[15px] font-semibold text-[color:var(--panel-text)]">Что делать</strong>
            <ul className="mt-2 grid gap-2 text-[15px] leading-7 text-[color:color-mix(in_oklab,var(--panel-text)_68%,white)]">
              {(feedback.recommendations || []).slice(0, 3).map((item, index) => (
                <li key={`${item}-${index}`}>• {item}</li>
              ))}
            </ul>
          </div>
        </div>
      ) : null}
    </div>
  );
};

const normalizeSettings = (settings) => ({
  pricing_rules: { ...defaultSettings.pricing_rules, ...(settings?.pricing_rules || {}) },
  garments: mergeNamedMap(defaultSettings.garments, settings?.garments),
  operations: mergeNamedMap(defaultSettings.operations, settings?.operations),
  materials: mergeNamedMap(defaultSettings.materials, settings?.materials),
  batch_discounts:
    settings?.batch_discounts?.length > 0
      ? settings.batch_discounts.map((item) => ({
          min_qty: Number(item.min_qty) || 0,
          max_qty: Number(item.max_qty) || 0,
          percent: Number(item.percent) || 0,
        }))
      : defaultSettings.batch_discounts,
  urgency: mergeNamedMap(defaultSettings.urgency, settings?.urgency),
  market_bands: mergeNamedMap(defaultSettings.market_bands, settings?.market_bands),
});

const mergeNamedMap = (defaults, incoming) => {
  const result = Object.fromEntries(
    Object.entries(defaults).map(([name, value]) => [name, { ...value }])
  );
  for (const [name, value] of Object.entries(incoming || {})) {
    result[name] = { ...(result[name] || {}), ...value };
  }
  return result;
};

const syncOrderForm = (current, settings) => ({
  ...current,
  garment_type: settings.garments[current.garment_type] ? current.garment_type : Object.keys(settings.garments)[0] || "",
  material_type: settings.materials[current.material_type] ? current.material_type : Object.keys(settings.materials)[0] || "",
  urgency: settings.urgency[current.urgency] ? current.urgency : Object.keys(settings.urgency)[0] || "",
  market_segment: settings.market_bands[current.market_segment] ? current.market_segment : Object.keys(settings.market_bands)[0] || "",
  operation_counts: Object.fromEntries(
    Object.keys(settings.operations).map((name) => [name, Number(current.operation_counts?.[name]) || 0])
  ),
});

const normalizeCalculatorMode = (value) => (value === "quick" ? "quick" : "masterpiece");

const formatMoney = (value) => new Intl.NumberFormat("ru-RU").format(Number(value) || 0);

const formatPercent = (value) => {
  const amount = Number(value) || 0;
  return Number.isInteger(amount) ? amount : amount.toFixed(2);
};

const formatConfidence = (value) => {
  switch (String(value || "").toLowerCase()) {
    case "high":
      return "Высокая";
    case "low":
      return "Низкая";
    default:
      return "Средняя";
  }
};

const formatMarketPosition = (value) => {
  switch (value) {
    case "below_market":
      return "Ниже рынка";
    case "above_market":
      return "Выше рынка";
    case "in_market":
      return "Внутри сегмента";
    default:
      return "Без сравнения";
  }
};

const marketStatusLabel = (status) => {
  switch (status) {
    case "below_market":
      return "Ниже рынка";
    case "above_market":
      return "Выше рынка";
    case "in_market":
      return "В рынке";
    default:
      return "Без рынка";
  }
};

const buildAIVerdict = ({ finalPricePerUnit, aiMidPrice, marketStatus, targetSegment, suggestedSegment }) => {
  const currentSegment = targetSegment || "целевого сегмента";
  const nextSegment = suggestedSegment || currentSegment;

  switch (marketStatus) {
    case "above_market":
      return `Ваш расчет ${formatMoney(finalPricePerUnit)} ₽/шт выше ${currentSegment} и ближе к сегменту «${nextSegment}».`;
    case "below_market":
      return `Ваш расчет ${formatMoney(finalPricePerUnit)} ₽/шт ниже ${currentSegment}; есть риск недооценить работу.`;
    case "in_market":
      if (aiMidPrice > 0) {
        return `Ваш расчет ${formatMoney(finalPricePerUnit)} ₽/шт остается в рынке, но отличается от AI-ориентира ${formatMoney(aiMidPrice)} ₽/шт.`;
      }
      return `Ваш расчет ${formatMoney(finalPricePerUnit)} ₽/шт находится внутри целевого сегмента.`;
    default:
      if (aiMidPrice > 0) {
        return `Ваш расчет ${formatMoney(finalPricePerUnit)} ₽/шт сопоставлен с AI-ориентиром ${formatMoney(aiMidPrice)} ₽/шт.`;
      }
      return `DeepSeek проверил ваш расчет ${formatMoney(finalPricePerUnit)} ₽/шт.`;
  }
};

const mapPanelError = (error) => {
  if (error?.message === "api_method_not_allowed") {
    return "API недоступно для записи. Обычно это значит, что proxy для /api ещё не применён.";
  }
  return error?.message || "Операция не выполнена.";
};

const ruleLabels = {
  labor_minute_rate: "Стоимость минуты",
  payroll_taxes_percent: "Начисления, %",
  overhead_percent: "Накладные, %",
  logistics_cost_per_unit: "Логистика / шт",
  margin_percent: "Маржа, %",
  min_margin_percent: "Мин. маржа, %",
  included_fittings: "Включено примерок",
  extra_fitting_minutes: "Минут на доп. примерку",
  custom_figure_coefficient: "Коэфф. нестанд. фигуры",
  child_coefficient: "Детский коэффициент",
  default_risk_percent: "Риск, %",
  default_consumables_per_unit: "Базовые расходники / шт",
};

const materialLabels = {
  coefficient: "Коэфф. ткани",
  fabric_cost_per_unit: "Ткань / шт",
  lining_cost_per_unit: "Подклад / шт",
  interfacing_cost_per_unit: "Дублерин / шт",
  thread_cost_per_unit: "Нитки / шт",
  hardware_cost_per_unit: "Фурнитура / шт",
  decor_cost_per_unit: "Декор / шт",
  packaging_cost_per_unit: "Упаковка / шт",
  consumables_cost_per_unit: "Расходники / шт",
  risk_percent: "Риск, %",
};

export default Panel;
