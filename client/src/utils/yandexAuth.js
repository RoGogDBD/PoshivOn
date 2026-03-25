const AUTH_RETURN_TO_KEY = "poshivon.auth.returnTo";

const getApiBase = () => import.meta.env.VITE_API_URL || "";

const parseErrorCode = async (response) => {
  try {
    const payload = await response.clone().json();
    return payload?.error || null;
  } catch {
    return null;
  }
};

const shouldRefreshAuth = (status, errorCode) =>
  status === 401 &&
  ["access_cookie_missing", "access_expired", "access_mismatch"].includes(errorCode);

export const refreshAuthSession = async () => {
  const apiBase = getApiBase();
  const response = await fetch(`${apiBase}/auth/refresh`, {
    method: "POST",
    credentials: "include",
  });
  return response.ok;
};

export const authFetch = async (path, options = {}, retryOnAuth = true) => {
  const apiBase = getApiBase();
  const response = await fetch(`${apiBase}${path}`, {
    credentials: "include",
    ...options,
  });

  if (!retryOnAuth) {
    return response;
  }

  const errorCode = await parseErrorCode(response);
  if (!shouldRefreshAuth(response.status, errorCode)) {
    return response;
  }

  const refreshed = await refreshAuthSession();
  if (!refreshed) {
    return response;
  }

  return fetch(`${apiBase}${path}`, {
    credentials: "include",
    ...options,
  });
};

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
    response_type: "code",
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

export const checkAuthStatus = async () => {
  const response = await authFetch("/auth/status", {
    method: "GET",
  });
  return response.ok;
};

export const fetchAuthProfile = async () => {
  const response = await authFetch("/auth/me", {
    method: "GET",
  });
  if (!response.ok) {
    throw new Error("profile_failed");
  }
  return response.json();
};

export const logout = async () => {
  const response = await authFetch("/auth/logout", {
    method: "POST",
  }, false);
  if (!response.ok) {
    throw new Error("logout_failed");
  }
};
