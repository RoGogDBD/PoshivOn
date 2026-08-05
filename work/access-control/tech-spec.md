---
created: 2026-08-05
status: draft
branch: dev
size: L
---

# Tech Spec: Доступ к панели по заявке с одобрением администратором

## Solution

Доступ к сервису становится явным состоянием пользователя, хранимым в БД и проверяемым
на сервере, а не следствием успешной авторизации через Яндекс.

Четыре опорных изменения:

1. **Личность в сессии.** Сегодня `oauth_sessions` не хранит, кому принадлежит сессия
   (`server/internal/auth/store.go:11-21`), а логин достаётся живым запросом к Яндексу в
   `HandleMe` (`server/internal/handler/auth.go:197`). Мы сохраняем `yandex_login`,
   `yandex_email` и `yandex_display_name` в строке сессии в момент входа. Это даёт серверу
   возможность узнать вызывающего без внешнего HTTP-запроса на каждое обращение и делает
   возможной авторизацию всех эндпоинтов.

2. **Состояние доступа на пользователе.** В `users` добавляются `role` и `has_access`.
   Гейт проверяет `has_access` (или роль администратора), а не наличие одобренной заявки —
   поэтому отзыв доступа работает независимо от истории заявок. Строка `users` начинает
   создаваться при входе, а не лениво при первой записи данных: сегодня `upsertUser`
   вызывается только из `UpsertSettings`/`CreateChat`/`AppendCalculation`
   (`server/internal/repository/postgres.go:140, 233, 363`), из-за чего список «все
   пользователи» не увидел бы как раз тех, ради кого фича делается.

3. **Единый защищённый контур API.** Появляются `/api/v1/access/*` (состояние доступа и
   подача заявки) и `/api/v1/admin/*` (список пользователей и переключение доступа).
   Существующие `/api/v1/users/{userID}/*` переводятся на ту же схему: `userID` берётся
   из сессии, а не из адреса запроса, и запрос отклоняется без выданного доступа.
   Без этого отзыв доступа остался бы декоративным — все данные лежат за этими маршрутами.

4. **Закрытие путей обхода входа.** Удаляется `POST /auth/yandex`, принимающий готовый
   OAuth-токен от клиента: после этой фичи из такой сессии выводится роль администратора,
   а штатный вход этот путь не использует — `buildYandexAuthUrl` ходит только через
   `response_type=code` (`client/src/utils/yandexAuth.js:125-142`). Добавляются проверка
   источника запроса на изменяющих операциях и параметр `state` в OAuth-редиректе.

Клиент получает третий вызов в bootstrap-эффекте панели (`client/src/pages/Panel.jsx:192-224`),
машина состояний `status` получает значение `no-access`, а тело панели — третью секцию
«Пользователи», видимую только администраторам.

Уведомление администраторам отправляется через `net/smtp` асинхронно: заявка создаётся и
подтверждается пользователю независимо от результата отправки письма.

## Architecture

### What we're building/modifying

Сервер:

- **`server/migrations/004_access_control.up.sql`** (новый) — `users.role`, `users.has_access`,
  `users.email`, `users.display_name`; поля личности на `oauth_sessions`; таблица `access_requests`.
- **`server/internal/service/access.go`** (новый) — `AccessService`: доменные правила доступа,
  заявок и админских операций; интерфейсы `UserRepository`, `AccessRequestRepository`;
  доменные ошибки `ErrForbidden`, `ErrConflict`.
- **`server/internal/service/notifier.go`** (новый) — `SMTPNotifier`: отправка письма
  администраторам о новой заявке. Отключается, если SMTP не сконфигурирован.
- **`server/internal/auth/store.go`** (правка) — поля личности в `Session`, их запись, чтение
  и сохранение при ротации токенов.
- **`server/internal/handler/auth.go`** (правка) — сохранение личности в сессию при входе,
  создание строки `users`, вынос проверки сессии в переиспользуемый вид, удаление
  `HandleYandexLogin`.
- **`server/internal/handler/middleware.go`** (новый) — `RequireAuth`, `RequireAccess`,
  `RequireAdmin`, `RequireSameOrigin`, идентичность в `context`.
- **`server/internal/handler/access.go`** (новый) — `AccessHandler` с четырьмя маршрутами.
- **`server/internal/handler/http.go`** (правка) — `userID` из контекста вместо адреса запроса,
  ветки 403 и 409 в `writeAPIDomainError`, фиксированный текст на 500.
- **`server/internal/repository/postgres.go`**, **`memory.go`** (правки) — реализации новых
  интерфейсов в обоих репозиториях.
- **`server/internal/config/config.go`** (правка) — SMTP, получатели уведомлений, контактный email.
- **`server/cmd/main.go`** (правка) — сборка и подключение middleware ко всем маршрутам `/api/v1/`.

Клиент:

- **`client/src/utils/accessApi.js`** (новый) — вызовы контура доступа и администрирования.
- **`client/src/components/AccessRequestBanner.jsx`** (новый) — плашка запроса доступа.
- **`client/src/components/AdminUsersSection.jsx`** (новый) — раздел «Пользователи».
- **`client/src/pages/Panel.jsx`** (правка) — гейт в bootstrap, `status: no-access`,
  третья секция и пункт навигации, охрана эффектов загрузки данных.
- **`client/src/utils/yandexAuth.js`** (правка) — `scope` и `state` в OAuth-запросе,
  удаление `persistYandexToken`.
- **`client/src/utils/panelApi.js`** (правка) — адреса без сегмента `userID`.
- **`client/src/pages/AuthCallback.jsx`** (правка) — удаление implicit-ветки, сверка `state`.

Деплой:

- **`.github/workflows/deploy.yml`**, **`.github/workflows/test.yml`** (новый),
  **`docker-compose.prod.yml`**, **`docker-compose.yml`**, **`client/nginx.conf`** —
  проброс переменных окружения, проксирование `/auth/refresh`, прогон тестов перед деплоем.

### How it works

**Вход.** `HandleYandexCode` после обмена кода на токен один раз запрашивает профиль Яндекса,
кладёт `login`, `default_email` и отображаемое имя в строку `oauth_sessions` и вызывает
`AccessService.EnsureUser` — строка `users` создаётся с `role='user'`, `has_access=false`;
у существующей строки обновляются только email и имя, роль и флаг доступа не трогаются.

**Проверка доступа.** `RequireAuth` проверяет сессию по кукам и кладёт личность в `context`.
`RequireAccess` поверх неё читает пользователя из БД и пропускает дальше только при
`has_access || role == 'admin'`. `RequireAdmin` пропускает только при `role == 'admin'`.
`RequireSameOrigin` на изменяющих методах сверяет заголовок `Origin` со списком разрешённых.

**Маршруты и их обвязка:**

| Префикс | Middleware |
|---|---|
| `/api/v1/users/` | `RequireSameOrigin` → `RequireAuth` → `RequireAccess` |
| `/api/v1/admin/` | `RequireSameOrigin` → `RequireAuth` → `RequireAdmin` |
| `/api/v1/access/` | `RequireSameOrigin` → `RequireAuth` |
| `/auth/*`, `/health`, `/metrics` | без изменений |

`/api/v1/access/` намеренно не закрыт `RequireAccess` — именно туда обращается пользователь,
у которого доступа ещё нет.

**Клиентская проверка.** При открытии `/panel` клиент вызывает `GET /api/v1/access/me`.
Ответ: `{login, display_name, email, role, has_access, request_status, contact_email}`.
Клиент решает: `has_access || role == 'admin'` → рабочий интерфейс, иначе → плашка.

**Заявка.** `POST /api/v1/access/requests` вставляет строку в `access_requests`.
Уникальность обеспечивает первичный ключ: вставка выполняется без предварительной проверки,
и нарушение ключа отображается на `ErrConflict` → 409. Если доступ уже выдан — 409 без записи.
После успешного создания в отдельной горутине отправляется письмо администраторам; ошибка
отправки логируется и на ответ не влияет.

**Админские операции.** `GET /api/v1/admin/users` возвращает всех пользователей с
`has_access`, `role`, `request_status`. `POST /api/v1/admin/users/{login}/access` с телом
`{"granted": bool}` переключает флаг и переводит заявку в `approved`/`rejected` с фиксацией
`decided_by` и `decided_at`.

**Отзыв доступа.** Снятие галочки ставит `has_access=false`. Следующий же запрос к
`/api/v1/users/*` от этого пользователя отклоняется `RequireAccess` с кодом 403 — открытая
вкладка перестаёт работать сразу, интерфейс переключится на плашку при перезагрузке.

### Shared resources

| Resource | Owner (creates) | Consumers | Instance count |
|----------|----------------|-----------|----------------|
| `*sql.DB` (MariaDB, database/sql) | `cmd/main.go:26` через `db.Open` | `auth.Store`, `migrations.Run` | 1 |
| `*gorm.DB` (MariaDB, GORM) | `cmd/main.go:119` через `db.OpenGORM` | `PostgresRepository` (настройки, чаты, расчёты, пользователи, заявки) | 1 |
| `SMTPNotifier` | `cmd/main.go` | `AccessHandler` | 1 (nil, если SMTP не сконфигурирован) |
| `AccessService` | `cmd/main.go` | `AccessHandler`, `AuthHandler` (только `EnsureUser`), middleware | 1 |

Новых тяжёлых ресурсов не появляется: `SMTPNotifier` не держит постоянного соединения —
`smtp.SendMail` открывает и закрывает соединение на каждое письмо.

## Decisions

### Decision 1: Личность хранится в строке сессии, а не запрашивается у Яндекса на каждый запрос
**Decision:** добавить `yandex_login`, `yandex_email`, `yandex_display_name` в `oauth_sessions`,
заполнять при входе и сохранять при ротации токенов в `HandleRefresh`.
**Rationale:** поддерживает US-13 и US-15 — серверная проверка «кто вызывает» нужна на каждом
обращении, и она не должна зависеть от доступности Яндекса. Сегодня единственный источник
логина — живой вызов `https://login.yandex.ru/info` (`server/internal/handler/auth.go:459-511`)
с 10-секундным таймаутом.
**Alternatives considered:** вызывать `fetchYandexProfile` в middleware на каждый запрос —
отклонено: внешний round-trip на горячем пути и авторизация, падающая вместе с Яндексом.
Хранить логин в отдельной куке — отклонено: значение подконтрольно клиенту.

### Decision 2: Существующие сессии после миграции считаются неопознанными
**Decision:** `yandex_login` объявляется nullable; сессия с `NULL` в этом поле отвергается
`RequireAuth` с кодом `session_identity_missing` (401). Клиент при этом коде отправляет
пользователя на повторный вход.
**Rationale:** [TECHNICAL] down-миграций в проекте нет (`server/migrations/migrate.go` не
имеет down-пути), бэкфилл невозможен — старые строки не содержат логина. Обесценивание
старых сессий — однократное неудобство при выкатке, и оно fail-closed.
**Alternatives considered:** ленивый бэкфилл через `fetchYandexProfile` при первом обращении —
отклонено: усложняет middleware ради одноразового сценария выкатки.

### Decision 3: Доступ гейтится флагом `users.has_access`, а не наличием одобренной заявки
**Decision:** `has_access BOOLEAN NOT NULL DEFAULT FALSE` на `users`; `access_requests` хранит
факт обращения, но не является источником истины о доступе.
**Rationale:** поддерживает US-10 и US-16 — отзыв доступа должен работать и у пользователя,
чья заявка когда-то была одобрена, без переписывания истории заявок.
**Alternatives considered:** считать доступ как «есть заявка со статусом approved» —
отклонено: отзыв потребовал бы менять статус исторической заявки, теряя факт одобрения.

### Decision 4: Роль администратора — колонка `users.role`, тип VARCHAR + CHECK
**Decision:** `role VARCHAR(16) NOT NULL DEFAULT 'user'` с `CHECK (role IN ('user','admin'))`.
Логины двух администраторов проставляются `UPDATE` в конце миграции по плейсхолдерам,
которые пользователь заменит перед выкаткой.
**Rationale:** поддерживает US-6 и US-7 — роль читается тем же запросом, что и список
пользователей, без второго источника данных. `VARCHAR + CHECK` вместо `ENUM` —
консистентность с существующим стилем схемы (`market_status VARCHAR(64)` в
`003_pricing_and_chat_delete.up.sql:16`, CHECK-констрейнты в `002_costing_schema.up.sql:46-53`).
**Alternatives considered:** `ADMIN_LOGINS` в переменных окружения — отклонено пользователем
при уточнении. ENUM-тип — отклонено ради консистентности стиля.

### Decision 5: Одна строка заявки на пользователя, `PRIMARY KEY (user_id)`
**Decision:** `access_requests` имеет `user_id VARCHAR(255) PRIMARY KEY` с FK на `users(id)`.
Создание заявки — вставка без предварительной проверки; нарушение первичного ключа
отображается на `ErrConflict`. Повторная заявка после отказа обновляет `status` и `created_at`,
**сохраняя** `decided_by` и `decided_at` предыдущего решения.
**Rationale:** поддерживает US-3 — «повторная подача при заявке на рассмотрении невозможна»
становится следствием ключа, а не проверки «прочитал-потом-записал», которая имеет гонку
при двух одновременных нажатиях. MariaDB не поддерживает ни частичные индексы
(`UNIQUE ... WHERE status='pending'` — синтаксис PostgreSQL), ни функциональные индексы,
поэтому уникальность именно pending-заявки декларативно не выражается.
**Alternatives considered:** отдельная строка на каждую подачу с полной историей —
отклонено: US не требует истории повторных обращений. Сохранение `decided_by`/`decided_at`
при перезаписи добавлено потому, что иначе теряется единственная запись о том, кто выдавал
доступ. Trade-off зафиксирован: сохраняется последнее обращение и последнее решение по нему.

### Decision 6: Проверка доступа применяется ко всем маршрутам `/api/v1/`
**Decision:** `RequireAuth` + `RequireAccess` навешиваются в том числе на существующий
`/api/v1/users/`; `userID` берётся из контекста, а сегмент адреса перестаёт быть источником
личности (`server/internal/handler/http.go:26-70`). Клиентские адреса теряют сегмент `userID`.
**Rationale:** поддерживает US-15 и US-16. Все данные продукта лежат за этими маршрутами,
и без проверки отзыв доступа не имеет эффекта: пользователь продолжает читать и писать
данные любого владельца. Дополнительно `upsertUser` (`server/internal/repository/postgres.go:550-555`)
сегодня достижим анонимно и создаёт строки в `users` — той самой таблице, где теперь
хранятся роль и флаг доступа.
Владельца нельзя подменить и через тело запроса: входные структуры `CreateChatInput`
(`server/internal/service/costing.go:164-166`) и `OrderInput` (`:87`) поля владельца не
содержат — `UserID` объявлен только у выходных типов `CalculationResult` (`:114`) и `Chat`
(`:154`), а `decodeJSON` включает `DisallowUnknownFields` (`server/internal/handler/http.go:230`),
поэтому лишнее поле `user_id` в теле даёт 400. После удаления сегмента адреса других
источников владельца не остаётся.
**Alternatives considered:** закрыть только новые эндпоинты — отклонено пользователем после
того, как аудит показал, что при этом US-10 и US-16 не выполняются на уровне API.

### Decision 7: Приём готового OAuth-токена удаляется
**Decision:** маршрут `POST /auth/yandex` и метод `HandleYandexLogin` удаляются, вместе с
implicit-веткой в `client/src/pages/AuthCallback.jsx:36-45` и функцией `persistYandexToken`.
**Rationale:** поддерживает US-17. Эндпоинт принимает access-токен из тела запроса и создаёт
сессию, не проверяя, какому приложению токен выдан; после этой фичи из такой сессии выводится
роль администратора. Штатный вход этот путь не использует — `buildYandexAuthUrl` формирует
только `response_type=code`, поэтому hash с `access_token` не приходит никогда.
**Alternatives considered:** оставить эндпоинт, добавив сверку `client_id` из ответа Яндекса —
отклонено пользователем: сохраняет неиспользуемую поверхность атаки ради флоу, который
приложение не применяет.

### Decision 8: Защита изменяющих запросов проверкой `Origin`
**Decision:** `RequireSameOrigin` на методах, отличных от GET и HEAD, сверяет заголовок
`Origin` со списком `CORS_ALLOWED_ORIGINS`; запрос без `Origin` или с посторонним значением
отклоняется с 403. При пустом списке проверка пропускается — это конфигурация локальной разработки.
**Rationale:** поддерживает US-13. Выдача доступа выполняется cookie-аутентифицированным
POST-запросом, а CORS-обёртка выставляет `Access-Control-Allow-Credentials: true`
(`server/internal/handler/cors.go:30`), то есть межсайтовое использование предусмотрено.
Без проверки защита от запроса с чужого сайта держится только на значении `COOKIE_SAMESITE`
из секретов.
**Alternatives considered:** токен синхронизации в форме — отклонено: требует хранилища
токенов и правки каждого клиентского вызова ради того же результата на SPA с единственным
источником. Опора только на `SameSite` — отклонено: значение приходит из непроверяемого
секрета и молча откатывается к `Lax` при опечатке (`server/internal/handler/auth.go:513-522`).

### Decision 9: Параметр `state` в OAuth-редиректе
**Decision:** `buildYandexAuthUrl` генерирует случайный `state`, сохраняет его в
`sessionStorage` рядом с существующим ключом возврата и передаёт в адрес авторизации;
`AuthCallback` сверяет значение до обмена кода и прерывает вход при расхождении.
**Rationale:** поддерживает US-17. Функция и так правится этой фичей ради `scope`
(Decision 12), а привязка ответа к инициировавшему браузеру — то, ради чего параметр
существует: без неё чужой код авторизации можно подсунуть в сессию пользователя.
**Alternatives considered:** отложить в отдельную задачу — отклонено пользователем: правка
затрагивает ровно те две функции, что и так меняются.

### Decision 10: Администратор всегда имеет доступ, независимо от флага
**Decision:** проверка доступа — `has_access || role == 'admin'`. Флаг `has_access` у
администратора игнорируется; в списке пользователей его галочка отмечена и недоступна для снятия.
**Rationale:** поддерживает US-14 и снимает риск из user-spec «администратор снимает
галочку у себя и теряется управление доступом».
**Alternatives considered:** запретить снятие галочки только у себя — отклонено: не спасает
от снятия галочки у второго администратора.

### Decision 11: Строка `users` создаётся при входе, не перезаписывая роль и доступ
**Decision:** `AccessService.EnsureUser(ctx, login, email, displayName)` вызывается из
обработчика входа. Реализация в GORM выполняет `INSERT ... ON CONFLICT DO UPDATE` со списком
обновляемых колонок, ограниченным `email` и `display_name`.
**Rationale:** поддерживает US-7 — список должен содержать всех, кто когда-либо входил.
Ограничение списка обновляемых колонок обязательно: `EnsureUser` срабатывает на каждом входе,
и обновление всех полей сбросило бы `role` в значение по умолчанию, разжаловав обоих
администраторов при первом же входе после выкатки.
**Alternatives considered:** создавать строку при подаче заявки — отклонено: администратор не
увидит вошедших, но не подавших заявку.

### Decision 12: Запрос OAuth-скоупа `login:email`
**Decision:** `buildYandexAuthUrl` начинает передавать `scope=login:info login:email`.
Если `default_email` в ответе Яндекса отсутствует, сохраняется пустой email, а письмо
администраторам содержит только логин.
**Rationale:** поддерживает US-5 — письмо должно содержать email заявителя. Сейчас
`buildYandexAuthUrl` не передаёт `scope` вообще, и набор разрешений определяется настройками
OAuth-приложения в консоли Яндекса.
**Alternatives considered:** считать email как `{login}@yandex.ru` — отклонено: неверно
для аккаунтов с привязанной внешней почтой и для доменных аккаунтов.

### Decision 13: Колонки доступа не добавляются в существующую GORM-модель `userModel`
**Decision:** `userModel` (`server/internal/repository/postgres.go:15-22`) остаётся с полями
`ID` и `CreatedAt`. Для колонок доступа вводится отдельная модель, отображённая на ту же
таблицу `users`. Инвариант «`userModel` не содержит колонок доступа» фиксируется комментарием
у объявления.
**Rationale:** [TECHNICAL] `upsertUser` вставляет `userModel` целиком из трёх мест
(`postgres.go:140, 233, 363`). Добавление в модель поля `Role` заставит GORM вставлять
`role=''`, что нарушает `chk_users_role` — сохранение настроек, создание чата и расчёт
перестанут работать для новых пользователей. Проверено на живой MariaDB 11.4:
`ERROR 4025 (23000): CONSTRAINT chk_users_role failed`.
**Alternatives considered:** добавить полям gorm-тег `default` — отклонено: GORM всё равно
включает поле в INSERT при явном нулевом значении, а поведение зависит от версии.

### Decision 14: Письмо администраторам отправляется асинхронно через `net/smtp`
**Decision:** `SMTPNotifier` на стандартной библиотеке (`net/smtp` + `mime` для RFC 2047
кодирования кириллической темы). Отправка — в отдельной горутине после успешного создания
заявки; ошибка логируется. При пустом `SMTP_HOST` нотификатор равен `nil`, отправка
пропускается, при старте пишется предупреждение.
**Rationale:** поддерживает US-5 и ограничение user-spec «сбой отправки письма не должен
ронять создание заявки». Отсутствие зависимости соответствует политике проекта — сегодня
4 прямых модуля в `server/go.mod`. Паттерн «внешний клиент, отключаемый пустой конфигурацией»
уже есть: `DeepSeekClient` и проверка `h.deepseek == nil` (`server/internal/handler/http.go:188-191`).
**Alternatives considered:** библиотека `go-mail`/`gomail` — отклонено: ради одного письма
с одним заголовком не стоит расширять зависимости. Синхронная отправка — отклонено:
нарушает ограничение user-spec.

### Decision 15: Гейт интерфейса проверяется при инициализации панели, без фонового опроса
**Decision:** `GET /api/v1/access/me` вызывается один раз в bootstrap-эффекте
(`client/src/pages/Panel.jsx:192-224`). Отзыв доступа переключает интерфейс на плашку при
следующей загрузке; данные при этом недоступны немедленно, потому что их отдаёт сервер.
**Rationale:** [TECHNICAL] поддерживает US-10 в формулировке user-spec («при следующем
открытии панели»). Фоновый polling добавил бы таймер и обработку гонок с открытыми формами
ради результата, который на уровне данных уже обеспечен `RequireAccess`.
**Alternatives considered:** опрос каждые N секунд — отклонено как не требуемое;
SSE/WebSocket — отклонено: инфраструктуры для них в проекте нет.

### Decision 16: Переключение доступа — POST, а не PATCH/PUT
**Decision:** `POST /api/v1/admin/users/{login}/access` с телом `{"granted": bool}`.
**Rationale:** [TECHNICAL] CORS-обёртка объявляет только `GET, POST, OPTIONS`
(`server/internal/handler/cors.go:33`). PATCH или PUT потребовали бы расширить список
разрешённых методов, то есть изменить общий CORS-контракт ради одного эндпоинта.
**Alternatives considered:** `PATCH /api/v1/admin/users/{login}` — отклонено: расширение
`Access-Control-Allow-Methods` затрагивает все маршруты.

### Decision 17: Внутренний текст ошибки перестаёт попадать в ответ
**Decision:** ветка `default` в `writeAPIDomainError` (`server/internal/handler/http.go:264`)
отдаёт фиксированный текст, а исходная ошибка пишется в лог.
**Rationale:** [TECHNICAL] сегодня ветка возвращает `err.Error()`, а ошибки репозитория
обёрнуты SQL-контекстом (`postgres.go:188`) — детали схемы утекают клиенту. Правка в общей
функции, а не в новом контуре: так закрываются и существующие маршруты, и объём меньше.
**Alternatives considered:** отдельный обработчик ошибок только для нового контура —
отклонено: две расходящиеся конвенции в одном пакете и утечка остаётся на старых маршрутах.

## Data Models

### Миграция `server/migrations/004_access_control.up.sql`

Следующий свободный номер — `004`; трёхзначный префикс обязателен, порядок применения —
лексикографическая сортировка (`server/migrations/migrate.go:25`). Файл применяется одним
`db.Exec` целиком (`migrate.go:43`), что работает благодаря `multiStatements=true` в DSN
(`server/internal/db/db.go:63`). Down-миграций в проекте нет.

Все операторы идемпотентны: MariaDB 11.4 поддерживает `IF NOT EXISTS` в том числе для
`ADD CONSTRAINT` и `CREATE INDEX` — проверено прогоном этой миграции против живого
контейнера `mariadb:11.4` поверх 001+002+003.

```sql
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS role VARCHAR(16) NOT NULL DEFAULT 'user',
  ADD COLUMN IF NOT EXISTS has_access BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS email VARCHAR(255) NULL,
  ADD COLUMN IF NOT EXISTS display_name VARCHAR(255) NULL,
  ADD CONSTRAINT IF NOT EXISTS chk_users_role CHECK (role IN ('user','admin'));

ALTER TABLE oauth_sessions
  ADD COLUMN IF NOT EXISTS yandex_login VARCHAR(255) NULL,
  ADD COLUMN IF NOT EXISTS yandex_email VARCHAR(255) NULL,
  ADD COLUMN IF NOT EXISTS yandex_display_name VARCHAR(255) NULL;

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

CREATE INDEX IF NOT EXISTS idx_users_role_has_access ON users(role, has_access);

UPDATE users SET role = 'admin' WHERE id IN ('ADMIN_LOGIN_1','ADMIN_LOGIN_2');
```

Логины подставляет пользователь перед выкаткой. Для аккаунта, ещё ни разу не входившего,
строки `users` не существует, и `UPDATE` его не затронет — поэтому порядок выкатки такой:
миграция применяется, оба администратора выполняют вход (строки создаются `EnsureUser`),
затем `UPDATE` повторяется вручную через `docker exec poshivon_db mariadb`. Процедура
зафиксирована в задаче деплоя.

### Доменные типы (`server/internal/service/access.go`)

```go
type Role string // "user" | "admin"

type AccessState struct {
    Login         string
    DisplayName   string
    Email         string
    Role          Role
    HasAccess     bool
    RequestStatus string // "" | "pending" | "approved" | "rejected"
}

type UserRecord struct {
    Login         string
    DisplayName   string
    Email         string
    Role          Role
    HasAccess     bool
    RequestStatus string
    RequestedAt   *time.Time
    CreatedAt     time.Time
}

type UserRepository interface {
    EnsureUser(ctx context.Context, login, email, displayName string) error
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

`CreateRequest` возвращает `ErrConflict` при нарушении первичного ключа — вызывающая сторона
не выполняет предварительного чтения (Decision 5).

Форма повторяет house style существующих интерфейсов (`server/internal/service/costing.go:168-183`):
узкие интерфейсы объявлены в `service`, реализованы в `repository`, `ctx` первым аргументом,
через границу проходят только доменные типы.

### Новые доменные ошибки

`server/internal/service/costing.go:15-18` определяет только `ErrInvalidArgument` и `ErrNotFound`.
Добавляются `ErrForbidden` и `ErrConflict`; `writeAPIDomainError`
(`server/internal/handler/http.go:251-266`) получает ветки 403 и 409 и фиксированный текст
на 500 (Decision 17).

### Конфигурация (`server/internal/config/config.go`)

| Переменная | Назначение | Значение по умолчанию |
|---|---|---|
| `SMTP_HOST` | Хост SMTP; пустой отключает отправку | `""` |
| `SMTP_PORT` | Порт SMTP | `587` |
| `SMTP_USER` | Учётная запись SMTP | `""` |
| `SMTP_PASSWORD` | Пароль SMTP | `""` |
| `SMTP_FROM` | Адрес отправителя | `""` |
| `ADMIN_NOTIFY_EMAILS` | Получатели уведомления, через запятую | `""` |
| `CONTACT_EMAIL` | Контакт, показываемый на плашке | `""` |

Разбор — существующими помощниками `envOrDefault` / `envInt` (`config.go:123-155`).

### Контракт API

| Метод и путь | Доступ | Ответы |
|---|---|---|
| `GET /api/v1/access/me` | авторизованный | 200 `{login,display_name,email,role,has_access,request_status,contact_email}`; 401 |
| `POST /api/v1/access/requests` | авторизованный без доступа | 201; 409; 401; 403 (посторонний `Origin`) |
| `GET /api/v1/admin/users` | только `role=admin` | 200 `{items:[UserRecord]}`; 401; 403 |
| `POST /api/v1/admin/users/{login}/access` | только `role=admin` | 204; 400; 401; 403; 404 |
| `/api/v1/users/**` (существующие) | авторизованный с доступом | как раньше, плюс 401 и 403 |

Клиентские адреса существующих маршрутов теряют сегмент `userID`: `/api/v1/users/settings`,
`/api/v1/users/chats`, `/api/v1/users/chats/{chatID}/calculate` и так далее — владелец
определяется сессией.

Все пути лежат под `/api/`, который прод-nginx уже проксирует на `poshivon_app:8080`
(`.github/workflows/deploy.yml:130-233`), поэтому новых правил проксирования для них не нужно.

Коды 401 из нового контура намеренно отличаются от `access_cookie_missing` / `access_expired` /
`access_mismatch`: только эти три значения запускают повторную попытку с refresh на клиенте
(`client/src/utils/yandexAuth.js:14-16`). 403 в retry-логике не участвует вовсе.

## Dependencies

### New packages

Новых модулей не добавляется. Используются пакеты стандартной библиотеки, ранее в проекте
не задействованные:

- `net/smtp` — отправка письма администраторам
- `mime` — RFC 2047 кодирование кириллической темы письма
- `net/mail` — валидация адресов из конфигурации
- `net/http/httptest` — интеграционные тесты обработчиков

### Using existing (from project)

- `gorm.io/gorm`, `gorm.io/driver/mysql` — новые модели и методы в `PostgresRepository`
- `database/sql` — поля личности в `auth.Store`
- `net/http` — middleware и обработчики
- `context` — передача личности вызывающего от middleware к обработчику

## Testing Strategy

**Feature size:** L

В репозитории сегодня один тестовый файл — `server/internal/service/costing_test.go` (293 строки),
стандартный `testing` без внешних зависимостей, ручные стабы интерфейсов, `t.Parallel()`.
HTTP-тестов, фикстур БД и прогона тестов в CI нет. Новые тесты следуют существующему стилю;
HTTP-обвязка строится на `net/http/httptest` из стандартной библиотеки.

Фича меняет модель авторизации всех маршрутов, поэтому проверка авторизации ведётся на двух
уровнях: правила — юнит-тестами, применение правил к маршрутам — интеграционными тестами
через реальный `http.ServeMux` с той же цепочкой middleware, что собирается в `main.go`.

### Unit tests

`server/internal/service/access_test.go` — на стабах интерфейсов, по образцу `costing_test.go`:

- Пользователь без доступа и без заявки: `has_access=false`, `request_status=""`
- Пользователь с ролью `admin` и `has_access=false` считается имеющим доступ (US-14)
- Создание заявки пользователем без доступа: заявка создаётся со статусом `pending`
- `CreateRequest` возвращает `ErrConflict`, когда репозиторий сообщает о нарушении ключа
- Заявка от пользователя, у которого доступ уже есть: `ErrConflict`, обращения к репозиторию нет
- Новая заявка после `rejected` разрешена
- `SetAccess(granted=true)` выставляет флаг и переводит заявку в `approved` с `decided_by`
- `SetAccess(granted=false)` снимает флаг и переводит заявку в `rejected`
- `SetAccess` для несуществующего логина: `ErrNotFound`

`server/internal/service/notifier_test.go`:

- Тема письма с кириллицей кодируется в RFC 2047 encoded-word
- Тело письма содержит логин и email заявителя
- Заявитель без email: письмо формируется, в теле только логин
- Конструктор с пустым `SMTP_HOST` возвращает `nil`
- Ошибка отправки не превращается в панику и возвращается вызывающей стороне

### Integration tests

`server/internal/repository/access_repo_test.go` — **контрактный набор, оформленный функцией
над фабрикой репозитория и запускаемый дважды**: против `MemoryRepository` всегда и против
`PostgresRepository`, когда задана `TEST_DB_DSN` (иначе `t.Skip`). Без этого продовый слой
хранения остаётся непокрытым, а именно на нём `RequireAdmin` строит решение об авторизации.

- `EnsureUser` создаёт строку с `role='user'`, `has_access=false`
- **`EnsureUser` для пользователя с `role='admin'` и `has_access=true` не сбрасывает ни то, ни
  другое** — регрессия на разжалование администраторов при входе (Decision 11)
- `EnsureUser` обновляет `email` и `display_name` при повторном вызове
- `GetUser` возвращает `role` и `has_access` непустыми — ловит потерянный gorm-тег
- `SetAccess` меняет флаг и виден последующему `GetUser`
- `ListUsers` возвращает всех, включая созданных только входом
- `CreateRequest` дважды подряд: второй вызов даёт `ErrConflict`
- `DecideRequest` сохраняет `decided_by` и `decided_at`
- Повторный `CreateRequest` после `rejected` сохраняет прежние `decided_by` и `decided_at`
- **Существующие пути записи не сломаны:** `UpsertSettings` и `CreateChat` для нового
  пользователя проходят после добавления колонок с CHECK (Decision 13)

`server/internal/handler/access_test.go` и `server/internal/handler/middleware_test.go` —
`httptest` + `MemoryRepository` + стаб резолвера сессии (интерфейс вводится ради этого;
`auth.Store` требует реального `*sql.DB` и стабу не поддаётся). Тесты собирают маршруты той
же функцией, что и `main.go`, чтобы проверялась фактическая обвязка, а не её копия:

- `GET /api/v1/access/me` без кук → 401
- `GET /api/v1/access/me` с сессией без `yandex_login` → 401 `session_identity_missing`
- `GET /api/v1/access/me` пользователем без доступа → 200, `has_access=false`
- `POST /api/v1/access/requests` дважды подряд → 201, затем 409
- `GET /api/v1/admin/users` без кук → 401; неадминистратором → 403
- `GET /api/v1/admin/users` администратором → 200, список содержит всех
- `POST /api/v1/admin/users/{other}/access` неадминистратором → 403 **и флаг не изменился**
- `POST /api/v1/admin/users/{self}/access` неадминистратором → 403 **и флаг не изменился**
  (US-13, «включая себя»)
- Тот же запрос без кук → 401 **и флаг не изменился**
- `POST /api/v1/admin/users/{unknown}/access` администратором → 404 **и ни один флаг не изменился**
- Администратором `granted=true` → 204, флаг выставлен; `granted=false` → 204, флаг снят
- Тело с неизвестным полем → 400
- Изменяющий запрос с посторонним `Origin` → 403 **и флаг не изменился** (Decision 8)
- Ошибка нотификатора при создании заявки → ответ по-прежнему 201
- `nil`-нотификатор → ответ 201, отправки нет
- Ошибка репозитория с SQL-текстом внутри → 500 **без этого текста в теле** (Decision 17)

`server/internal/handler/http_test.go` — те же маршруты продукта после перевода на сессию:

- `GET /api/v1/users/chats` без кук → 401
- Тот же запрос пользователем без доступа → 403 (US-16)
- Пользователем с доступом → 200, возвращаются данные **его** владельца
- Данные другого владельца недостижимы: сегмента `userID` в адресе больше нет,
  подстановка чужого логина в тело или query результата не меняет (US-15)
- `POST /auth/yandex` → 404, маршрут удалён (US-17)

`server/internal/auth/store_test.go` (требует `TEST_DB_DSN`, иначе `t.Skip`):

- `CreateSession` сохраняет `yandex_login`, `yandex_email`, `yandex_display_name`
- **`UpdateSessionTokens` сохраняет личность при ротации токенов** — иначе пользователи
  теряют личность примерно через время жизни токена, а сценарий со свежим входом этого
  не воспроизводит

### Client tests

None. В `client/package.json` нет ни тест-раннера, ни скрипта `test`, ни тестовых зависимостей —
создание клиентской тестовой инфраструктуры кратно превышает объём фичи. Клиентские изменения
проверяются `Verify-user` в задачах и через Playwright MCP в Agent Verification Plan.
Из-за этого в клиентских задачах `test-reviewer` заменён на `code-reviewer` — рецензировать
нечего.

### E2E tests

None — E2E-инфраструктуры (раннер как зависимость, CI-джоба) в проекте нет. Сквозные сценарии
покрываются проверками через Playwright MCP в Agent Verification Plan и пост-деплой-задачей.

### CI

Добавляется `.github/workflows/test.yml`: `setup-go` + `go test ./...` на push и pull request,
и он же становится `needs:` для job деплоя в `.github/workflows/deploy.yml`. Сегодня тесты
не запускаются нигде — единственный воркфлоу срабатывает на тег и только собирает образы,
поэтому критерий «все существующие тесты проходят» не имеет механизма исполнения.
Все тесты, кроме помеченных `TEST_DB_DSN`, герметичны и в CI не требуют БД.

## Agent Verification Plan

**Source:** user-spec "How to Verify" section.

### Verification approach

Автоматические тесты покрывают доменные правила, контракт HTTP и обвязку маршрутов.
Сверх них агент проверяет то, что тестами не берётся:

1. **Миграция на реальной MariaDB.** Поднять dev-окружение (`docker compose up -d db app`),
   убедиться, что `poshivon_app` не в crash-loop (миграция применяется на старте с
   `log.Fatalf` при ошибке — `server/cmd/main.go:32-34`), проверить схему через
   `docker exec poshivon_db mariadb`, найти строку `004_access_control` в `schema_migrations`
   и **применить миграцию повторно**, убедившись в идемпотентности.

2. **Сквозной HTTP-сценарий против запущенного сервера** через curl с реальными куками:
   отсутствие доступа → подача заявки → повторная подача (409) → выдача доступа
   администратором → чтение данных → отзыв доступа → тот же запрос данных отклонён (403).
   Ключевые негативные проверки: попытка выдать доступ неадминистратором (в том числе себе)
   завершается 403 **и не меняет флаг в БД**; изменяющий запрос с посторонним `Origin`
   отклонён; `POST /auth/yandex` отвечает 404.

   Замечание по dev-окружению: `docker-compose.yml` не задаёт `APP_STORAGE`, поэтому
   приложение по умолчанию работает на `MemoryRepository`
   (`server/internal/config/config.go:58`, `server/cmd/main.go:115-117`) при том, что
   к MariaDB подключается и миграции применяет. Для проверок против БД нужно явно
   выставить `APP_STORAGE=mysql`.

3. **Отрисовка панели** через Playwright MCP на `localhost` для трёх состояний:
   пользователь без доступа (видна плашка, рабочего интерфейса нет, в network-панели нет
   запросов к `/settings`, `/chats` и `/calculations`); пользователь с доступом, но без прав
   администратора (рабочий интерфейс есть, **пункта «Пользователи» в навигации нет** — US-12);
   администратор (пункт виден, список отрисован с галочками).

Пост-деплой проверяется отдельной задачей: доступность контура за прод-nginx, реальная
доставка письма и корректность прод-кук.

### Tools required

- `bash` + `docker compose` / `docker exec` — миграции, состояние контейнеров и БД
- `curl` — контракт HTTP-эндпоинтов, коды ответов и негативные сценарии
- Playwright MCP — отрисовка плашки, видимость раздела администратора, отсутствие запросов
  данных у пользователя без доступа

## Risks

| Risk | Mitigation |
|------|-----------|
| Миграция падает на проде — контейнер уходит в crash-loop, down-миграций нет (`server/migrations/migrate.go`, `server/cmd/main.go:32-34`) | Миграция идемпотентна и проверена прогоном против `mariadb:11.4`; в задаче деплоя зафиксирована процедура ручного отката через `docker exec poshivon_db mariadb` |
| `EnsureUser` на каждом входе сбрасывает `role`, разжалуя обоих администраторов и оставляя систему без управления доступом | Список обновляемых колонок ограничен `email` и `display_name` (Decision 11); отдельный контрактный тест на обоих репозиториях фиксирует инвариант |
| Добавление колонок доступа в `userModel` ломает `UpsertSettings`, `CreateChat` и `AppendCalculation` через `chk_users_role` — воспроизведено на живой MariaDB | Модель `userModel` остаётся неизменной, колонки доступа читаются и пишутся отдельной моделью (Decision 13); контрактный тест проверяет, что существующие пути записи работают |
| Перевод `/api/v1/users/*` на личность из сессии ломает клиент, если адреса не обновлены синхронно | Серверная и клиентская части — соседние задачи в разных волнах; интеграционные тесты фиксируют новый контракт, `Verify-user` проверяет работу панели целиком |
| Все существующие сессии становятся неопознанными после выкатки (Decision 2) | Ожидаемое поведение, fail-closed; клиент при коде `session_identity_missing` отправляет на повторный вход |
| `default_email` не приходит от Яндекса, если в консоли OAuth-приложения не включено разрешение `login:email` | Письмо формируется и без email; в задаче деплоя зафиксирован пункт «включить разрешение в консоли Яндекса», проверяется вручную пользователем |
| SMTP-креды утекают в репозиторий — pre-commit хуков и gitleaks в проекте нет | Значения только в GitHub Secrets и `.env` на сервере; в задаче деплоя явный запрет на значения в compose-файлах и коде |
| Переменная окружения не доходит до контейнера: `envs:` в `.github/workflows/deploy.yml:126` — allowlist, отсутствующая в нём переменная молча теряется | Задача деплоя проходит все пять переходов и заканчивается проверкой `docker exec poshivon_app env` |
| `/auth/refresh` не проксируется на проде и возвращает SPA с кодом 200 — клиентский retry молча не работает (`.github/workflows/deploy.yml:130-233`, `client/nginx.conf:13-67`) | Проксирование добавляется в задаче деплоя; вынесено в User-Spec Deviations |
| Пользователь без доступа всё ещё грузит данные: эффекты в `client/src/pages/Panel.jsx:230-278` (`/settings`, `/chats`) и `:280-308` (`/calculations`) завязаны на `profile`, а не на `status` | Эффекты получают охрану по признаку доступа; сервер в любом случае отвечает 403; отсутствие запросов проверяется через Playwright MCP |
| Оба администратора теряют доступ из-за снятия галочки | Роль хранится отдельно от флага, проверка — `has_access \|\| role=='admin'` (Decision 10) |
| Новые интерфейсы не реализованы в `MemoryRepository` — dev-окружение перестаёт собираться из-за compile-time assertions (`server/internal/repository/memory.go:28-30`) | Обе реализации создаются в одной задаче; assertions добавляются сразу |
| `ya_access` хранит сырой access-токен Яндекса, `yandex_refresh_token` лежит в БД открытым текстом (`server/internal/auth/store.go:13-14`) | Вне объёма фичи: развязка сессии от upstream-токена — самостоятельный рефакторинг. Зафиксировано как принятый риск; `Decision 12` расширяет скоуп токена, что увеличивает цену утечки |
| Прометеевские метки: `normalizePath` (`server/internal/handler/metrics.go:23-45`) не сворачивает короткие логины | После Decision 6 сегмент `userID` исчезает из адресов `/api/v1/users/*`, а оставшийся `{login}` в админском маршруте доступен только администраторам |

## User-Spec Deviations

- **Расширение US-6 (два админ-аккаунта):** user-spec говорит «конкретные логины будут указаны
  пользователем перед деплоем». Tech-spec фиксирует механизм: логины подставляются в
  `UPDATE users SET role='admin' WHERE id IN (...)` в миграции `004`, причём администратор
  должен сначала хотя бы раз войти. Порядок выкатки описан в задаче деплоя.
  → [PENDING USER APPROVAL]

- **Добавлено: проксирование `/auth/refresh` на проде** (не следует ни из одного требования
  user-spec). Причина: маршрут не проксируется ни воркфлоу-конфигом
  (`.github/workflows/deploy.yml:130-233`), ни `client/nginx.conf:13-67`, из-за чего запрос
  попадает на SPA и возвращает 200 с HTML; клиентская логика обновления сессии
  (`client/src/utils/yandexAuth.js:18-25`) считает это успехом и повторяет запрос со старой
  кукой. Дефект существует до этой фичи, но гейт делает истёкшую сессию постоянно наблюдаемым
  состоянием. → [PENDING USER APPROVAL]

- **Добавлено: воркфлоу прогона тестов в CI.** User-spec этого не требует. Причина: критерий
  «все существующие тесты проходят» сегодня не исполняется нигде — единственный воркфлоу
  срабатывает на тег и только собирает образы. Объём — один файл на ~15 строк.
  → [PENDING USER APPROVAL]

- **Добавлено: запрос OAuth-скоупа `login:email`** (Decision 12). Формально следует из US-5,
  но требует изменения вне кода — включения разрешения в консоли OAuth-приложения Яндекса
  и повторного согласия у уже авторизованных пользователей. → [PENDING USER APPROVAL]

- **Добавлено: поля `users.email` и `users.display_name`.** User-spec не упоминает их
  хранение. Нужны для письма администраторам (US-5) и чтобы администратор в списке понимал,
  кому выдаёт доступ (US-7). → [PENDING USER APPROVAL]

- **Уточнение US-3 (повторная заявка):** user-spec не описывает, что происходит после отказа.
  Tech-spec разрешает подать заявку заново после `rejected`, обновляя строку и сохраняя
  сведения о предыдущем решении (Decision 5). История повторных обращений не сохраняется.
  → [PENDING USER APPROVAL]

- **Уточнение US-10 (отзыв доступа):** user-spec формулирует эффект как «при следующем
  открытии панели». Tech-spec даёт более сильную гарантию на уровне данных: следующий же
  запрос к API отклоняется, независимо от того, перезагрузил ли пользователь вкладку
  (Decision 15). → [PENDING USER APPROVAL]

- **Изменение клиентских адресов API.** Существующие вызовы теряют сегмент `userID`
  (`/api/v1/users/{id}/chats` → `/api/v1/users/chats`), потому что владелец определяется
  сессией (Decision 6). User-spec адресов не описывает; изменение видно только внутри кода.
  → [PENDING USER APPROVAL]

## Acceptance Criteria

- [ ] Миграция `004_access_control` применяется на чистой MariaDB 11.4 и на БД с текущей
      прод-схемой без ошибок; повторный прогон того же файла также проходит
- [ ] `MemoryRepository` и `PostgresRepository` оба реализуют `UserRepository` и
      `AccessRequestRepository`; compile-time assertions добавлены для обеих реализаций
- [ ] `EnsureUser` при повторном входе не изменяет `role` и `has_access` — проверено
      контрактным тестом на обоих репозиториях
- [ ] `UpsertSettings`, `CreateChat` и расчёт работают для нового пользователя после
      добавления колонок с CHECK-констрейнтом
- [ ] `GET /api/v1/access/me` без валидных кук возвращает 401
- [ ] `GET /api/v1/admin/users` неадминистратором возвращает 403
- [ ] `POST /api/v1/admin/users/{login}/access` неадминистратором возвращает 403 и не изменяет
      флаг доступа — в том числе когда `{login}` равен логину вызывающего
- [ ] Изменяющий запрос с посторонним `Origin` отклоняется и не изменяет состояние
- [ ] Повторная подача заявки при статусе `pending` возвращает 409
- [ ] Подача заявки пользователем, у которого доступ уже есть, возвращает 409
- [ ] Пользователь с `role='admin'` и `has_access=false` проходит проверку доступа
- [ ] Запрос к `/api/v1/users/**` без кук возвращает 401, а без выданного доступа — 403
- [ ] Пользователь не может получить данные другого владельца ни через адрес, ни через тело запроса
- [ ] `POST /auth/yandex` возвращает 404
- [ ] `UpdateSessionTokens` сохраняет личность сессии при ротации токенов
- [ ] Ошибка отправки письма не влияет на код ответа при создании заявки
- [ ] При незаданном `SMTP_HOST` сервер стартует, пишет предупреждение в лог, заявки создаются
- [ ] Строка `users` появляется после входа, до каких-либо действий в панели
- [ ] Пользователь без доступа не отправляет запросов к `/settings`, `/chats`, `/calculations`
- [ ] Пользователь без прав администратора не видит пункт «Пользователи» в навигации
- [ ] 403 и 409 корректно отображаются в `writeAPIDomainError`
- [ ] Ответ 500 не содержит текста внутренней ошибки
- [ ] Все существующие тесты проходят; регрессий в `costing_test.go` нет
- [ ] `go test ./...` запускается в CI и блокирует деплой при падении
- [ ] Новые переменные окружения доходят до контейнера `poshivon_app` на проде
- [ ] Ни один SMTP-секрет не присутствует в репозитории

## Implementation Tasks

### Wave 1 (independent)

#### Task 1: Миграция схемы доступа
- **Description:** Создать `004_access_control.up.sql` с колонками роли, доступа, email и
  отображаемого имени на `users`, полями личности на `oauth_sessions` и таблицей
  `access_requests`. Нужно как фундамент для всей фичи. Результат: миграция применяется на
  MariaDB 11.4 поверх текущей схемы и остаётся идемпотентной при повторном прогоне.
  Схема — в разделе Data Models.
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-smoke:** `docker compose up -d db app && sleep 20 && docker compose logs app | tail -30` → нет `Ошибка применения миграций`; `docker exec poshivon_db mariadb -uposhivon -pposhivon poshivon -e "SHOW COLUMNS FROM users; SHOW COLUMNS FROM oauth_sessions; SHOW CREATE TABLE access_requests; SELECT version FROM schema_migrations;"` → присутствуют все новые колонки, таблица и версия `004_access_control`; затем повторить содержимое файла через тот же `mariadb` → выполняется без ошибок
- **Files to modify:** `server/migrations/004_access_control.up.sql`
- **Files to read:** `server/migrations/002_costing_schema.up.sql`, `server/migrations/003_pricing_and_chat_delete.up.sql`, `server/migrations/migrate.go`, `server/internal/db/db.go`

#### Task 2: Отправка письма администраторам
- **Description:** Создать `SMTPNotifier` на `net/smtp` с RFC 2047 кодированием темы и
  добавить в конфигурацию SMTP-переменные, список получателей и контактный email из раздела
  Data Models. Нужно для US-5. Результат: сервис отправляет письмо о новой заявке списку
  администраторов и конструируется в `nil` при незаданном `SMTP_HOST` (Decision 14).
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-smoke:** `docker run --rm -d -p 1025:1025 -p 8025:8025 --name mailpit axllent/mailpit`, отправка тестового письма временным `go run`-скриптом против `SMTP_HOST=localhost SMTP_PORT=1025`, затем `curl -s localhost:8025/api/v1/messages` → письмо присутствует, тема декодируется в кириллицу
- **Files to modify:** `server/internal/service/notifier.go`, `server/internal/service/notifier_test.go`, `server/internal/config/config.go`
- **Files to read:** `server/internal/service/deepseek.go`, `server/cmd/main.go`

### Wave 2 (depends on Wave 1)

#### Task 3: Доменный сервис доступа
- **Description:** Создать `AccessService` с интерфейсами `UserRepository` и
  `AccessRequestRepository`, доменными ошибками `ErrForbidden` и `ErrConflict` и правилами
  из Decisions 3, 5, 10, 11. Нужно как единственное место, где живут правила доступа.
  Результат: правила покрыты юнит-тестами на стабах, включая приоритет роли администратора
  над флагом и отображение нарушения ключа на `ErrConflict`.
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Files to modify:** `server/internal/service/access.go`, `server/internal/service/access_test.go`, `server/internal/service/costing.go`
- **Files to read:** `server/internal/service/costing_test.go`, `server/internal/service/README.md`

### Wave 3 (depends on Wave 2)

#### Task 4: Реализации репозиториев доступа
- **Description:** Реализовать `UserRepository` и `AccessRequestRepository` в
  `PostgresRepository` и `MemoryRepository`, добавить compile-time assertions и контрактный
  набор тестов над фабрикой, запускаемый против обоих хранилищ. Модель `userModel` при этом
  не расширяется (Decision 13). Нужно, потому что продовый слой хранения — основание для
  решений об авторизации. Результат: оба репозитория удовлетворяют интерфейсам, контрактные
  тесты проходят, существующие пути записи не сломаны.
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-smoke:** `docker compose up -d db`, затем `TEST_DB_DSN=... go test ./internal/repository/...` в контейнере `golang:1.25-alpine` → контрактный набор проходит против MariaDB, а не только пропускается
- **Files to modify:** `server/internal/repository/postgres.go`, `server/internal/repository/memory.go`, `server/internal/repository/access_repo_test.go`
- **Files to read:** `server/internal/service/access.go`, `server/internal/repository/README.md`

#### Task 5: Личность в сессии и middleware авторизации
- **Description:** Сохранять личность Яндекса в `oauth_sessions` при входе и при ротации
  токенов, удалить `HandleYandexLogin`, вынести проверку сессии в вид, доступный вне
  `AuthHandler`, и создать `RequireAuth`, `RequireAccess`, `RequireAdmin`, `RequireSameOrigin`
  с передачей личности через `context`. Также добавить ветки 403 и 409 и фиксированный текст
  на 500 в `writeAPIDomainError`. Нужно для US-13, US-15, US-17. Поведение старых сессий —
  Decision 2, проверка источника — Decision 8, текст ошибок — Decision 17.
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-smoke:** `docker compose up -d db && TEST_DB_DSN=... go test ./internal/auth/... ./internal/handler/...` → тесты сохранения личности и её переживания ротации токенов проходят
- **Files to modify:** `server/internal/auth/store.go`, `server/internal/auth/store_test.go`, `server/internal/handler/auth.go`, `server/internal/handler/middleware.go`, `server/internal/handler/http.go`
- **Files to read:** `server/internal/service/access.go`, `server/internal/handler/cors.go`, `server/cmd/main.go`

### Wave 4 (depends on Wave 3)

#### Task 6: Эндпоинты доступа и администрирования, сборка маршрутов
- **Description:** Создать `AccessHandler` с четырьмя маршрутами из раздела Data Models,
  собрать в `main.go` все зависимости и цепочки middleware для всех префиксов `/api/v1/`
  согласно таблице в разделе Architecture, и построить обвязку интеграционных тестов на
  `httptest` — тестов уровня handler в проекте нет. Сборка маршрутов выносится в функцию,
  переиспользуемую тестами, чтобы проверялась фактическая обвязка. Результат: контракт
  эндпоинтов и коды 401/403/404/409 покрыты тестами, включая проверку, что отказ не меняет
  состояние хранилища.
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-smoke:** `curl -i localhost:8080/api/v1/admin/users` → 401; `curl -i -X POST localhost:8080/api/v1/admin/users/someone/access -H 'Content-Type: application/json' -d '{"granted":true}'` → 401; `curl -i -X POST localhost:8080/auth/yandex` → 404
- **Files to modify:** `server/internal/handler/access.go`, `server/internal/handler/access_test.go`, `server/internal/handler/middleware_test.go`, `server/cmd/main.go`
- **Files to read:** `server/internal/handler/http.go`, `server/internal/handler/middleware.go`, `server/internal/service/access.go`

### Wave 5 (depends on Wave 4)

#### Task 7: Перевод существующих маршрутов на личность из сессии
- **Description:** Убрать сегмент `userID` из адресов `/api/v1/users/*` и брать владельца из
  контекста, заполняемого middleware. Нужно для US-15 и US-16: пока владелец приходит из
  адреса запроса, отзыв доступа не имеет эффекта, а данные любого пользователя доступны
  напрямую (Decision 6). Результат: маршруты продукта отвечают 401 без сессии, 403 без
  доступа и отдают данные только владельца сессии.
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-smoke:** `curl -i localhost:8080/api/v1/users/chats` → 401; тот же запрос с куками пользователя без доступа → 403
- **Files to modify:** `server/internal/handler/http.go`, `server/internal/handler/http_test.go`, `server/internal/handler/access.go`
- **Files to read:** `server/internal/handler/middleware.go`, `server/internal/service/costing.go`, `server/api/openapi.yaml`

### Wave 6 (depends on Wave 5)

#### Task 8: Клиентский гейт доступа и правки авторизации
- **Description:** Добавить слой вызовов контура доступа, третий вызов в bootstrap-эффекте
  панели, значение `no-access` в машину состояний `status`, охрану эффектов загрузки данных,
  функциональную плашку запроса доступа, а также `scope` и `state` в OAuth-URL с удалением
  implicit-ветки входа. Обновить адреса существующих вызовов под контракт из Task 7.
  Нужно для US-1..US-4, US-11, US-17. Результат: пользователь без доступа видит плашку
  вместо рабочего интерфейса и не отправляет запросов за данными.
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor
- **Verify-user:** открыть `localhost:5173/panel` аккаунтом без доступа → видна плашка, рабочего интерфейса нет, во вкладке Network нет запросов к `/settings`, `/chats`, `/calculations`; нажать «Запросить доступ» → плашка сообщает, что заявка на рассмотрении, кнопка неактивна; выйти и войти заново → вход проходит успешно
- **Files to modify:** `client/src/utils/accessApi.js`, `client/src/utils/yandexAuth.js`, `client/src/utils/panelApi.js`, `client/src/pages/AuthCallback.jsx`, `client/src/pages/Panel.jsx`, `client/src/components/AccessRequestBanner.jsx`
- **Files to read:** `client/src/App.jsx`, `client/src/hooks/useAuthModal.js`

### Wave 7 (depends on Wave 6)

#### Task 9: Раздел «Пользователи» для администратора
- **Description:** Добавить пункт навигации, видимый только при роли администратора, и секцию
  со списком всех пользователей и переключателем доступа напротив каждого. Нужно для US-7..US-10
  и US-12. Результат: администратор выдаёт и отзывает доступ из панели, обычный пользователь
  раздела не видит; галочка администратора отмечена и недоступна для снятия (Decision 10).
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor
- **Verify-user:** открыть `localhost:5173/panel` администратором → виден пункт «Пользователи», в списке есть все аккаунты с галочками; переключение галочки меняет доступ и сохраняется после перезагрузки; под обычным аккаунтом с доступом пункт не отображается
- **Files to modify:** `client/src/components/AdminUsersSection.jsx`, `client/src/pages/Panel.jsx`
- **Files to read:** `client/src/utils/accessApi.js`, `client/src/components/AccessRequestBanner.jsx`

### Wave 8 (depends on Wave 7)

#### Task 10: Вёрстка плашки и раздела пользователей
- **Description:** Привести плашку запроса доступа и раздел «Пользователи» к оформлению
  панели: существующие блоки `.panel__card`, `.panel__notice`, `.panel__empty`,
  `.panel-chat-list__*` в `client/src/App.css` и модификаторы темы `.panel--light` /
  `.panel--dark`. Нужно, потому что оба компонента созданы предыдущими задачами
  функционально, без оформления. Результат: новые экраны неотличимы по стилю от остальной
  панели в обеих темах и на узком экране.
- **Skill:** layout-writing
- **Reviewers:** layout-reviewer
- **Verify-user:** открыть панель в светлой и тёмной теме, шириной 1440px и 390px → плашка и список пользователей выглядят как остальные карточки панели, ничего не выходит за границы, горизонтальной прокрутки нет
- **Files to modify:** `client/src/components/AccessRequestBanner.jsx`, `client/src/components/AdminUsersSection.jsx`, `client/src/App.css`
- **Files to read:** `client/src/pages/Panel.jsx`, `client/src/index.css`

### Wave 9 (depends on Wave 8)

#### Task 11: Конфигурация деплоя, проксирование и прогон тестов в CI
- **Description:** Провести новые переменные окружения через все пять переходов от секрета
  GitHub до контейнера, добавить проксирование `/auth/refresh` в обе nginx-конфигурации,
  создать воркфлоу прогона `go test ./...` и сделать его обязательным для деплоя, а также
  зафиксировать порядок выкатки администраторов из раздела Data Models. Нужно, потому что
  переменная, отсутствующая в allowlist `envs:`, теряется молча, а тесты сегодня не
  запускаются нигде. Результат: сервер на проде видит новые переменные, `/auth/refresh`
  доходит до бэкенда, падение тестов блокирует деплой.
- **Skill:** deploy-pipeline
- **Reviewers:** code-reviewer, security-auditor, deploy-reviewer
- **Verify-smoke:** `docker exec poshivon_app env | grep -E 'SMTP_|ADMIN_NOTIFY|CONTACT_EMAIL'` → переменные присутствуют; `curl -i -X POST https://<host>/auth/refresh` → ответ JSON от бэкенда, не HTML SPA; `act -W .github/workflows/test.yml` либо push в ветку → джоба тестов отрабатывает
- **Files to modify:** `.github/workflows/deploy.yml`, `.github/workflows/test.yml`, `docker-compose.prod.yml`, `docker-compose.yml`, `client/nginx.conf`
- **Files to read:** `server/internal/config/config.go`, `server/Dockerfile`

### Audit Wave

#### Task 12: Code Audit
- **Description:** Full-feature code quality audit. Read all source files created/modified in this feature (from decisions.md + tech-spec "Files to modify"). Review holistically for cross-component issues: duplicate resource initialization, shared resources compliance with Architecture decisions, architectural consistency. Write audit report.
- **Skill:** code-reviewing
- **Reviewers:** none

#### Task 13: Security Audit
- **Description:** Full-feature security audit. Read all source files created/modified in this feature. Analyze for OWASP Top 10 across all components, cross-component auth/data flow. Write audit report.
- **Skill:** security-auditor
- **Reviewers:** none

#### Task 14: Test Audit
- **Description:** Full-feature test quality audit. Read all test files created in this feature. Verify coverage, meaningful assertions, test pyramid balance across all components. Write audit report.
- **Skill:** test-master
- **Reviewers:** none

### Final Wave

#### Task 15: Pre-deploy QA
- **Description:** Acceptance testing: run all tests, verify acceptance criteria from user-spec and tech-spec
- **Skill:** pre-deploy-qa
- **Reviewers:** none

#### Task 16: Deploy
- **Description:** Deploy + verify logs
- **Skill:** deploy-pipeline
- **Reviewers:** none

#### Task 17: Post-deploy verification
- **Description:** Live environment verification:
  - Контейнер `poshivon_app` поднялся, миграция `004_access_control` применена, нет crash-loop — tool: bash (`docker compose logs`, `docker exec poshivon_db mariadb`)
  - `GET /api/v1/access/me` без кук отвечает 401 через прод-nginx — tool: curl
  - `GET /api/v1/users/chats` без кук отвечает 401, с сессией без доступа — 403 — tool: curl
  - `GET /api/v1/admin/users` неадминистратором отвечает 403 — tool: curl
  - `POST /auth/yandex` отвечает 404 — tool: curl
  - `POST /auth/refresh` доходит до бэкенда, а не до SPA — tool: curl
  - Панель без доступа показывает плашку, под администратором — раздел «Пользователи», под обычным пользователем с доступом пункта нет — tool: Playwright MCP
  - Письмо о новой заявке доставлено на почту администраторов — tool: проверка пользователем (доставка зависит от боевых SMTP-кредов)
  Tools: curl, bash, Playwright MCP
- **Skill:** post-deploy-qa
- **Reviewers:** none
