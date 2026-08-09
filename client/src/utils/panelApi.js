import { authFetch } from "./yandexAuth.js";

const request = async (path, options = {}) => {
  const response = await authFetch(path, {
    headers: {
      "Content-Type": "application/json",
      ...(options.headers || {}),
    },
    ...options,
  });

  if (!response.ok) {
    let message = `request_failed:${response.status}`;
    try {
      const payload = await response.json();
      if (payload?.error) {
        message = payload.error;
      }
    } catch {
      // Ignore JSON decode failures for error payloads.
    }
    if (message === `request_failed:${response.status}` && response.status === 405) {
      message = "api_method_not_allowed";
    }
    const error = new Error(message);
    error.status = response.status;
    throw error;
  }

  if (response.status === 204) {
    return null;
  }

  return response.json();
};

// Сегмента владельца в адресах больше нет: владельца сервер берёт из сессии (Decision 6,
// US-15). Логин в пути означал бы, что данные любого владельца доступны подстановкой
// чужого логина, а отзыв доступа на этой поверхности ничего не менял бы.
export const getUserSettings = async () => request(`/api/v1/users/settings`);

export const saveUserSettings = async (settings) => {
  await request(`/api/v1/users/settings`, {
    method: "POST",
    body: JSON.stringify(settings),
  });
};

export const listChats = async () => request(`/api/v1/users/chats`);

export const createChat = async (title) =>
  request(`/api/v1/users/chats`, {
    method: "POST",
    body: JSON.stringify({ title }),
  });

export const deleteChat = async (chatID) => {
  await request(`/api/v1/users/chats/${chatID}`, {
    method: "DELETE",
  });
};

export const restoreChat = async (chatID) => {
  await request(`/api/v1/users/chats/${chatID}/restore`, {
    method: "POST",
  });
};

export const listChatCalculations = async (chatID) =>
  request(`/api/v1/users/chats/${chatID}/calculations`);

export const calculateInChat = async (chatID, order) =>
  request(`/api/v1/users/chats/${chatID}/calculate`, {
    method: "POST",
    body: JSON.stringify(order),
  });

export const analyzeMarketWithAI = async (payload) =>
  request(`/api/v1/users/market-feedback`, {
    method: "POST",
    body: JSON.stringify(payload),
  });
