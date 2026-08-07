package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RoGogDBD/PoshivOn/internal/service"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// testDBDSNEnv содержит DSN формата go-sql-driver/mysql на живую MariaDB с применёнными
// миграциями 001..004. Обязателен parseTime=true — контракт читает TIMESTAMP в time.Time.
// Без переменной DB-половина контракта пропускается (t.Skip), Memory-половина идёт всегда.
//
// Пример:
//
//	TEST_DB_DSN='poshivon:poshivon@tcp(127.0.0.1:3306)/poshivon_test?parseTime=true&charset=utf8mb4'
const testDBDSNEnv = "TEST_DB_DSN"

// accessRepoFactory отдаёт пару репозиториев одного хранилища. Один и тот же набор
// contract*-функций прогоняется через две реализации фабрики — на этом и держится
// требование «обе реализации ведут себя одинаково».
type accessRepoFactory func() (service.UserRepository, service.AccessRequestRepository)

// contractFixturePrefix — общий префикс логинов, которые создаёт этот файл. DB-половина
// работает в общей, не пересоздаваемой схеме, поэтому изоляция строится на никогда не
// повторяющемся идентификаторе, а уборка — на удалении всего по префиксу в TestMain.
const contractFixturePrefix = "t3contract"

var loginCounter atomic.Uint64

// newLogin выдаёт логин, уникальный на весь прогон. Только нижний регистр: users.id
// наследует utf8mb4_uca1400_ai_ci, который регистронезависим, поэтому 'Ab' и 'ab' — одна
// строка, и смешение регистров дало бы ложные коллизии между тестами.
func newLogin(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("%s_%d_%d", contractFixturePrefix, time.Now().UnixNano(), loginCounter.Add(1))
}

var (
	testDBOnce sync.Once
	testDB     *gorm.DB
	testDBErr  error
)

// openTestDB открывает соединение один раз на пакет: контракт прогоняется десятками
// тестов, и по соединению на тест исчерпало бы пул MariaDB на ровном месте.
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv(testDBDSNEnv)
	if dsn == "" {
		t.Skipf("%s is not set — skipping the DB-backed half of the contract", testDBDSNEnv)
	}
	testDBOnce.Do(func() {
		testDB, testDBErr = gorm.Open(gormmysql.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if testDBErr != nil {
			return
		}
		sqlDB, err := testDB.DB()
		if err != nil {
			testDBErr = err
			return
		}
		testDBErr = sqlDB.Ping()
	})
	if testDBErr != nil {
		t.Fatalf("open %s: %v", testDBDSNEnv, testDBErr)
	}
	return testDB
}

func TestMain(m *testing.M) {
	code := m.Run()
	purgeContractFixtures()
	os.Exit(code)
}

// purgeContractFixtures убирает строки, созданные этим файлом. Удаления users достаточно:
// user_settings, chats, calculations и access_requests висят на нём через FK ON DELETE CASCADE.
func purgeContractFixtures() {
	if testDB == nil || testDBErr != nil {
		return
	}
	testDB.Exec("DELETE FROM users WHERE id LIKE ?", contractFixturePrefix+"%")
}

// runAgainstBothStores прогоняет одну contract-функцию по обеим реализациям.
// Memory идёт всегда, Postgres — только при заданном TEST_DB_DSN.
func runAgainstBothStores(t *testing.T, contract func(t *testing.T, factory accessRepoFactory)) {
	t.Helper()

	t.Run("memory", func(t *testing.T) {
		contract(t, func() (service.UserRepository, service.AccessRequestRepository) {
			repo := NewMemoryRepository()
			return repo, repo
		})
	})

	t.Run("postgres", func(t *testing.T) {
		db := openTestDB(t)
		contract(t, func() (service.UserRepository, service.AccessRequestRepository) {
			repo := NewPostgresRepository(db)
			return repo, repo
		})
	})
}

// seedAdmin проставляет роль администратора в обход интерфейсов — намеренно.
// В проде админов сеет миграция 004 прямым INSERT'ом, а не EnsureUser; в UserRepository
// метода смены роли нет и быть не должно (Decision 11). Поэтому подготовка состояния
// зависит от хранилища, а проверяемое поведение — нет.
func seedAdmin(t *testing.T, users service.UserRepository, login string) {
	t.Helper()

	switch repo := users.(type) {
	case *MemoryRepository:
		repo.mu.Lock()
		defer repo.mu.Unlock()
		record, ok := repo.usersByLogin[login]
		if !ok {
			t.Fatalf("seedAdmin: user %q does not exist in MemoryRepository", login)
		}
		record.Role = service.RoleAdmin
		record.HasAccess = true
		repo.usersByLogin[login] = record
	case *PostgresRepository:
		result := repo.db.Exec("UPDATE users SET role = 'admin', has_access = TRUE WHERE id = ?", login)
		if result.Error != nil {
			t.Fatalf("seedAdmin: %v", result.Error)
		}
		if result.RowsAffected == 0 {
			t.Fatalf("seedAdmin: user %q does not exist in the database", login)
		}
	default:
		t.Fatalf("seedAdmin: unsupported repository type %T", users)
	}
}

// --- EnsureUser ---------------------------------------------------------------------

func TestAccessRepoContract_EnsureUserCreatesUser(t *testing.T) {
	runAgainstBothStores(t, contractEnsureUserCreatesUser)
}

func contractEnsureUserCreatesUser(t *testing.T, factory accessRepoFactory) {
	users, _ := factory()
	ctx := context.Background()
	login := newLogin(t)

	if err := users.EnsureUser(ctx, login, "new@example.com", "New User"); err != nil {
		t.Fatalf("EnsureUser: %v", err)
	}

	record, err := users.GetUser(ctx, login)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if record.Login != login {
		t.Errorf("Login = %q, want %q", record.Login, login)
	}
	if record.Role != service.RoleUser {
		t.Errorf("Role = %q, want %q — новый пользователь не должен создаваться администратором", record.Role, service.RoleUser)
	}
	if record.HasAccess {
		t.Error("HasAccess = true, want false — вход сам по себе доступа не даёт")
	}
	if record.Email != "new@example.com" {
		t.Errorf("Email = %q, want %q", record.Email, "new@example.com")
	}
	if record.DisplayName != "New User" {
		t.Errorf("DisplayName = %q, want %q", record.DisplayName, "New User")
	}
	if record.RequestStatus != "" {
		t.Errorf("RequestStatus = %q, want empty — заявки ещё нет", record.RequestStatus)
	}
	if record.RequestedAt != nil {
		t.Errorf("RequestedAt = %v, want nil", record.RequestedAt)
	}
	if record.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero, want the row creation time")
	}
}

func TestAccessRepoContract_EnsureUserPreservesAdminRoleAndAccess(t *testing.T) {
	runAgainstBothStores(t, contractEnsureUserPreservesAdminRoleAndAccess)
}

// Главный тест файла: повторный вход администратора не должен его разжаловать
// (Decision 11). Без него вторая авторизация после выкатки молча сбрасывает role в 'user'
// и has_access в false, и управление доступом теряется вместе с последним админом.
func contractEnsureUserPreservesAdminRoleAndAccess(t *testing.T, factory accessRepoFactory) {
	users, _ := factory()
	ctx := context.Background()
	login := newLogin(t)

	if err := users.EnsureUser(ctx, login, "admin@example.com", "Admin"); err != nil {
		t.Fatalf("EnsureUser (first login): %v", err)
	}
	seedAdmin(t, users, login)

	if err := users.EnsureUser(ctx, login, "admin@example.com", "Admin"); err != nil {
		t.Fatalf("EnsureUser (second login): %v", err)
	}

	record, err := users.GetUser(ctx, login)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if record.Role != service.RoleAdmin {
		t.Errorf("Role = %q, want %q — повторный вход разжаловал администратора", record.Role, service.RoleAdmin)
	}
	if !record.HasAccess {
		t.Error("HasAccess = false, want true — повторный вход снял флаг доступа")
	}
}

func TestAccessRepoContract_EnsureUserUpdatesProfileFields(t *testing.T) {
	runAgainstBothStores(t, contractEnsureUserUpdatesProfileFields)
}

func contractEnsureUserUpdatesProfileFields(t *testing.T, factory accessRepoFactory) {
	users, _ := factory()
	ctx := context.Background()
	login := newLogin(t)

	if err := users.EnsureUser(ctx, login, "old@example.com", "Old Name"); err != nil {
		t.Fatalf("EnsureUser (first): %v", err)
	}
	before, err := users.GetUser(ctx, login)
	if err != nil {
		t.Fatalf("GetUser (first): %v", err)
	}

	if err := users.EnsureUser(ctx, login, "new@example.com", "New Name"); err != nil {
		t.Fatalf("EnsureUser (second): %v", err)
	}

	after, err := users.GetUser(ctx, login)
	if err != nil {
		t.Fatalf("GetUser (second): %v", err)
	}
	if after.Email != "new@example.com" {
		t.Errorf("Email = %q, want %q", after.Email, "new@example.com")
	}
	if after.DisplayName != "New Name" {
		t.Errorf("DisplayName = %q, want %q", after.DisplayName, "New Name")
	}
	if !after.CreatedAt.Equal(before.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v — повторный вход не должен переписывать дату создания", after.CreatedAt, before.CreatedAt)
	}
}

// --- GetUser ------------------------------------------------------------------------

func TestAccessRepoContract_GetUserReturnsRoleAndAccess(t *testing.T) {
	runAgainstBothStores(t, contractGetUserReturnsRoleAndAccess)
}

// Ловит потерянный gorm-тег на отдельной модели колонок доступа: без тега поле молча
// остаётся нулевым, и роль приезжает пустой строкой, а доступ — false для всех.
func contractGetUserReturnsRoleAndAccess(t *testing.T, factory accessRepoFactory) {
	users, _ := factory()
	ctx := context.Background()
	login := newLogin(t)

	if err := users.EnsureUser(ctx, login, "person@example.com", "Person"); err != nil {
		t.Fatalf("EnsureUser: %v", err)
	}
	if err := users.SetAccess(ctx, login, true); err != nil {
		t.Fatalf("SetAccess: %v", err)
	}

	record, err := users.GetUser(ctx, login)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if record.Role == "" {
		t.Error("Role is empty — колонка role не доехала до UserRecord")
	}
	if record.Role != service.RoleUser {
		t.Errorf("Role = %q, want %q", record.Role, service.RoleUser)
	}
	if !record.HasAccess {
		t.Error("HasAccess = false, want true — колонка has_access не доехала до UserRecord")
	}
	if record.Email != "person@example.com" || record.DisplayName != "Person" {
		t.Errorf("Email/DisplayName = %q/%q, want %q/%q", record.Email, record.DisplayName, "person@example.com", "Person")
	}
}

func TestAccessRepoContract_GetUserUnknownLoginNotFound(t *testing.T) {
	runAgainstBothStores(t, contractGetUserUnknownLoginNotFound)
}

func contractGetUserUnknownLoginNotFound(t *testing.T, factory accessRepoFactory) {
	users, _ := factory()

	_, err := users.GetUser(context.Background(), newLogin(t))
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("GetUser error = %v, want service.ErrNotFound", err)
	}
}

// --- SetAccess ----------------------------------------------------------------------

func TestAccessRepoContract_SetAccessGrant(t *testing.T) {
	runAgainstBothStores(t, contractSetAccessGrant)
}

func contractSetAccessGrant(t *testing.T, factory accessRepoFactory) {
	users, _ := factory()
	ctx := context.Background()
	login := newLogin(t)

	if err := users.EnsureUser(ctx, login, "grant@example.com", "Grant"); err != nil {
		t.Fatalf("EnsureUser: %v", err)
	}
	if err := users.SetAccess(ctx, login, true); err != nil {
		t.Fatalf("SetAccess(true): %v", err)
	}

	record, err := users.GetUser(ctx, login)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if !record.HasAccess {
		t.Error("HasAccess = false, want true")
	}
}

func TestAccessRepoContract_SetAccessRevoke(t *testing.T) {
	runAgainstBothStores(t, contractSetAccessRevoke)
}

// Отдельная функция, а не второй assert в грантующем тесте: путь записи, который умеет
// только выставлять true (частая ошибка — переиспользование литерала структуры без
// обнуления поля), в направлении «выдать» выглядит полностью исправным.
func contractSetAccessRevoke(t *testing.T, factory accessRepoFactory) {
	users, _ := factory()
	ctx := context.Background()
	login := newLogin(t)

	if err := users.EnsureUser(ctx, login, "revoke@example.com", "Revoke"); err != nil {
		t.Fatalf("EnsureUser: %v", err)
	}
	if err := users.SetAccess(ctx, login, true); err != nil {
		t.Fatalf("SetAccess(true): %v", err)
	}
	if err := users.SetAccess(ctx, login, false); err != nil {
		t.Fatalf("SetAccess(false): %v", err)
	}

	record, err := users.GetUser(ctx, login)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if record.HasAccess != false {
		t.Error("HasAccess = true, want false — отзыв доступа не записался")
	}
}

func TestAccessRepoContract_SetAccessUnknownLoginNotFound(t *testing.T) {
	runAgainstBothStores(t, contractSetAccessUnknownLoginNotFound)
}

func contractSetAccessUnknownLoginNotFound(t *testing.T, factory accessRepoFactory) {
	users, _ := factory()

	err := users.SetAccess(context.Background(), newLogin(t), true)
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("SetAccess error = %v, want service.ErrNotFound", err)
	}
}

func TestAccessRepoContract_SetAccessRepeatedGrantIsNotNotFound(t *testing.T) {
	runAgainstBothStores(t, contractSetAccessRepeatedGrantIsNotNotFound)
}

// Сверх списка TDD Anchor. MySQL/MariaDB считают «затронутой» только реально изменённую
// строку: повторный SetAccess(login, true) даёт RowsAffected() == 0 при существующем
// пользователе. Наивное «0 строк → ErrNotFound» превратило бы второй клик администратора
// по уже выданному доступу в 404.
func contractSetAccessRepeatedGrantIsNotNotFound(t *testing.T, factory accessRepoFactory) {
	users, _ := factory()
	ctx := context.Background()
	login := newLogin(t)

	if err := users.EnsureUser(ctx, login, "repeat@example.com", "Repeat"); err != nil {
		t.Fatalf("EnsureUser: %v", err)
	}
	if err := users.SetAccess(ctx, login, true); err != nil {
		t.Fatalf("SetAccess(true) first: %v", err)
	}
	if err := users.SetAccess(ctx, login, true); err != nil {
		t.Fatalf("SetAccess(true) repeated: %v — повтор без изменения значения не должен быть ошибкой", err)
	}

	record, err := users.GetUser(ctx, login)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if !record.HasAccess {
		t.Error("HasAccess = false, want true")
	}
}

// --- ListUsers ----------------------------------------------------------------------

func TestAccessRepoContract_ListUsersIncludesLoginOnlyUsers(t *testing.T) {
	runAgainstBothStores(t, contractListUsersIncludesLoginOnlyUsers)
}

func contractListUsersIncludesLoginOnlyUsers(t *testing.T, factory accessRepoFactory) {
	users, requests := factory()
	ctx := context.Background()
	loginOnly := newLogin(t)
	withRequest := newLogin(t)

	if err := users.EnsureUser(ctx, loginOnly, "only@example.com", "Only Logged In"); err != nil {
		t.Fatalf("EnsureUser(loginOnly): %v", err)
	}
	if err := users.EnsureUser(ctx, withRequest, "req@example.com", "Requested"); err != nil {
		t.Fatalf("EnsureUser(withRequest): %v", err)
	}
	if err := requests.CreateRequest(ctx, withRequest); err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}

	list, err := users.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}

	byLogin := make(map[string]service.UserRecord, len(list))
	for _, record := range list {
		byLogin[record.Login] = record
	}

	found, ok := byLogin[loginOnly]
	if !ok {
		t.Fatalf("ListUsers does not contain %q — пользователь, который только вошёл, из списка выпал", loginOnly)
	}
	if found.Role != service.RoleUser || found.HasAccess {
		t.Errorf("%q: Role/HasAccess = %q/%v, want %q/false", loginOnly, found.Role, found.HasAccess, service.RoleUser)
	}
	if found.Email != "only@example.com" || found.DisplayName != "Only Logged In" {
		t.Errorf("%q: Email/DisplayName = %q/%q, want %q/%q", loginOnly, found.Email, found.DisplayName, "only@example.com", "Only Logged In")
	}
	if found.RequestStatus != "" {
		t.Errorf("%q: RequestStatus = %q, want empty", loginOnly, found.RequestStatus)
	}

	requested, ok := byLogin[withRequest]
	if !ok {
		t.Fatalf("ListUsers does not contain %q", withRequest)
	}
	if requested.RequestStatus != "pending" {
		t.Errorf("%q: RequestStatus = %q, want %q — статус заявки должен приезжать тем же запросом", withRequest, requested.RequestStatus, "pending")
	}
	if requested.RequestedAt == nil {
		t.Errorf("%q: RequestedAt is nil, want the request creation time", withRequest)
	}
}

// --- CreateRequest / GetRequest / DecideRequest --------------------------------------

func TestAccessRepoContract_CreateRequestConflictWhilePending(t *testing.T) {
	runAgainstBothStores(t, contractCreateRequestConflictWhilePending)
}

func contractCreateRequestConflictWhilePending(t *testing.T, factory accessRepoFactory) {
	users, requests := factory()
	ctx := context.Background()
	login := newLogin(t)

	if err := users.EnsureUser(ctx, login, "pending@example.com", "Pending"); err != nil {
		t.Fatalf("EnsureUser: %v", err)
	}
	if err := requests.CreateRequest(ctx, login); err != nil {
		t.Fatalf("CreateRequest (first): %v", err)
	}

	err := requests.CreateRequest(ctx, login)
	if !errors.Is(err, service.ErrConflict) {
		t.Fatalf("CreateRequest (second) error = %v, want service.ErrConflict", err)
	}

	request, err := requests.GetRequest(ctx, login)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if request.UserID != login {
		t.Errorf("UserID = %q, want %q", request.UserID, login)
	}
	if request.Status != "pending" {
		t.Errorf("Status = %q, want %q", request.Status, "pending")
	}
	if request.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero, want the request creation time")
	}
	if request.DecidedAt != nil || request.DecidedBy != "" {
		t.Errorf("DecidedAt/DecidedBy = %v/%q, want nil/empty — заявку никто не рассматривал", request.DecidedAt, request.DecidedBy)
	}

	state, err := users.GetUser(ctx, login)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if state.RequestStatus != "pending" {
		t.Errorf("UserRecord.RequestStatus = %q, want %q", state.RequestStatus, "pending")
	}
}

func TestAccessRepoContract_GetRequestUnknownLoginNotFound(t *testing.T) {
	runAgainstBothStores(t, contractGetRequestUnknownLoginNotFound)
}

// Сверх списка TDD Anchor: GetRequest зафиксирован спецификацией, но в остальных
// сценариях вызывается только по существующей строке — ветка «заявки нет» иначе не покрыта.
func contractGetRequestUnknownLoginNotFound(t *testing.T, factory accessRepoFactory) {
	_, requests := factory()

	_, err := requests.GetRequest(context.Background(), newLogin(t))
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("GetRequest error = %v, want service.ErrNotFound", err)
	}
}

func TestAccessRepoContract_DecideRequestPreservesDecision(t *testing.T) {
	runAgainstBothStores(t, contractDecideRequestPreservesDecision)
}

func contractDecideRequestPreservesDecision(t *testing.T, factory accessRepoFactory) {
	users, requests := factory()
	ctx := context.Background()
	login := newLogin(t)

	if err := users.EnsureUser(ctx, login, "decide@example.com", "Decide"); err != nil {
		t.Fatalf("EnsureUser: %v", err)
	}
	if err := requests.CreateRequest(ctx, login); err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	if err := requests.DecideRequest(ctx, login, "approved", "admin-one"); err != nil {
		t.Fatalf("DecideRequest: %v", err)
	}

	request, err := requests.GetRequest(ctx, login)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if request.Status != "approved" {
		t.Errorf("Status = %q, want %q", request.Status, "approved")
	}
	if request.DecidedBy != "admin-one" {
		t.Errorf("DecidedBy = %q, want %q — потерян след того, кто выдал доступ", request.DecidedBy, "admin-one")
	}
	if request.DecidedAt == nil {
		t.Fatal("DecidedAt is nil, want the decision time")
	}
	if request.DecidedAt.IsZero() {
		t.Error("DecidedAt is zero, want the decision time")
	}
}

func TestAccessRepoContract_DecideRequestNoRowIsNotError(t *testing.T) {
	runAgainstBothStores(t, contractDecideRequestNoRowIsNotError)
}

// Администратор вправе выдать доступ тому, кто заявку не подавал; заводить её задним
// числом не нужно, поэтому отсутствие строки — no-op, а не ошибка (Decision 5).
func contractDecideRequestNoRowIsNotError(t *testing.T, factory accessRepoFactory) {
	users, requests := factory()
	ctx := context.Background()
	login := newLogin(t)

	if err := users.EnsureUser(ctx, login, "norequest@example.com", "No Request"); err != nil {
		t.Fatalf("EnsureUser: %v", err)
	}

	if err := requests.DecideRequest(ctx, login, "approved", "admin-one"); err != nil {
		t.Fatalf("DecideRequest without an existing request: %v, want nil", err)
	}

	if _, err := requests.GetRequest(ctx, login); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("GetRequest error = %v, want service.ErrNotFound — решение не должно создавать заявку", err)
	}
}

func TestAccessRepoContract_CreateRequestAfterRejectedSucceeds(t *testing.T) {
	runAgainstBothStores(t, contractCreateRequestAfterRejectedSucceeds)
}

func contractCreateRequestAfterRejectedSucceeds(t *testing.T, factory accessRepoFactory) {
	users, requests := factory()
	ctx := context.Background()
	login := newLogin(t)

	if err := users.EnsureUser(ctx, login, "again@example.com", "Again"); err != nil {
		t.Fatalf("EnsureUser: %v", err)
	}
	if err := requests.CreateRequest(ctx, login); err != nil {
		t.Fatalf("CreateRequest (first): %v", err)
	}
	if err := requests.DecideRequest(ctx, login, "rejected", "admin-one"); err != nil {
		t.Fatalf("DecideRequest: %v", err)
	}
	rejected, err := requests.GetRequest(ctx, login)
	if err != nil {
		t.Fatalf("GetRequest (after decision): %v", err)
	}

	if err := requests.CreateRequest(ctx, login); err != nil {
		t.Fatalf("CreateRequest (after rejected): %v, want nil", err)
	}

	resubmitted, err := requests.GetRequest(ctx, login)
	if err != nil {
		t.Fatalf("GetRequest (after resubmission): %v", err)
	}
	if resubmitted.Status != "pending" {
		t.Errorf("Status = %q, want %q — повторная подача после отказа должна вернуть заявку на рассмотрение", resubmitted.Status, "pending")
	}
	if resubmitted.DecidedBy != rejected.DecidedBy {
		t.Errorf("DecidedBy = %q, want %q — upsert не должен затирать прошлое решение", resubmitted.DecidedBy, rejected.DecidedBy)
	}
	if resubmitted.DecidedAt == nil {
		t.Fatal("DecidedAt is nil, want the preserved previous decision time")
	}
	if !resubmitted.DecidedAt.Equal(*rejected.DecidedAt) {
		t.Errorf("DecidedAt = %v, want %v — upsert не должен затирать прошлое решение", resubmitted.DecidedAt, rejected.DecidedAt)
	}
}

// --- DB-only ------------------------------------------------------------------------

func TestAccessRepoContract_CreateRequestRowsAffectedSemantics(t *testing.T) {
	db := openTestDB(t)
	users := NewPostgresRepository(db)
	ctx := context.Background()
	login := newLogin(t)

	if err := users.EnsureUser(ctx, login, "rows@example.com", "Rows"); err != nil {
		t.Fatalf("EnsureUser: %v", err)
	}

	// Тот же оператор, что выполняет CreateRequest. Проверяем именно числа, на которых
	// держится Decision 5: они верны, только пока DSN не включает clientFoundRows=true.
	const upsert = `INSERT INTO access_requests (user_id, status, created_at)
VALUES (?, 'pending', ?)
ON DUPLICATE KEY UPDATE
  status     = IF(status = 'pending', status, VALUES(status)),
  created_at = IF(status = 'pending', created_at, VALUES(created_at))`

	insert := db.WithContext(ctx).Exec(upsert, login, time.Now().UTC())
	if insert.Error != nil {
		t.Fatalf("upsert (insert): %v", insert.Error)
	}
	if insert.RowsAffected != 1 {
		t.Errorf("RowsAffected after insert = %d, want 1", insert.RowsAffected)
	}

	repeat := db.WithContext(ctx).Exec(upsert, login, time.Now().UTC())
	if repeat.Error != nil {
		t.Fatalf("upsert (while pending): %v", repeat.Error)
	}
	if repeat.RowsAffected != 0 {
		t.Errorf("RowsAffected while pending = %d, want 0 — на этом нуле держится ErrConflict", repeat.RowsAffected)
	}

	if err := db.WithContext(ctx).Exec("UPDATE access_requests SET status = 'rejected' WHERE user_id = ?", login).Error; err != nil {
		t.Fatalf("reject: %v", err)
	}

	resubmit := db.WithContext(ctx).Exec(upsert, login, time.Now().UTC().Add(time.Second))
	if resubmit.Error != nil {
		t.Fatalf("upsert (after rejected): %v", resubmit.Error)
	}
	if resubmit.RowsAffected != 2 {
		t.Errorf("RowsAffected after rejected = %d, want 2 — обновление существующей строки", resubmit.RowsAffected)
	}
}

// TestAccessRepoContract_ExistingWritePathsSurviveAccessColumns — регрессия Decision 13.
// Требует живой CHECK-констрейнт chk_users_role, поэтому только DB-backed. Если бы колонки
// доступа добавили в userModel, GORM включил бы role=<пустая строка> в INSERT всех трёх путей записи, и
// каждый из них упал бы с ERROR 4025 (23000): CONSTRAINT chk_users_role failed.
func TestAccessRepoContract_ExistingWritePathsSurviveAccessColumns(t *testing.T) {
	db := openTestDB(t)
	repo := NewPostgresRepository(db)
	ctx := context.Background()

	t.Run("UpsertSettings", func(t *testing.T) {
		login := newLogin(t)
		if err := repo.UpsertSettings(ctx, login, service.DefaultUserSettings()); err != nil {
			t.Fatalf("UpsertSettings for a brand-new user: %v", err)
		}
		if _, err := repo.GetSettings(ctx, login); err != nil {
			t.Fatalf("GetSettings: %v", err)
		}
		assertUserRowIsValid(t, db, login)
	})

	t.Run("CreateChat", func(t *testing.T) {
		login := newLogin(t)
		now := time.Now().UTC().Truncate(time.Second)
		chat := service.Chat{
			UserID:    login,
			ID:        fmt.Sprintf("chat-%d", loginCounter.Add(1)),
			Title:     "Новый чат",
			CreatedAt: now,
			UpdatedAt: now,
		}
		if _, err := repo.CreateChat(ctx, chat); err != nil {
			t.Fatalf("CreateChat for a brand-new user: %v", err)
		}
		chats, err := repo.ListChats(ctx, login)
		if err != nil {
			t.Fatalf("ListChats: %v", err)
		}
		if len(chats) != 1 {
			t.Fatalf("ListChats returned %d chats, want 1", len(chats))
		}
		assertUserRowIsValid(t, db, login)
	})

	t.Run("AppendCalculation", func(t *testing.T) {
		login := newLogin(t)
		now := time.Now().UTC().Truncate(time.Second)
		result := service.CalculationResult{
			UserID:       login,
			ChatID:       fmt.Sprintf("chat-%d", loginCounter.Add(1)),
			GarmentType:  "dress",
			MaterialType: "cotton",
			Urgency:      "normal",
			MarketStatus: "ok",
			Quantity:     3,
			PricePerUnit: 1000,
			Subtotal:     3000,
			Total:        3000,
			CreatedAt:    now,
		}
		if err := repo.AppendCalculation(ctx, result); err != nil {
			t.Fatalf("AppendCalculation for a brand-new user: %v", err)
		}
		items, err := repo.ListCalculations(ctx, login, result.ChatID)
		if err != nil {
			t.Fatalf("ListCalculations: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("ListCalculations returned %d items, want 1", len(items))
		}
		assertUserRowIsValid(t, db, login)
	})
}

// assertUserRowIsValid проверяет, что строка, созданную существующим upsertUser, БД
// заполнила значениями по умолчанию, а не пустой ролью.
func assertUserRowIsValid(t *testing.T, db *gorm.DB, login string) {
	t.Helper()

	var row struct {
		Role      string `gorm:"column:role"`
		HasAccess bool   `gorm:"column:has_access"`
	}
	result := db.Table("users").Select("role, has_access").Where("id = ?", login).Scan(&row)
	if result.Error != nil {
		t.Fatalf("read back users row: %v", result.Error)
	}
	if result.RowsAffected == 0 {
		t.Fatalf("users row for %q was not created", login)
	}
	if row.Role != string(service.RoleUser) {
		t.Errorf("role = %q, want %q — upsertUser не должен писать роль вообще, за него это делает DEFAULT", row.Role, service.RoleUser)
	}
	if row.HasAccess {
		t.Error("has_access = true, want false — существующие пути записи не должны выдавать доступ")
	}
}
