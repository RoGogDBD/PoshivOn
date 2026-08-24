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
		// DB_PASSWORD теперь обязателен на любом нереальном хранилище (TestLoad_DBPassword
		// проверяет это отдельно) — этот тест проверяет CORS/cookie-ветку, а не пароль, и
		// падать по другой причине не должен.
		t.Setenv("DB_PASSWORD", "test-password")
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

// TestLoad_DBPassword — раньше пустой DB_PASSWORD на реальном хранилище молча подставлял
// дефолт "poshivon" (значение, видное в исходниках всем). Теперь это явный отказ при старте:
// дешевле обнаружить забытый секрет по логу деплоя, чем по тихому подключению чужим паролем.
func TestLoad_DBPassword(t *testing.T) {
	setEnv := func(t *testing.T, storage, password string) {
		t.Helper()
		t.Chdir(t.TempDir())
		t.Setenv("APP_STORAGE", storage)
		t.Setenv("DB_PASSWORD", password)
		// Вне зоны действия этого теста — заведомо валидные значения, чтобы не споткнуться
		// о TestLoad_OriginFallbackGuard-ветку при storage=mysql.
		t.Setenv("CORS_ALLOWED_ORIGINS", "https://poshivon.ru")
	}

	t.Run("memory + пусто — успех, пароль не используется", func(t *testing.T) {
		setEnv(t, "memory", "")

		if _, err := Load(); err != nil {
			t.Fatalf("Load() вернул ошибку на memory-хранилище: %v", err)
		}
	})

	t.Run("mysql + пусто — жёсткий отказ", func(t *testing.T) {
		setEnv(t, "mysql", "")

		if _, err := Load(); err == nil {
			t.Fatal("Load() не вернул ошибку при пустом DB_PASSWORD на реальном хранилище")
		}
	})

	t.Run("mysql + пароль из одних пробелов — тоже отказ", func(t *testing.T) {
		setEnv(t, "mysql", "   ")

		if _, err := Load(); err == nil {
			t.Fatal("Load() не вернул ошибку при DB_PASSWORD из пробелов")
		}
	})

	t.Run("mysql + задан — успех", func(t *testing.T) {
		setEnv(t, "mysql", "s3cret")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() вернул ошибку при заданном пароле: %v", err)
		}
		if cfg.DBPassword != "s3cret" {
			t.Errorf("DBPassword = %q, ожидался s3cret", cfg.DBPassword)
		}
	})

	t.Run("mariadb (другой регистр Storage) + пусто — тоже отказ", func(t *testing.T) {
		setEnv(t, "MariaDB", "")

		if _, err := Load(); err == nil {
			t.Fatal("Load() не вернул ошибку при пустом DB_PASSWORD с APP_STORAGE в смешанном регистре")
		}
	})

	t.Run("http + пусто — успех, пароль не используется", func(t *testing.T) {
		setEnv(t, "http", "")
		t.Setenv("DB_SERVICE_URL", "https://db-service.example")

		if _, err := Load(); err != nil {
			t.Fatalf("Load() вернул ошибку на http-хранилище без DB_PASSWORD: %v", err)
		}
	})
}

// TestLoad_DBServiceURL — симметрично TestLoad_DBPassword: на APP_STORAGE=http бэкенд ходит
// в db-service по HTTPS и без DB_SERVICE_URL сходил бы в пустой baseURL, провалившись на
// первом же запросе неясной сетевой ошибкой вместо понятного отказа при старте.
func TestLoad_DBServiceURL(t *testing.T) {
	setEnv := func(t *testing.T, storage, dbServiceURL string) {
		t.Helper()
		t.Chdir(t.TempDir())
		t.Setenv("APP_STORAGE", storage)
		t.Setenv("DB_SERVICE_URL", dbServiceURL)
		t.Setenv("CORS_ALLOWED_ORIGINS", "https://poshivon.ru")
	}

	t.Run("http + пусто — жёсткий отказ", func(t *testing.T) {
		setEnv(t, "http", "")

		if _, err := Load(); err == nil {
			t.Fatal("Load() не вернул ошибку при пустом DB_SERVICE_URL на APP_STORAGE=http")
		}
	})

	t.Run("http + DB_SERVICE_URL из пробелов — тоже отказ", func(t *testing.T) {
		setEnv(t, "http", "   ")

		if _, err := Load(); err == nil {
			t.Fatal("Load() не вернул ошибку при DB_SERVICE_URL из пробелов")
		}
	})

	// SEC — HTTPRepository шлёт в этот адрес IAM-токен и пользовательские данные в каждом
	// запросе; не-https адрес увёл бы их в открытый текст молча (security audit).
	t.Run("http + не https схема — отказ", func(t *testing.T) {
		setEnv(t, "http", "http://db-service.example")

		if _, err := Load(); err == nil {
			t.Fatal("Load() не вернул ошибку при DB_SERVICE_URL со схемой http (не https)")
		}
	})

	t.Run("http + не URL вовсе — отказ", func(t *testing.T) {
		setEnv(t, "http", "db-service.example")

		if _, err := Load(); err == nil {
			t.Fatal("Load() не вернул ошибку при DB_SERVICE_URL без схемы")
		}
	})

	t.Run("http + задан https — успех", func(t *testing.T) {
		setEnv(t, "http", "https://db-service.example")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() вернул ошибку при заданном DB_SERVICE_URL: %v", err)
		}
		if cfg.DBServiceURL != "https://db-service.example" {
			t.Errorf("DBServiceURL = %q, ожидался https://db-service.example", cfg.DBServiceURL)
		}
	})

	t.Run("memory + пусто — успех, DB_SERVICE_URL не используется", func(t *testing.T) {
		setEnv(t, "memory", "")

		if _, err := Load(); err != nil {
			t.Fatalf("Load() вернул ошибку на memory-хранилище: %v", err)
		}
	})
}

// TestLoad_Port — PORT приоритетнее APP_PORT (так Yandex Serverless Containers сообщает
// приложению, на каком порту его вызовут), но без PORT в окружении поведение прежнее.
func TestLoad_Port(t *testing.T) {
	t.Run("PORT не задан — используется APP_PORT", func(t *testing.T) {
		t.Chdir(t.TempDir())
		t.Setenv("APP_PORT", "9090")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Port != "9090" {
			t.Errorf("Port = %q, ожидался 9090 (из APP_PORT)", cfg.Port)
		}
	})

	t.Run("PORT задан — имеет приоритет над APP_PORT", func(t *testing.T) {
		t.Chdir(t.TempDir())
		t.Setenv("APP_PORT", "9090")
		t.Setenv("PORT", "8888")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Port != "8888" {
			t.Errorf("Port = %q, ожидался 8888 (из PORT)", cfg.Port)
		}
	})

	t.Run("ничего не задано — дефолт 8080", func(t *testing.T) {
		t.Chdir(t.TempDir())

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Port != "8080" {
			t.Errorf("Port = %q, ожидался дефолт 8080", cfg.Port)
		}
	})
}
