package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Host        string
	Port        string
	DatabaseURL string
	Storage     string
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

	// ContactEmail показывается на плашке пользователю без доступа: куда писать, чтобы
	// доступ выдали. Пустое значение — штатная конфигурация, плашка тогда без контакта.
	ContactEmail string

	DeepSeekAPIKey      string
	DeepSeekAPIEndpoint string
	DeepSeekModel       string
	DeepSeekTimeoutSec  int
	DeepSeekMaxRetries  int
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
		ContactEmail:    envOrDefault("CONTACT_EMAIL", ""),

		DeepSeekAPIKey:      envOrDefault("DEEPSEEK_API_KEY", ""),
		DeepSeekAPIEndpoint: envOrDefault("DEEPSEEK_API_ENDPOINT", "https://api.deepseek.com/v1/chat/completions"),
		DeepSeekModel:       envOrDefault("DEEPSEEK_MODEL", "deepseek-chat"),
		DeepSeekTimeoutSec:  envInt("DEEPSEEK_TIMEOUT_SEC", 45),
		DeepSeekMaxRetries:  envInt("DEEPSEEK_MAX_RETRIES", 3),
	}

	// Пустой CORS_ALLOWED_ORIGINS переводит RequireSameOrigin на сравнение Origin с
	// scheme://r.Host (Decision 8), а scheme берётся отсюда же, из COOKIE_SECURE. За https-
	// прокси на порту 443 (неявном и потому отсутствующем в Origin) это совпадает по
	// счастливому стечению обстоятельств; за любым прокси, переписывающим Host (dev-nginx —
	// $host обрезает порт, Vite proxy — changeOrigin), не совпадает вовсе, и любой мутирующий
	// запрос, включая сам вход, получает 403 без единой подсказки о причине (Task 11 audit,
	// SEC-11-08/C1). Молчание здесь дороже лишней строки в логе при старте.
	//
	// Проверка смотрит не на сырую строку, а на то же самое «есть хоть один непустой
	// элемент», что и splitCSV в main.go, которая на самом деле строит allowlist: голая
	// `cfg.AllowedOrigins == ""` пропустила бы значения вроде " " или "," — они непусты как
	// строка, но splitCSV превращает их в пустой список, и RequireSameOrigin всё равно
	// падает в тот же сломанный fallback.
	if !hasNonEmptyCSVEntry(cfg.AllowedOrigins) && !cfg.CookieSecure {
		problem := "CORS_ALLOWED_ORIGINS пуст и COOKIE_SECURE=false — RequireSameOrigin " +
			"сравнивает Origin с http://<r.Host> и почти наверняка отклонит запросы через " +
			"nginx/Vite прокси, включая вход"
		// APP_STORAGE=memory — единственный доступный здесь сигнал «это точно не прод»: он
		// заведомо стоит по умолчанию в локальном прогоне и никогда осознанно — в реальном
		// деплое (там нужна настоящая БД). На нём — предупреждение и запуск: разработчик
		// сам разберётся с 403 при первом же запросе. На любом другом хранилище это уже не
		// «не забыл настроить локально», а неверно настроенный прод/стейджинг с реальными
		// пользователями — там молчаливый провал каждого мутирующего запроса дороже, чем
		// падение при старте с понятной причиной (Task 11 audit, SEC-11-08/C1).
		if cfg.Storage != "memory" {
			return nil, fmt.Errorf("config: %s; задайте оба значения явно (APP_STORAGE=%q)", problem, cfg.Storage)
		}
		log.Printf("config: %s; задайте оба значения явно на проде", problem)
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

// hasNonEmptyCSVEntry повторяет ровно ту часть логики splitCSV (cmd/main.go), от которой
// зависит предупреждение выше: split по запятой + TrimSpace на каждом элементе + отбрасывание
// пустых. Раздельная копия, а не общая функция, — main.go принадлежит пакету main и не может
// быть импортирован отсюда без цикла; здесь достаточно самого правила «есть ли реально
// непустой Origin», не полного результата разбора.
func hasNonEmptyCSVEntry(csv string) bool {
	for _, part := range strings.Split(csv, ",") {
		if strings.TrimSpace(part) != "" {
			return true
		}
	}
	return false
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
