package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed *.up.sql
var migrationsFS embed.FS

// migrationLockName — имя именованной блокировки MariaDB (GET_LOCK/RELEASE_LOCK). Держит
// один и тот же процесс миграций от гонки на холодном старте нескольких параллельных
// инстансов db-service: без неё второй инстанс попадал на duplicate-key при вставке в
// schema_migrations (а на "голом" ALTER TABLE без IF NOT EXISTS — потенциально на гонке
// самого DDL). GET_LOCK специфичен для соединения, поэтому вся последовательность —
// от захвата до освобождения — обязана идти через одно и то же физическое соединение
// (см. Run), а не через ambient-пул *sql.DB, где Exec/Query могут разъехаться по разным
// соединениям.
const migrationLockName = "poshivon_schema_migrations"

// migrationLockTimeoutSecDefault — сколько ждать чужую блокировку по умолчанию, прежде чем
// сдаться. Холодный старт MariaDB внутри db-service (Фаза 2 плана) может занимать до ~15с
// сам по себе; таймаут ожидания блокировки должен быть заметно больше этого, иначе второй
// параллельный инстанс откажется по таймауту раньше, чем первый вообще успеет что-то
// сделать. Значение зависит от внешней, не контролируемой этим кодом величины (латентность
// холодного старта конкретной платформы) — поэтому настраивается переменной окружения, а
// не жёстко зашито: если платформенный холодный старт вырастет, поправить можно без
// пересборки.
const migrationLockTimeoutSecDefault = 30

func migrationLockTimeoutSec() int {
	raw := os.Getenv("MIGRATION_LOCK_TIMEOUT_SEC")
	if raw == "" {
		return migrationLockTimeoutSecDefault
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return migrationLockTimeoutSecDefault
	}
	return value
}

func Run(db *sql.DB) error {
	ctx := context.Background()

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire dedicated connection for migrations: %w", err)
	}
	defer conn.Close()

	lockTimeoutSec := migrationLockTimeoutSec()
	acquired, err := acquireLock(ctx, conn, lockTimeoutSec)
	if err != nil {
		return err
	}
	if !acquired {
		return fmt.Errorf("could not acquire migration lock %q within %ds: another instance is migrating",
			migrationLockName, lockTimeoutSec)
	}
	defer releaseLock(ctx, conn)

	if err := ensureSchemaMigrations(ctx, conn); err != nil {
		return err
	}

	entries, err := fs.Glob(migrationsFS, "*.up.sql")
	if err != nil {
		return fmt.Errorf("migration glob failed: %w", err)
	}
	sort.Strings(entries)

	applied, err := loadAppliedVersions(ctx, conn)
	if err != nil {
		return err
	}

	for _, path := range entries {
		version := extractVersion(path)
		if applied[version] {
			continue
		}

		sqlBytes, err := migrationsFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", path, err)
		}

		if _, err := conn.ExecContext(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply migration %s: %w", path, err)
		}

		if _, err := conn.ExecContext(ctx,
			"INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)",
			version,
			time.Now().UTC(),
		); err != nil {
			return fmt.Errorf("record migration %s: %w", path, err)
		}
	}

	return nil
}

// acquireLock захватывает именованную блокировку MariaDB на выделенном соединении conn.
// GET_LOCK возвращает 1 при успехе, 0 по таймауту, NULL при ошибке (например, если сессия
// прервалась) — последнее приходит в Go как sql.ErrNoRows не бывает, а как NULL-скан в *int;
// используем sql.NullInt64, чтобы отличить "0 — не дождались" от "NULL — что-то пошло не так".
func acquireLock(ctx context.Context, conn *sql.Conn, timeoutSec int) (bool, error) {
	var result sql.NullInt64
	row := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", migrationLockName, timeoutSec)
	if err := row.Scan(&result); err != nil {
		return false, fmt.Errorf("acquire migration lock: %w", err)
	}
	if !result.Valid {
		return false, fmt.Errorf("acquire migration lock: GET_LOCK returned NULL (connection error)")
	}
	return result.Int64 == 1, nil
}

// releaseLock освобождает блокировку на том же соединении, где она была взята. Ошибку
// логировать некуда (Run уже возвращает основной результат) и она не критична: соединение
// закрывается сразу после (defer conn.Close() в Run), а MariaDB сама снимает именованные
// блокировки при закрытии сессии — RELEASE_LOCK здесь для аккуратности, а не единственная
// гарантия освобождения.
func releaseLock(ctx context.Context, conn *sql.Conn) {
	_, _ = conn.ExecContext(ctx, "SELECT RELEASE_LOCK(?)", migrationLockName)
}

func ensureSchemaMigrations(ctx context.Context, conn *sql.Conn) error {
	_, err := conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL
		)
	`)
	return err
}

func loadAppliedVersions(ctx context.Context, conn *sql.Conn) (map[string]bool, error) {
	rows, err := conn.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := map[string]bool{}
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}
	return applied, rows.Err()
}

func extractVersion(path string) string {
	return strings.TrimSuffix(path, ".up.sql")
}
