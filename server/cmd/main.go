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

	store := auth.NewStore(database)
	authHandler := handler.NewAuthHandler(store, cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// Простейший healthcheck для проверки доступности сервиса.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/auth/yandex", authHandler.HandleYandexLogin)
	mux.HandleFunc("/auth/yandex/code", authHandler.HandleYandexCode)
	mux.HandleFunc("/auth/status", authHandler.HandleStatus)
	mux.HandleFunc("/auth/me", authHandler.HandleMe)
	mux.HandleFunc("/auth/refresh", authHandler.HandleRefresh)
	mux.HandleFunc("/auth/logout", authHandler.HandleLogout)

	handlerWithCORS := handler.WithCORS(handler.CORSConfig{
		AllowedOrigins: splitCSV(cfg.AllowedOrigins),
	}, mux)

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
