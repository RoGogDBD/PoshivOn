import { useEffect, useState } from "react";

const TOKEN_SCRIPT_SRC =
  "https://yastatic.net/s3/passport-sdk/autofill/v1/sdk-suggest-token-with-polyfills-latest.js";
const TOKEN_SCRIPT_LOAD_TIMEOUT_MS = 6000;
let tokenScriptPromise;

const AuthCallback = () => {
  const [status, setStatus] = useState("pending");

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

      const existingScript = document.querySelector(`script[src="${TOKEN_SCRIPT_SRC}"]`);
      const script =
        existingScript ||
        Object.assign(document.createElement("script"), {
          src: TOKEN_SCRIPT_SRC,
          async: true,
        });

      if (!existingScript) {
        document.body.appendChild(script);
      }

      return waitForTokenReady(script);
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

        window.YaSendSuggestToken(window.location.origin, { flag: true });
        setStatus("success");
      })
      .catch((error) => {
        console.log("Не удалось загрузить скрипт передачи токена.", error);
        setStatus("error");
      });
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
