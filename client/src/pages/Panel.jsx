import { useEffect, useState } from "react";
import { checkAuthStatus, fetchAuthProfile, logout } from "../utils/yandexAuth.js";

const Panel = () => {
  const [status, setStatus] = useState("checking");
  const [profile, setProfile] = useState(null);
  const [activeSection, setActiveSection] = useState("history");
  const [theme, setTheme] = useState(() => localStorage.getItem("panelTheme") || "light");

  useEffect(() => {
    let isActive = true;
    checkAuthStatus()
      .then((ok) => {
        if (!isActive) {
          return;
        }
        if (!ok) {
          window.location.replace("/");
          return;
        }
        return fetchAuthProfile()
          .then((data) => {
            if (isActive) {
              setProfile(data);
            }
          })
          .catch(() => {});
      })
      .then(() => {
        if (isActive) {
          setStatus("ready");
        }
      })
      .catch(() => {
        if (isActive) {
          window.location.replace("/");
        }
      });
    return () => {
      isActive = false;
    };
  }, []);

  useEffect(() => {
    localStorage.setItem("panelTheme", theme);
  }, [theme]);

  const handleLogout = () => {
    logout()
      .catch(() => {})
      .finally(() => {
        window.location.replace("/");
      });
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
            className={`panel__link ${activeSection === "history" ? "panel__link--active" : ""}`}
            type="button"
            onClick={() => setActiveSection("history")}
          >
            История
          </button>
          <button
            className={`panel__link ${activeSection === "settings" ? "panel__link--active" : ""}`}
            type="button"
            onClick={() => setActiveSection("settings")}
          >
            Настройка
          </button>
        </nav>
      </aside>
      <main className="panel__content">
        <div className="panel__header">
          <div>
            <p className="panel__eyebrow">Добро пожаловать</p>
            <h1>{profile?.name ? `Здравствуйте, ${profile.name}` : "Здравствуйте"}</h1>
          </div>
          <button className="panel__logout" type="button" onClick={handleLogout}>
            Выйти
          </button>
        </div>
        {activeSection === "history" ? (
          <div className="panel__card">
            <h2>История</h2>
            <p>Здесь появятся события и активность по вашему аккаунту.</p>
          </div>
        ) : (
          <div className="panel__card">
            <h2>Настройки</h2>
            <div className="panel__setting">
              <div>
                <p className="panel__setting-title">Тема панели</p>
                <p className="panel__setting-text">Меняйте оформление панели под себя.</p>
              </div>
              <button
                className="panel__theme-toggle"
                type="button"
                onClick={() => setTheme(theme === "light" ? "dark" : "light")}
              >
                {theme === "light" ? "Темная" : "Светлая"}
              </button>
            </div>
          </div>
        )}
      </main>
    </div>
  );
};

export default Panel;
