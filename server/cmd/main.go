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
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

	mux := newMux(authHandler, apiHandler)

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

// newMux собирает таблицу маршрутов. Вынесено из main() отдельной функцией ради того,
// чтобы состав маршрутов проверялся тестом (main_test.go), а не только компилятором:
// go build ловит ссылку на удалённый обработчик, но не забытую регистрацию — а именно
// рассинхрон между удалением HandleYandexLogin и его регистрацией тех-спек уже ловил
// однажды на собственной валидации. Обёртки уровня всего сервера (CORS, метрики) остаются
// в main(): здесь только маршруты.
func newMux(authHandler *handler.AuthHandler, apiHandler *handler.APIHandler) *http.ServeMux {
	mux := http.NewServeMux()

	apiHandler.Register(mux)

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// Простейший healthcheck для проверки доступности сервиса.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/metrics", promhttp.Handler())
	// Маршрута /auth/yandex здесь нет намеренно: приём готового OAuth-токена от клиента
	// удалён вместе с обработчиком (Decision 7) — он создавал сессию, не проверяя, какому
	// приложению выдан токен, а из сессии теперь выводится роль администратора.
	mux.HandleFunc("/auth/yandex/code", authHandler.HandleYandexCode)
	mux.HandleFunc("/auth/status", authHandler.HandleStatus)
	mux.HandleFunc("/auth/me", authHandler.HandleMe)
	mux.HandleFunc("/auth/refresh", authHandler.HandleRefresh)
	mux.HandleFunc("/auth/logout", authHandler.HandleLogout)

	return mux
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
	default:
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("неподдерживаемый APP_STORAGE=%q", cfg.Storage)
	}
}
