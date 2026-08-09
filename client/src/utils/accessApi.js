import { authFetch } from "./yandexAuth.js";

// Разбор ответа контура доступа. Ошибка несёт `status` (как в panelApi.js): плашке нужно
// отличать штатный 409 «заявка уже есть» от настоящих сбоев, а по тексту сообщения это
// делать нельзя — тело у 409 не гарантировано.
const request = async (path, options = {}) => {
  const response = await authFetch(path, options);

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

  // 201/204 приходят без тела — response.json() на пустом теле бросил бы SyntaxError.
  if (response.status === 201 || response.status === 204) {
    return null;
  }

  return response.json();
};

// fetchAccessState — GET /api/v1/access/me.
//
// Отдаёт {login, display_name, email, role, has_access, request_status, contact_email}.
// Ветки 403 у этого маршрута нет: «доступа нет» — это 200 с has_access: false, а 401
// означает невалидную сессию и пробрасывается вызывающему как ошибка (bootstrap-эффект
// панели обрабатывает её так же, как провал fetchAuthProfile, — редиректом на "/").
export const fetchAccessState = async () =>
  request("/api/v1/access/me", {
    method: "GET",
  });

// createAccessRequest — POST /api/v1/access/requests.
//
// Тела у запроса нет: заявитель — владелец сессии (US-15). 201 — заявка создана,
// 409 — заявка уже на рассмотрении либо доступ уже выдан (Decision 5); 409 бросается как
// ошибка со status === 409, и вызывающий обязан трактовать её как штатный ответ.
export const createAccessRequest = async () =>
  request("/api/v1/access/requests", {
    method: "POST",
  });

// fetchAdminUsers — GET /api/v1/admin/users (только для роли admin).
//
// Отдаёт {items: [{login, display_name, email, role, has_access, request_status,
// requested_at, created_at}]}. Список — все, кто хоть раз входил, а не только те, у кого
// есть заявка. Не-админ сюда не попадает: маршрут закрыт RequireAdmin и ответит 403,
// который прилетит вызывающему ошибкой со status === 403.
export const fetchAdminUsers = async () =>
  request("/api/v1/admin/users", {
    method: "GET",
  });

// setUserAccess — POST /api/v1/admin/users/{login}/access.
//
// Ответ — 204 без тела (request() вернёт null), поэтому подтверждением служит сам факт
// успешного запроса: новое значение флага вызывающий берёт из отправленного granted.
// 404 — незнакомый login, 403 — вызывающий не админ.
//
// Сервер не отказывает, когда цель запроса — админ: RequireAdmin проверяет только роль
// вызывающего. Запрет на снятие доступа у админов живёт на клиенте (Decision 10,
// AdminUsersSection), и эта функция для админских строк просто не должна вызываться.
export const setUserAccess = async (login, granted) =>
  request(`/api/v1/admin/users/${encodeURIComponent(login)}/access`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ granted }),
  });
