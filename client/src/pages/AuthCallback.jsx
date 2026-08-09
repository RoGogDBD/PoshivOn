import { useEffect, useRef, useState } from "react";
import {
  consumeAuthState,
  exchangeYandexCode,
  getAuthRedirectTarget,
  getRedirectUri,
} from "../utils/yandexAuth.js";

const AuthCallback = () => {
  const [status, setStatus] = useState("pending");
  // consumeAuthState() снимает state из sessionStorage при первом вызове — одноразово и
  // намеренно (см. комментарий ниже). React StrictMode в dev дважды подряд монтирует этот
  // эффект на одном и том же экземпляре компонента, и без guard'а второй проход находит
  // state уже снятым первым и считает это несовпадением, показывая ложную ошибку до
  // редиректа. hasRun переживает двойной вызов StrictMode (тот же экземпляр, тот же ref) и
  // гарантирует, что тело эффекта отработает ровно один раз за настоящее монтирование.
  const hasRun = useRef(false);

  useEffect(() => {
    if (hasRun.current) {
      return undefined;
    }
    hasRun.current = true;

    let isActive = true;

    const finalize = (nextStatus) => {
      if (isActive) {
        setStatus(nextStatus);
      }
    };

    const run = async () => {
      const searchParams = new URLSearchParams(window.location.search);
      const hashParams = new URLSearchParams(window.location.hash.replace(/^#/, ""));

      const error = searchParams.get("error") || hashParams.get("error");
      if (error) {
        finalize("error");
        return;
      }

      // Ветки приёма токена из хэша (#access_token=...) больше нет (Decision 7): штатный
      // вход её не создаёт — buildYandexAuthUrl запрашивает только response_type=code, —
      // поэтому такой адрес мог появиться лишь при ручной сборке атакующим. Теперь он
      // уходит в тот же finalize("error"), что и любой колбэк без code.
      try {
        const code = searchParams.get("code");
        if (code) {
          // Сверка state связывает ответ Яндекса с браузером, который начал вход
          // (Decision 9). Значение одноразовое и снимается из sessionStorage до сверки —
          // независимо от её результата. Отсутствие сохранённого state (например, при
          // прямом открытии /auth?code=...) считается несовпадением.
          const expectedState = consumeAuthState();
          const returnedState = searchParams.get("state");
          if (!expectedState || !returnedState || returnedState !== expectedState) {
            finalize("error");
            return;
          }

          await exchangeYandexCode(code, getRedirectUri());
          finalize("success");
          window.location.replace(getAuthRedirectTarget());
          return;
        }

        finalize("error");
      } catch (authError) {
        console.log("Не удалось завершить авторизацию.", authError);
        finalize("error");
      }
    };

    run();
    return () => {
      isActive = false;
    };
  }, []);

  const content = (() => {
    if (status === "pending") {
      return "Проверяем авторизацию...";
    }
    if (status === "success") {
      return "Авторизация завершена. Возвращаемся назад...";
    }
    if (status === "error") {
      return "Ошибка авторизации. Попробуйте снова.";
    }
    return "Токен не найден. Попробуйте снова.";
  })();

  return (
    <div className="page">
      <main className="section">
        <div className="container">
          <h1>Авторизация</h1>
          <p>{content}</p>
        </div>
      </main>
    </div>
  );
};

export default AuthCallback;
