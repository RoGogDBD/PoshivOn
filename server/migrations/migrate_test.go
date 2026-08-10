package migrations

import (
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// testAdminDSNEnv — DSN с правом CREATE/DROP DATABASE. Обычный TEST_DB_DSN (см.
// repository/access_repo_test.go) не подходит: у прикладного пользователя нет права
// создавать базы, а этому тесту нужна чистая, ни с кем не разделяемая схема — он
// воспроизводит порядок применения миграций с нуля, а не работает поверх уже
// смигрированной shared-базы.
//
// Форма значения — DSN БЕЗ имени базы (единственное отличие от TEST_DB_DSN):
//
//	TEST_MIGRATION_ADMIN_DSN='<user>:<password>@tcp(<host>:<port>)/?parseTime=true&charset=utf8mb4&multiStatements=true'
const testAdminDSNEnv = "TEST_MIGRATION_ADMIN_DSN"

// newThrowawayDB создаёт одноразовую базу через adminDSN и возвращает открытое соединение
// к ней. Пропускает тест (t.Skip), если TEST_MIGRATION_ADMIN_DSN не задан.
//
// Порядок cleanup'ов важен: DROP DATABASE регистрируется через t.Cleanup ПОСЛЕ закрытия
// admin-соединения тоже через t.Cleanup (а не через обычный defer в вызывающем тесте) —
// t.Cleanup вызывается в порядке LIFO, поэтому зарегистрированный первым Close отработает
// последним, и DROP всегда идёт по ещё живому соединению. Раньше здесь был обычный
// `defer admin.Close()`: он выполняется до t.Cleanup, поэтому DROP бил по уже закрытому
// пулу, ошибка терялась в `_, _ =`, и одноразовые базы копились без предупреждения.
func newThrowawayDB(t *testing.T) *sql.DB {
	t.Helper()

	adminDSN := os.Getenv(testAdminDSNEnv)
	if adminDSN == "" {
		t.Skipf("%s is not set — skipping the migration-ordering regression test (SEC-11-01)", testAdminDSNEnv)
	}

	admin, err := sql.Open("mysql", adminDSN)
	if err != nil {
		t.Fatalf("open %s: %v", testAdminDSNEnv, err)
	}
	t.Cleanup(func() {
		if err := admin.Close(); err != nil {
			t.Logf("close %s connection: %v", testAdminDSNEnv, err)
		}
	})

	dbName := fmt.Sprintf("migration_race_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE " + dbName); err != nil {
		t.Fatalf("create database %s: %v", dbName, err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP DATABASE IF EXISTS " + dbName); err != nil {
			t.Errorf("drop database %s: %v (leaked — clean up manually)", dbName, err)
		}
	})

	parts := strings.SplitN(adminDSN, "/", 2)
	if len(parts) != 2 {
		t.Fatalf("%s must be of the form user:pass@tcp(host:port)/?params (no database name)", testAdminDSNEnv)
	}
	db, err := sql.Open("mysql", parts[0]+"/"+dbName+parts[1])
	if err != nil {
		t.Fatalf("open scoped connection: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Logf("close scoped connection: %v", err)
		}
	})
	return db
}

// TestAdminSeed_SurvivesCollisionUnderLegacyCollation — регрессия на SEC-11-01.
//
// Миграция 004 сеет администраторов под коллацией, унаследованной от базы
// (регистро-/акцент-/пробело-нечувствительной); 005, которая делает сравнение
// байт-точным, идёт строго после и не переприменяет 004 (каждый файл — ровно один раз,
// migrate.go). Строка, созданная до этой фичи и совпадающая с логином администратора
// только под старой коллацией, поэтому получала бы role='admin' от 004 вместо настоящей
// строки администратора. 006 переустанавливает инвариант сразу после 005, до того как
// сервер начинает принимать запросы (migrations.Run вызывается в main() до
// ListenAndServe, синхронно и с остановкой при ошибке) — то есть до того, как настоящий
// администратор вообще успел бы войти.
//
// Тест применяет файлы миграций напрямую (не через Run — см. отдельный тест ниже, который
// проходит через настоящую точку входа), чтобы получить контролируемое окно ровно между
// «005 применена» и «006 применена» и смоделировать в нём то, что владелец коллизионной
// строки успел бы сделать, пока она ещё числится админом.
func TestAdminSeed_SurvivesCollisionUnderLegacyCollation(t *testing.T) {
	db := newThrowawayDB(t)

	before, from004to005, from006 := splitMigrationsAt004And006(t)
	applyMigrationFiles(t, db, before)

	// Строка, созданная до этой фичи, чей логин совпадает с администраторским только под
	// старой коллацией (users.id ещё utf8mb4_uca1400_ai_ci на этом этапе — 005 меняет её
	// только внутри from004to005). Нижний регистр — самый частый случай на практике, но
	// суть та же для акцента/концевого пробела (Decision 19 закрывает все три вектора
	// разом, и 006 не зависит от того, каким именно из них строка коллизировала).
	seedCollidingRow(t, db)

	applyMigrationFiles(t, db, from004to005)

	// Пока строка ошибочно числится админом (004 применена, 006 — ещё нет), Decision 10
	// даёт ей полный доступ вообще без обращения к has_access. Владелец строки мог этим
	// окном воспользоваться и сам себе явно выставить has_access=true через
	// POST /api/v1/admin/users/{login}/access, прежде чем 006 успела бы откатить role, —
	// это ровно то, что должно перестать работать: 006 обязана сбросить и has_access тоже,
	// не только role.
	simulateSelfGrantDuringEscalationWindow(t, db)

	applyMigrationFiles(t, db, from006)

	assertAdminInvariant(t, db)
}

// TestAdminSeed_SurvivesCollisionUnderLegacyCollation_ViaRun — тот же инвариант, но через
// настоящую точку входа приложения, а не ручное применение файлов.
//
// Первый тест доказывает, что содержимое SQL-файлов верно; этот — что реальный код,
// который его исполняет (Run, вызываемый из main() синхронно и до старта сервера), тоже
// применяет их в предполагаемом порядке и с предполагаемым результатом. Симулирует не
// первый деплой приложения целиком, а первый деплой ИМЕННО этой фичи на уже существующую
// базу: 001-003 применены и записаны в schema_migrations заранее (как это было бы после
// многих предыдущих релизов до появления access-control), в users уже сидит коллизионная
// строка, и только тогда вызывается Run — ровно та последовательность, что произошла бы в
// реальном проде при первом деплое этой фичи.
func TestAdminSeed_SurvivesCollisionUnderLegacyCollation_ViaRun(t *testing.T) {
	db := newThrowawayDB(t)

	before, _, _ := splitMigrationsAt004And006(t)
	applyMigrationFiles(t, db, before)
	markMigrationsApplied(t, db, before)

	seedCollidingRow(t, db)

	if err := Run(db); err != nil {
		t.Fatalf("Run: %v", err)
	}

	assertAdminInvariant(t, db)
	assertRecordedAsApplied(t, db, "006_admin_identity_reassert")

	// Повторный вызов — то, что происходит при каждом следующем деплое: 004-006 уже
	// записаны в schema_migrations, поэтому Run не должен трогать их снова и не должен
	// падать.
	if err := Run(db); err != nil {
		t.Fatalf("второй вызов Run: %v", err)
	}
	assertAdminInvariant(t, db)
}

func seedCollidingRow(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec("INSERT INTO users (id) VALUES ('rogogdbd')"); err != nil {
		t.Fatalf("seed colliding row: %v", err)
	}
}

func simulateSelfGrantDuringEscalationWindow(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec("UPDATE users SET has_access = TRUE WHERE id = 'rogogdbd'"); err != nil {
		t.Fatalf("simulate attacker self-grant: %v", err)
	}
}

// markMigrationsApplied записывает файлы в schema_migrations так, как это сделал бы Run,
// не применяя их SQL повторно (он уже применён вызывающим тестом отдельно) — имитирует
// состояние базы, на которой эти версии стоят с прошлых, не связанных с этой фичей,
// деплоев.
func markMigrationsApplied(t *testing.T, db *sql.DB, files []string) {
	t.Helper()
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR(255) PRIMARY KEY,
		applied_at TIMESTAMP NOT NULL
	)`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	for _, path := range files {
		version := extractVersion(path)
		if _, err := db.Exec(
			"INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)",
			version, time.Now().UTC(),
		); err != nil {
			t.Fatalf("mark %s applied: %v", version, err)
		}
	}
}

func assertRecordedAsApplied(t *testing.T, db *sql.DB, version string) {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version).Scan(&count); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if count != 1 {
		t.Errorf("schema_migrations не содержит %q ровно один раз (count=%d)", version, count)
	}
}

// assertAdminInvariant — общая проверка обоих тестов: после блока 004→006 admin ровно у
// двух настоящих логинов (с has_access), а коллизионная строка 'rogogdbd' — обычный
// пользователь без доступа.
func assertAdminInvariant(t *testing.T, db *sql.DB) {
	t.Helper()

	rows, err := db.Query("SELECT id, role, has_access FROM users ORDER BY id")
	if err != nil {
		t.Fatalf("query users: %v", err)
	}
	defer rows.Close()

	type row struct {
		id        string
		role      string
		hasAccess bool
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.role, &r.hasAccess); err != nil {
			t.Fatalf("scan row: %v", err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate rows: %v", err)
	}

	admins := map[string]bool{}
	for _, r := range got {
		if r.role == "admin" {
			admins[r.id] = true
		}
		if r.id == "rogogdbd" {
			if r.role != "user" {
				t.Errorf("colliding row 'rogogdbd' still has role=%q, want 'user' — SEC-11-01 not closed", r.role)
			}
			if r.hasAccess {
				t.Errorf("colliding row 'rogogdbd' still has has_access=true after demotion — a self-grant made during the escalation window survives 006")
			}
		}
	}

	wantAdmins := map[string]bool{"RoGogDBD": true, "irina2000aleshina": true}
	if len(admins) != len(wantAdmins) {
		t.Fatalf("admin set = %v, want exactly %v (all rows: %+v)", admins, wantAdmins, got)
	}
	for login := range wantAdmins {
		if !admins[login] {
			t.Errorf("expected %q to hold role=admin after 004->006, it does not (all rows: %+v)", login, got)
		}
	}
	for _, login := range []string{"RoGogDBD", "irina2000aleshina"} {
		for _, r := range got {
			if r.id == login && !r.hasAccess {
				t.Errorf("%q has role=admin but has_access=false, want true", login)
			}
		}
	}
}

// splitMigrationsAt004And006 делит отсортированный список файлов миграций на «до 004»,
// «004 и 005» и «006 и далее» — тот же glob+sort, что использует Run, поэтому список
// синхронизирован с реальным набором миграций автоматически. Три группы, а не две: тесту
// нужно окно ровно между «005 применена» и «006 применена», чтобы смоделировать то, что
// владелец коллизионной строки успел бы сделать, пока она ещё числится админом.
func splitMigrationsAt004And006(t *testing.T) (before, from004to005, from006 []string) {
	t.Helper()
	entries, err := fs.Glob(migrationsFS, "*.up.sql")
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	sort.Strings(entries)

	at004, at006 := -1, -1
	for i, path := range entries {
		switch {
		case strings.HasPrefix(path, "004_"):
			at004 = i
		case strings.HasPrefix(path, "006_"):
			at006 = i
		}
	}
	if at004 == -1 {
		t.Fatal("004_access_control.up.sql not found among embedded migrations")
	}
	if at006 == -1 {
		t.Fatal("006_admin_identity_reassert.up.sql not found among embedded migrations")
	}
	return entries[:at004], entries[at004:at006], entries[at006:]
}

func applyMigrationFiles(t *testing.T, db *sql.DB, files []string) {
	t.Helper()
	for _, path := range files {
		sqlBytes, err := migrationsFS.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if _, err := db.Exec(string(sqlBytes)); err != nil {
			t.Fatalf("apply %s: %v", path, err)
		}
	}
}
