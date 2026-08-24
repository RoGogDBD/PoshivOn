// Package dbservice реализует db-service — Serverless Container, в котором вместе живут
// MariaDB и тонкая HTTP-обёртка над существующим репозиторным слоем (server/internal/
// repository). Бэкенд вызывает db-service по HTTPS вместо прямого SQL-подключения; сам
// db-service подключается к своей MariaDB через localhost (см. entrypoint-скрипт в
// server/docker-entrypoint-dbservice.sh).
package dbservice

import (
	"fmt"
	"os"
	"strings"
)

// Config — минимальный набор настроек db-service. Намеренно НЕ переиспользует
// server/internal/config.Config: тот безусловно требует валидную комбинацию
// CORS_ALLOWED_ORIGINS/COOKIE_SECURE (SEC-11-08/C1) — правило для браузерного трафика,
// которого у db-service нет вообще. Общий Config заставил бы этот процесс либо получать
// фиктивные cookie-переменные без всякого смысла, либо не стартовать.
type Config struct {
	// Port — порт, на котором db-service принимает HTTP-вызовы от бэкенда. Serverless
	// Containers выставляет переменную PORT автоматически при запуске ревизии.
	Port string

	DBHost     string
	DBPort     string
	DBName     string
	DBUser     string
	DBPassword string
}

func LoadConfig() (*Config, error) {
	cfg := &Config{
		Port: envOrDefault("PORT", "8081"),

		// 127.0.0.1 — дефолт намеренный: MariaDB и обёртка живут в одном инстансе
		// контейнера, localhost — это и есть штатный путь, а не частный случай.
		DBHost:     envOrDefault("DB_HOST", "127.0.0.1"),
		DBPort:     envOrDefault("DB_PORT", "3306"),
		DBName:     envOrDefault("DB_NAME", "poshivon"),
		DBUser:     envOrDefault("DB_USER", "poshivon"),
		DBPassword: envOrDefault("DB_PASSWORD", ""),
	}

	// Только DB_PASSWORD не имеет дефолта и требует явной проверки: DBHost/DBPort/DBName/
	// DBUser всегда непусты по построению (envOrDefault подставляет разумные значения),
	// а секрет молчаливого дефолта иметь не должен (тот же принцип, что и в основном
	// server/internal/config.Config).
	if strings.TrimSpace(cfg.DBPassword) == "" {
		return nil, fmt.Errorf("dbservice config: DB_PASSWORD is required")
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
