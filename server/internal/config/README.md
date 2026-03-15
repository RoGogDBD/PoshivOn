В этом пакете хранятся конфигурации приложения.

# Config Package

Пакет отвечает за загрузку и управление конфигурацией приложения.

## Возможности

- Чтение переменных окружения
- Загрузка конфигурации из `.env` файла
- Значения по умолчанию для всех параметров
- Приоритет: переменные окружения > .env файл > значения по умолчанию

## Основные переменные окружения

- `APP_HOST`, `APP_PORT`
- `DATABASE_URL` (или `DB_HOST`, `DB_PORT`, `DB_NAME`, `DB_USER`, `DB_PASSWORD`)
- `COOKIE_DOMAIN`, `COOKIE_PATH`, `COOKIE_SECURE`, `COOKIE_SAMESITE`
- `YANDEX_CLIENT_ID` (fallback: `VITE_YA_CLIENT_ID`)
- `YANDEX_CLIENT_SECRET` (fallback: `VITE_YA_CLIENT_SECRET`)
- `YANDEX_TOKEN_URL`
- `YANDEX_REDIRECT_URI` (fallback: `VITE_YA_REDIRECT_URI`)
- `CORS_ALLOWED_ORIGINS`
- `REFRESH_TTL_HOURS`
