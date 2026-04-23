# PoshivOn

<p align="center">
  <img src="./client/public/favicon.png" alt="PoshivOn" width="220">
</p>

<p align="center">
  Сервис для расчёта стоимости пошива с настраиваемыми правилами, историей расчётов, авторизацией через Яндекс ID и AI-подсказками по рыночному позиционированию заказа.
</p>

<p align="center">
  <a href="#обзор">Обзор</a> •
  <a href="#возможности">Возможности</a> •
  <a href="#архитектура">Архитектура</a> •
  <a href="#быстрый-старт">Быстрый старт</a> •
  <a href="#переменные-окружения">Переменные окружения</a> •
  <a href="#api">API</a>
</p>

## Обзор

`PoshivOn` собран вокруг практической задачи ателье, мастерских и небольших производств: быстро считать стоимость пошива без ручных таблиц, отдельных калькуляторов и разрозненных формул. В одном проекте сведены лендинг, рабочая панель, пользовательские настройки калькуляции, история расчётов по чатам и интеграция с DeepSeek для оценки рыночного диапазона цены.

Во фронтенде хранится вся прикладная часть работы с заказом: тарифы, материалы, операции, срочность, скидки по объёму и история. Бэкенд принимает эти данные, сохраняет настройки, считает заказ в режимах `quick` и `masterpiece`, а при необходимости дополняет результат AI-комментарием по рынку.

> [!NOTE]
> Сервер поддерживает `memory`-хранилище и SQL-режим через `APP_STORAGE=mysql|mariadb|postgres`. В текущем Docker-окружении проект поднимается с `MariaDB`.

## Возможности

- Два режима калькуляции: быстрый расчёт и детализированная калькуляция себестоимости.
- Настраиваемые правила пользователя: изделия, операции, материалы, срочность, наценки, скидки по партиям.
- История расчётов по чатам с созданием, удалением и восстановлением.
- Авторизация через Яндекс ID с cookie-сессиями и refresh-механикой.
- AI-анализ через DeepSeek для оценки рыночного сегмента, рисков и рекомендаций.
- OpenAPI-спецификация для backend API.
- Готовый Docker Compose для локального запуска и production-компоновка с GHCR + Nginx.

## Интерфейс и сценарий

Основной поток внутри сервиса:

```text
+------------------+     +------------------------+     +------------------+
| Яндекс ID        | --> | Панель настроек        | --> | Чат расчёта      |
| вход и сессия    |     | изделия, материалы,    |     | заказ / клиент   |
|                  |     | операции, скидки       |     |                  |
+------------------+     +------------------------+     +------------------+
                                                               |
                                                               v
                 +---------------------------+     +------------------------+
                 | Режим quick/masterpiece   | --> | Итог расчёта           |
                 | расчёт цены и себестоимости|    | история + AI-оценка    |
                 +---------------------------+     +------------------------+
```

В обычном сценарии пользователь входит через Яндекс ID, настраивает собственные правила ценообразования, создаёт чат под заказ или клиента и запускает расчёт партии. На выходе сервис возвращает итоговую стоимость, детализацию по составляющим и, в режиме `masterpiece`, комментарий по рыночному позиционированию.

## Архитектура

Проект состоит из трёх основных частей:

- `client/` — React + Vite интерфейс: лендинг, авторизация, рабочая панель и история расчётов.
- `server/` — Go API: конфигурация, OAuth-обработчики, калькулятор стоимости, интеграция с DeepSeek, миграции.
- `deploy/` и `docker-compose*.yml` — локальное и production-развёртывание через контейнеры.

Схема компонентов:

```text
+---------------------------+        HTTP / JSON        +---------------------------+
| client/                   | ----------------------->  | server/                   |
| React + Vite              |                           | Go API + handlers         |
| лендинг + panel           | <-----------------------  | auth + costing + DeepSeek |
+---------------------------+                           +---------------------------+
                                                                    |
                                               +--------------------+--------------------+
                                               |                                         |
                                               v                                         v
                                  +------------------------+                +------------------------+
                                  | MariaDB                |                | DeepSeek API           |
                                  | настройки, чаты,       |                | market feedback,       |
                                  | история расчётов       |                | рекомендации           |
                                  +------------------------+                +------------------------+
```

Технологический стек:

- Frontend: `React 18`, `Vite 5`, `Tailwind CSS 4`
- Backend: `Go 1.25`, `net/http`, `GORM`
- Database: `MariaDB 11.4`
- Auth: `Yandex OAuth`
- AI: `DeepSeek Chat Completions API`
- Infra: `Docker Compose`, `Nginx`, `GitHub Actions`, `GHCR`

## Структура проекта

```text
.
├── client/                  # React-приложение и статика
├── server/
│   ├── api/openapi.yaml     # контракт API
│   ├── cmd/main.go          # точка входа backend
│   ├── internal/            # handlers, services, config, db, repositories
│   └── migrations/          # SQL-миграции
├── deploy/                  # production-конфиг Nginx
├── docker-compose.yml       # локальное окружение
├── docker-compose.prod.yml  # production-окружение
└── Makefile                 # базовые команды
```

## Быстрый старт

### Требования

- `Docker` и `Docker Compose`
- Для локальной разработки без Docker: `Go 1.25.5+` и `Node.js 20+`
- `Yandex OAuth` приложение для полноценной авторизации
- `DeepSeek API key` для включения AI-анализа

### Запуск через Docker Compose

Локально проект удобнее всего смотреть через `Docker Compose`. Достаточно подготовить `.env` в корне репозитория и поднять окружение:

```bash
make up
```

После старта будут доступны:

- `http://localhost:5173` — web
- `http://localhost:8080/health` — healthcheck backend
- `localhost:3306` — MariaDB

> [!TIP]
> `client/nginx.conf` уже проксирует `/api`, `/auth` и `/health` во внутренний сервис `app`, поэтому локальный запуск не требует отдельного reverse proxy.

### Локальная разработка без Docker

Если удобнее запускать части проекта отдельно, backend и frontend можно поднимать независимо.

Backend:

```bash
cd server
go run ./cmd
```

Frontend:

```bash
cd client
npm install
npm run dev
```

В dev-режиме `Vite` сам проксирует `/api`, `/auth` и `/health` на `http://127.0.0.1:8080`, поэтому фронтенд остаётся привязан к локальному backend без дополнительной настройки.

## Переменные окружения

Ниже минимальный набор переменных, с которым проект можно поднять локально:

```env
APP_STORAGE=mysql
APP_HOST=0.0.0.0
APP_PORT=8080

DB_HOST=db
DB_PORT=3306
DB_NAME=poshivon
DB_USER=poshivon
DB_PASSWORD=poshivon

CORS_ALLOWED_ORIGINS=http://localhost:5173

YANDEX_CLIENT_ID=
YANDEX_CLIENT_SECRET=
YANDEX_REDIRECT_URI=http://localhost:5173/auth

DEEPSEEK_API_KEY=
DEEPSEEK_API_ENDPOINT=https://api.deepseek.com/v1/chat/completions
DEEPSEEK_MODEL=deepseek-chat
DEEPSEEK_TIMEOUT_SEC=45
DEEPSEEK_MAX_RETRIES=3
```

Основные переменные:

| Переменная | Назначение |
| --- | --- |
| `APP_STORAGE` | Режим хранилища: `memory`, `mysql`, `mariadb`, `postgres` |
| `DB_*` | Подключение к SQL-базе |
| `CORS_ALLOWED_ORIGINS` | Разрешённые origin'ы фронтенда |
| `YANDEX_CLIENT_ID` / `YANDEX_CLIENT_SECRET` | OAuth-приложение Яндекса |
| `YANDEX_REDIRECT_URI` | URL возврата после авторизации |
| `DEEPSEEK_API_KEY` | Включает AI-анализ рынка |
| `COOKIE_SECURE`, `COOKIE_SAMESITE`, `COOKIE_DOMAIN` | Настройки cookie-сессий |
| `REFRESH_TTL_HOURS` | Срок жизни refresh-сессии |

> [!IMPORTANT]
> Без `DEEPSEEK_API_KEY` backend продолжит работать штатно, но AI-анализ рынка останется выключенным.

## API

Контракт API описан в [server/api/openapi.yaml](./server/api/openapi.yaml).

Ключевые маршруты:

- `GET /health` — проверка доступности сервиса
- `GET|POST /api/v1/users/{userID}/settings` — загрузка и сохранение настроек
- `GET|POST /api/v1/users/{userID}/chats` — список и создание чатов
- `DELETE /api/v1/users/{userID}/chats/{chatID}` — удаление чата
- `POST /api/v1/users/{userID}/chats/{chatID}/restore` — восстановление чата
- `POST /api/v1/users/{userID}/chats/{chatID}/calculate` — расчёт стоимости заказа
- `GET /api/v1/users/{userID}/chats/{chatID}/calculations` — история расчётов
- `POST /api/v1/users/{userID}/market-feedback` — AI-анализ по рынку

Для быстрой проверки backend:

```bash
curl http://localhost:8080/health
```

Пример запроса на расчёт:

```bash
curl -X POST http://localhost:8080/api/v1/users/demo/chats/chat-1/calculate \
  -H "Content-Type: application/json" \
  -d '{
    "garment_type": "Пиджак",
    "material_type": "Костюмная ткань",
    "urgency": "Стандарт",
    "market_segment": "Средний",
    "quantity": 15,
    "fittings": 1,
    "is_custom_figure": false,
    "is_child": false,
    "operation_counts": {
      "Карман накладной": 2,
      "Подклад": 1
    }
  }'
```

## Production

В репозитории уже лежит базовая production-компоновка:

- `docker-compose.prod.yml`
- `deploy/nginx.conf`
- GitHub Actions workflow `.github/workflows/deploy.yml`

Текущий пайплайн собирает образы `poshivon-app` и `poshivon-web`, публикует их в `GHCR`, после чего разворачивает стек на удалённом сервере через `SSH`.

## Полезные материалы

- [OpenAPI спецификация](./server/api/openapi.yaml)
- [Описание BPMN/UML логики расчёта](./server/docs/bpmn-uml-description.md)
- [README по сервисному слою](./server/internal/service/README.md)

## Статус проекта

Сейчас проект уже закрывает основной сценарий расчёта стоимости пошива и нормально поднимается локально в контейнерах. README намеренно описывает текущее состояние репозитория без лишних обещаний и без секций, которые дублируют отдельные служебные файлы.
