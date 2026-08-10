package config

import "testing"

// TestLoad_ContactEmail: CONTACT_EMAIL действительно читается из окружения.
//
// Тест, проверяющий только «значение из Config доезжает до ответа», прошёл бы и на поле,
// которое Load() никогда не заполняет: тогда на проде плашка показывала бы пустой контакт
// при заданной переменной. Здесь проверяется вторая половина пути — от окружения до Config.
func TestLoad_ContactEmail(t *testing.T) {
	// Load() ищет .env вверх по дереву и подставляет из него незаданные переменные. Пустой
	// временный каталог делает тест независимым от того, что лежит в .env у разработчика:
	// иначе строка CONTACT_EMAIL в локальном файле ломала бы ветку «по умолчанию пусто».
	t.Run("значение берётся из окружения", func(t *testing.T) {
		t.Chdir(t.TempDir())
		t.Setenv("CONTACT_EMAIL", "help@poshivon.example")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.ContactEmail != "help@poshivon.example" {
			t.Errorf("ContactEmail = %q, ожидался help@poshivon.example", cfg.ContactEmail)
		}
	})

	t.Run("по умолчанию пусто", func(t *testing.T) {
		// Переменная не задана — плашка обходится без контакта, а не падает.
		t.Chdir(t.TempDir())
		t.Setenv("CONTACT_EMAIL", "")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.ContactEmail != "" {
			t.Errorf("ContactEmail = %q, ожидалась пустая строка", cfg.ContactEmail)
		}
	})
}

// TestLoad_OriginFallbackGuard — SEC-11-08/C1: пустой CORS_ALLOWED_ORIGINS вместе с
// COOKIE_SECURE=false переводит RequireSameOrigin на сравнение с r.Host, которое ломается
// почти любым прокси. На APP_STORAGE=memory (заведомо не прод) это только предупреждение —
// Load() обязан вернуться успешно, иначе локальный запуск без единой настроенной переменной
// стал бы невозможен. На любом другом хранилище (реальная БД — сигнал настоящего деплоя)
// Load() обязан отказать явной ошибкой, а не оставить сервер стартовать в конфигурации,
// которая отклонит собственный логин без единой подсказки о причине.
func TestLoad_OriginFallbackGuard(t *testing.T) {
	setEnv := func(t *testing.T, storage, origins, secure string) {
		t.Helper()
		t.Chdir(t.TempDir())
		t.Setenv("APP_STORAGE", storage)
		t.Setenv("CORS_ALLOWED_ORIGINS", origins)
		t.Setenv("COOKIE_SECURE", secure)
	}

	t.Run("memory + пусто + insecure — предупреждение, не отказ", func(t *testing.T) {
		setEnv(t, "memory", "", "false")

		if _, err := Load(); err != nil {
			t.Fatalf("Load() вернул ошибку на memory-хранилище: %v", err)
		}
	})

	t.Run("mysql + пусто + insecure — жёсткий отказ", func(t *testing.T) {
		setEnv(t, "mysql", "", "false")

		if _, err := Load(); err == nil {
			t.Fatal("Load() не вернул ошибку на реальном хранилище с небезопасной комбинацией")
		}
	})

	t.Run("mysql + пусто из пробелов и запятых + insecure — тоже отказ", func(t *testing.T) {
		// " ,, " непусто как строка, но splitCSV (main.go) превращает это в пустой allowlist —
		// проверка обязана смотреть на разобранный результат, а не на сырое значение.
		setEnv(t, "mysql", " ,, ", "false")

		if _, err := Load(); err == nil {
			t.Fatal("Load() не вернул ошибку на allowlist, пустом после разбора CSV")
		}
	})

	t.Run("mysql + непустой allowlist + insecure — успех", func(t *testing.T) {
		setEnv(t, "mysql", "https://poshivon.ru", "false")

		if _, err := Load(); err != nil {
			t.Fatalf("Load() вернул ошибку при заданном allowlist: %v", err)
		}
	})

	t.Run("mysql + пусто + secure — успех", func(t *testing.T) {
		setEnv(t, "mysql", "", "true")

		if _, err := Load(); err != nil {
			t.Fatalf("Load() вернул ошибку при COOKIE_SECURE=true: %v", err)
		}
	})
}
