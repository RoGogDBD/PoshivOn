import { useEffect, useRef, useState } from "react";
import { fetchAdminUsers, setUserAccess } from "../utils/accessApi.js";

// Подпись статуса заявки — простой текст рядом с логином, а не отдельный сценарий
// «одобрить/отклонить»: решение принимается тем же чекбоксом доступа (tech-spec,
// «Админские операции»). Незнакомый статус лучше не показывать, чем показать сырой код.
const requestStatusLabel = (status) => {
  switch (status) {
    case "pending":
      return "заявка на рассмотрении";
    case "approved":
      return "заявка одобрена";
    case "rejected":
      return "заявка отклонена";
    default:
      return "";
  }
};

// UserRow — одна строка списка. Вынесена отдельно, чтобы разметка не разрасталась внутри
// .map() основного компонента.
const UserRow = ({ user, pending, onToggle }) => {
  // Админская строка определяется по роли самой строки, а не по сравнению с логином
  // текущей сессии: заблокированы обе админские строки, иначе двое администраторов могут
  // снять доступ друг другу (Decision 10).
  const isAdminRow = user.role === "admin";
  const statusLabel = requestStatusLabel(user.request_status);

  return (
    <div className="panel-chat-list__item">
      <strong>{user.display_name || user.login}</strong>
      <span>
        {user.login}
        {user.email ? ` · ${user.email}` : ""}
      </span>
      <span>
        {isAdminRow ? "администратор" : "пользователь"}
        {statusLabel ? ` · ${statusLabel}` : ""}
      </span>
      <label>
        {/* Админская строка: checked + disabled и никакого onChange — это единственная
            защита от запроса на снятие доступа у админа, сервер такой вызов не отклоняет
            (RequireAdmin смотрит только на роль вызывающего). disabled к тому же снимает
            предупреждение React о controlled input без onChange. */}
        {isAdminRow ? (
          <input type="checkbox" checked disabled />
        ) : (
          <input
            type="checkbox"
            checked={Boolean(user.has_access)}
            disabled={pending}
            onChange={(event) => onToggle(user.login, event.target.checked)}
          />
        )}
        {" Доступ"}
      </label>
    </div>
  );
};

// useAdminUsers — загрузка списка и переключение доступа, отдельно от разметки.
//
// Список тянется один раз при монтировании: приложение не поллит (Decision 15), поэтому
// после переключения флага строка обновляется локально, а полная перезагрузка списка
// происходит при следующем открытии панели. Локальное состояние строки меняется только
// после успешного ответа: оптимистичное переключение оставило бы галочку в неверном
// положении на любой сетевой ошибке или 403.
const useAdminUsers = () => {
  const [users, setUsers] = useState([]);
  // status: "loading" -> "ready" | "failed". "failed" — именно сбой загрузки списка;
  // ошибка переключения флага живёт отдельно в notice и список не рушит.
  const [status, setStatus] = useState("loading");
  const [notice, setNotice] = useState("");
  // Логины строк, по которым сейчас летит запрос — набор, а не одно значение: параллельный
  // тоггл двух разных строк не должен снимать блокировку с первой, пока летит вторая.
  const [pendingLogins, setPendingLogins] = useState(() => new Set());
  const isMounted = useRef(true);

  useEffect(() => {
    isMounted.current = true;
    return () => {
      isMounted.current = false;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;

    const load = async () => {
      try {
        const payload = await fetchAdminUsers();
        if (cancelled) {
          return;
        }
        // items может не прийти вовсе при неожиданной форме ответа — пустой список
        // безопаснее, чем падение рендера на .map у undefined.
        setUsers(Array.isArray(payload?.items) ? payload.items : []);
        setStatus("ready");
      } catch {
        if (cancelled) {
          return;
        }
        setStatus("failed");
        setNotice("Не удалось загрузить список пользователей. Обновите страницу.");
      }
    };

    load();

    return () => {
      cancelled = true;
    };
  }, []);

  const toggleAccess = async (login, granted) => {
    setPendingLogins((previous) => new Set(previous).add(login));
    setNotice("");
    try {
      await setUserAccess(login, granted);
      if (!isMounted.current) {
        return;
      }
      setUsers((previous) =>
        previous.map((user) =>
          user.login === login ? { ...user, has_access: granted } : user
        )
      );
    } catch {
      if (!isMounted.current) {
        return;
      }
      setNotice(`Не удалось изменить доступ для «${login}». Попробуйте ещё раз.`);
    } finally {
      if (isMounted.current) {
        setPendingLogins((previous) => {
          const next = new Set(previous);
          next.delete(login);
          return next;
        });
      }
    }
  };

  return { users, status, notice, pendingLogins, toggleAccess };
};

// AdminUsersSection — раздел «Пользователи» (US-7..US-10, US-12), виден только админу.
//
// Классы берутся из App.css как есть (.panel__card / .panel__notice / .panel__empty /
// .panel-chat-list__item) — собственного стиля у раздела нет, вид доводит Task 17 (Фаза 2).
const AdminUsersSection = () => {
  const { users, status, notice, pendingLogins, toggleAccess } = useAdminUsers();

  return (
    <section className="panel-admin-users">
      <div className="panel__card">
        <h2>Пользователи</h2>
        <p className="panel__empty">
          Отметка «Доступ» открывает пользователю рабочую панель. Доступ администраторов
          снять здесь нельзя.
        </p>

        {status === "loading" ? <p className="panel__empty">Загружаем список...</p> : null}
        {notice ? <p className="panel__notice">{notice}</p> : null}

        {status === "ready" && users.length === 0 ? (
          <p className="panel__empty">Пользователей пока нет.</p>
        ) : null}

        {status === "ready"
          ? users.map((user) => (
              <UserRow
                key={user.login}
                user={user}
                pending={pendingLogins.has(user.login)}
                onToggle={toggleAccess}
              />
            ))
          : null}
      </div>
    </section>
  );
};

export default AdminUsersSection;
