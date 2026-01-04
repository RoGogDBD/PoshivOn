COMPOSE = docker compose

.PHONY: up down build logs restart

# Сборка контейнеров.
build:
	$(COMPOSE) build

# Запуск окружения.
up:
	$(COMPOSE) up -d

# Остановка окружения.
down:
	$(COMPOSE) down

# Просмотр логов.
logs:
	$(COMPOSE) logs -f

# Перезапуск окружения.
restart:
	$(COMPOSE) down
	$(COMPOSE) up -d