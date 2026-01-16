import { useEffect, useMemo, useState } from "react";

const TOKEN_SCRIPT_SRCS = [
  "https://yastatic.net/s3/passport-sdk/autofill/v1/sdk-suggest-token-with-polyfills-latest.js",
  "https://yastatic.net/s3/passport-sdk/autofill/v1/sdk-suggest-token-latest.js",
];
const TOKEN_SCRIPT_LOAD_TIMEOUT_MS = 6000;
let tokenScriptPromise;

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
    const waitForTokenReady = (script) =>
      new Promise((resolve) => {
        if (window.YaSendSuggestToken) {
          resolve(true);
          return;
        }

        const finalize = (ok) => resolve(ok);
        const loaded = script?.dataset?.yaLoaded === "true";
        const errored = script?.dataset?.yaError === "true";

        if (loaded) {
          finalize(!!window.YaSendSuggestToken);
          return;
        }

        if (errored) {
          finalize(false);
          return;
        }

        if (script) {
          script.addEventListener(
            "load",
            () => {
              script.dataset.yaLoaded = "true";
              finalize(!!window.YaSendSuggestToken);
            },
            { once: true }
          );
          script.addEventListener(
            "error",
            () => {
              script.dataset.yaError = "true";
              finalize(false);
            },
            { once: true }
          );
        }

        window.setTimeout(
          () => finalize(!!window.YaSendSuggestToken),
          TOKEN_SCRIPT_LOAD_TIMEOUT_MS
        );
      });

    const loadTokenScript = async () => {
      if (window.YaSendSuggestToken) {
        return true;
      }

      for (const src of TOKEN_SCRIPT_SRCS) {
        const existingScript = document.querySelector(`script[src="${src}"]`);
        const script =
          existingScript ||
          Object.assign(document.createElement("script"), {
            src,
            async: true,
          });

        if (!existingScript) {
          document.body.appendChild(script);
        }

        const ready = await waitForTokenReady(script);
        if (ready) {
          return true;
        }
      }

      return false;
    };

    if (!tokenScriptPromise) {
      tokenScriptPromise = loadTokenScript();
    }

    tokenScriptPromise
      .then((ready) => {
        if (!ready) {
          console.log("Не удалось загрузить скрипт передачи токена.");
          return;
        }

        window.YaSendSuggestToken(window.location.origin, {
          onSuccess: () => {
            setStatus((prev) => (prev === "pending" ? "success" : prev));
          },
          onError: () => {
            setStatus((prev) => (prev === "pending" ? "error" : prev));
          },
        });
      })
      .catch((error) => {
        console.log("Не удалось загрузить скрипт передачи токена.", error);
      });

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
