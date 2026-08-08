package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/RoGogDBD/PoshivOn/internal/auth"
	"github.com/RoGogDBD/PoshivOn/internal/service"
)

// ---------------------------------------------------------------------------------------
// Общие помощники пакета handler для тестов.
// ---------------------------------------------------------------------------------------

// recordingHandler — «следующий» обработчик цепочки. Считает вызовы и запоминает личность,
// которую middleware положил в контекст: «пропустил ли» и «что передал дальше» — два разных
// утверждения, и второе без него не проверить.
type recordingHandler struct {
	calls       int
	identity    Identity
	hadIdentity bool
}

func (h *recordingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.calls++
	h.identity, h.hadIdentity = IdentityFromContext(r.Context())
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// errorCode достаёт машиночитаемый слаг из тела ответа. Именно по нему клиент принимает
// решение о повторе (client/src/utils/yandexAuth.js:14-16), поэтому проверяем значение,
// а не только статус.
func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()

	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("тело ответа не JSON-объект со строковыми значениями: %v (%q)", err, rec.Body.String())
	}
	return payload["error"]
}

// callLog фиксирует порядок обращений к стабам. Нужен там, где утверждение звучит как
// «А произошло до Б», а не просто «А произошло».
type callLog struct {
	mu      sync.Mutex
	entries []string
}

func (l *callLog) add(entry string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, entry)
}

func (l *callLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.entries...)
}

func (l *callLog) indexOf(entry string) int {
	for i, item := range l.snapshot() {
		if item == entry {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------------------
// Стабы репозиториев. Задача допускает локальный стаб прямо в _test.go: реализации Task 3
// нужны прод-сборке main.go, а не этим тестам. Сам AccessService при этом настоящий —
// правило Decision 10 проверяется тем кодом, который работает в проде.
// ---------------------------------------------------------------------------------------

type stubUserRepo struct {
	log       *callLog
	users     map[string]service.UserRecord
	ensured   []stubEnsureCall
	getErr    error
	ensureErr error
}

type stubEnsureCall struct {
	Login       string
	Email       string
	DisplayName string
}

func newStubUserRepo(records ...service.UserRecord) *stubUserRepo {
	users := make(map[string]service.UserRecord, len(records))
	for _, record := range records {
		users[record.Login] = record
	}
	return &stubUserRepo{users: users}
}

func (r *stubUserRepo) EnsureUser(_ context.Context, login, email, displayName string) error {
	r.log.add("EnsureUser")
	r.ensured = append(r.ensured, stubEnsureCall{Login: login, Email: email, DisplayName: displayName})
	if r.ensureErr != nil {
		return r.ensureErr
	}
	if r.users == nil {
		r.users = make(map[string]service.UserRecord)
	}
	record, ok := r.users[login]
	if !ok {
		record = service.UserRecord{Login: login, Role: service.RoleUser}
	}
	record.Email = email
	record.DisplayName = displayName
	r.users[login] = record
	return nil
}

func (r *stubUserRepo) GetUser(_ context.Context, login string) (service.UserRecord, error) {
	r.log.add("GetUser")
	if r.getErr != nil {
		return service.UserRecord{}, r.getErr
	}
	record, ok := r.users[login]
	if !ok {
		return service.UserRecord{}, fmt.Errorf("user %q not found: %w", login, service.ErrNotFound)
	}
	return record, nil
}

func (r *stubUserRepo) ListUsers(_ context.Context) ([]service.UserRecord, error) {
	items := make([]service.UserRecord, 0, len(r.users))
	for _, record := range r.users {
		items = append(items, record)
	}
	return items, nil
}

func (r *stubUserRepo) SetAccess(_ context.Context, login string, granted bool) error {
	record, ok := r.users[login]
	if !ok {
		return fmt.Errorf("user %q not found: %w", login, service.ErrNotFound)
	}
	record.HasAccess = granted
	r.users[login] = record
	return nil
}

type stubRequestRepo struct{}

func (r *stubRequestRepo) CreateRequest(_ context.Context, _ string) error { return nil }

func (r *stubRequestRepo) GetRequest(_ context.Context, login string) (service.AccessRequest, error) {
	return service.AccessRequest{}, fmt.Errorf("access request for user %q not found: %w", login, service.ErrNotFound)
}

func (r *stubRequestRepo) DecideRequest(_ context.Context, _, _, _ string) error { return nil }

var _ service.UserRepository = (*stubUserRepo)(nil)
var _ service.AccessRequestRepository = (*stubRequestRepo)(nil)

func newAccessService(repo *stubUserRepo) *service.AccessService {
	return service.NewAccessService(repo, &stubRequestRepo{})
}

// resolverReturning — стаб проверки сессии. Через него проходят случаи, недостижимые без
// живой БД: сессия есть, но личности в ней нет.
func resolverReturning(session *auth.Session, err error) SessionResolver {
	return func(*http.Request) (*auth.Session, error) {
		return session, err
	}
}

func requestWithIdentity(method, target string, identity Identity) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	return req.WithContext(ContextWithIdentity(req.Context(), identity))
}

// ---------------------------------------------------------------------------------------
// RequireAuth
// ---------------------------------------------------------------------------------------

// TestRequireAuth_MissingCookies: без кук — 401 и до следующего обработчика запрос не доходит.
// Резолвер здесь настоящий (StoreSessionResolver), но поверх нулевого хранилища: проверка кук
// идёт до обращения к БД, и это же доказывает, что проверка сессии больше не требует ни
// *AuthHandler, ни живого *auth.Store.
func TestRequireAuth_MissingCookies(t *testing.T) {
	cases := []struct {
		name     string
		cookies  []*http.Cookie
		wantCode string
	}{
		{
			name:     "нет ни одной куки",
			wantCode: "access_cookie_missing",
		},
		{
			name:     "есть access, нет refresh",
			cookies:  []*http.Cookie{{Name: accessCookieName, Value: "ya-access"}},
			wantCode: "refresh_cookie_missing",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			next := &recordingHandler{}
			middleware := RequireAuth(StoreSessionResolver(nil), next)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/access/me", nil)
			for _, cookie := range testCase.cookies {
				req.AddCookie(cookie)
			}
			rec := httptest.NewRecorder()

			middleware.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("статус = %d, ожидался 401", rec.Code)
			}
			if got := errorCode(t, rec); got != testCase.wantCode {
				t.Errorf("код ошибки = %q, ожидался %q", got, testCase.wantCode)
			}
			if next.calls != 0 {
				t.Errorf("следующий обработчик вызван %d раз, ожидалось 0", next.calls)
			}
		})
	}
}

// TestRequireAuth_NullIdentitySession: сессия есть, личности нет — 401 session_identity_missing
// (Decision 2, fail-closed). Пустая строка и пробелы равносильны NULL: логин из пробелов
// в users.id не отобразится ни на кого.
func TestRequireAuth_NullIdentitySession(t *testing.T) {
	cases := []struct {
		name  string
		login sql.NullString
	}{
		{name: "NULL в колонке", login: sql.NullString{}},
		{name: "пустая строка", login: sql.NullString{String: "", Valid: true}},
		{name: "только пробелы", login: sql.NullString{String: "   ", Valid: true}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			next := &recordingHandler{}
			session := &auth.Session{ID: 42, YandexLogin: testCase.login}
			middleware := RequireAuth(resolverReturning(session, nil), next)

			rec := httptest.NewRecorder()
			middleware.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/access/me", nil))

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("статус = %d, ожидался 401", rec.Code)
			}
			if got := errorCode(t, rec); got != "session_identity_missing" {
				t.Errorf("код ошибки = %q, ожидался session_identity_missing", got)
			}
			if next.calls != 0 {
				t.Errorf("следующий обработчик вызван %d раз, ожидалось 0", next.calls)
			}
		})
	}
}

// TestRequireAuth_PutsIdentityInContext: пропущенный запрос несёт личность дальше по цепочке —
// на этом держится US-15 (владелец данных берётся из сессии, а не из адреса).
func TestRequireAuth_PutsIdentityInContext(t *testing.T) {
	next := &recordingHandler{}
	session := &auth.Session{
		ID:                7,
		YandexLogin:       sql.NullString{String: "ivanov", Valid: true},
		YandexEmail:       sql.NullString{String: "ivanov@yandex.ru", Valid: true},
		YandexDisplayName: sql.NullString{String: "Иван Иванов", Valid: true},
	}
	middleware := RequireAuth(resolverReturning(session, nil), next)

	rec := httptest.NewRecorder()
	middleware.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/access/me", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("статус = %d, ожидался 200", rec.Code)
	}
	if next.calls != 1 {
		t.Fatalf("следующий обработчик вызван %d раз, ожидался 1", next.calls)
	}
	if !next.hadIdentity {
		t.Fatalf("личности нет в контексте следующего обработчика")
	}
	want := Identity{Login: "ivanov", Email: "ivanov@yandex.ru", DisplayName: "Иван Иванов"}
	if next.identity != want {
		t.Errorf("личность = %+v, ожидалась %+v", next.identity, want)
	}
}

// TestRequireAuth_ResolverErrorSlugsUnchanged: набор 401-слагов проверки сессии — контракт
// с клиентом, а не деталь реализации. Слаг проходит наружу как есть.
func TestRequireAuth_ResolverErrorSlugsUnchanged(t *testing.T) {
	slugs := []string{
		"access_cookie_missing",
		"refresh_cookie_missing",
		"session_not_found",
		"session_expired",
		"access_expired",
		"access_mismatch",
	}

	for _, slug := range slugs {
		t.Run(slug, func(t *testing.T) {
			next := &recordingHandler{}
			middleware := RequireAuth(resolverReturning(nil, errors.New(slug)), next)

			rec := httptest.NewRecorder()
			middleware.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/access/me", nil))

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("статус = %d, ожидался 401", rec.Code)
			}
			if got := errorCode(t, rec); got != slug {
				t.Errorf("код ошибки = %q, ожидался %q", got, slug)
			}
			if next.calls != 0 {
				t.Errorf("следующий обработчик вызван %d раз, ожидалось 0", next.calls)
			}
		})
	}
}

// TestRequireAuth_UnknownResolverErrorIsNotLeaked: причина отказа, которой нет в контракте
// с клиентом, не проходит наружу текстом — 401 отдаёт известный код, а исходная ошибка
// остаётся в логе. Резолвер подменяем, и без фильтра сюда уехала бы любая внутренняя ошибка.
func TestRequireAuth_UnknownResolverErrorIsNotLeaked(t *testing.T) {
	restore := silenceLog(t)
	defer restore()

	next := &recordingHandler{}
	leaky := errors.New("dial tcp 127.0.0.1:3306: connect: connection refused")
	middleware := RequireAuth(resolverReturning(nil, leaky), next)

	rec := httptest.NewRecorder()
	middleware.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/access/me", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("статус = %d, ожидался 401", rec.Code)
	}
	if got := errorCode(t, rec); got != "session_not_found" {
		t.Errorf("код ошибки = %q, ожидался известный клиенту session_not_found", got)
	}
	if body := rec.Body.String(); containsAny(body, "3306", "connection refused") {
		t.Errorf("тело ответа содержит внутренний текст ошибки: %q", body)
	}
	if next.calls != 0 {
		t.Errorf("следующий обработчик вызван %d раз, ожидалось 0", next.calls)
	}
}

// TestRequireAuth_IdentityMissingCodeDoesNotTriggerClientRetry: session_identity_missing обязан
// отличаться от слагов, на которые клиент запускает refresh-повтор
// (shouldRefreshAuth, client/src/utils/yandexAuth.js:14-16). Повтор такую сессию не чинит:
// личность не появится от одной только ротации токена.
func TestRequireAuth_IdentityMissingCodeDoesNotTriggerClientRetry(t *testing.T) {
	retrySlugs := []string{"access_cookie_missing", "access_expired", "access_mismatch"}

	next := &recordingHandler{}
	middleware := RequireAuth(resolverReturning(&auth.Session{ID: 1}, nil), next)
	rec := httptest.NewRecorder()
	middleware.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/access/me", nil))

	got := errorCode(t, rec)
	for _, slug := range retrySlugs {
		if got == slug {
			t.Fatalf("код %q входит в список повторов клиента — клиент зациклит refresh", got)
		}
	}
}

// ---------------------------------------------------------------------------------------
// RequireAccess / RequireAdmin
// ---------------------------------------------------------------------------------------

// TestRequireAccess_AdminBypassesHasAccessFalse: администратор со снятым has_access проходит
// (US-14, Decision 10) — тот же инвариант, что в юнит-тестах AccessService, но на реально
// навешенном middleware.
func TestRequireAccess_AdminBypassesHasAccessFalse(t *testing.T) {
	repo := newStubUserRepo(service.UserRecord{
		Login:     "admin-login",
		Role:      service.RoleAdmin,
		HasAccess: false,
	})

	next := &recordingHandler{}
	middleware := RequireAccess(newAccessService(repo), next)

	rec := httptest.NewRecorder()
	middleware.ServeHTTP(rec, requestWithIdentity(http.MethodGet, "/api/v1/users/chats", Identity{Login: "admin-login"}))

	if rec.Code != http.StatusOK {
		t.Fatalf("статус = %d, ожидался 200 (администратор проходит при has_access=false)", rec.Code)
	}
	if next.calls != 1 {
		t.Errorf("следующий обработчик вызван %d раз, ожидался 1", next.calls)
	}
}

func TestRequireAccess_Gate(t *testing.T) {
	cases := []struct {
		name       string
		record     service.UserRecord
		identity   Identity
		wantStatus int
		wantNext   int
	}{
		{
			name:       "пользователь с доступом проходит",
			record:     service.UserRecord{Login: "user-ok", Role: service.RoleUser, HasAccess: true},
			identity:   Identity{Login: "user-ok"},
			wantStatus: http.StatusOK,
			wantNext:   1,
		},
		{
			name:       "пользователь без доступа отклонён",
			record:     service.UserRecord{Login: "user-no", Role: service.RoleUser, HasAccess: false},
			identity:   Identity{Login: "user-no"},
			wantStatus: http.StatusForbidden,
			wantNext:   0,
		},
		{
			name:       "неизвестный логин отклонён как без доступа",
			record:     service.UserRecord{Login: "someone-else", Role: service.RoleUser, HasAccess: true},
			identity:   Identity{Login: "ghost"},
			wantStatus: http.StatusForbidden,
			wantNext:   0,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			next := &recordingHandler{}
			middleware := RequireAccess(newAccessService(newStubUserRepo(testCase.record)), next)

			rec := httptest.NewRecorder()
			middleware.ServeHTTP(rec, requestWithIdentity(http.MethodGet, "/api/v1/users/chats", testCase.identity))

			if rec.Code != testCase.wantStatus {
				t.Fatalf("статус = %d, ожидался %d", rec.Code, testCase.wantStatus)
			}
			if next.calls != testCase.wantNext {
				t.Errorf("следующий обработчик вызван %d раз, ожидалось %d", next.calls, testCase.wantNext)
			}
			if testCase.wantStatus == http.StatusForbidden {
				if got := errorCode(t, rec); got != "forbidden" {
					t.Errorf("тело = %q, ожидался фиксированный текст forbidden", got)
				}
			}
		})
	}
}

// TestRequireAccess_WithoutIdentityFailsClosed: цепочка без RequireAuth впереди не должна
// пропускать запрос «за отсутствием личности» — это была бы дыра, а не деградация.
func TestRequireAccess_WithoutIdentityFailsClosed(t *testing.T) {
	next := &recordingHandler{}
	middleware := RequireAccess(newAccessService(newStubUserRepo()), next)

	rec := httptest.NewRecorder()
	middleware.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/users/chats", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("статус = %d, ожидался 401", rec.Code)
	}
	if next.calls != 0 {
		t.Errorf("следующий обработчик вызван %d раз, ожидалось 0", next.calls)
	}
}

// TestRequireAccess_RepositoryFailureIsNotAPass: сбой хранилища — 500, а не пропуск.
func TestRequireAccess_RepositoryFailureIsNotAPass(t *testing.T) {
	repo := newStubUserRepo()
	repo.getErr = errors.New("dial tcp 127.0.0.1:3306: connect: connection refused")

	next := &recordingHandler{}
	middleware := RequireAccess(newAccessService(repo), next)

	rec := httptest.NewRecorder()
	middleware.ServeHTTP(rec, requestWithIdentity(http.MethodGet, "/api/v1/users/chats", Identity{Login: "ivanov"}))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("статус = %d, ожидался 500", rec.Code)
	}
	if next.calls != 0 {
		t.Errorf("следующий обработчик вызван %d раз, ожидалось 0", next.calls)
	}
	if body := rec.Body.String(); containsAny(body, "3306", "connection refused") {
		t.Errorf("тело ответа содержит внутренний текст ошибки: %q", body)
	}
}

func TestRequireAdmin_Gate(t *testing.T) {
	cases := []struct {
		name       string
		record     service.UserRecord
		identity   Identity
		wantStatus int
		wantNext   int
	}{
		{
			name:       "администратор проходит",
			record:     service.UserRecord{Login: "admin-login", Role: service.RoleAdmin, HasAccess: false},
			identity:   Identity{Login: "admin-login"},
			wantStatus: http.StatusOK,
			wantNext:   1,
		},
		{
			name:       "пользователь с доступом, но без роли, отклонён",
			record:     service.UserRecord{Login: "user-ok", Role: service.RoleUser, HasAccess: true},
			identity:   Identity{Login: "user-ok"},
			wantStatus: http.StatusForbidden,
			wantNext:   0,
		},
		{
			name:       "неизвестный логин отклонён",
			record:     service.UserRecord{Login: "admin-login", Role: service.RoleAdmin},
			identity:   Identity{Login: "ghost"},
			wantStatus: http.StatusForbidden,
			wantNext:   0,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			next := &recordingHandler{}
			middleware := RequireAdmin(newAccessService(newStubUserRepo(testCase.record)), next)

			rec := httptest.NewRecorder()
			middleware.ServeHTTP(rec, requestWithIdentity(http.MethodGet, "/api/v1/admin/users", testCase.identity))

			if rec.Code != testCase.wantStatus {
				t.Fatalf("статус = %d, ожидался %d", rec.Code, testCase.wantStatus)
			}
			if next.calls != testCase.wantNext {
				t.Errorf("следующий обработчик вызван %d раз, ожидалось %d", next.calls, testCase.wantNext)
			}
		})
	}
}

func TestRequireAdmin_WithoutIdentityFailsClosed(t *testing.T) {
	next := &recordingHandler{}
	middleware := RequireAdmin(newAccessService(newStubUserRepo()), next)

	rec := httptest.NewRecorder()
	middleware.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/ivanov/access", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("статус = %d, ожидался 401", rec.Code)
	}
	if next.calls != 0 {
		t.Errorf("следующий обработчик вызван %d раз, ожидалось 0", next.calls)
	}
}

// ---------------------------------------------------------------------------------------
// RequireSameOrigin
// ---------------------------------------------------------------------------------------

// TestRequireSameOrigin_EmptyAllowlistFallsBackToHost: пустой CORS_ALLOWED_ORIGINS — не
// повод пропускать проверку (переписанный Decision 8). Сверка идёт со scheme://r.Host,
// схема берётся из конфигурации (secure), а не хардкодится.
func TestRequireSameOrigin_EmptyAllowlistFallsBackToHost(t *testing.T) {
	cases := []struct {
		name       string
		secure     bool
		origin     string
		wantStatus int
	}{
		{name: "origin совпадает с хостом (http)", secure: false, origin: "http://app.example", wantStatus: http.StatusOK},
		{name: "посторонний origin при пустом списке", secure: false, origin: "http://evil.example", wantStatus: http.StatusForbidden},
		{name: "origin отсутствует", secure: false, origin: "", wantStatus: http.StatusForbidden},
		{name: "origin совпадает с хостом (https в проде)", secure: true, origin: "https://app.example", wantStatus: http.StatusOK},
		{name: "та же схема, что у клиента, но не прод", secure: true, origin: "http://app.example", wantStatus: http.StatusForbidden},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			next := &recordingHandler{}
			middleware := RequireSameOrigin(nil, testCase.secure, next)

			req := httptest.NewRequest(http.MethodPost, "http://app.example/api/v1/access/requests", nil)
			req.Host = "app.example"
			if testCase.origin != "" {
				req.Header.Set("Origin", testCase.origin)
			}
			rec := httptest.NewRecorder()

			middleware.ServeHTTP(rec, req)

			if rec.Code != testCase.wantStatus {
				t.Fatalf("статус = %d, ожидался %d", rec.Code, testCase.wantStatus)
			}
			wantNext := 0
			if testCase.wantStatus == http.StatusOK {
				wantNext = 1
			}
			if next.calls != wantNext {
				t.Errorf("следующий обработчик вызван %d раз, ожидалось %d", next.calls, wantNext)
			}
		})
	}
}

// TestRequireSameOrigin_Allowlist: непустой список — сверка с ним, и только с ним.
func TestRequireSameOrigin_Allowlist(t *testing.T) {
	allowed := []string{"https://app.example", "https://admin.example"}

	cases := []struct {
		name       string
		origin     string
		wantStatus int
	}{
		{name: "первый разрешённый", origin: "https://app.example", wantStatus: http.StatusOK},
		{name: "второй разрешённый", origin: "https://admin.example", wantStatus: http.StatusOK},
		{name: "посторонний", origin: "https://evil.example", wantStatus: http.StatusForbidden},
		{name: "тот же хост, но не из списка", origin: "https://api.example", wantStatus: http.StatusForbidden},
		{name: "origin отсутствует", origin: "", wantStatus: http.StatusForbidden},
		{name: "null origin", origin: "null", wantStatus: http.StatusForbidden},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			next := &recordingHandler{}
			middleware := RequireSameOrigin(allowed, true, next)

			req := httptest.NewRequest(http.MethodPost, "https://api.example/api/v1/access/requests", nil)
			req.Host = "api.example"
			if testCase.origin != "" {
				req.Header.Set("Origin", testCase.origin)
			}
			rec := httptest.NewRecorder()

			middleware.ServeHTTP(rec, req)

			if rec.Code != testCase.wantStatus {
				t.Fatalf("статус = %d, ожидался %d", rec.Code, testCase.wantStatus)
			}
			if testCase.wantStatus == http.StatusForbidden && next.calls != 0 {
				t.Errorf("следующий обработчик вызван %d раз, ожидалось 0", next.calls)
			}
		})
	}
}

// TestRequireSameOrigin_SafeMethodsNeverBlocked: GET и HEAD не блокируются никогда,
// независимо от Origin — иначе сломается обычная навигация и предзагрузка.
func TestRequireSameOrigin_SafeMethodsNeverBlocked(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		for _, origin := range []string{"", "https://evil.example"} {
			t.Run(fmt.Sprintf("%s origin=%q", method, origin), func(t *testing.T) {
				next := &recordingHandler{}
				middleware := RequireSameOrigin([]string{"https://app.example"}, true, next)

				req := httptest.NewRequest(method, "https://api.example/api/v1/access/me", nil)
				req.Host = "api.example"
				if origin != "" {
					req.Header.Set("Origin", origin)
				}
				rec := httptest.NewRecorder()

				middleware.ServeHTTP(rec, req)

				if rec.Code != http.StatusOK {
					t.Fatalf("статус = %d, ожидался 200", rec.Code)
				}
				if next.calls != 1 {
					t.Errorf("следующий обработчик вызван %d раз, ожидался 1", next.calls)
				}
			})
		}
	}
}

// TestRequireSameOrigin_MutatingMethodsCovered: список изменяющих методов не ограничен POST.
func TestRequireSameOrigin_MutatingMethodsCovered(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			next := &recordingHandler{}
			middleware := RequireSameOrigin([]string{"https://app.example"}, true, next)

			req := httptest.NewRequest(method, "https://api.example/api/v1/users/chats/1", nil)
			req.Host = "api.example"
			rec := httptest.NewRecorder()

			middleware.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("статус = %d, ожидался 403 (запрос без Origin)", rec.Code)
			}
			if next.calls != 0 {
				t.Errorf("следующий обработчик вызван %d раз, ожидалось 0", next.calls)
			}
		})
	}
}

// containsAny — «есть ли в теле ответа хоть один внутренний фрагмент». Отдельный помощник,
// чтобы утверждение об утечке читалось одной строкой в нескольких тестах.
func containsAny(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}
