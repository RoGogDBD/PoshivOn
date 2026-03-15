package config

import (
	"os"
	"strings"
)

type Config struct {
	Host        string
	Port        string
	DatabaseURL string
	Storage     string
	LogLevel    string
}

func Load() (*Config, error) {
	envPaths := []string{
		".env",
		"../.env",
		"../../.env",
		"../../../.env",
	}

	for _, path := range envPaths {
		loadEnvFile(path)
	}

	cfg := &Config{
		Host:        envOrDefault("APP_HOST", "0.0.0.0"),
		Port:        envOrDefault("APP_PORT", "8080"),
		DatabaseURL: envOrDefault("DATABASE_URL", ""),
		Storage:     envOrDefault("APP_STORAGE", "memory"),
		LogLevel:    envOrDefault("LOG_LEVEL", "info"),
	}

	return cfg, nil
}

func loadEnvFile(filePath string) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}

		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
