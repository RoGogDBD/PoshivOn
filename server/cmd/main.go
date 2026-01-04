package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	defaultHost = "0.0.0.0"
	defaultPort = "8080"
)

func main() {
	host := envOrDefault("APP_HOST", defaultHost)
	port := envOrDefault("APP_PORT", defaultPort)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// Простейший healthcheck для проверки доступности сервиса.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:              fmt.Sprintf("%s:%s", host, port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("HTTP-сервер запущен на %s:%s", host, port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Ошибка запуска сервера: %v", err)
	}
}

// envOrDefault возвращает значение переменной окружения или значение по умолчанию.
func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
