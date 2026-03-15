package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/RoGogDBD/PoshivOn/internal/config"
	"github.com/RoGogDBD/PoshivOn/internal/handler"
	"github.com/RoGogDBD/PoshivOn/internal/repository"
	"github.com/RoGogDBD/PoshivOn/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Ошибка загрузки конфигурации: %v", err)
	}

	mux := http.NewServeMux()

	settingsRepo, chatRepo, cleanup, err := buildRepositories(cfg)
	if err != nil {
		log.Fatalf("Ошибка инициализации репозитория: %v", err)
	}
	defer cleanup()

	costingService := service.NewCostingService(settingsRepo, chatRepo)
	apiHandler := handler.NewAPIHandler(costingService)
	apiHandler.Register(mux)

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// Простейший healthcheck для проверки доступности сервиса.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:              fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("HTTP-сервер запущен на %s:%s", cfg.Host, cfg.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Ошибка запуска сервера: %v", err)
	}
}

func buildRepositories(cfg *config.Config) (
	service.UserSettingsRepository,
	service.ChatCalculationRepository,
	func(),
	error,
) {
	switch strings.ToLower(cfg.Storage) {
	case "", "memory":
		repo := repository.NewMemoryRepository()
		return repo, repo, func() {}, nil
	case "postgres":
		if cfg.DatabaseURL == "" {
			return nil, nil, nil, errors.New("DATABASE_URL обязателен для APP_STORAGE=postgres")
		}

		db, err := sql.Open("postgres", cfg.DatabaseURL)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("open postgres connection: %w", err)
		}
		if err := db.Ping(); err != nil {
			_ = db.Close()
			return nil, nil, nil, fmt.Errorf("ping postgres connection: %w", err)
		}

		repo := repository.NewPostgresRepository(db)
		return repo, repo, func() { _ = db.Close() }, nil
	default:
		return nil, nil, nil, fmt.Errorf("неподдерживаемый APP_STORAGE=%q", cfg.Storage)
	}
}
