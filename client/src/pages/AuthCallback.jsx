import { useEffect, useState } from "react";

const AuthCallback = () => {
  const [status, setStatus] = useState("pending");

  const resolveRedirectTarget = () => {
    const params = new URLSearchParams(window.location.search);
    const explicit =
      params.get("returnTo") || params.get("next") || params.get("redirect");
    if (explicit) {
      return explicit;
    }
    return import.meta.env.VITE_AUTH_SUCCESS_REDIRECT || "/";
  };

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const code = params.get("code");
    const apiBase = import.meta.env.VITE_API_URL || "";
    const redirectUri =
      import.meta.env.VITE_YA_REDIRECT_URI || `${window.location.origin}/auth`;

    if (code) {
      fetch(`${apiBase}/auth/yandex/code`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        credentials: "include",
        body: JSON.stringify({ code, redirect_uri: redirectUri }),
      })
        .then((response) => {
          if (!response.ok) {
            throw new Error("exchange_failed");
          }
          setStatus("success");
          const target = resolveRedirectTarget();
          window.setTimeout(() => {
            window.location.replace(target);
          }, 300);
        })
        .catch((error) => {
          console.log("Не удалось обменять код.", error);
          setStatus("error");
        });
      return;
    }

    setStatus("error");
  }, []);

  const content = (() => {
    if (status === "pending") {
      return "Проверяем авторизацию...";
    }
    if (status === "success") {
      return "Токен передан. Можно закрыть эту страницу.";
    }
    if (status === "error") {
      return "Ошибка передачи токена. Закройте окно и попробуйте снова.";
    }
    return "Токен не найден. Закройте окно и попробуйте снова.";
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
