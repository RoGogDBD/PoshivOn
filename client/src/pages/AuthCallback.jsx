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

  const redirectAfterSuccess = () => {
    const target = resolveRedirectTarget();
    if (window.opener && !window.opener.closed) {
      window.opener.location.replace(target);
      window.close();
      return;
    }
    window.location.replace(target);
  };

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
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
