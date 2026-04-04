COMPOSE = docker compose

.PHONY: up down build logs restart

# Сборка контейнеров.
build:
	$(COMPOSE) build

# Запуск окружения.
up:
	NPM_CONFIG_PRODUCTION=false $(COMPOSE) up -d

# Остановка окружения.
down:
	docker compose -f docker-compose.yml down -v --rmi all
	docker system prune -af --volumes

# Просмотр логов.
logs:
	$(COMPOSE) logs -f

# Перезапуск окружения.
restart:
	$(COMPOSE) down
	$(COMPOSE) up -d