// Command dbservice — точка входа db-service: MariaDB и эта HTTP-обёртка живут в одном
// инстансе Serverless Container (см. план миграции, раздел «Фаза 2»). Бэкенд вызывает
// db-service по HTTPS вместо прямого SQL-подключения.
package main

import (
	"log"
	"net/http"
	"time"

	"github.com/RoGogDBD/PoshivOn/internal/auth"
	backendconfig "github.com/RoGogDBD/PoshivOn/internal/config"
	"github.com/RoGogDBD/PoshivOn/internal/db"
	"github.com/RoGogDBD/PoshivOn/internal/dbservice"
	"github.com/RoGogDBD/PoshivOn/internal/repository"
	"github.com/RoGogDBD/PoshivOn/migrations"
)

func main() {
	cfg, err := dbservice.LoadConfig()
	if err != nil {
		log.Fatalf("Ошибка загрузки конфигурации db-service: %v", err)
	}

	// db.OpenGORM принимает *config.Config (общий с бэкендом пакет) ради переиспользования
	// buildDSN — тут не вызывается config.Load(), поэтому валидация CORS/cookie
	// (SEC-11-08/C1), которая живёт только внутри Load(), к db-service не применяется вообще
	// и не мешает ему стартовать без cookie-настроек, которые ему не нужны.
	dbCfg := &backendconfig.Config{
		DBHost:     cfg.DBHost,
		DBPort:     cfg.DBPort,
		DBName:     cfg.DBName,
		DBUser:     cfg.DBUser,
		DBPassword: cfg.DBPassword,
	}

	gormDB, err := db.OpenGORM(dbCfg)
	if err != nil {
		log.Fatalf("Ошибка подключения к MariaDB: %v", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		log.Fatalf("Ошибка получения sql.DB из GORM-соединения: %v", err)
	}
	defer sqlDB.Close()

	// db-service сам прогоняет миграции при старте — у него уже есть прямой SQL-доступ к
	// своей же MariaDB, бэкенду он больше не требуется. GET_LOCK внутри Run защищает от
	// гонки, если платформа поднимет параллельный инстанс на холодном старте.
	if err := migrations.Run(sqlDB); err != nil {
		log.Fatalf("Ошибка применения миграций: %v", err)
	}

	repo := repository.NewPostgresRepository(gormDB)
	// auth.Store — тот же прямой SQL-доступ, что и у repo, только для сессий входа (своя
	// таблица oauth_sessions, отдельная от пятёрки репозиториев выше). При APP_STORAGE=http
	// у бэкенда нет вообще никакого прямого доступа к БД, поэтому сессии тоже идут через
	// db-service — найдено на реальном деплое: без этого бэкенд падал на старте, пытаясь
	// открыть прямое подключение только ради auth.NewStore.
	sessions := auth.NewStore(sqlDB)
	mux := dbservice.Routes(dbservice.Deps{
		Users:        repo,
		AccessReqs:   repo,
		Settings:     repo,
		Chats:        repo,
		Calculations: repo,
		Sessions:     sessions,
	})

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		// Полные ReadTimeout/WriteTimeout/IdleTimeout — не только заголовки: без них
		// медленное или зависшее тело запроса (или клиент, не читающий ответ) держит
		// соединение неограниченно долго. Из-за co-location архитектуры (MariaDB и эта
		// HTTP-обёртка — один и тот же инстанс) это не изолированная проблема одного
		// запроса, а риск исчерпать ресурсы инстанса целиком (security audit db-service).
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("db-service запущен на порту %s", cfg.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Ошибка запуска db-service: %v", err)
	}
}
