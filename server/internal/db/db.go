package db

import (
	"database/sql"
	"fmt"
	"net/url"

	_ "github.com/go-sql-driver/mysql"

	"github.com/RoGogDBD/PoshivOn/internal/config"
)

func Open(cfg *config.Config) (*sql.DB, error) {
	dsn := cfg.DatabaseURL
	if dsn == "" {
		dsn = buildDSN(cfg)
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

func buildDSN(cfg *config.Config) string {
	params := url.Values{}
	params.Set("parseTime", "true")
	params.Set("charset", "utf8mb4")
	params.Set("collation", "utf8mb4_unicode_ci")
	params.Set("multiStatements", "true")

	user := cfg.DBUser
	password := cfg.DBPassword
	addr := fmt.Sprintf("%s:%s", cfg.DBHost, cfg.DBPort)
	dbName := cfg.DBName

	return fmt.Sprintf("%s:%s@tcp(%s)/%s?%s", user, password, addr, dbName, params.Encode())
}
