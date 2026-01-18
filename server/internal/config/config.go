package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Host        string
	Port        string
	DatabaseURL string
	LogLevel    string
	DBHost      string
	DBPort      string
	DBName      string
	DBUser      string
	DBPassword  string

	CookieDomain   string
	CookiePath     string
	CookieSecure   bool
	CookieSameSite string

	YandexClientID     string
	YandexClientSecret string
	YandexTokenURL     string
	YandexRedirectURI  string
	YandexUserInfoURL  string

	AllowedOrigins  string
	RefreshTTLHours int
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
		LogLevel:    envOrDefault("LOG_LEVEL", "info"),
		DBHost:      envOrDefault("DB_HOST", "127.0.0.1"),
		DBPort:      envOrDefault("DB_PORT", "3306"),
		DBName:      envOrDefault("DB_NAME", "poshivon"),
		DBUser:      envOrDefault("DB_USER", "poshivon"),
		DBPassword:  envOrDefault("DB_PASSWORD", "poshivon"),

		CookieDomain:   envOrDefault("COOKIE_DOMAIN", ""),
		CookiePath:     envOrDefault("COOKIE_PATH", "/"),
		CookieSecure:   envBool("COOKIE_SECURE", false),
		CookieSameSite: envOrDefault("COOKIE_SAMESITE", "Lax"),

		YandexClientID:     envOrDefault("YANDEX_CLIENT_ID", envOrDefault("VITE_YA_CLIENT_ID", "")),
		YandexClientSecret: envOrDefault("YANDEX_CLIENT_SECRET", envOrDefault("VITE_YA_CLIENT_SECRET", "")),
		YandexTokenURL:     envOrDefault("YANDEX_TOKEN_URL", "https://oauth.yandex.ru/token"),
		YandexRedirectURI:  envOrDefault("YANDEX_REDIRECT_URI", envOrDefault("VITE_YA_REDIRECT_URI", "")),
		YandexUserInfoURL:  envOrDefault("YANDEX_USERINFO_URL", "https://login.yandex.ru/info"),

		AllowedOrigins:  envOrDefault("CORS_ALLOWED_ORIGINS", ""),
		RefreshTTLHours: envInt("REFRESH_TTL_HOURS", 720),
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

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
