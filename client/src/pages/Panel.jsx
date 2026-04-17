import { useEffect, useMemo, useState } from "react";
import { checkAuthStatus, fetchAuthProfile, logout } from "../utils/yandexAuth.js";
import {
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
          <article className="panel-summary__card">
            <span className="panel-summary__label">Активный чат</span>
            <strong>{activeChat?.title || "Не выбран"}</strong>
            <p>{activeChat ? `${activeChat.calculations_count || 0} расчётов сохранено` : "Создайте чат и начните расчёт."}</p>
          </article>
          <article className="panel-summary__card">
            <span className="panel-summary__label">Всего чатов</span>
            <strong>{chats.length}</strong>
            <p>Чаты изолированы по пользователю и имеют собственную историю расчётов.</p>
          </article>
          <article className="panel-summary__card">
            <span className="panel-summary__label">Сумма по чату</span>
            <strong>{formatMoney(totalHistoryAmount)} ₽</strong>
            <p>Сумма сохранённых расчётов в выбранном чате.</p>
          </article>
        </section>

        {activeSection === "settings" ? (
          <section className="panel__card panel__card--settings">
            <h2>Модель расчёта</h2>
            <form className="panel-form" onSubmit={handleSaveSettings}>
              <div className="panel-form__block">
                <h3>Режим</h3>
                <div className="panel-mode-switch">
                  {calculatorModes.map((mode) => (
                    <button
                      key={mode.value}
                      className={`panel-mode-switch__item ${calculatorMode === mode.value ? "panel-mode-switch__item--active" : ""}`}
                      type="button"
                      onClick={() => handleCalculatorModeChange(mode.value)}
                    >
                      <strong>{mode.label}</strong>
                      <span>{mode.description}</span>
                    </button>
                  ))}
                </div>
              </div>

              {isQuickCalculator ? (
                <>
                  <div className="panel-form__block">
                    <h3>Изделия</h3>
                    <div className="panel-settings-table">
                      {Object.entries(settings.garments).map(([name, item]) => (
                        <div className="panel-settings-table__row panel-settings-table__row--compact" key={name}>
                          <strong>{name}</strong>
                          <label className="panel-form__row">
                            <span>Мин. цена / шт</span>
                            <input type="number" min="0" value={item.quick_price} onChange={(event) => handleGarmentChange(name, "quick_price", event.target.value)} />
                          </label>
                        </div>
                      ))}
                    </div>
                  </div>

                  <div className="panel-form__block">
                    <h3>Усложнения</h3>
                    <div className="panel-settings-table">
                      {Object.entries(settings.operations).map(([name, item]) => (
                        <div className="panel-settings-table__row panel-settings-table__row--compact" key={name}>
                          <strong>{name}</strong>
                          <label className="panel-form__row">
                            <span>Надбавка, %</span>
                            <input type="number" step="0.01" min="0" value={item.quick_percent} onChange={(event) => handleOperationSettingChange(name, "quick_percent", event.target.value)} />
                          </label>
                        </div>
                      ))}
                    </div>
                  </div>

                  <DiscountsBlock settings={settings} handleDiscountChange={handleDiscountChange} />
                </>
              ) : (
                <>
                  <div className="panel-form__block">
                    <h3>Общие правила</h3>
                    <div className="panel-form__grid panel-form__grid--compact">
                      {Object.entries(settings.pricing_rules)
                        .filter(([key]) => key !== "calculator_mode")
                        .map(([key, value]) => (
                          <label className="panel-form__row" key={key}>
                            <span>{ruleLabels[key] || key}</span>
                            <input type="number" step="0.01" min="0" value={value} onChange={(event) => handleRuleChange(key, event.target.value)} />
                          </label>
                        ))}
                    </div>
                  </div>

                  <div className="panel-form__block">
                    <h3>Изделия</h3>
                    <div className="panel-settings-table">
                      {Object.entries(settings.garments).map(([name, item]) => (
                        <div className="panel-settings-table__row" key={name}>
                          <strong>{name}</strong>
                          <label className="panel-form__row">
                            <span>База, мин</span>
                            <input type="number" min="0" value={item.base_minutes} onChange={(event) => handleGarmentChange(name, "base_minutes", event.target.value)} />
                          </label>
                          <label className="panel-form__row">
                            <span>Коэфф.</span>
                            <input type="number" step="0.01" min="0" value={item.complexity_coeff} onChange={(event) => handleGarmentChange(name, "complexity_coeff", event.target.value)} />
                          </label>
                        </div>
                      ))}
                    </div>
                  </div>

                  <div className="panel-form__block">
                    <h3>Операции</h3>
                    <div className="panel-settings-table">
                      {Object.entries(settings.operations).map(([name, item]) => (
                        <div className="panel-settings-table__row" key={name}>
                          <strong>{name}</strong>
                          <label className="panel-form__row">
                            <span>Минуты</span>
                            <input type="number" min="0" value={item.additional_minutes} onChange={(event) => handleOperationSettingChange(name, "additional_minutes", event.target.value)} />
                          </label>
                          <label className="panel-form__row">
                            <span>Материалы/шт</span>
                            <input type="number" min="0" value={item.additional_material_per_unit} onChange={(event) => handleOperationSettingChange(name, "additional_material_per_unit", event.target.value)} />
                          </label>
                        </div>
                      ))}
                    </div>
                  </div>

                  <div className="panel-form__block">
                    <h3>Материалы</h3>
                    <div className="panel-settings-stack">
                      {Object.entries(settings.materials).map(([name, item]) => (
                        <div className="panel-settings-card" key={name}>
                          <div className="panel-settings-card__header">
                            <strong>{name}</strong>
                          </div>
                          <div className="panel-form__grid panel-form__grid--compact">
                            {Object.entries(item).map(([key, value]) => (
                              <label className="panel-form__row" key={key}>
                                <span>{materialLabels[key] || key}</span>
                                <input type="number" step="0.01" min="0" value={value} onChange={(event) => handleMaterialChange(name, key, event.target.value)} />
                              </label>
                            ))}
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>

                  <DiscountsBlock settings={settings} handleDiscountChange={handleDiscountChange} />

                  <div className="panel-form__block">
                    <h3>Срочность</h3>
                    <div className="panel-form__grid panel-form__grid--compact">
                      {Object.entries(settings.urgency).map(([name, item]) => (
                        <label className="panel-form__row" key={name}>
                          <span>{name}</span>
                          <input type="number" step="0.01" min="0" value={item.percent} onChange={(event) => handleUrgencyChange(name, event.target.value)} />
                        </label>
                      ))}
                    </div>
                  </div>

                  <div className="panel-form__block">
                    <h3>Рынок</h3>
                    <div className="panel-settings-stack">
                      {Object.entries(settings.market_bands).map(([name, item]) => (
                        <div className="panel-settings-card" key={name}>
                          <div className="panel-settings-card__header">
                            <strong>{name}</strong>
                          </div>
                          <div className="panel-form__grid panel-form__grid--compact">
                            <label className="panel-form__row">
                              <span>Мин</span>
                              <input type="number" min="0" value={item.min_price_per_unit} onChange={(event) => handleMarketBandChange(name, "min_price_per_unit", event.target.value)} />
                            </label>
                            <label className="panel-form__row">
                              <span>Средняя</span>
                              <input type="number" min="0" value={item.average_price_per_unit} onChange={(event) => handleMarketBandChange(name, "average_price_per_unit", event.target.value)} />
                            </label>
                            <label className="panel-form__row">
                              <span>Макс</span>
                              <input type="number" min="0" value={item.max_price_per_unit} onChange={(event) => handleMarketBandChange(name, "max_price_per_unit", event.target.value)} />
                            </label>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                </>
              )}

              <div className="panel-form__footer">
                <button className="panel__theme-toggle" type="submit" disabled={isSavingSettings}>
                  {isSavingSettings ? "Сохраняем..." : "Сохранить"}
                </button>
                {settingsNotice ? <p className="panel__notice">{settingsNotice}</p> : null}
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
                      <button className="panel__theme-toggle" type="submit" disabled={isCalculating}>
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
                      <article className="panel-history__item" key={`${item.created_at}-${index}`}>
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
  <div className="panel-form__block">
    <h3>Скидки по партиям</h3>
    {settings.batch_discounts.map((discount, index) => (
      <div className="panel-form__grid panel-form__grid--compact" key={`${discount.min_qty}-${discount.max_qty}-${index}`}>
        <label className="panel-form__row">
          <span>От</span>
          <input type="number" min="1" value={discount.min_qty} onChange={(event) => handleDiscountChange(index, "min_qty", event.target.value)} />
        </label>
        <label className="panel-form__row">
          <span>До</span>
          <input type="number" min="1" value={discount.max_qty} onChange={(event) => handleDiscountChange(index, "max_qty", event.target.value)} />
        </label>
        <label className="panel-form__row">
          <span>Скидка, %</span>
          <input type="number" step="0.01" min="0" max="100" value={discount.percent} onChange={(event) => handleDiscountChange(index, "percent", event.target.value)} />
        </label>
      </div>
    ))}
  </div>
);

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
