import { useEffect, useState } from "react";

// Версия текста уведомления — при существенном изменении политики увеличить, чтобы
// пользователи, уже нажавшие «Принимаю» на прошлой версии, увидели баннер заново.
const CONSENT_VERSION = "1";
const STORAGE_KEY = "poshivon.cookieConsent";

const hasStoredConsent = () => {
  try {
    return localStorage.getItem(STORAGE_KEY) === CONSENT_VERSION;
  } catch {
    // localStorage недоступен (приватный режим, отключённые куки в самом браузере) —
    // тогда баннер просто показывается на каждый визит, это безопасная деградация.
    return false;
  }
};

// CookieConsent — уведомление при первом входе на сайт: какие данные собираются и зачем
// (ст. 6, 9 152-ФЗ «О персональных данных» — согласие на обработку персональных данных,
// которые сервис получает через OAuth Яндекса, и информирование об использовании cookie).
// Не блокирует работу сайта и не выключает служебные cookie сессии до согласия: они
// строго необходимы для входа, которого сам пользователь и добивается, и не являются
// трекинговыми/рекламными — сервис их не устанавливает и не использует.
const CookieConsent = () => {
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    if (!hasStoredConsent()) {
      setVisible(true);
    }
  }, []);

  const accept = () => {
    try {
      localStorage.setItem(STORAGE_KEY, CONSENT_VERSION);
    } catch {
      // Не удалось сохранить — баннер просто покажется снова при следующем визите,
      // это не повод блокировать закрытие сейчас.
    }
    setVisible(false);
  };

  if (!visible) {
    return null;
  }

  return (
    <div className="cookie-consent" role="region" aria-label="Уведомление об использовании cookie">
      <p className="cookie-consent__text">
        Сайт использует cookie, необходимые для входа и работы личного кабинета, и
        обрабатывает данные, полученные при авторизации через Яндекс (логин, имя,
        email) — в соответствии с законодательством РФ о персональных данных.
        Продолжая пользоваться сайтом, вы соглашаетесь с этим.{" "}
        <a href="/privacy">Подробнее в политике конфиденциальности</a>.
      </p>
      <button type="button" className="btn btn-primary cookie-consent__accept" onClick={accept}>
        Принимаю
      </button>
    </div>
  );
};

export default CookieConsent;
