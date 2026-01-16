import { useEffect } from "react";

const BUTTON_CONTAINER_ID = "yandex-id-button";
const SCRIPT_SRC = "https://yastatic.net/s3/passport-sdk/autofill/v1/sdk-suggest.js";

const initAuthSuggest = () => {
  if (!window.YaAuthSuggest?.init) {
    return;
  }

  const oauthQueryParams = {
    client_id: "privet_hacker",
    response_type: "token",
    redirect_uri: "https://poshivon.ru/auth",
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
