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

	store := auth.NewStore(database)
	authHandler := handler.NewAuthHandler(store, cfg)

	mux := http.NewServeMux()

	settingsRepo, chatRepo, calculationRepo, cleanup, err := buildRepositories(cfg)
	if err != nil {
		log.Fatalf("Ошибка инициализации репозитория: %v", err)
	}
	defer cleanup()

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
	apiHandler.Register(mux)

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// Простейший healthcheck для проверки доступности сервиса.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/auth/yandex", authHandler.HandleYandexLogin)
	mux.HandleFunc("/auth/yandex/code", authHandler.HandleYandexCode)
	mux.HandleFunc("/auth/status", authHandler.HandleStatus)
	mux.HandleFunc("/auth/me", authHandler.HandleMe)
	mux.HandleFunc("/auth/refresh", authHandler.HandleRefresh)
	mux.HandleFunc("/auth/logout", authHandler.HandleLogout)

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

func buildRepositories(cfg *config.Config) (
	service.UserSettingsRepository,
	service.ChatRepository,
	service.ChatCalculationRepository,
	func(),
	error,
) {
	switch strings.ToLower(cfg.Storage) {
	case "", "memory":
		repo := repository.NewMemoryRepository()
		return repo, repo, repo, func() {}, nil
	case "postgres", "mysql", "mariadb":
		dbConn, err := db.OpenGORM(cfg)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("open sql connection: %w", err)
		}

		repo := repository.NewPostgresRepository(dbConn)
		return repo, repo, repo, func() {
			sqlDB, err := dbConn.DB()
			if err == nil {
				_ = sqlDB.Close()
			}
		}, nil
	default:
		return nil, nil, nil, nil, fmt.Errorf("неподдерживаемый APP_STORAGE=%q", cfg.Storage)
	}
}
