const AUTH_RETURN_TO_KEY = "poshivon.auth.returnTo";

const getApiBase = () => import.meta.env.VITE_API_URL || "";

const normalizeRedirectUri = (value) => {
  if (!value) {
    return value;
  }
  return value.endsWith("/") ? value.slice(0, -1) : value;
};

export const persistYandexToken = async (data) => {
  if (!data?.access_token) {
    throw new Error("missing_access_token");
  }

  const apiBase = getApiBase();
  const response = await fetch(`${apiBase}/auth/yandex`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    credentials: "include",
    body: JSON.stringify(data),
  });

  if (!response.ok) {
    throw new Error("persist_failed");
  }
};

export const exchangeYandexCode = async (code, redirectUri) => {
  if (!code) {
    throw new Error("missing_code");
  }

  const apiBase = getApiBase();
  const response = await fetch(`${apiBase}/auth/yandex/code`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    credentials: "include",
    body: JSON.stringify({
      code,
      redirect_uri: redirectUri,
    }),
  });

  if (!response.ok) {
    throw new Error("exchange_failed");
  }
};

export const saveAuthReturnTo = (value) => {
  try {
    if (value) {
      sessionStorage.setItem(AUTH_RETURN_TO_KEY, value);
    }
  } catch (error) {
    console.log("Не удалось сохранить адрес возврата.", error);
  }
};

export const consumeAuthReturnTo = () => {
  try {
    const value = sessionStorage.getItem(AUTH_RETURN_TO_KEY);
    sessionStorage.removeItem(AUTH_RETURN_TO_KEY);
    return value;
  } catch (error) {
    console.log("Не удалось получить адрес возврата.", error);
    return null;
  }
};

export const buildYandexAuthUrl = () => {
  const clientId = import.meta.env.VITE_YA_CLIENT_ID;
  if (!clientId) {
    return null;
  }

  const redirectUri = normalizeRedirectUri(
    import.meta.env.VITE_YA_REDIRECT_URI || `${window.location.origin}/auth`
  );

  const params = new URLSearchParams({
    response_type: "token",
    client_id: clientId,
    redirect_uri: redirectUri,
  });

  return `https://oauth.yandex.ru/authorize?${params.toString()}`;
};

export const getAuthRedirectTarget = () =>
  consumeAuthReturnTo() || import.meta.env.VITE_AUTH_SUCCESS_REDIRECT || "/";

export const getRedirectUri = () =>
  normalizeRedirectUri(
    import.meta.env.VITE_YA_REDIRECT_URI || `${window.location.origin}/auth`
  );
