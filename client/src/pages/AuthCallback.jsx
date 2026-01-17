import { useEffect, useState } from "react";
import {
  exchangeYandexCode,
  getAuthRedirectTarget,
  getRedirectUri,
  persistYandexToken,
} from "../utils/yandexAuth.js";

const AuthCallback = () => {
  const [status, setStatus] = useState("pending");

  useEffect(() => {
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

      const accessToken = hashParams.get("access_token");
      const refreshToken = hashParams.get("refresh_token");
      const expiresIn = hashParams.get("expires_in");

      try {
        if (accessToken) {
          await persistYandexToken({
            access_token: accessToken,
            refresh_token: refreshToken || undefined,
            expires_in: expiresIn ? Number(expiresIn) : undefined,
          });
          finalize("success");
          window.location.replace(getAuthRedirectTarget());
          return;
        }

        const code = searchParams.get("code");
        if (code) {
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
