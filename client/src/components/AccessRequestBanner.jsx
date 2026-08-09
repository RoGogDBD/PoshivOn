import { useState } from "react";
import { createAccessRequest } from "../utils/accessApi.js";

// AccessRequestBanner — экран пользователя без доступа (US-1..US-4).
//
// Состояние доступа приходит пропсами из bootstrap-эффекта Panel.jsx: повторно дёргать
// GET /api/v1/access/me здесь незачем — ответ уже получен на том же рендере.
//
// Классы взяты из App.css как есть (.panel__card / .panel__notice / .panel__empty);
// собственного styling-слоя у плашки нет — вид под остальную панель доводит Task 17.
const AccessRequestBanner = ({ contactEmail = "", requestStatus = "" }) => {
  // "pending" сразу, если заявка уже на рассмотрении; после отказа (rejected) подать
  // заявку снова можно — сервер это разрешает, поэтому кнопка остаётся активной.
  const [requestState, setRequestState] = useState(
    requestStatus === "pending" ? "pending" : "idle"
  );
  const [notice, setNotice] = useState("");

  const handleRequest = async () => {
    setRequestState("sending");
    setNotice("");
    try {
      await createAccessRequest();
      setRequestState("pending");
    } catch (error) {
      // 409 — не сбой, а штатный ответ: заявка уже на рассмотрении либо доступ уже выдан
      // (Decision 5). Для пользователя это тот же результат, что и успешная подача.
      if (error?.status === 409) {
        setRequestState("pending");
        return;
      }
      setRequestState("idle");
      setNotice("Не удалось отправить заявку. Попробуйте позже.");
    }
  };

  const isSending = requestState === "sending";
  const isPending = requestState === "pending";

  const buttonLabel = (() => {
    if (isPending) {
      return "Заявка на рассмотрении";
    }
    if (isSending) {
      return "Отправляем...";
    }
    return "Запросить доступ";
  })();

  return (
    <div className="panel__card">
      <h2>Доступ к панели ещё не открыт</h2>
      <p className="panel__empty">
        Рабочий интерфейс станет доступен после того, как администратор подтвердит доступ.
      </p>
      {/* Пустой contact_email — ожидаемое значение, а не ошибка загрузки: строка просто
          не показывается. */}
      {contactEmail ? (
        <p className="panel__notice">Связаться с администратором: {contactEmail}</p>
      ) : null}
      <button
        className="panel__theme-toggle"
        type="button"
        onClick={handleRequest}
        disabled={isSending || isPending}
      >
        {buttonLabel}
      </button>
      {isPending ? (
        <p className="panel__notice">Заявка на рассмотрении. Мы сообщим о решении.</p>
      ) : null}
      {notice ? <p className="panel__notice">{notice}</p> : null}
    </div>
  );
};

export default AccessRequestBanner;
