import { useEffect, useState } from "react";
import { checkAuthStatus } from "../utils/yandexAuth.js";

const Panel = () => {
  const [status, setStatus] = useState("checking");

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
        setStatus("ready");
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

  if (status !== "ready") {
    return (
      <div className="page panel">
        <main className="panel__content">
          <p>Проверяем доступ...</p>
        </main>
      </div>
    );
  }

  return (
    <div className="page panel">
      <aside className="panel__sidebar">
        <div className="panel__brand">PoshivOn</div>
        <nav className="panel__nav">
          <button className="panel__link panel__link--active" type="button">
            История
          </button>
          <button className="panel__link" type="button">
            Настройки
          </button>
        </nav>
      </aside>
      <main className="panel__content">
        <h1>Панель</h1>
        <p>Добро пожаловать! Здесь появится история действий и настройки аккаунта.</p>
      </main>
    </div>
  );
};

export default Panel;
