import { useEffect, useMemo, useState } from "react";
import { checkAuthStatus, fetchAuthProfile, logout } from "../utils/yandexAuth.js";
import {
  calculateInChat,
  createChat,
  getUserSettings,
  listChatCalculations,
  listChats,
  saveUserSettings,
} from "../utils/panelApi.js";

const defaultSettings = {
  base_prices: {
    "Пиджак": 5000,
    "Юбка": 2000,
    "Рубашка": 3000,
  },
  surcharge_percent: {
    "Карман": 20,
    "Трудная ткань": 20,
    "Молния": 20,
  },
  batch_discounts: [
    { min_qty: 1, max_qty: 10, percent: 0 },
    { min_qty: 11, max_qty: 50, percent: 5 },
    { min_qty: 51, max_qty: 100, percent: 10 },
  ],
};

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
  const [orderForm, setOrderForm] = useState({
    garment_type: "Пиджак",
    quantity: 15,
    complicationCounts: {
      "Карман": 2,
      "Трудная ткань": 1,
      "Молния": 0,
    },
  });
  const [calcNotice, setCalcNotice] = useState("");
  const [isSavingSettings, setIsSavingSettings] = useState(false);
  const [isCreatingChat, setIsCreatingChat] = useState(false);
  const [isCalculating, setIsCalculating] = useState(false);

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
        if (isActive) {
          setSettings(normalizeSettings(loaded));
        }
      } catch (error) {
        if (isActive && error?.status !== 404) {
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

  const garmentOptions = useMemo(
    () => Object.keys(settings.base_prices).sort((a, b) => a.localeCompare(b)),
    [settings.base_prices]
  );
  const complicationOptions = useMemo(
    () => Object.keys(settings.surcharge_percent).sort((a, b) => a.localeCompare(b)),
    [settings.surcharge_percent]
  );
  const activeChat = chats.find((chat) => chat.id === activeChatID) || null;

  const handleLogout = () => {
    logout()
      .catch(() => {})
      .finally(() => {
        window.location.replace("/");
      });
  };

  const handleSettingsChange = (group, key, value) => {
    setSettings((current) => ({
      ...current,
      [group]: {
        ...current[group],
        [key]: Number(value) || 0,
      },
    }));
  };

  const handleDiscountChange = (index, field, value) => {
    setSettings((current) => {
      const nextDiscounts = current.batch_discounts.map((item, itemIndex) => {
        if (itemIndex !== index) {
          return item;
        }
        return {
          ...item,
          [field]: Number(value) || 0,
        };
      });
      return {
        ...current,
        batch_discounts: nextDiscounts,
      };
    });
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
      setSettingsNotice(error.message || "Не удалось сохранить настройки.");
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
      setChatNotice(error.message || "Не удалось создать чат.");
    } finally {
      setIsCreatingChat(false);
    }
  };

  const handleComplicationCountChange = (name, value) => {
    setOrderForm((current) => ({
      ...current,
      complicationCounts: {
        ...current.complicationCounts,
        [name]: Math.max(0, Number(value) || 0),
      },
    }));
  };

  const handleCalculate = async (event) => {
    event.preventDefault();
    if (!userID || !activeChatID) {
      return;
    }

    const complications = [];
    for (const name of complicationOptions) {
      const repeats = orderForm.complicationCounts[name] || 0;
      for (let i = 0; i < repeats; i += 1) {
        complications.push(name);
      }
    }

    setIsCalculating(true);
    setCalcNotice("");
    try {
      const result = await calculateInChat(userID, activeChatID, {
        garment_type: orderForm.garment_type,
        quantity: Number(orderForm.quantity) || 0,
        complications,
      });
      setHistory((current) => [...current, result]);
      setChats((current) =>
        current.map((chat) =>
          chat.id === activeChatID
            ? {
                ...chat,
                updated_at: result.created_at,
                calculations_count: (chat.calculations_count || 0) + 1,
              }
            : chat
        )
      );
      setCalcNotice(`Расчёт выполнен. Итог: ${formatMoney(result.total)} ₽`);
    } catch (error) {
      setCalcNotice(error.message || "Не удалось выполнить расчёт.");
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
            Настройки цены
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

        {activeSection === "settings" ? (
          <section className="panel__card">
            <h2>Настройка ценообразования</h2>
            <form className="panel-form" onSubmit={handleSaveSettings}>
              <div className="panel-form__block">
                <h3>Базовые цены</h3>
                {Object.entries(settings.base_prices).map(([name, value]) => (
                  <label className="panel-form__row" key={name}>
                    <span>{name}</span>
                    <input
                      type="number"
                      min="0"
                      value={value}
                      onChange={(event) =>
                        handleSettingsChange("base_prices", name, event.target.value)
                      }
                    />
                  </label>
                ))}
              </div>

              <div className="panel-form__block">
                <h3>Усложнения, %</h3>
                {Object.entries(settings.surcharge_percent).map(([name, value]) => (
                  <label className="panel-form__row" key={name}>
                    <span>{name}</span>
                    <input
                      type="number"
                      min="0"
                      value={value}
                      onChange={(event) =>
                        handleSettingsChange("surcharge_percent", name, event.target.value)
                      }
                    />
                  </label>
                ))}
              </div>

              <div className="panel-form__block">
                <h3>Скидки по партиям</h3>
                {settings.batch_discounts.map((discount, index) => (
                  <div className="panel-form__grid" key={`${discount.min_qty}-${discount.max_qty}-${index}`}>
                    <label className="panel-form__row">
                      <span>От</span>
                      <input
                        type="number"
                        min="1"
                        value={discount.min_qty}
                        onChange={(event) =>
                          handleDiscountChange(index, "min_qty", event.target.value)
                        }
                      />
                    </label>
                    <label className="panel-form__row">
                      <span>До</span>
                      <input
                        type="number"
                        min="1"
                        value={discount.max_qty}
                        onChange={(event) =>
                          handleDiscountChange(index, "max_qty", event.target.value)
                        }
                      />
                    </label>
                    <label className="panel-form__row">
                      <span>Скидка, %</span>
                      <input
                        type="number"
                        min="0"
                        max="100"
                        value={discount.percent}
                        onChange={(event) =>
                          handleDiscountChange(index, "percent", event.target.value)
                        }
                      />
                    </label>
                  </div>
                ))}
              </div>

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
                <p>У каждого пользователя свои чаты и своя история расчётов.</p>
              </div>
              <div className="panel-chat-list__create">
                <input
                  type="text"
                  placeholder="Название чата"
                  value={chatTitleDraft}
                  onChange={(event) => setChatTitleDraft(event.target.value)}
                />
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
                    <button
                      className={`panel-chat-list__item ${chat.id === activeChatID ? "panel-chat-list__item--active" : ""}`}
                      key={chat.id}
                      type="button"
                      onClick={() => setActiveChatID(chat.id)}
                    >
                      <strong>{chat.title}</strong>
                      <span>{chat.calculations_count || 0} расчётов</span>
                    </button>
                  ))
                )}
              </div>
            </div>

            <div className="panel-workspace__main">
              <div className="panel__card">
                <h2>{activeChat ? activeChat.title : "Выберите чат"}</h2>
                {activeChat ? (
                  <form className="panel-form" onSubmit={handleCalculate}>
                    <label className="panel-form__row">
                      <span>Изделие</span>
                      <select
                        value={orderForm.garment_type}
                        onChange={(event) =>
                          setOrderForm((current) => ({
                            ...current,
                            garment_type: event.target.value,
                          }))
                        }
                      >
                        {garmentOptions.map((name) => (
                          <option key={name} value={name}>
                            {name}
                          </option>
                        ))}
                      </select>
                    </label>

                    <label className="panel-form__row">
                      <span>Размер партии</span>
                      <input
                        type="number"
                        min="1"
                        value={orderForm.quantity}
                        onChange={(event) =>
                          setOrderForm((current) => ({
                            ...current,
                            quantity: Number(event.target.value) || 0,
                          }))
                        }
                      />
                    </label>

                    <div className="panel-form__block">
                      <h3>Усложнения</h3>
                      {complicationOptions.map((name) => (
                        <label className="panel-form__row" key={name}>
                          <span>{name}</span>
                          <input
                            type="number"
                            min="0"
                            value={orderForm.complicationCounts[name] || 0}
                            onChange={(event) =>
                              handleComplicationCountChange(name, event.target.value)
                            }
                          />
                        </label>
                      ))}
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
                {historyStatus === "error" ? (
                  <p className="panel__notice">Не удалось загрузить историю расчётов.</p>
                ) : null}
                {historyStatus !== "loading" && history.length === 0 ? (
                  <p className="panel__empty">В этом чате пока нет расчётов.</p>
                ) : null}
                <div className="panel-history">
                  {history.map((item, index) => (
                    <article className="panel-history__item" key={`${item.created_at}-${index}`}>
                      <div className="panel-history__head">
                        <strong>{item.garment_type}</strong>
                        <span>{new Date(item.created_at).toLocaleString("ru-RU")}</span>
                      </div>
                      <div className="panel-history__stats">
                        <span>Партия: {item.quantity}</span>
                        <span>За единицу: {formatMoney(item.price_per_unit)} ₽</span>
                        <span>Итого: {formatMoney(item.total)} ₽</span>
                      </div>
                      <ul className="panel-history__list">
                        {item.applied_surcharges.map((surcharge) => (
                          <li key={`${item.created_at}-${surcharge.name}`}>
                            {surcharge.name} × {surcharge.repeats}: +{surcharge.percent}%
                          </li>
                        ))}
                        <li>
                          Скидка: {item.discount_percent}% ({formatMoney(item.discount_amount)} ₽)
                        </li>
                      </ul>
                    </article>
                  ))}
                </div>
              </div>
            </div>
          </section>
        )}
      </main>
    </div>
  );
};

const normalizeSettings = (settings) => ({
  base_prices: { ...defaultSettings.base_prices, ...(settings?.base_prices || {}) },
  surcharge_percent: {
    ...defaultSettings.surcharge_percent,
    ...(settings?.surcharge_percent || {}),
  },
  batch_discounts:
    settings?.batch_discounts?.length > 0
      ? settings.batch_discounts.map((item) => ({
          min_qty: Number(item.min_qty) || 0,
          max_qty: Number(item.max_qty) || 0,
          percent: Number(item.percent) || 0,
        }))
      : defaultSettings.batch_discounts,
});

const formatMoney = (value) => new Intl.NumberFormat("ru-RU").format(Number(value) || 0);

export default Panel;
