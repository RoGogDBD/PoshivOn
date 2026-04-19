import { useEffect, useMemo, useState } from "react";
import { checkAuthStatus, fetchAuthProfile, logout } from "../utils/yandexAuth.js";
import {
  analyzeMarketWithAI,
  calculateInChat,
  createChat,
  deleteChat,
  getUserSettings,
  listChatCalculations,
  listChats,
  saveUserSettings,
} from "../utils/panelApi.js";

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

const defaultSettings = {
  pricing_rules: {
    calculator_mode: "masterpiece",
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

const Panel = () => {
  const [status, setStatus] = useState("checking");
  const [profile, setProfile] = useState(null);
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
      } catch {
        if (isActive) {
          window.location.replace("/");
        }
        return;
      }

      if (isActive) {
        setStatus("ready");
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

  useEffect(() => {
    if (!userID) {
      return;
    }

    let isActive = true;

    const loadSettings = async () => {
      try {
        const loaded = await getUserSettings(userID);
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
        const payload = await listChats(userID);
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
  }, [userID]);

  useEffect(() => {
    if (!userID || !activeChatID) {
      setHistory([]);
      return;
    }

    let isActive = true;
    setHistoryStatus("loading");

    listChatCalculations(userID, activeChatID)
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
  }, [userID, activeChatID]);

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
      await saveUserSettings(userID, settings);
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
      const chat = await createChat(userID, chatTitleDraft.trim());
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
      await deleteChat(userID, chatID);
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
      const result = await calculateInChat(userID, activeChatID, payload);
      let nextResult = result;
      let nextNotice = `Расчёт выполнен. Итог: ${formatMoney(result.total)} ₽`;

      if (normalizeCalculatorMode(result.calculation_mode) === "masterpiece") {
        try {
          const aiFeedback = await analyzeMarketWithAI(userID, buildAIPayloadFromCalculation(result));
          nextResult = { ...result, ai_feedback: aiFeedback };
          nextNotice = `${nextNotice}. Оценка DeepSeek добавлена.`;
        } catch (error) {
          nextNotice = `${nextNotice}. Оценку DeepSeek получить не удалось: ${mapPanelError(error)}`;
        }
      }

      setHistory((current) => [...current, nextResult]);
      setChats((current) =>
        current
          .map((chat) =>
            chat.id === activeChatID
              ? {
                  ...chat,
                  updated_at: nextResult.created_at,
                  calculations_count: (chat.calculations_count || 0) + 1,
                }
              : chat
          )
          .sort((a, b) => new Date(b.updated_at) - new Date(a.updated_at))
      );
      setCalcNotice(nextNotice);
    } catch (error) {
      setCalcNotice(mapPanelError(error));
    } finally {
      setIsCalculating(false);
    }
  };

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
        <div className="panel__brand">PoshivOn</div>
        <nav className="panel__nav">
          <button
            className={`panel__link ${activeSection === "workspace" ? "panel__link--active" : ""}`}
            type="button"
            onClick={() => setActiveSection("workspace")}
          >
            Чаты и расчёты
          </button>
          <button
            className={`panel__link ${activeSection === "settings" ? "panel__link--active" : ""}`}
            type="button"
            onClick={() => setActiveSection("settings")}
          >
            Настройки модели
          </button>
        </nav>
      </aside>
      <main className="panel__content">
        <div className="panel__header">
          <div>
            <p className="panel__eyebrow">Рабочая панель</p>
            <h1>{profile?.name ? `Здравствуйте, ${profile.name}` : "Здравствуйте"}</h1>
            <p className="panel__meta">Пользователь: {userID}</p>
          </div>
          <div className="panel__actions">
            <button
              className="panel__theme-toggle"
              type="button"
              onClick={() => setTheme(theme === "light" ? "dark" : "light")}
            >
              {theme === "light" ? "Темная" : "Светлая"}
            </button>
            <button className="panel__logout" type="button" onClick={handleLogout}>
              Выйти
            </button>
          </div>
        </div>

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

        {activeSection === "settings" ? (
          <section className="panel-settings rounded-[32px] border p-5 shadow-[0_28px_80px_var(--settings-shell-shadow)] backdrop-blur-xl motion-safe:animate-fade-rise [background:var(--settings-shell-bg)] [border-color:var(--settings-shell-border)] sm:p-7">
            <div className="mb-6 flex flex-col gap-5 lg:flex-row lg:items-start lg:justify-between">
              <div className="max-w-3xl">
                <span className="inline-flex rounded-full border px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.22em] text-[color:var(--settings-muted)] [background:var(--settings-accent-soft)] [border-color:var(--settings-card-border)]">
                  Настройка модели
                </span>
                <h2 className="mt-4 text-[30px] font-semibold tracking-[-0.04em] text-[color:var(--settings-text)] sm:text-[38px]">
                  Модель расчёта
                </h2>
                <p className="mt-3 max-w-2xl text-sm leading-6 text-[color:var(--settings-muted)] sm:text-[15px]">
                  Спокойная рабочая панель без жесткого белого фона: параметры сгруппированы по смыслу, а все поля читаются на мягких матовых поверхностях.
                </p>
              </div>
              <div className={`${settingsInsetClass} grid min-w-[220px] gap-2 self-start lg:max-w-[260px]`}>
                <span className="text-xs font-semibold uppercase tracking-[0.18em] text-[color:var(--settings-subtle)]">Активный режим</span>
                <strong className="text-xl font-semibold tracking-[-0.03em] text-[color:var(--settings-text)]">
                  {calculatorModes.find((mode) => mode.value === calculatorMode)?.label || "Шедевр"}
                </strong>
                <p className="text-sm leading-6 text-[color:var(--settings-muted)]">
                  {isQuickCalculator
                    ? "Быстрый тарифный режим для чернового просчёта без детальной себестоимости."
                    : "Полная модель с минутами, материалами, срочностью, скидками и рыночными диапазонами."}
                </p>
              </div>
            </div>

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
                            <strong className="text-base font-semibold tracking-[-0.02em] text-[color:var(--settings-text)]">{name}</strong>
                            <p className="mt-1 text-sm text-[color:var(--settings-muted)]">Фиксированная минимальная цена на единицу изделия.</p>
                          </div>
                          <SettingsField label="Мин. цена / шт">
                            <SettingsNumberInput min="0" value={item.quick_price} onChange={(event) => handleGarmentChange(name, "quick_price", event.target.value)} />
                          </SettingsField>
                        </div>
                      ))}
                    </div>
                  </SettingsSection>

                  <SettingsSection title="Усложнения" description="Процентные надбавки, которые добавляются к базовой цене в быстром режиме.">
                    <div className="grid gap-4">
                      {Object.entries(settings.operations).map(([name, item]) => (
                        <div
                          className="grid gap-4 rounded-[24px] border p-4 [background:color-mix(in_oklab,var(--settings-card-bg)_90%,transparent)] [border-color:var(--settings-card-border)] md:grid-cols-[minmax(0,1fr)_minmax(220px,280px)] md:items-end"
                          key={name}
                        >
                          <div>
                            <strong className="text-base font-semibold tracking-[-0.02em] text-[color:var(--settings-text)]">{name}</strong>
                            <p className="mt-1 text-sm text-[color:var(--settings-muted)]">Добавка к цене за дополнительную сложность.</p>
                          </div>
                          <SettingsField label="Надбавка, %">
                            <SettingsNumberInput step="0.01" min="0" value={item.quick_percent} onChange={(event) => handleOperationSettingChange(name, "quick_percent", event.target.value)} />
                          </SettingsField>
                        </div>
                      ))}
                    </div>
                  </SettingsSection>

                  <DiscountsBlock settings={settings} handleDiscountChange={handleDiscountChange} />
                </>
              ) : (
                <>
                  <SettingsSection title="Общие правила" description="Базовые коэффициенты и ставки, влияющие на расчёт себестоимости и цены.">
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
                            <strong className="text-base font-semibold tracking-[-0.02em] text-[color:var(--settings-text)]">{name}</strong>
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
                  </SettingsSection>

                  <SettingsSection title="Операции" description="Дополнительные минуты и материалы, увеличивающие стоимость единицы.">
                    <div className="grid gap-4">
                      {Object.entries(settings.operations).map(([name, item]) => (
                        <div
                          className="grid gap-4 rounded-[24px] border p-4 [background:color-mix(in_oklab,var(--settings-card-bg)_90%,transparent)] [border-color:var(--settings-card-border)] xl:grid-cols-[minmax(0,1fr)_minmax(180px,220px)_minmax(180px,220px)] xl:items-end"
                          key={name}
                        >
                          <div>
                            <strong className="text-base font-semibold tracking-[-0.02em] text-[color:var(--settings-text)]">{name}</strong>
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

                  <DiscountsBlock settings={settings} handleDiscountChange={handleDiscountChange} />

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
        ) : (
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

                    <label className="panel-form__row">
                      <span>Комментарий</span>
                      <textarea value={orderForm.comment} onChange={(event) => handleOrderChange("comment", event.target.value)} rows="3" />
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
                            <CalculationAIFeedback calculation={item} feedback={item.ai_feedback} />
                          </>
                        )}
                      </article>
                    );
                  })}
                </div>
              </div>
            </div>
          </section>
        )}
      </main>
    </div>
  );
};

const DiscountsBlock = ({ settings, handleDiscountChange }) => (
  <SettingsSection
    title="Скидки по партиям"
    description="Диапазоны количества и процент скидки для автоматического уменьшения цены на крупные заказы."
  >
    <div className="grid gap-4">
      {settings.batch_discounts.map((discount, index) => (
        <div
          className="grid gap-4 rounded-[24px] border p-4 [background:color-mix(in_oklab,var(--settings-card-bg)_90%,transparent)] [border-color:var(--settings-card-border)] md:grid-cols-3"
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
        </div>
      ))}
    </div>
  </SettingsSection>
);

const CalculationAIFeedback = ({ calculation, feedback }) => {
  if (!feedback || !calculation) {
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

const buildAIPayloadFromCalculation = (item) => ({
  garment_type: item.garment_type,
  material_type: item.material_type,
  market_segment: item.market_segment,
  urgency: item.urgency,
  quantity: Number(item.quantity) || 0,
  fittings: Number(item.fittings) || 0,
  is_custom_figure: Boolean(item.is_custom_figure),
  is_child: Boolean(item.is_child),
  comment: item.comment || "",
  operation_counts: Object.fromEntries(
    (item.applied_operations || [])
      .filter((operation) => Number(operation.count) > 0)
      .map((operation) => [operation.name, Number(operation.count) || 0])
  ),
  calculation: {
    calculation_mode: item.calculation_mode,
    base_price_per_unit_rub: Number(item.min_allowed_price_per_unit) || 0,
    cost_price_per_unit_rub: Number(item.cost_price_per_unit) || 0,
    price_before_discount_rub: Number(item.price_before_discount_per_unit) || 0,
    min_allowed_price_rub: Number(item.min_allowed_price_per_unit) || 0,
    final_price_per_unit_rub: Number(item.price_per_unit) || 0,
    final_total_rub: Number(item.total) || 0,
    discount_percent: Number(item.discount_percent) || 0,
    discount_amount_rub: Number(item.discount_amount) || 0,
    market_status: item.market_status || "",
  },
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
