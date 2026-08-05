---
created: 2026-08-05
status: draft
branch: dev
size: M
---

# Tech Spec: Доступ к панели по заявке с одобрением администратором

## Solution

Доступ к рабочему интерфейсу панели становится явным состоянием пользователя, хранимым в БД,
а не следствием успешной авторизации через Яндекс.

Три опорных изменения:

1. **Личность в сессии.** Сегодня `oauth_sessions` не хранит, кому принадлежит сессия
   (`server/internal/auth/store.go:11-21`), а логин достаётся живым запросом к Яндексу в
   `HandleMe` (`server/internal/handler/auth.go:197`). Мы сохраняем `yandex_login` и
   `yandex_email` в строке сессии в момент входа. Это даёт серверу возможность узнать
   вызывающего без внешнего HTTP-запроса на каждое обращение и делает возможной
   авторизацию новых эндпоинтов.

2. **Состояние доступа на пользователе.** В `users` добавляются `role` и `has_access`.
   Гейт проверяет `has_access` (или роль администратора), а не наличие одобренной заявки —
   поэтому отзыв доступа работает независимо от истории заявок. Строка `users` начинает
   создаваться при входе, а не лениво при первой записи данных: сегодня `upsertUser`
   вызывается только из `UpsertSettings`/`CreateChat`/`AppendCalculation`
   (`server/internal/repository/postgres.go:140, 233, 363`), из-за чего список «все
   пользователи» не увидел бы как раз тех, ради кого фича делается.

3. **Новый защищённый контур API.** Появляются `/api/v1/access/*` (состояние доступа и
   подача заявки) и `/api/v1/admin/*` (список пользователей и переключение доступа) с
   собственными middleware `RequireAuth` и `RequireAdmin`. Существующие
   `/api/v1/users/{userID}/*` в рамках этой фичи не переделываются — см. User-Spec Deviations.

Клиент получает третий вызов в bootstrap-эффекте панели (`client/src/pages/Panel.jsx:192-224`),
машина состояний `status` получает значение `no-access`, а тело панели — третью секцию
«Пользователи», видимую только администраторам.

Уведомление администраторам отправляется через `net/smtp` асинхронно: заявка создаётся и
подтверждается пользователю независимо от результата отправки письма.

## Architecture

### What we're building/modifying

Сервер:

- **`server/migrations/004_access_control.up.sql`** (новый) — `users.role`, `users.has_access`,
  `oauth_sessions.yandex_login`, `oauth_sessions.yandex_email`, таблица `access_requests`.
- **`server/internal/service/access.go`** (новый) — `AccessService`: доменные правила доступа,
  заявок и админских операций; интерфейсы `UserRepository`, `AccessRequestRepository`;
  доменные ошибки `ErrForbidden`, `ErrConflict`.
- **`server/internal/service/notifier.go`** (новый) — `SMTPNotifier`: отправка письма
  администраторам о новой заявке. Отключается, если SMTP не сконфигурирован.
- **`server/internal/auth/store.go`** (правка) — поля личности в `Session`, их запись и чтение.
- **`server/internal/handler/auth.go`** (правка) — сохранение логина и email в сессию при входе,
  создание строки `users`, вынос проверки сессии в переиспользуемый вид.
- **`server/internal/handler/middleware.go`** (новый) — `RequireAuth`, `RequireAdmin`,
  идентичность в `context`.
- **`server/internal/handler/access.go`** (новый) — `AccessHandler` с четырьмя маршрутами.
- **`server/internal/repository/postgres.go`**, **`memory.go`** (правки) — реализации новых
  интерфейсов в обоих репозиториях.
- **`server/internal/config/config.go`** (правка) — SMTP и контактный email.
- **`server/cmd/main.go`** (правка) — сборка и регистрация нового контура.

Клиент:

- **`client/src/utils/accessApi.js`** (новый) — вызовы нового контура API.
- **`client/src/components/AccessRequestBanner.jsx`** (новый) — плашка запроса доступа.
- **`client/src/components/AdminUsersSection.jsx`** (новый) — раздел «Пользователи».
- **`client/src/pages/Panel.jsx`** (правка) — гейт в bootstrap, `status: no-access`,
  третья секция и пункт навигации, охрана эффектов загрузки данных.
- **`client/src/utils/yandexAuth.js`** (правка) — параметр `scope` в OAuth-запросе.

Деплой:

- **`.github/workflows/deploy.yml`**, **`docker-compose.prod.yml`**, **`docker-compose.yml`**,
  **`client/nginx.conf`** (правки) — проброс новых переменных окружения и проксирование
  `/auth/refresh`.

### How it works

**Вход.** `HandleYandexCode` / `HandleYandexLogin` после получения access-токена один раз
запрашивают профиль Яндекса, кладут `login` и `default_email` в строку `oauth_sessions`
и вызывают `AccessService.EnsureUser(login, email)` — строка `users` создаётся с
`role='user'`, `has_access=false`, существующая не перезаписывается.

**Проверка доступа.** Клиент при открытии `/panel` вызывает `GET /api/v1/access/me`.
`RequireAuth` проверяет сессию по кукам и кладёт логин в `context`. Ответ:
`{login, name, email, role, has_access, request_status, contact_email}`.
Клиент решает: `has_access || role == 'admin'` → рабочий интерфейс, иначе → плашка.

**Заявка.** `POST /api/v1/access/requests` создаёт строку в `access_requests` со статусом
`pending`. Если у пользователя уже есть строка со статусом `pending` — 409. Если доступ уже
выдан — 409. После успешного создания в отдельной горутине отправляется письмо
администраторам; ошибка отправки логируется и на ответ не влияет.

**Админские операции.** `RequireAdmin` поверх `RequireAuth` читает роль пользователя из БД
по логину из контекста; не-администратор получает 403.
`GET /api/v1/admin/users` возвращает всех пользователей с `has_access`, `role`,
`request_status`. `POST /api/v1/admin/users/{login}/access` с телом `{"granted": bool}`
переключает флаг и переводит заявку в `approved`/`rejected` с фиксацией `decided_by` и
`decided_at`.

**Отзыв доступа.** Снятие галочки ставит `has_access=false`. Пользователь с уже открытой
вкладкой продолжает видеть интерфейс до перезагрузки — гейт проверяется при
инициализации панели. Это принято осознанно, см. Decision 8.

### Shared resources

| Resource | Owner (creates) | Consumers | Instance count |
|----------|----------------|-----------|----------------|
| `*sql.DB` (MariaDB, database/sql) | `cmd/main.go:26` через `db.Open` | `auth.Store`, `migrations.Run` | 1 |
| `*gorm.DB` (MariaDB, GORM) | `cmd/main.go:119` через `db.OpenGORM` | `PostgresRepository` (настройки, чаты, расчёты, пользователи, заявки) | 1 |
| `SMTPNotifier` | `cmd/main.go` | `AccessHandler` | 1 (nil, если SMTP не сконфигурирован) |
| `AccessService` | `cmd/main.go` | `AccessHandler`, `AuthHandler` (только `EnsureUser`) | 1 |

Новых тяжёлых ресурсов не появляется: `SMTPNotifier` не держит постоянного соединения —
`smtp.SendMail` открывает и закрывает соединение на каждое письмо.

## Decisions

### Decision 1: Личность хранится в строке сессии, а не запрашивается у Яндекса на каждый запрос
**Decision:** добавить `yandex_login` и `yandex_email` в `oauth_sessions`, заполнять при входе
и при ротации токенов в `HandleRefresh`.
**Rationale:** поддерживает US-13 — серверная проверка «кто вызывает» нужна на каждом
обращении к админским эндпоинтам, и она не должна зависеть от доступности Яндекса.
Сегодня единственный источник логина — живой вызов `https://login.yandex.ru/info`
(`server/internal/handler/auth.go:459-511`) с 10-секундным таймаутом.
**Alternatives considered:** вызывать `fetchYandexProfile` в middleware на каждый запрос —
отклонено: внешний round-trip на горячем пути и авторизация, падающая вместе с Яндексом.
Хранить логин в отдельной куке — отклонено: значение подконтрольно клиенту.

### Decision 2: Существующие сессии после миграции считаются неопознанными
**Decision:** `yandex_login` в `oauth_sessions` объявляется nullable; сессия с `NULL` в этом
поле проходит `/auth/status` и `/auth/me` как раньше, но отвергается `RequireAuth` новых
эндпоинтов с кодом `session_identity_missing` (401). Клиент при таком ответе отправляет
пользователя на повторный вход.
**Rationale:** [TECHNICAL] down-миграций в проекте нет (`server/migrations/migrate.go` не
имеет down-пути), бэкфилл невозможен — старые строки не содержат логина, и восстановить его
можно только вызовом к Яндексу с сохранённым токеном. Обесценивание старых сессий —
однократное неудобство при выкатке.
**Alternatives considered:** ленивый бэкфилл через `fetchYandexProfile` при первом обращении —
отклонено: усложняет middleware ради одноразового сценария выкатки.

### Decision 3: Доступ гейтится флагом `users.has_access`, а не наличием одобренной заявки
**Decision:** `has_access BOOLEAN NOT NULL DEFAULT FALSE` на `users`; `access_requests` хранит
факт обращения, но не является источником истины о доступе.
**Rationale:** поддерживает US-10 — отзыв доступа должен работать и у пользователя,
чья заявка когда-то была одобрена, без удаления или переписывания истории заявок.
**Alternatives considered:** считать доступ как «есть заявка со статусом approved» —
отклонено: отзыв потребовал бы менять статус исторической заявки, теряя факт одобрения.

### Decision 4: Роль администратора — колонка `users.role`, тип VARCHAR + CHECK
**Decision:** `role VARCHAR(16) NOT NULL DEFAULT 'user'` с
`CHECK (role IN ('user','admin'))`. Логины двух администраторов проставляются
отдельным `UPDATE` в той же миграции по значениям-плейсхолдерам, которые пользователь
заменит перед выкаткой.
**Rationale:** поддерживает US-6 и US-7 — роль читается тем же запросом, что и список
пользователей, без второго источника данных.
`VARCHAR + CHECK` вместо `ENUM` — консистентность с существующим стилем схемы
(`market_status VARCHAR(64)` в `003_pricing_and_chat_delete.up.sql:16`, CHECK-констрейнты
в `002_costing_schema.up.sql:46-53`); ENUM в проекте не используется нигде.
**Alternatives considered:** `ADMIN_LOGINS` в переменных окружения — отклонено пользователем
при уточнении. ENUM-тип — отклонено ради консистентности стиля.

### Decision 5: Одна строка заявки на пользователя, `PRIMARY KEY (user_id)`
**Decision:** `access_requests` имеет `user_id VARCHAR(255) PRIMARY KEY` с FK на `users(id)`;
повторная заявка после отказа перезаписывает строку.
**Rationale:** поддерживает US-3 — «повторная подача при заявке на рассмотрении невозможна»
становится следствием первичного ключа, а не отдельной проверки.
MariaDB не поддерживает ни частичные индексы (`UNIQUE ... WHERE status='pending'` — синтаксис
PostgreSQL), ни функциональные индексы, поэтому «одна pending-заявка на пользователя»
иначе потребовала бы либо непроверенного трюка с generated-колонкой, либо блокировки
`SELECT ... FOR UPDATE` в сервисном слое.
**Alternatives considered:** отдельная строка на каждую подачу с историей — отклонено:
US не требует истории повторных обращений, а уникальность pending-заявки на MariaDB
декларативно не выражается. Trade-off зафиксирован: сохраняется только последнее обращение
и последнее решение по нему.

### Decision 6: Новый API-контур защищается на сервере, существующий — нет
**Decision:** `RequireAuth` и `RequireAdmin` применяются только к `/api/v1/access/*` и
`/api/v1/admin/*`. Обработчик `/api/v1/users/{userID}/*` (`server/internal/handler/http.go:26-70`)
остаётся без изменений.
**Rationale:** поддерживает US-13 — без серверной проверки любой пользователь выдал бы доступ
сам себе через прямой вызов API, и одобрение потеряло бы смысл. При этом переделка
авторизации существующих эндпоинтов — отдельный пре-существующий долг с собственным объёмом,
выведенный пользователем за рамки фичи. См. User-Spec Deviations.
**Alternatives considered:** закрыть весь `/api/v1/*` разом — отклонено пользователем при
уточнении объёма. Оставить и новые эндпоинты открытыми — отклонено: делает фичу неработающей.

### Decision 7: Письмо администраторам отправляется асинхронно через `net/smtp`
**Decision:** `SMTPNotifier` на стандартной библиотеке (`net/smtp` + `mime` для RFC 2047
кодирования кириллической темы). Отправка — в отдельной горутине после успешного создания
заявки; ошибка логируется. Если `SMTP_HOST` пуст, нотификатор равен `nil` и отправка
пропускается с записью в лог при старте.
**Rationale:** поддерживает US-5 и ограничение user-spec «сбой отправки письма не должен
ронять создание заявки». Отсутствие зависимости соответствует политике проекта — сегодня
всего 5 прямых модулей в `server/go.mod`. Паттерн «внешний клиент, отключаемый пустой
конфигурацией» уже есть: `DeepSeekClient` и проверка `h.deepseek == nil`
(`server/internal/handler/http.go:188-191`).
**Alternatives considered:** библиотека `go-mail`/`gomail` — отклонено: ради одного письма
с одним заголовком не стоит расширять зависимости. Синхронная отправка — отклонено:
нарушает ограничение user-spec.

### Decision 8: Гейт проверяется при инициализации панели, без фонового опроса
**Decision:** `GET /api/v1/access/me` вызывается один раз в bootstrap-эффекте
(`client/src/pages/Panel.jsx:192-224`). Отзыв доступа вступает в силу при следующей
загрузке панели.
**Rationale:** [TECHNICAL] поддерживает US-10 в формулировке user-spec («при следующем
открытии панели»). Фоновый polling добавил бы таймер, обработку гонок с открытыми формами
и нагрузку, не требуемую ни одним критерием приёмки.
**Alternatives considered:** опрос каждые N секунд с принудительным выходом — отклонено как
не требуемое; SSE/WebSocket — отклонено: инфраструктуры для них в проекте нет.

### Decision 9: Переключение доступа — POST, а не PATCH/PUT
**Decision:** `POST /api/v1/admin/users/{login}/access` с телом `{"granted": bool}`.
**Rationale:** [TECHNICAL] CORS-обёртка объявляет только `GET, POST, OPTIONS`
(`server/internal/handler/cors.go:33`). PATCH или PUT потребовали бы расширить список
разрешённых методов, то есть изменить общий CORS-контракт ради одного эндпоинта.
**Alternatives considered:** `PATCH /api/v1/admin/users/{login}` — отклонено: расширение
`Access-Control-Allow-Methods` затрагивает все маршруты и выходит за рамки фичи.

### Decision 10: Администратор всегда имеет доступ, независимо от флага
**Decision:** проверка доступа — `has_access || role == 'admin'`. Флаг `has_access` у
администратора игнорируется как в API, так и в UI; в списке пользователей галочка
администратора отображается отмеченной и недоступной для снятия.
**Rationale:** поддерживает US-14 и снимает риск из user-spec «администратор снимает
галочку у себя и теряется управление доступом».
**Alternatives considered:** запретить снятие галочки только у себя — отклонено: не спасает
от снятия галочки у второго администратора.

### Decision 11: Строка `users` создаётся при входе
**Decision:** `AccessService.EnsureUser(ctx, login, email)` вызывается из обработчиков входа
после успешного создания сессии.
**Rationale:** поддерживает US-7 — список должен содержать всех, кто когда-либо входил.
Сегодня `upsertUser` (`server/internal/repository/postgres.go:550-555`) срабатывает только при
сохранении настроек, создании чата или расчёте, поэтому пользователь, который вошёл и упёрся
в плашку, в `users` вообще не появится, а FK из `access_requests` будет некуда указывать.
**Alternatives considered:** создавать строку при подаче заявки — отклонено: администратор не
увидит вошедших, но не подавших заявку; вход остался бы невидимым событием.

### Decision 12: Запрос OAuth-скоупа `login:email`
**Decision:** `buildYandexAuthUrl` (`client/src/utils/yandexAuth.js:125-142`) начинает
передавать `scope=login:info login:email`. Если `default_email` в ответе Яндекса отсутствует,
`EnsureUser` сохраняет пустой email, а письмо администраторам содержит только логин.
**Rationale:** поддерживает US-5 — письмо должно содержать email заявителя.
Сейчас `buildYandexAuthUrl` не передаёт `scope` вообще, и набор разрешений определяется
настройками OAuth-приложения в консоли Яндекса.
**Alternatives considered:** считать email как `{login}@yandex.ru` — отклонено: неверно
для аккаунтов с привязанной внешней почтой и для доменных аккаунтов.

## Data Models

### Миграция `server/migrations/004_access_control.up.sql`

Следующий свободный номер — `004`; трёхзначный префикс обязателен, порядок применения — простая
лексикографическая сортировка (`server/migrations/migrate.go:25`). Файл применяется одним
`db.Exec` целиком (`migrate.go:43`), что работает благодаря `multiStatements=true` в DSN
(`server/internal/db/db.go:63`). Down-миграций в проекте нет.

```sql
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS role VARCHAR(16) NOT NULL DEFAULT 'user',
  ADD COLUMN IF NOT EXISTS has_access BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS email VARCHAR(255) NULL,
  ADD CONSTRAINT chk_users_role CHECK (role IN ('user','admin'));

ALTER TABLE oauth_sessions
  ADD COLUMN IF NOT EXISTS yandex_login VARCHAR(255) NULL,
  ADD COLUMN IF NOT EXISTS yandex_email VARCHAR(255) NULL;

CREATE TABLE IF NOT EXISTS access_requests (
    user_id     VARCHAR(255)  NOT NULL PRIMARY KEY,
    status      VARCHAR(16)   NOT NULL DEFAULT 'pending',
    message     VARCHAR(1000) NULL,
    created_at  TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    decided_at  TIMESTAMP     NULL,
    decided_by  VARCHAR(255)  NULL,
    CONSTRAINT fk_access_requests_user
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT chk_access_requests_status
        CHECK (status IN ('pending','approved','rejected'))
);

CREATE INDEX idx_users_role_has_access ON users(role, has_access);
```

`ADD COLUMN IF NOT EXISTS` — расширение MariaDB, уже применённое в
`003_pricing_and_chat_delete.up.sql:2-7`; проект работает на `mariadb:11.4` в dev и prod.

Проставление администраторов — отдельным оператором в конце файла, с логинами-плейсхолдерами:

```sql
UPDATE users SET role = 'admin' WHERE id IN ('ADMIN_LOGIN_1','ADMIN_LOGIN_2');
```

Логины подставляет пользователь перед выкаткой. Для аккаунта, ещё ни разу не входившего в
систему, строки `users` не существует, и `UPDATE` его не затронет — поэтому порядок выкатки
такой: миграция применяется, оба администратора выполняют вход (строки создаются
`EnsureUser`), затем `UPDATE` повторяется вручную через `docker exec poshivon_db mariadb`.
Процедура зафиксирована в задаче деплоя.

### Доменные типы (`server/internal/service/access.go`)

```go
type Role string // "user" | "admin"

type AccessState struct {
    Login         string
    Email         string
    Role          Role
    HasAccess     bool
    RequestStatus string // "" | "pending" | "approved" | "rejected"
}

type UserRecord struct {
    Login         string
    Email         string
    Role          Role
    HasAccess     bool
    RequestStatus string
    RequestedAt   *time.Time
    CreatedAt     time.Time
}

type UserRepository interface {
    EnsureUser(ctx context.Context, login, email string) error
    GetUser(ctx context.Context, login string) (UserRecord, error)
    ListUsers(ctx context.Context) ([]UserRecord, error)
    SetAccess(ctx context.Context, login string, granted bool) error
}

type AccessRequestRepository interface {
    CreateRequest(ctx context.Context, login, message string) error
    GetRequest(ctx context.Context, login string) (AccessRequest, error)
    DecideRequest(ctx context.Context, login, status, decidedBy string) error
}
```

Форма повторяет house style существующих интерфейсов (`server/internal/service/costing.go:168-183`):
узкие интерфейсы объявлены в `service`, реализованы в `repository`, `ctx` первым аргументом,
через границу проходят только доменные типы.

### Новые доменные ошибки

`server/internal/service/costing.go:15-18` определяет только `ErrInvalidArgument` и `ErrNotFound`.
Добавляются `ErrForbidden` и `ErrConflict`, и `writeAPIDomainError`
(`server/internal/handler/http.go:251-266`) получает ветки 403 и 409.

### Контракт API

| Метод и путь | Доступ | Ответы |
|---|---|---|
| `GET /api/v1/access/me` | авторизованный | 200 `{login,name,email,role,has_access,request_status,contact_email}`; 401 |
| `POST /api/v1/access/requests` | авторизованный без доступа | 201; 409 (заявка на рассмотрении или доступ уже есть); 401 |
| `GET /api/v1/admin/users` | только `role=admin` | 200 `{items:[UserRecord]}`; 401; 403 |
| `POST /api/v1/admin/users/{login}/access` | только `role=admin` | 204; 400; 401; 403; 404 |

Все пути лежат под `/api/`, который прод-nginx уже проксирует на `poshivon_app:8080`
(`.github/workflows/deploy.yml:130-233`), поэтому новых правил проксирования для них не нужно.

Коды ошибок 401 из нового контура намеренно отличаются от
`access_cookie_missing` / `access_expired` / `access_mismatch`: только эти три значения
запускают повторную попытку с refresh на клиенте (`client/src/utils/yandexAuth.js:14-16`).
403 не участвует в retry-логике вовсе, что и требуется.

## Dependencies

### New packages

Новых модулей не добавляется. Используются пакеты стандартной библиотеки, ранее в проекте
не задействованные:

- `net/smtp` — отправка письма администраторам
- `mime` — RFC 2047 кодирование кириллической темы письма
- `net/mail` — валидация адресов из конфигурации

### Using existing (from project)

- `gorm.io/gorm`, `gorm.io/driver/mysql` — новые модели и методы в `PostgresRepository`
- `database/sql` — поля личности в `auth.Store`
- `net/http` — middleware и обработчики нового контура
- `context` — передача личности вызывающего от middleware к обработчику

## Testing Strategy

**Feature size:** M

В репозитории сегодня один тестовый файл — `server/internal/service/costing_test.go` (293 строки),
стандартный `testing` без внешних зависимостей, ручные стабы интерфейсов, `t.Parallel()`.
HTTP-тестов, фикстур БД и CI-джобы для тестов нет. Новые тесты следуют существующему стилю;
HTTP-обвязка строится на `net/http/httptest` из стандартной библиотеки.

### Unit tests

`server/internal/service/access_test.go` — на стабах интерфейсов, по образцу `costing_test.go`:

- Пользователь без доступа и без заявки: состояние `has_access=false`, `request_status=""`
- Пользователь с ролью `admin` и `has_access=false` считается имеющим доступ (US-14)
- Создание заявки пользователем без доступа: заявка создаётся со статусом `pending`
- Повторная заявка при статусе `pending`: `ErrConflict`
- Заявка от пользователя, у которого доступ уже есть: `ErrConflict`
- Новая заявка после `rejected`: разрешена, перезаписывает строку
- `SetAccess(granted=true)` выставляет флаг и переводит заявку в `approved` с `decided_by`
- `SetAccess(granted=false)` снимает флаг и переводит заявку в `rejected`
- `SetAccess` для несуществующего логина: `ErrNotFound`
- `EnsureUser` для существующего пользователя не сбрасывает `role` и `has_access`

`server/internal/service/notifier_test.go`:

- Тема письма с кириллицей кодируется в RFC 2047 encoded-word
- Тело письма содержит логин и email заявителя
- Заявитель без email: письмо формируется, в теле только логин
- Нотификатор с пустым `SMTP_HOST` не конструируется (возвращает `nil`)

### Integration tests

`server/internal/handler/access_test.go` — `httptest.NewRecorder` + `MemoryRepository` +
стаб резолвера сессии (интерфейс вводится ради этого; `auth.Store` требует реального
`*sql.DB` и стабу не поддаётся):

- `GET /api/v1/access/me` без кук → 401
- `GET /api/v1/access/me` с сессией без `yandex_login` → 401 `session_identity_missing`
- `GET /api/v1/access/me` пользователем без доступа → 200, `has_access=false`
- `POST /api/v1/access/requests` дважды подряд → 201, затем 409
- `GET /api/v1/admin/users` без кук → 401
- `GET /api/v1/admin/users` неадминистратором → 403 (US-12)
- `GET /api/v1/admin/users` администратором → 200, в списке присутствуют все пользователи
- `POST /api/v1/admin/users/{login}/access` неадминистратором → 403 **и флаг в репозитории
  не изменился** (US-13 — проверка именно последствия, не только кода ответа)
- `POST /api/v1/admin/users/{login}/access` администратором с `granted=true` → 204, флаг выставлен
- То же с `granted=false` → 204, флаг снят
- `POST /api/v1/admin/users/{unknown}/access` → 404
- Тело с неизвестным полем → 400 (`decodeJSON` использует `DisallowUnknownFields`)

`server/internal/repository/access_repo_test.go` — контрактные тесты на `MemoryRepository`,
проверяющие те же инварианты, что ожидает сервис (создание, повторное `EnsureUser`,
`SetAccess`, список).

Тесты, требующие живой MariaDB (реальное применение миграции, поведение FK и CHECK),
в автоматический набор не входят — тестовой БД и CI-джобы в проекте нет. Применение миграции
проверяется smoke-проверкой в задаче миграции и в Agent Verification Plan.

### E2E tests

None — E2E-инфраструктуры (Playwright/Cypress как зависимость, CI-джоба) в проекте нет,
и её создание кратно превышает объём фичи. Сквозные сценарии покрываются проверками
через Playwright MCP в Agent Verification Plan.

## Agent Verification Plan

**Source:** user-spec "How to Verify" section.

### Verification approach

Автоматические тесты покрывают доменные правила и HTTP-контракт нового контура на
`MemoryRepository`. Сверх них агент проверяет три вещи, которые тестами не берутся:

1. **Миграция на реальной MariaDB.** Поднять dev-окружение (`docker compose up -d db app`),
   убедиться, что `poshivon_app` не в crash-loop (миграция применяется на старте с
   `log.Fatalf` при ошибке — `server/cmd/main.go:32-34`), и проверить схему через
   `docker exec poshivon_db mariadb -e "SHOW COLUMNS FROM users; SHOW CREATE TABLE access_requests;"`.
   Отдельно проверить строку в `schema_migrations` для версии `004_access_control`.

2. **Сквозной HTTP-сценарий против запущенного сервера** через curl с реальными куками:
   отсутствие доступа → подача заявки → повторная подача (409) → выдача доступа
   администратором → проверка состояния. Ключевая проверка — попытка выдать доступ
   неадминистратором завершается 403 **и не меняет флаг в БД** (US-13).

   Замечание по dev-окружению: `docker-compose.yml` не задаёт `APP_STORAGE`, поэтому
   приложение по умолчанию работает на `MemoryRepository`
   (`server/internal/config/config.go:58`, `server/cmd/main.go:115-117`) при том, что
   к MariaDB подключается и миграции применяет. Для проверок против БД нужно явно
   выставить `APP_STORAGE=mysql`.

3. **Отрисовка панели** через Playwright MCP на `localhost` для трёх состояний:
   пользователь без доступа (видна плашка, рабочего интерфейса нет, в network-панели нет
   запросов к `/settings` и `/chats`), пользователь с доступом (рабочий интерфейс),
   администратор (виден пункт «Пользователи» и список с галочками).

Пост-деплой проверяется отдельной задачей: доступность нового контура за прод-nginx,
реальная доставка письма и корректность прод-кук.

### Tools required

- `bash` + `docker compose` / `docker exec` — миграции, состояние контейнеров, состояние БД
- `curl` — контракт HTTP-эндпоинтов и коды ответов
- Playwright MCP — отрисовка плашки, раздела администратора и отсутствие запросов данных у
  пользователя без доступа

## Risks

| Risk | Mitigation |
|------|-----------|
| Миграция падает на проде — контейнер уходит в crash-loop, down-миграций нет (`server/migrations/migrate.go`, `server/cmd/main.go:32-34`) | Проверка миграции на dev-MariaDB той же версии `11.4` до выкатки; в задаче деплоя зафиксирована процедура ручного отката через `docker exec poshivon_db mariadb` |
| `ADD CONSTRAINT ... CHECK` внутри общего `ALTER TABLE` не поддерживает `IF NOT EXISTS` для констрейнта — повторный прогон файла упадёт | Файл применяется ровно один раз (`migrate.go:32-36`); повторный прогон возможен только при потере строки в `schema_migrations`, что описано как ручной сценарий восстановления |
| Все существующие сессии становятся неопознанными после выкатки (Decision 2) | Ожидаемое поведение; клиент при коде `session_identity_missing` отправляет на повторный вход, а не показывает ошибку |
| `default_email` не приходит от Яндекса, если в консоли OAuth-приложения не включено разрешение `login:email` | Письмо формируется и без email — только логин; в задаче деплоя зафиксирован пункт «включить разрешение в консоли Яндекса», проверяется вручную пользователем |
| SMTP-креды утекают в репозиторий — pre-commit хуков и gitleaks в проекте нет | Значения только в GitHub Secrets и `.env` на сервере; в задаче деплоя явный запрет на значения в compose-файлах и коде |
| Переменная окружения не доходит до контейнера: `envs:` в `.github/workflows/deploy.yml:126` — allowlist, отсутствующая в нём переменная молча теряется | Задача деплоя проходит все пять переходов (secret → `env:` → `envs:` → heredoc `.env` → `environment:` в `docker-compose.prod.yml`) и заканчивается проверкой `docker exec poshivon_app env` |
| `/auth/refresh` не проксируется на проде и возвращает SPA с кодом 200 — клиентский retry молча не работает (`.github/workflows/deploy.yml:130-233`, `client/nginx.conf:13-67`) | Проксирование добавляется в задаче деплоя; вынесено в User-Spec Deviations как расширение объёма |
| `writeAPIDomainError` отдаёт клиенту `err.Error()` на 500 (`server/internal/handler/http.go:264`) — новые ошибки БД утекут в ответ | Все ожидаемые ошибки нового контура отображаются на явные ветки 400/403/404/409; ветка 500 в новом контуре отдаёт фиксированный текст |
| Пользователь без доступа всё ещё грузит `/settings` и `/chats`: эффекты в `client/src/pages/Panel.jsx:230-278, 280-308` завязаны на `profile`, а не на `status` | Эффекты получают охрану по признаку доступа; отсутствие этих запросов проверяется через Playwright MCP |
| Оба администратора теряют доступ из-за снятия галочки | Роль хранится отдельно от флага, проверка доступа — `has_access \|\| role=='admin'` (Decision 10) |
| Новые интерфейсы не реализованы в `MemoryRepository` — dev-окружение перестаёт собираться из-за compile-time assertions (`server/internal/repository/memory.go:28-30`) | Обе реализации создаются в одной задаче; assertions добавляются сразу |

## User-Spec Deviations

- **Расширение US-6 (два админ-аккаунта):** user-spec говорит «конкретные логины будут указаны
  пользователем перед деплоем». Tech-spec фиксирует механизм: логины подставляются в
  `UPDATE users SET role='admin' WHERE id IN (...)` в миграции `004`, причём администратор
  должен сначала хотя бы раз войти (иначе строки `users` ещё нет и `UPDATE` её не затронет).
  Порядок выкатки описан в задаче деплоя. → [PENDING USER APPROVAL]

- **Добавлено: проксирование `/auth/refresh` на проде** (не следует ни из одного требования
  user-spec). Причина: маршрут не проксируется ни воркфлоу-конфигом
  (`.github/workflows/deploy.yml:130-233`), ни `client/nginx.conf:13-67`, из-за чего
  запрос попадает на SPA и возвращает 200 с HTML; клиентская логика обновления сессии
  (`client/src/utils/yandexAuth.js:18-25`) считает это успехом и повторяет запрос со
  старой кукой. Дефект существует до этой фичи, но гейт делает истёкшую сессию
  постоянно наблюдаемым состоянием. Правка — две строки в nginx-конфигурациях.
  → [PENDING USER APPROVAL]

- **Добавлено: запрос OAuth-скоупа `login:email`** (Decision 12). Формально следует из US-5
  («письмо содержит email заявителя»), но требует изменения вне кода — включения разрешения
  в консоли OAuth-приложения Яндекса и повторного согласия у уже авторизованных
  пользователей. → [PENDING USER APPROVAL]

- **Сужение относительно US-13:** user-spec требует «неадминистратор не может выдать доступ
  никому, включая себя, обращаясь к API напрямую». Tech-spec обеспечивает это для нового
  контура (`/api/v1/access/*`, `/api/v1/admin/*`), но **не** закрывает существующие
  `/api/v1/users/{userID}/*`, которые остаются полностью неаутентифицированными
  (`server/internal/handler/http.go:26-70`). Практическое следствие: пользователь без доступа
  не может выдать себе доступ, но может напрямую читать и писать данные расчётов любого
  пользователя, как и до этой фичи. Решение принято пользователем при уточнении объёма;
  зафиксировано как отдельный технический долг. → [PENDING USER APPROVAL]

- **Добавлено: поле `users.email`.** User-spec не упоминает хранение email. Нужно для письма
  администраторам (US-5) и для отображения в списке пользователей (US-7), чтобы администратор
  понимал, кому выдаёт доступ. → [PENDING USER APPROVAL]

- **Уточнение US-3 (повторная заявка):** user-spec не описывает, что происходит после отказа.
  Tech-spec разрешает подать заявку заново после `rejected`, перезаписывая предыдущую строку;
  история повторных обращений не сохраняется (Decision 5). → [PENDING USER APPROVAL]

## Acceptance Criteria

- [ ] Миграция `004_access_control` применяется на чистой MariaDB 11.4 и на БД с текущей
      прод-схемой без ошибок; строка версии появляется в `schema_migrations`
- [ ] `MemoryRepository` и `PostgresRepository` оба реализуют `UserRepository` и
      `AccessRequestRepository`; compile-time assertions добавлены для обеих реализаций
- [ ] `GET /api/v1/access/me` без валидных кук возвращает 401
- [ ] `GET /api/v1/admin/users` неадминистратором возвращает 403
- [ ] `POST /api/v1/admin/users/{login}/access` неадминистратором возвращает 403 и не изменяет
      флаг доступа в хранилище
- [ ] Повторная подача заявки при статусе `pending` возвращает 409
- [ ] Подача заявки пользователем, у которого доступ уже есть, возвращает 409
- [ ] Пользователь с `role='admin'` и `has_access=false` проходит проверку доступа
- [ ] Ошибка отправки письма не влияет на код ответа при создании заявки
- [ ] При незаданном `SMTP_HOST` сервер стартует, пишет предупреждение в лог, заявки создаются
- [ ] Строка `users` появляется после входа, до каких-либо действий в панели
- [ ] Пользователь без доступа не отправляет запросов к `/api/v1/users/{login}/settings` и `/chats`
- [ ] 403 и 409 корректно отображаются в `writeAPIDomainError`
- [ ] Ответ 500 из нового контура не содержит текста внутренней ошибки
- [ ] Все существующие тесты проходят; регрессий в `costing_test.go` нет
- [ ] Новые переменные окружения доходят до контейнера `poshivon_app` на проде
- [ ] Ни один SMTP-секрет не присутствует в репозитории

## Implementation Tasks

### Wave 1 (independent)

#### Task 1: Миграция схемы доступа
- **Description:** Создать `004_access_control.up.sql` с колонками роли, флага доступа и email
  на `users`, полями личности на `oauth_sessions` и таблицей `access_requests`. Нужно как
  фундамент для всей фичи. Результат: миграция применяется на MariaDB 11.4 и на текущей
  прод-схеме без ошибок. Схема и оператор проставления администраторов — в разделе Data Models.
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-smoke:** `docker compose up -d db app && sleep 20 && docker compose logs app | tail -30` → нет `Ошибка применения миграций`; затем `docker exec poshivon_db mariadb -uposhivon -pposhivon poshivon -e "SHOW COLUMNS FROM users; SHOW COLUMNS FROM oauth_sessions; SHOW CREATE TABLE access_requests; SELECT version FROM schema_migrations;"` → присутствуют `role`, `has_access`, `email`, `yandex_login`, `yandex_email`, таблица `access_requests` и версия `004_access_control`
- **Files to modify:** `server/migrations/004_access_control.up.sql`
- **Files to read:** `server/migrations/002_costing_schema.up.sql`, `server/migrations/003_pricing_and_chat_delete.up.sql`, `server/migrations/migrate.go`, `server/internal/db/db.go`

#### Task 2: Отправка письма администраторам
- **Description:** Создать `SMTPNotifier` на `net/smtp` с RFC 2047 кодированием темы и
  добавить SMTP-поля в конфигурацию. Нужно для US-5. Результат: сервис умеет отправить письмо
  о новой заявке списку администраторов и возвращает `nil` при незаданном `SMTP_HOST`.
  Асинхронность и обработка ошибок — Decision 7.
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-smoke:** `docker run --rm -d -p 1025:1025 -p 8025:8025 --name mailpit axllent/mailpit`, отправка тестового письма через временный `go run`-скрипт против `SMTP_HOST=localhost SMTP_PORT=1025`, затем `curl -s localhost:8025/api/v1/messages` → письмо присутствует, тема декодируется в кириллицу
- **Files to modify:** `server/internal/service/notifier.go`, `server/internal/service/notifier_test.go`, `server/internal/config/config.go`
- **Files to read:** `server/internal/service/deepseek.go`, `server/cmd/main.go`

### Wave 2 (depends on Wave 1)

#### Task 3: Доменный сервис доступа
- **Description:** Создать `AccessService` с интерфейсами `UserRepository` и
  `AccessRequestRepository`, доменными ошибками `ErrForbidden` и `ErrConflict` и правилами
  из Decisions 3, 5, 10, 11. Нужно как единственное место, где живут правила доступа.
  Результат: правила покрыты юнит-тестами на стабах, включая приоритет роли администратора
  над флагом.
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Files to modify:** `server/internal/service/access.go`, `server/internal/service/access_test.go`, `server/internal/service/costing.go`
- **Files to read:** `server/internal/service/costing_test.go`, `server/internal/service/README.md`

#### Task 4: Реализации репозиториев доступа
- **Description:** Реализовать `UserRepository` и `AccessRequestRepository` в
  `PostgresRepository` и `MemoryRepository`, добавить compile-time assertions и расширить
  `buildRepositories`, возвращающий сегодня фиксированный кортеж из трёх репозиториев.
  Нужно, потому что без реализации в обоих хранилищах dev-окружение перестаёт собираться.
  Результат: оба репозитория удовлетворяют новым интерфейсам, контрактные тесты на
  `MemoryRepository` проходят.
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Files to modify:** `server/internal/repository/postgres.go`, `server/internal/repository/memory.go`, `server/internal/repository/access_repo_test.go`, `server/cmd/main.go`
- **Files to read:** `server/internal/service/access.go`, `server/internal/repository/README.md`

#### Task 5: Личность в сессии и middleware авторизации
- **Description:** Сохранять логин и email Яндекса в `oauth_sessions` при входе и ротации
  токенов, вызывать `EnsureUser` после успешного входа, вынести проверку сессии в вид,
  доступный вне `AuthHandler`, и добавить `RequireAuth` и `RequireAdmin` с передачей личности
  через `context`. Также добавить ветки 403 и 409 в `writeAPIDomainError`. Нужно для US-13.
  Поведение старых сессий — Decision 2, граница применения middleware — Decision 6.
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Files to modify:** `server/internal/auth/store.go`, `server/internal/handler/auth.go`, `server/internal/handler/middleware.go`, `server/internal/handler/http.go`
- **Files to read:** `server/internal/service/access.go`, `server/internal/handler/cors.go`, `server/cmd/main.go`

### Wave 3 (depends on Wave 2)

#### Task 6: Эндпоинты доступа и администрирования
- **Description:** Создать `AccessHandler` с четырьмя маршрутами из раздела Data Models,
  зарегистрировать его в `main.go` под `/api/v1/access/` и `/api/v1/admin/` за middleware из
  Task 5, и построить HTTP-обвязку для интеграционных тестов на `httptest` и
  `MemoryRepository` — сегодня тестов уровня handler в проекте нет. Результат: контракт
  эндпоинтов и коды 401/403/404/409 покрыты интеграционными тестами, включая проверку, что
  403 не меняет состояние хранилища.
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-smoke:** `curl -i localhost:8080/api/v1/admin/users` → 401; `curl -i -X POST localhost:8080/api/v1/admin/users/someone/access -H 'Content-Type: application/json' -d '{"granted":true}'` → 401
- **Files to modify:** `server/internal/handler/access.go`, `server/internal/handler/access_test.go`, `server/cmd/main.go`
- **Files to read:** `server/internal/handler/http.go`, `server/internal/handler/middleware.go`, `server/internal/service/access.go`

### Wave 4 (depends on Wave 3)

#### Task 7: Гейт доступа и плашка запроса в панели
- **Description:** Добавить клиентский слой вызовов нового контура, третий вызов в
  bootstrap-эффекте панели, значение `no-access` в машину состояний `status`, охрану эффектов
  загрузки настроек и чатов, компонент плашки с кнопкой запроса и контактным email, а также
  параметр `scope` в OAuth-URL (Decision 12). Нужно для US-1, US-2, US-3, US-4, US-11.
  Результат: пользователь без доступа видит плашку вместо рабочего интерфейса и не отправляет
  запросов за данными.
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-user:** открыть `localhost:5173/panel` аккаунтом без доступа → видна плашка, рабочего интерфейса нет, во вкладке Network нет запросов к `/settings` и `/chats`; нажать «Запросить доступ» → плашка сообщает, что заявка на рассмотрении, кнопка неактивна
- **Files to modify:** `client/src/utils/accessApi.js`, `client/src/components/AccessRequestBanner.jsx`, `client/src/pages/Panel.jsx`, `client/src/utils/yandexAuth.js`
- **Files to read:** `client/src/utils/panelApi.js`, `client/src/App.css`, `client/src/components/AuthModal.jsx`

#### Task 8: Раздел «Пользователи» для администратора
- **Description:** Добавить третий пункт навигации, видимый только при роли администратора,
  и секцию со списком всех пользователей и переключателем доступа напротив каждого.
  Тело панели сегодня — бинарный тернарник, его нужно превратить в цепочку, а блок сводки
  над ним перенести внутрь рабочей ветки. Нужно для US-7, US-8, US-9, US-10, US-12.
  Галочка администратора отмечена и недоступна для снятия (Decision 10).
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-user:** открыть `localhost:5173/panel` аккаунтом администратора → виден пункт «Пользователи», в списке есть все аккаунты с галочками; переключение галочки меняет доступ и сохраняется после перезагрузки; под обычным аккаунтом пункт не отображается
- **Files to modify:** `client/src/components/AdminUsersSection.jsx`, `client/src/pages/Panel.jsx`, `client/src/App.css`
- **Files to read:** `client/src/utils/accessApi.js`, `client/src/components/AccessRequestBanner.jsx`

### Wave 5 (depends on Wave 4)

#### Task 9: Проброс конфигурации и правка проксирования
- **Description:** Провести новые переменные окружения (SMTP, список получателей, контактный
  email) через все пять переходов от секрета GitHub до контейнера, добавить проксирование
  `/auth/refresh` в обе nginx-конфигурации и зафиксировать порядок выкатки администраторов
  из раздела Data Models. Нужно, потому что переменная, отсутствующая в allowlist `envs:`,
  теряется молча. Результат: сервер на проде видит новые переменные, `/auth/refresh`
  доходит до бэкенда.
- **Skill:** deploy-pipeline
- **Reviewers:** code-reviewer, security-auditor, deploy-reviewer
- **Verify-smoke:** `docker exec poshivon_app env | grep -E 'SMTP_|ADMIN_NOTIFY|CONTACT_EMAIL'` → переменные присутствуют; `curl -i -X POST https://<host>/auth/refresh` → ответ JSON от бэкенда, не HTML SPA
- **Files to modify:** `.github/workflows/deploy.yml`, `docker-compose.prod.yml`, `docker-compose.yml`, `client/nginx.conf`
- **Files to read:** `server/internal/config/config.go`, `deploy/nginx.conf`

### Audit Wave

#### Task 10: Code Audit
- **Description:** Full-feature code quality audit. Read all source files created/modified in this feature (from decisions.md + tech-spec "Files to modify"). Review holistically for cross-component issues: duplicate resource initialization, shared resources compliance with Architecture decisions, architectural consistency. Write audit report.
- **Skill:** code-reviewing
- **Reviewers:** none

#### Task 11: Security Audit
- **Description:** Full-feature security audit. Read all source files created/modified in this feature. Analyze for OWASP Top 10 across all components, cross-component auth/data flow. Write audit report.
- **Skill:** security-auditor
- **Reviewers:** none

#### Task 12: Test Audit
- **Description:** Full-feature test quality audit. Read all test files created in this feature. Verify coverage, meaningful assertions, test pyramid balance across all components. Write audit report.
- **Skill:** test-master
- **Reviewers:** none

### Final Wave

#### Task 13: Pre-deploy QA
- **Description:** Acceptance testing: run all tests, verify acceptance criteria from user-spec and tech-spec
- **Skill:** pre-deploy-qa
- **Reviewers:** none

#### Task 14: Deploy
- **Description:** Deploy + verify logs
- **Skill:** deploy-pipeline
- **Reviewers:** none

#### Task 15: Post-deploy verification
- **Description:** Live environment verification:
  - Контейнер `poshivon_app` поднялся, миграция `004_access_control` применена, нет crash-loop — tool: bash (`docker compose logs`, `docker exec poshivon_db mariadb`)
  - `GET /api/v1/access/me` без кук отвечает 401 через прод-nginx — tool: curl
  - `GET /api/v1/admin/users` неадминистратором отвечает 403 — tool: curl
  - `POST /auth/refresh` доходит до бэкенда, а не до SPA — tool: curl
  - Панель под аккаунтом без доступа показывает плашку, под администратором — раздел «Пользователи» — tool: Playwright MCP
  - Письмо о новой заявке доставлено на почту администраторов — tool: проверка пользователем (доставка зависит от боевых SMTP-кредов)
  Tools: curl, bash, Playwright MCP
- **Skill:** post-deploy-qa
- **Reviewers:** none
