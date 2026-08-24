package dbservice

import "testing"

// TestLoadConfig — db-service не использует общий config.Config бэкенда: тот безусловно
// проверяет CORS/cookie-настройки (SEC-11-08/C1), которые db-service не имеет отношения к
// вызывающему браузер трафику вообще, и заставил бы этот процесс либо получать фиктивные
// cookie-переменные, либо не стартовать. Здесь — отдельный, короткий набор из пяти
// переменных, которые реально нужны db-service.
func TestLoadConfig(t *testing.T) {
	setEnv := func(t *testing.T, pairs map[string]string) {
		t.Helper()
		for k, v := range pairs {
			t.Setenv(k, v)
		}
	}

	t.Run("пароль обязателен", func(t *testing.T) {
		setEnv(t, map[string]string{
			"DB_PASSWORD": "",
		})

		if _, err := LoadConfig(); err == nil {
			t.Fatal("LoadConfig() не вернул ошибку при пустом DB_PASSWORD")
		}
	})

	t.Run("пароль из одних пробелов — тоже отказ", func(t *testing.T) {
		setEnv(t, map[string]string{
			"DB_PASSWORD": "   ",
		})

		if _, err := LoadConfig(); err == nil {
			t.Fatal("LoadConfig() не вернул ошибку при DB_PASSWORD из пробелов")
		}
	})

	t.Run("минимальный набор — успех с дефолтами для host/port", func(t *testing.T) {
		setEnv(t, map[string]string{
			"DB_PASSWORD": "s3cret",
			"DB_NAME":     "poshivon",
			"DB_USER":     "poshivon",
			"PORT":        "",
			"DB_HOST":     "",
			"DB_PORT":     "",
		})

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if cfg.DBHost != "127.0.0.1" {
			t.Errorf("DBHost = %q, ожидался 127.0.0.1 (MariaDB в том же контейнере)", cfg.DBHost)
		}
		if cfg.DBPort != "3306" {
			t.Errorf("DBPort = %q, ожидался 3306", cfg.DBPort)
		}
		if cfg.Port != "8081" {
			t.Errorf("Port = %q, ожидался дефолт 8081", cfg.Port)
		}
	})

	t.Run("PORT из окружения переопределяет дефолт", func(t *testing.T) {
		setEnv(t, map[string]string{
			"DB_PASSWORD": "s3cret",
			"PORT":        "9999",
		})

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if cfg.Port != "9999" {
			t.Errorf("Port = %q, ожидался 9999 из окружения", cfg.Port)
		}
	})

	t.Run("DB_USER/DB_NAME не заданы — используются дефолты, не ошибка", func(t *testing.T) {
		// В отличие от DB_PASSWORD, у этих полей есть разумные непустые дефолты
		// (совпадающие с server/internal/config.Config) — это не секреты, молчаливая
		// подстановка тут безопасна.
		setEnv(t, map[string]string{
			"DB_PASSWORD": "s3cret",
			"DB_USER":     "",
			"DB_NAME":     "",
		})

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if cfg.DBUser != "poshivon" {
			t.Errorf("DBUser = %q, ожидался дефолт poshivon", cfg.DBUser)
		}
		if cfg.DBName != "poshivon" {
			t.Errorf("DBName = %q, ожидался дефолт poshivon", cfg.DBName)
		}
	})
}
