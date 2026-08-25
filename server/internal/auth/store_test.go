package auth

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RoGogDBD/PoshivOn/internal/service"
	_ "github.com/go-sql-driver/mysql"
)

// testDBDSNEnv содержит DSN формата go-sql-driver/mysql на живую MariaDB с применёнными
// миграциями 001..005. Обязателен parseTime=true — сессия читает DATETIME в time.Time.
// Без переменной весь файл пропускается: личность сессии живёт в колонках, и проверить
// её сохранение можно только против настоящей схемы.
//
// Форма значения (учётные данные подставьте свои — рабочая пара логин/пароль здесь
// не нужна даже для локальной БД):
//
//	TEST_DB_DSN='<user>:<password>@tcp(<host>:<port>)/<database>?parseTime=true&charset=utf8mb4'
const testDBDSNEnv = "TEST_DB_DSN"

// sessionFixturePrefix уходит в исходный токен, из которого считается хеш. Сам хеш
// непрозрачен, поэтому уборка идёт по списку созданных хешей, а не по префиксу.
const sessionFixturePrefix = "t4store"

var fixtureCounter atomic.Uint64

var (
	testDBOnce sync.Once
	testDB     *sql.DB
	testDBErr  error
)

// openTestDB открывает соединение один раз на пакет: по соединению на тест исчерпало бы
// пул MariaDB на ровном месте.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv(testDBDSNEnv))
	if dsn == "" {
		t.Skipf("%s не задан — тесты хранилища сессий требуют живой MariaDB", testDBDSNEnv)
	}

	testDBOnce.Do(func() {
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			testDBErr = err
			return
		}
		if err := db.Ping(); err != nil {
			testDBErr = err
			return
		}
		testDB = db
	})

	if testDBErr != nil {
		t.Fatalf("подключение к тестовой БД (%s): %v", testDBDSNEnv, testDBErr)
	}
	return testDB
}

// newRefreshHash выдаёт хеш, уникальный на весь прогон: refresh_token_hash — UNIQUE,
// и повторное значение уронило бы соседний тест, а не текущий.
func newRefreshHash(t *testing.T) string {
	t.Helper()
	return HashRefreshToken(fmt.Sprintf("%s-%d-%d", sessionFixturePrefix, time.Now().UnixNano(), fixtureCounter.Add(1)))
}

// cleanupSession снимает строку после теста. Схема общая и не пересоздаётся, поэтому
// за собой убирает каждый тест сам.
func cleanupSession(t *testing.T, db *sql.DB, hashes ...string) {
	t.Helper()
	t.Cleanup(func() {
		for _, hash := range hashes {
			if _, err := db.Exec("DELETE FROM oauth_sessions WHERE refresh_token_hash = ?", hash); err != nil {
				t.Errorf("уборка сессии %s: %v", hash, err)
			}
		}
	})
}

func newSessionFixture(hash string) *Session {
	now := time.Now().UTC().Truncate(time.Second)
	return &Session{
		RefreshTokenHash:   hash,
		YandexAccessToken:  "ya-access-token",
		YandexRefreshToken: sql.NullString{String: "ya-refresh-token", Valid: true},
		AccessExpiresAt:    now.Add(time.Hour),
		RefreshExpiresAt:   now.Add(720 * time.Hour),
		CreatedAt:          now,
		UpdatedAt:          now,
		YandexLogin:        sql.NullString{String: sessionFixturePrefix + "-login", Valid: true},
		YandexEmail:        sql.NullString{String: sessionFixturePrefix + "@example.com", Valid: true},
		YandexDisplayName:  sql.NullString{String: "Тестовый Пользователь", Valid: true},
	}
}

func assertIdentity(t *testing.T, got *Session, wantLogin, wantEmail, wantDisplayName string) {
	t.Helper()

	if !got.YandexLogin.Valid || got.YandexLogin.String != wantLogin {
		t.Errorf("yandex_login = %+v, ожидался %q", got.YandexLogin, wantLogin)
	}
	if !got.YandexEmail.Valid || got.YandexEmail.String != wantEmail {
		t.Errorf("yandex_email = %+v, ожидался %q", got.YandexEmail, wantEmail)
	}
	if !got.YandexDisplayName.Valid || got.YandexDisplayName.String != wantDisplayName {
		t.Errorf("yandex_display_name = %+v, ожидался %q", got.YandexDisplayName, wantDisplayName)
	}
}

// TestAuthStore_CreateSessionPersistsIdentity: вход записывает личность, и она читается
// обратно. Без этого middleware не может узнать вызывающего, не сходив в Яндекс (Decision 1).
func TestAuthStore_CreateSessionPersistsIdentity(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)

	hash := newRefreshHash(t)
	cleanupSession(t, db, hash)

	session := newSessionFixture(hash)
	if err := store.CreateSession(session); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if session.ID == 0 {
		t.Fatalf("CreateSession не проставил ID сессии")
	}

	found, err := store.FindByRefreshHash(hash)
	if err != nil {
		t.Fatalf("FindByRefreshHash: %v", err)
	}

	if found.ID != session.ID {
		t.Errorf("ID = %d, ожидался %d", found.ID, session.ID)
	}
	assertIdentity(t, found, sessionFixturePrefix+"-login", sessionFixturePrefix+"@example.com", "Тестовый Пользователь")
}

// TestAuthStore_UpdateSessionTokensPreservesIdentity: ротация токенов не стирает личность.
// Отдельный тест, а не ветка предыдущего: свежий вход через ротацию не проходит и не поймал
// бы регрессию, при которой пользователь теряет личность примерно раз в срок жизни
// access-токена (Decision 1).
func TestAuthStore_UpdateSessionTokensPreservesIdentity(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)

	originalHash := newRefreshHash(t)
	rotatedHash := newRefreshHash(t)
	cleanupSession(t, db, originalHash, rotatedHash)

	session := newSessionFixture(originalHash)
	if err := store.CreateSession(session); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := store.UpdateSessionTokens(
		session.ID,
		rotatedHash,
		"ya-access-token-rotated",
		sql.NullString{String: "ya-refresh-token-rotated", Valid: true},
		now.Add(2*time.Hour),
		now.Add(720*time.Hour),
	); err != nil {
		t.Fatalf("UpdateSessionTokens: %v", err)
	}

	found, err := store.FindByRefreshHash(rotatedHash)
	if err != nil {
		t.Fatalf("FindByRefreshHash после ротации: %v", err)
	}

	if found.ID != session.ID {
		t.Fatalf("после ротации найдена другая сессия: ID = %d, ожидался %d", found.ID, session.ID)
	}
	if found.YandexAccessToken != "ya-access-token-rotated" {
		t.Errorf("yandex_access_token = %q, ожидался обновлённый", found.YandexAccessToken)
	}
	assertIdentity(t, found, sessionFixturePrefix+"-login", sessionFixturePrefix+"@example.com", "Тестовый Пользователь")
}

// TestAuthStore_NullYandexLoginRoundTrips: домиграционная строка (личности нет вовсе)
// читается без ошибки, поля пустые. Ровно на этом состоянии в проде основано решение
// RequireAuth ответить session_identity_missing (Decision 2) — стаб резолвера в
// handler-тестах через NULL-колонку физически не проходит.
func TestAuthStore_NullYandexLoginRoundTrips(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)

	hash := newRefreshHash(t)
	cleanupSession(t, db, hash)

	now := time.Now().UTC().Truncate(time.Second)
	if _, err := db.Exec(`
		INSERT INTO oauth_sessions (
			refresh_token_hash,
			yandex_access_token,
			yandex_refresh_token,
			access_expires_at,
			refresh_expires_at,
			revoked_at,
			created_at,
			updated_at,
			yandex_login,
			yandex_email,
			yandex_display_name
		) VALUES (?, ?, NULL, ?, ?, NULL, ?, ?, NULL, NULL, NULL)
	`,
		hash,
		"legacy-access-token",
		now.Add(time.Hour),
		now.Add(720*time.Hour),
		now,
		now,
	); err != nil {
		t.Fatalf("вставка домиграционной сессии: %v", err)
	}

	found, err := store.FindByRefreshHash(hash)
	if err != nil {
		t.Fatalf("FindByRefreshHash домиграционной сессии: %v", err)
	}

	if found.YandexLogin.Valid || found.YandexLogin.String != "" {
		t.Errorf("yandex_login = %+v, ожидался пустой NULL", found.YandexLogin)
	}
	if found.YandexEmail.Valid || found.YandexEmail.String != "" {
		t.Errorf("yandex_email = %+v, ожидался пустой NULL", found.YandexEmail)
	}
	if found.YandexDisplayName.Valid || found.YandexDisplayName.String != "" {
		t.Errorf("yandex_display_name = %+v, ожидался пустой NULL", found.YandexDisplayName)
	}
}

// TestAuthStore_RevokeByRefreshHash_NotFound — против настоящей MariaDB, не заглушки
// (code review): в дальнейшем на этой ошибке основан HTTP-статус на границе db-service
// (dbservice.classifyError, errors.Is(err, service.ErrNotFound) → 404) — до этого теста
// реальный RowsAffected()==0 код на строке 164-169 store.go проверялся только фейком,
// который просто возвращал sentinel напрямую, не проходя через настоящий UPDATE.
func TestAuthStore_RevokeByRefreshHash_NotFound(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)

	hash := newRefreshHash(t) // ни разу не вставлялась — гарантированно нет такой строки

	err := store.RevokeByRefreshHash(hash)
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err = %v, ожидалась обёртка service.ErrNotFound", err)
	}
}

// TestAuthStore_RevokeByRefreshHash_Success — симметрично: реальный UPDATE реальной строки
// не должен возвращать ErrNotFound.
func TestAuthStore_RevokeByRefreshHash_Success(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)

	hash := newRefreshHash(t)
	cleanupSession(t, db, hash)

	if err := store.CreateSession(newSessionFixture(hash)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if err := store.RevokeByRefreshHash(hash); err != nil {
		t.Fatalf("RevokeByRefreshHash: %v", err)
	}

	found, err := store.FindByRefreshHash(hash)
	if err != nil {
		t.Fatalf("FindByRefreshHash после revoke: %v", err)
	}
	if !found.RevokedAt.Valid {
		t.Errorf("RevokedAt.Valid = false после успешного RevokeByRefreshHash, ожидался true")
	}
}
