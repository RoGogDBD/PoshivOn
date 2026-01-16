import { useEffect, useMemo, useState } from "react";

const parseAuthParams = () => {
  const hashParams = new URLSearchParams(window.location.hash.replace(/^#/, ""));
  const queryParams = new URLSearchParams(window.location.search);

  const params = {
    accessToken: hashParams.get("access_token") || queryParams.get("access_token"),
    tokenType: hashParams.get("token_type") || queryParams.get("token_type"),
    expiresIn: hashParams.get("expires_in") || queryParams.get("expires_in"),
    error: hashParams.get("error") || queryParams.get("error"),
    errorDescription:
      hashParams.get("error_description") || queryParams.get("error_description"),
  };

  return params;
};

const AuthCallback = () => {
  const [status, setStatus] = useState("pending");

  const params = useMemo(() => parseAuthParams(), []);

  useEffect(() => {
    if (params.error) {
      setStatus("error");
      return;
    }

    if (params.accessToken) {
      localStorage.setItem("ya_access_token", params.accessToken);
      if (params.expiresIn) {
        localStorage.setItem("ya_token_expires_in", params.expiresIn);
      }
      if (params.tokenType) {
        localStorage.setItem("ya_token_type", params.tokenType);
      }
      setStatus("success");
      return;
    }

    setStatus("empty");
  }, [params]);

  const content = (() => {
    if (status === "pending") {
      return "Проверяем авторизацию...";
    }
    if (status === "success") {
      return "Авторизация успешна. Токен сохранен.";
    }
    if (status === "error") {
      return `Ошибка авторизации: ${params.errorDescription || params.error}`;
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
