import { useEffect } from "react";

const BUTTON_CONTAINER_ID = "yandex-id-button";
const SCRIPT_SRC =
  "https://yastatic.net/s3/passport-sdk/autofill/v1/sdk-suggest-with-polyfills-latest.js";

const persistToken = (data) => {
  if (!data?.access_token) {
    return;
  }

  localStorage.setItem("ya_access_token", data.access_token);
  if (data.expires_in) {
    localStorage.setItem("ya_token_expires_in", String(data.expires_in));
  }
  if (data.token_type) {
    localStorage.setItem("ya_token_type", data.token_type);
  }
};

const initAuthSuggest = () => {
  if (!window.YaAuthSuggest?.init) {
    return;
  }

  const clientId = import.meta.env.VITE_YA_CLIENT_ID;
  if (!clientId) {
    console.log("Не задан VITE_YA_CLIENT_ID для Яндекс ID.");
    return;
  }

  const redirectUri =
    import.meta.env.VITE_YA_REDIRECT_URI || `${window.location.origin}/auth`;

  const oauthQueryParams = {
    client_id: clientId,
    response_type: "token",
    redirect_uri: redirectUri,
  };
  const tokenPageOrigin = window.location.origin;

  window.YaAuthSuggest.init(oauthQueryParams, tokenPageOrigin, {
    view: "button",
    parentId: BUTTON_CONTAINER_ID,
    buttonSize: "xxl",
    buttonView: "main",
    buttonTheme: "light",
    buttonBorderRadius: "28",
    buttonIcon: "ya",
  })
    .then(({ handler }) => handler())
    .then((data) => {
      console.log("Сообщение с токеном", data);
      persistToken(data);
    })
    .catch((error) => {
      console.log("Обработка ошибки", error);
    });
};

const clearAuthContainer = () => {
  const container = document.getElementById(BUTTON_CONTAINER_ID);
  if (container) {
    container.innerHTML = "";
  }
};

const ensureAuthScript = () => {
  if (window.YaAuthSuggest?.init) {
    initAuthSuggest();
    return;
  }

  const existingScript = document.querySelector(`script[src="${SCRIPT_SRC}"]`);
  if (existingScript) {
    existingScript.addEventListener("load", initAuthSuggest, { once: true });
    return;
  }

  const script = document.createElement("script");
  script.src = SCRIPT_SRC;
  script.async = true;
  script.addEventListener("load", initAuthSuggest, { once: true });
  script.addEventListener(
    "error",
    () => {
      console.log("Не удалось загрузить виджет Яндекс ID.");
    },
    { once: true }
  );
  document.body.appendChild(script);
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

    clearAuthContainer();
    ensureAuthScript();

    return () => {
      window.removeEventListener("keydown", handleKeyDown);
    };
  }, [isOpen, onClose]);
};
