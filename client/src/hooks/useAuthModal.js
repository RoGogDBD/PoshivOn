import { useEffect } from "react";

const BUTTON_CONTAINER_ID = "yandex-id-button";
const SCRIPT_SRC =
  "https://yastatic.net/s3/passport-sdk/autofill/v1/sdk-suggest-with-polyfills-latest.js";
const SCRIPT_LOAD_TIMEOUT_MS = 6000;
let authScriptPromise;

const persistToken = async (data) => {
  if (!data?.access_token) {
    return;
  }

  const apiBase = import.meta.env.VITE_API_URL || "";
  await fetch(`${apiBase}/auth/yandex`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    credentials: "include",
    body: JSON.stringify(data),
  });
};

const initAuthSuggest = () => {
  if (!window.YaAuthSuggest?.init) {
    return;
  }

  const container = document.getElementById(BUTTON_CONTAINER_ID);
  if (!container) {
    return;
  }
  container.innerHTML = "";

  const clientId = import.meta.env.VITE_YA_CLIENT_ID;
  if (!clientId) {
    console.log("Не задан VITE_YA_CLIENT_ID для Яндекс ID.");
    return;
  }

  const redirectUri =
    import.meta.env.VITE_YA_REDIRECT_URI || `${window.location.origin}/auth`;

  const oauthQueryParams = {
    client_id: clientId,
    response_type: "code",
    redirect_uri: redirectUri,
  };
  let tokenPageOrigin = window.location.origin;
  try {
    tokenPageOrigin = new URL(redirectUri).origin;
  } catch (error) {
    console.log("Не удалось определить origin для страницы приема токена.", error);
  }

  window.YaAuthSuggest.init(oauthQueryParams, tokenPageOrigin, {
    view: "button",
    parentId: BUTTON_CONTAINER_ID,
    buttonSize: "xxl",
    buttonView: "main",
    buttonTheme: "light",
    buttonBorderRadius: "28",
    buttonIcon: "ya",
  })
    .then(({ handler }) => {
      const originalOpen = window.open;
      window.open = (url) => {
        if (url) {
          window.location.assign(url);
        }
        return window;
      };
      return handler().finally(() => {
        window.open = originalOpen;
      });
    })
    .then((data) => {
      console.log("Сообщение с токеном", data);
      return persistToken(data);
    })
    .catch((error) => {
      console.log("Обработка ошибки", error);
    });
};

const waitForSuggestReady = (script) =>
  new Promise((resolve) => {
    if (window.YaAuthSuggest?.init) {
      resolve(true);
      return;
    }

    const finalize = (ok) => resolve(ok);
    const loaded = script?.dataset?.yaLoaded === "true";
    const errored = script?.dataset?.yaError === "true";

    if (loaded) {
      finalize(!!window.YaAuthSuggest?.init);
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
          finalize(!!window.YaAuthSuggest?.init);
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

    window.setTimeout(() => finalize(!!window.YaAuthSuggest?.init), SCRIPT_LOAD_TIMEOUT_MS);
  });

const loadSuggestScript = async () => {
  if (window.YaAuthSuggest?.init) {
    return true;
  }

  const existingScript = document.querySelector(`script[src="${SCRIPT_SRC}"]`);
  const script =
    existingScript ||
    Object.assign(document.createElement("script"), {
      src: SCRIPT_SRC,
      async: true,
    });

  if (!existingScript) {
    document.body.appendChild(script);
  }

  return waitForSuggestReady(script);
};

const ensureAuthScript = () => {
  if (!authScriptPromise) {
    authScriptPromise = loadSuggestScript();
  }

  authScriptPromise
    .then((ready) => {
      if (ready) {
        initAuthSuggest();
        return;
      }
      console.log("Не удалось загрузить виджет Яндекс ID.");
    })
    .catch((error) => {
      console.log("Не удалось загрузить виджет Яндекс ID.", error);
    });
};

export const useAuthModal = (isOpen, onClose) => {
  useEffect(() => {
    if (isOpen) {
      document.body.classList.add("modal-open");
      return () => {
        document.body.classList.remove("modal-open");
      };
    }

    document.body.classList.remove("modal-open");
    return undefined;
  }, [isOpen]);

  useEffect(() => {
    if (!isOpen) {
      return undefined;
    }

    const handleKeyDown = (event) => {
      if (event.key === "Escape") {
        onClose();
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    ensureAuthScript();

    return () => {
      window.removeEventListener("keydown", handleKeyDown);
    };
  }, [isOpen, onClose]);
};
