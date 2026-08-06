# Execution Plan: Доступ к панели по заявке с одобрением администратором (Phase 1 — MVP)

**Created:** 2026-08-06

---

## Wave 1 (independent)

### Task 1: Миграция схемы доступа
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-smoke:** `docker compose up -d db app && sleep 20 && docker compose logs app | tail -30` → нет `Ошибка применения миграций`; повторный прогон файла — без ошибок, ровно 2 строки `role='admin'`

## Wave 2 (depends on Wave 1)

### Task 2: Доменный сервис доступа
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer

## Wave 3 (depends on Wave 2)

### Task 3: Реализации репозиториев доступа
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-smoke:** контрактный набор тестов проходит против реальной MariaDB (`TEST_DB_DSN`), не только пропускается

## Wave 4 (depends on Wave 2, Wave 3 — Task 4 needs Task 3's concrete repos)

### Task 4: Личность в сессии и middleware авторизации
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-smoke:** `go build ./...` собирается целиком (ловит рассинхрон между удалением `HandleYandexLogin` и его регистрацией в `main.go`); тесты сохранения личности и её переживания ротации токенов — против реальной MariaDB

## Wave 5 (depends on Wave 3, Wave 4)

### Task 5: Эндпоинты доступа и администрирования, сборка маршрутов
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-smoke:** `curl` на `/api/v1/admin/users` без кук → 401; тот же POST без `Origin` → 403; `POST /auth/yandex` → 404

## Wave 6 (depends on Wave 5)

### Task 6: Перевод существующих маршрутов на личность из сессии
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor, test-reviewer
- **Verify-smoke:** `curl /api/v1/users/chats` без кук → 401; с сессией без доступа → 403

## Wave 7 (depends on Wave 6)

### Task 7: Клиентский гейт доступа и правки авторизации
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor
- **Verify-user:** открыть панель без доступа → плашка вместо интерфейса, нет запросов к `/settings`/`/chats`/`/calculations`; подать заявку → статус «на рассмотрении»; старый URL с `#access_token=...` не создаёт сессию; подменённый `state` прерывает вход

## Wave 8 (depends on Wave 7)

### Task 8: Раздел «Пользователи» для администратора
- **Skill:** code-writing
- **Reviewers:** code-reviewer, security-auditor
- **Verify-user:** администратор видит пункт «Пользователи», список со всеми аккаунтами, переключение галочки работает и переживает перезагрузку; обычный пользователь пункт не видит

## Wave 9 (depends on Wave 8)

### Task 9: Конфигурация деплоя, проксирование и прогон тестов в CI
- **Skill:** deploy-pipeline
- **Reviewers:** code-reviewer, security-auditor, deploy-reviewer
- **Verify-smoke:** `CONTACT_EMAIL` доходит до контейнера; `/auth/refresh` отвечает от бэкенда, не SPA; джоба `test` блокирует `build-and-deploy` при падении

## Wave 10 (Audit Wave — depends on Waves 1-9, parallel, reviewers: none)

### Task 10: Code Audit
- **Skill:** code-reviewing

### Task 11: Security Audit
- **Skill:** security-auditor

### Task 12: Test Audit
- **Skill:** test-master

## Wave 11 (depends on Wave 10)

### Task 13: Pre-deploy QA
- **Skill:** pre-deploy-qa
- Прогон полного тестового набора против реальной MariaDB, сверка Phase-1 Acceptance Criteria

## Wave 12 (depends on Wave 11) — ⚠️ реальный деплой на прод

### Task 14: Deploy
- **Skill:** deploy-pipeline
- **Verify-smoke:** тег → workflow → джоба `test` проходит перед `build-and-deploy`; логи контейнера без crash-loop; версия `004_access_control` в `schema_migrations`

## Wave 13 (depends on Wave 12) — ⚠️ проверка живого прод-окружения

### Task 15: Post-deploy verification
- **Skill:** post-deploy-qa
- Playwright MCP + curl против `https://poshivon.ru` — гейт, роли, коды ответов на живом окружении

## User decision on scope

Approved: Waves 1-11 run automatically (through Pre-deploy QA). Wave 12 (Deploy) and Wave 13
(Post-deploy verification) are **not** started automatically — the lead stops after Wave 11
and waits for explicit user go-ahead before touching production.

## Checks requiring user involvement

- [ ] Task 7: ручная проверка плашки/гейта в браузере
- [ ] Task 8: ручная проверка раздела «Пользователи»
- [ ] **Wave 12 (Task 14) — реальный деплой на прод.** Требует тега/пуша и реального обновления боевого окружения. Нужно явное подтверждение перед стартом этой волны.
- [ ] **Wave 13 (Task 15) — проверка живого прод-окружения**, включая доставку письма (US-5, если уже настроено) — доставку письма агент подтвердить не может, это ручная проверка пользователем.
- [ ] После всех волн: финальная приёмка результата целиком
