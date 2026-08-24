package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/RoGogDBD/PoshivOn/internal/auth"
	"github.com/RoGogDBD/PoshivOn/internal/config"
	"github.com/RoGogDBD/PoshivOn/internal/db"
	"github.com/RoGogDBD/PoshivOn/internal/handler"
	"github.com/RoGogDBD/PoshivOn/internal/repository"
	"github.com/RoGogDBD/PoshivOn/internal/service"
	"github.com/RoGogDBD/PoshivOn/migrations"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Ошибка загрузки конфигурации: %v", err)
	}

	database, err := db.Open(cfg)
	if err != nil {
		log.Fatalf("Ошибка подключения к БД: %v", err)
	}
	defer database.Close()

	if err := migrations.Run(database); err != nil {
		log.Fatalf("Ошибка применения миграций: %v", err)
	}

	settingsRepo, chatRepo, calculationRepo, userRepo, accessRequestRepo, cleanup, err := buildRepositories(cfg)
	if err != nil {
		log.Fatalf("Ошибка инициализации репозитория: %v", err)
	}
	defer cleanup()

	store := auth.NewStore(database)
	// AccessService собирается из тех же конкретных репозиториев, что и всё остальное:
	// строка users должна появляться при входе по-настоящему, а не в заглушке.
	accessService := service.NewAccessService(userRepo, accessRequestRepo)
	authHandler := handler.NewAuthHandler(store, cfg, accessService)

	costingService := service.NewCostingService(settingsRepo, chatRepo, calculationRepo)
	deepSeekClient, err := service.NewDeepSeekClient(service.DeepSeekConfig{
		APIKey:        cfg.DeepSeekAPIKey,
		APIEndpoint:   cfg.DeepSeekAPIEndpoint,
		Model:         cfg.DeepSeekModel,
		Timeout:       time.Duration(cfg.DeepSeekTimeoutSec) * time.Second,
		ConnectTimout: 10 * time.Second,
		MaxRetries:    cfg.DeepSeekMaxRetries,
	})
	if err != nil {
		log.Fatalf("Ошибка инициализации DeepSeek клиента: %v", err)
	}

	apiHandler := handler.NewAPIHandler(costingService, deepSeekClient)
	accessHandler := handler.NewAccessHandler(accessService, cfg.ContactEmail)

	mux := newMux(cfg, authHandler, apiHandler, accessHandler, accessService, handler.StoreSessionResolver(store))

	handlerWithCORS := handler.WithCORS(handler.CORSConfig{
		AllowedOrigins: splitCSV(cfg.AllowedOrigins),
	}, handler.WithMetrics(mux))

	server := &http.Server{
		Addr:              fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Handler:           handlerWithCORS,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("HTTP-сервер запущен на %s:%s", cfg.Host, cfg.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Ошибка запуска сервера: %v", err)
	}
}

// newMux — единственное место, где конфигурация превращается в зависимости обвязки.
// Сама таблица маршрутов и цепочки middleware живут в handler.BuildRoutes: её вызывают и
// main(), и тесты пакета handler, поэтому проверяется фактическая сборка, а не её копия.
//
// Здесь остаётся ровно перевод cfg → RouteDeps, и именно он — то место, где ошибка проводки
// (забытый cfg.CookieSecure, неразобранный CSV) скомпилировалась бы и осталась незамеченной
// всеми тестами пакета handler, которые RouteDeps собирают руками. Отсюда отдельный тест на
// него в main_test.go. Обёртки уровня всего сервера (CORS, метрики) — в main().
func newMux(
	cfg *config.Config,
	authHandler *handler.AuthHandler,
	apiHandler *handler.APIHandler,
	accessHandler *handler.AccessHandler,
	accessService *service.AccessService,
	resolveSession handler.SessionResolver,
) *http.ServeMux {
	return handler.BuildRoutes(handler.RouteDeps{
		Auth:           authHandler,
		API:            apiHandler,
		Access:         accessHandler,
		AccessService:  accessService,
		ResolveSession: resolveSession,
		AllowedOrigins: splitCSV(cfg.AllowedOrigins),
		CookieSecure:   cfg.CookieSecure,
	})
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// buildRepositories отдаёт все интерфейсы хранилища, которые нужны сервисам. Конкретный
// репозиторий один и тот же на все пять: и настройки с чатами, и пользователей с заявками
// обслуживает одна реализация — выбор хранилища от этого не зависит.
func buildRepositories(cfg *config.Config) (
	service.UserSettingsRepository,
	service.ChatRepository,
	service.ChatCalculationRepository,
	service.UserRepository,
	service.AccessRequestRepository,
	func(),
	error,
) {
	switch strings.ToLower(cfg.Storage) {
	case "", "memory":
		repo := repository.NewMemoryRepository()
		return repo, repo, repo, repo, repo, func() {}, nil
	case "postgres", "mysql", "mariadb":
		dbConn, err := db.OpenGORM(cfg)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, fmt.Errorf("open sql connection: %w", err)
		}

		repo := repository.NewPostgresRepository(dbConn)
		return repo, repo, repo, repo, repo, func() {
			sqlDB, err := dbConn.DB()
			if err == nil {
				_ = sqlDB.Close()
			}
		}, nil
	case "http":
		// noProxyTransport: ни metadata-сервис (169.254.169.254), ни db-service никогда не
		// должны идти через прокси — оба всегда внутри одной сети Yandex Cloud. По умолчанию
		// http.Client слушается HTTP_PROXY/HTTPS_PROXY из окружения (http.ProxyFromEnvironment);
		// явное отключение — дешёвая защита в глубину на случай, если эти переменные когда-то
		// окажутся выставлены по ошибке (security audit, HTTPRepository client).
		noProxyTransport := &http.Transport{Proxy: nil}

		// Токен берётся с metadata-сервиса Yandex Cloud: тот же самый сервисный аккаунт,
		// под которым запущена ревизия бэкенда (--service-account-id), — impersonation не
		// нужен, это не отладочный путь, а штатная авторизация инстанса перед db-service.
		// Таймаут короткий и свой, отдельный от клиента db-service ниже: metadata — локальный
		// для инстанса сервис, отвечающий обычно за миллисекунды; при зависшем ответе он не
		// должен подвешивать запрос дольше, чем реально нужно (code review, security audit —
		// раньше здесь стоял http.DefaultClient без таймаута вовсе).
		metadataClient := &http.Client{Timeout: 5 * time.Second, Transport: noProxyTransport}
		tokens := repository.NewMetadataTokenSource(metadataClient)

		// Таймаут клиента должен покрывать холодный старт db-service (execution-timeout
		// его ревизии, сейчас 90с — см. память проекта poshivon-backend-migration), иначе
		// легитимный медленный первый запрос после простоя обрывался бы здесь раньше, чем
		// на стороне db-service.
		httpClient := &http.Client{Timeout: 100 * time.Second, Transport: noProxyTransport}
		repo := repository.NewHTTPRepository(cfg.DBServiceURL, httpClient, tokens)
		return repo, repo, repo, repo, repo, func() {}, nil
	default:
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("неподдерживаемый APP_STORAGE=%q", cfg.Storage)
	}
}
