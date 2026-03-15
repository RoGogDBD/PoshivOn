const getAPIBase = () => import.meta.env.VITE_API_URL || "";

const request = async (path, options = {}) => {
  const response = await fetch(`${getAPIBase()}${path}`, {
    credentials: "include",
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
    const error = new Error(message);
    error.status = response.status;
    throw error;
  }

  if (response.status === 204) {
    return null;
  }

  return response.json();
};

export const getUserSettings = async (userID) => request(`/api/v1/users/${userID}/settings`);

export const saveUserSettings = async (userID, settings) => {
  await request(`/api/v1/users/${userID}/settings`, {
    method: "POST",
    body: JSON.stringify(settings),
  });
};

export const listChats = async (userID) => request(`/api/v1/users/${userID}/chats`);

export const createChat = async (userID, title) =>
  request(`/api/v1/users/${userID}/chats`, {
    method: "POST",
    body: JSON.stringify({ title }),
  });

export const listChatCalculations = async (userID, chatID) =>
  request(`/api/v1/users/${userID}/chats/${chatID}/calculations`);

export const calculateInChat = async (userID, chatID, order) =>
  request(`/api/v1/users/${userID}/chats/${chatID}/calculate`, {
    method: "POST",
    body: JSON.stringify(order),
  });
