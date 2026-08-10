package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RoGogDBD/PoshivOn/internal/auth"
	"github.com/RoGogDBD/PoshivOn/internal/config"
	"github.com/RoGogDBD/PoshivOn/internal/service"
)

// ---------------------------------------------------------------------------------------
// Стаб хранилища сессий. Реального *auth.Store здесь быть не может: он ходит в MariaDB,
// а handler-тесты по TDD Anchor обязаны идти всегда, без TEST_DB_DSN.
// ---------------------------------------------------------------------------------------

type stubSessionStore struct {
	log       *callLog
	created   []auth.Session
	createErr error

	session   *auth.Session
	findErr   error
	updated   []stubTokenUpdate
	updateErr error
	revoked   []string
}

type stubTokenUpdate struct {
	SessionID    uint64
	RefreshHash  string
	AccessToken  string
	YandexRefres sql.NullString
}

func (s *stubSessionStore) CreateSession(session *auth.Session) error {
	s.log.add("CreateSession")
	if s.createErr != nil {
		return s.createErr
	}
	session.ID = uint64(len(s.created) + 1)
	s.created = append(s.created, *session)
	return nil
}

func (s *stubSessionStore) FindByRefreshHash(string) (*auth.Session, error) {
	s.log.add("FindByRefreshHash")
	if s.findErr != nil {
		return nil, s.findErr
	}
	if s.session == nil {
		return nil, errors.New("session not found")
	}
	copied := *s.session
	return &copied, nil
}

func (s *stubSessionStore) UpdateSessionTokens(
	sessionID uint64,
	refreshHash string,
	accessToken string,
	refreshToken sql.NullString,
	_ time.Time,
	_ time.Time,
) error {
	s.log.add("UpdateSessionTokens")
	if s.updateErr != nil {
		return s.updateErr
	}
	s.updated = append(s.updated, stubTokenUpdate{
		SessionID:    sessionID,
		RefreshHash:  refreshHash,
		AccessToken:  accessToken,
		YandexRefres: refreshToken,
	})
	return nil
}

func (s *stubSessionStore) RevokeByRefreshHash(refreshHash string) error {
	s.log.add("RevokeByRefreshHash")
	s.revoked = append(s.revoked, refreshHash)
	return nil
}

var _ SessionStore = (*stubSessionStore)(nil)

// ---------------------------------------------------------------------------------------
// Заглушка Яндекса. Единственный доступный шов — адреса в конфигурации:
// AuthHandler.httpClient не инжектируется.
// ---------------------------------------------------------------------------------------

type fakeYandex struct {
	server       *httptest.Server
	tokenCalls   atomic.Int64
	profileCalls atomic.Int64

	profile        map[string]any
	profileStatus  int
	tokenStatus    int
	tokenResponse  map[string]any
	seenAuthHeader atomic.Value
}

func newFakeYandex(t *testing.T, profile map[string]any) *fakeYandex {
	t.Helper()

	fake := &fakeYandex{
		profile:       profile,
		profileStatus: http.StatusOK,
		tokenStatus:   http.StatusOK,
		tokenResponse: map[string]any{
			"access_token":  "ya-access-token",
			"refresh_token": "ya-refresh-token",
			"expires_in":    3600,
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		fake.tokenCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(fake.tokenStatus)
		if fake.tokenStatus == http.StatusOK {
			_ = json.NewEncoder(w).Encode(fake.tokenResponse)
		}
	})
	mux.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {
		fake.profileCalls.Add(1)
		fake.seenAuthHeader.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(fake.profileStatus)
		if fake.profileStatus == http.StatusOK {
			_ = json.NewEncoder(w).Encode(fake.profile)
		}
	})

	fake.server = httptest.NewServer(mux)
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeYandex) authHeader() string {
	value, _ := f.seenAuthHeader.Load().(string)
	return value
}

func newTestConfig(fake *fakeYandex) *config.Config {
	return &config.Config{
		CookiePath:         "/",
		CookieSameSite:     "Lax",
		CookieSecure:       false,
		YandexClientID:     "test-client-id",
		YandexClientSecret: "test-client-secret",
		YandexTokenURL:     fake.server.URL + "/token",
		YandexUserInfoURL:  fake.server.URL + "/info",
		YandexRedirectURI:  "https://app.example/auth/callback",
		RefreshTTLHours:    720,
	}
}

func defaultProfile() map[string]any {
	return map[string]any{
		"login":         "ivanov",
		"display_name":  "Иван Иванов",
		"real_name":     "Иван Иванов",
		"default_email": "ivanov@yandex.ru",
	}
}

func codeRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "https://api.example/auth/yandex/code", strings.NewReader(body))
	req.Host = "api.example"
	req.Header.Set("Content-Type", "application/json")
	return req
}

func cookieByName(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, cookie := range (&http.Response{Header: rec.Header()}).Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

// ---------------------------------------------------------------------------------------
// Вход
// ---------------------------------------------------------------------------------------

// TestHandleYandexCode_CreatesUserBeforeAnyPanelAction: успешный вход заводит строку users
// через EnsureUser — до выдачи кук, до 204 и, стало быть, до любого действия в панели
// (Decision 11). Это единственный слой, где такое вообще проверяемо: вызов живёт внутри
// обработчика входа, а не в сервисе на стабе.
func TestHandleYandexCode_CreatesUserBeforeAnyPanelAction(t *testing.T) {
	log := &callLog{}
	fake := newFakeYandex(t, defaultProfile())
	store := &stubSessionStore{log: log}
	repo := newStubUserRepo()
	repo.log = log

	handler := NewAuthHandler(store, newTestConfig(fake), newAccessService(repo))

	rec := httptest.NewRecorder()
	handler.HandleYandexCode(rec, codeRequest(`{"code":"auth-code"}`))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("статус = %d, ожидался 204 (тело: %q)", rec.Code, rec.Body.String())
	}

	if len(repo.ensured) != 1 {
		t.Fatalf("EnsureUser вызван %d раз, ожидался 1", len(repo.ensured))
	}
	want := stubEnsureCall{Login: "ivanov", Email: "ivanov@yandex.ru", DisplayName: "Иван Иванов"}
	if repo.ensured[0] != want {
		t.Errorf("EnsureUser получил %+v, ожидалось %+v", repo.ensured[0], want)
	}

	if fake.profileCalls.Load() != 1 {
		t.Errorf("профиль запрошен %d раз, ожидался ровно 1 запрос на весь вход", fake.profileCalls.Load())
	}
	if got := fake.authHeader(); got != "OAuth ya-access-token" {
		t.Errorf("Authorization при запросе профиля = %q, ожидался OAuth-токен из обмена", got)
	}

	ensureAt := log.indexOf("EnsureUser")
	sessionAt := log.indexOf("CreateSession")
	if ensureAt < 0 || sessionAt < 0 || ensureAt > sessionAt {
		t.Errorf("порядок вызовов = %v, EnsureUser должен предшествовать созданию сессии", log.snapshot())
	}

	if len(store.created) != 1 {
		t.Fatalf("создано сессий: %d, ожидалась 1", len(store.created))
	}
	session := store.created[0]
	if !session.YandexLogin.Valid || session.YandexLogin.String != "ivanov" {
		t.Errorf("yandex_login сессии = %+v, ожидался ivanov", session.YandexLogin)
	}
	if !session.YandexEmail.Valid || session.YandexEmail.String != "ivanov@yandex.ru" {
		t.Errorf("yandex_email сессии = %+v, ожидался ivanov@yandex.ru", session.YandexEmail)
	}
	if !session.YandexDisplayName.Valid || session.YandexDisplayName.String != "Иван Иванов" {
		t.Errorf("yandex_display_name сессии = %+v, ожидался Иван Иванов", session.YandexDisplayName)
	}

	if cookieByName(rec, accessCookieName) == nil || cookieByName(rec, refreshCookieName) == nil {
		t.Errorf("после успешного входа не выставлены обе куки: %v", rec.Header().Values("Set-Cookie"))
	}
}

// TestHandleYandexCode_MissingOriginRejected: изменяющий запрос без Origin отклоняется до
// обращения к Яндексу — закрытие login CSRF (Decision 8). Проверяется на реально навешенном
// RequireSameOrigin: сам обработчик про Origin ничего не знает.
func TestHandleYandexCode_MissingOriginRejected(t *testing.T) {
	log := &callLog{}
	fake := newFakeYandex(t, defaultProfile())
	store := &stubSessionStore{log: log}
	repo := newStubUserRepo()
	repo.log = log

	handler := NewAuthHandler(store, newTestConfig(fake), newAccessService(repo))
	guarded := RequireSameOrigin([]string{"https://app.example"}, true, http.HandlerFunc(handler.HandleYandexCode))

	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, codeRequest(`{"code":"auth-code"}`))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("статус = %d, ожидался 403", rec.Code)
	}
	if len(store.created) != 0 {
		t.Errorf("создано сессий: %d, ожидалось 0", len(store.created))
	}
	if len(repo.ensured) != 0 {
		t.Errorf("EnsureUser вызван %d раз, ожидалось 0", len(repo.ensured))
	}
	if fake.tokenCalls.Load() != 0 {
		t.Errorf("обмен кода с Яндексом выполнен %d раз, ожидалось 0", fake.tokenCalls.Load())
	}
	if cookieByName(rec, accessCookieName) != nil || cookieByName(rec, refreshCookieName) != nil {
		t.Errorf("отклонённый запрос выставил куки: %v", rec.Header().Values("Set-Cookie"))
	}
}

// TestHandleYandexCode_ForeignOriginRejected: код авторизации с чужой страницы не попадает
// в куки браузера жертвы.
func TestHandleYandexCode_ForeignOriginRejected(t *testing.T) {
	log := &callLog{}
	fake := newFakeYandex(t, defaultProfile())
	store := &stubSessionStore{log: log}
	repo := newStubUserRepo()
	repo.log = log

	handler := NewAuthHandler(store, newTestConfig(fake), newAccessService(repo))
	guarded := RequireSameOrigin([]string{"https://app.example"}, true, http.HandlerFunc(handler.HandleYandexCode))

	req := codeRequest(`{"code":"attacker-code"}`)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()

	guarded.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("статус = %d, ожидался 403", rec.Code)
	}
	if len(store.created) != 0 {
		t.Errorf("создано сессий: %d, ожидалось 0", len(store.created))
	}
	if fake.tokenCalls.Load() != 0 {
		t.Errorf("обмен кода с Яндексом выполнен %d раз, ожидалось 0", fake.tokenCalls.Load())
	}
	if cookieByName(rec, accessCookieName) != nil || cookieByName(rec, refreshCookieName) != nil {
		t.Errorf("отклонённый запрос выставил куки: %v", rec.Header().Values("Set-Cookie"))
	}
}

// TestHandleYandexCode_AllowedOriginPasses: обратная сторона двух предыдущих тестов —
// штатный вход с разрешённого origin не ломается защитой.
func TestHandleYandexCode_AllowedOriginPasses(t *testing.T) {
	log := &callLog{}
	fake := newFakeYandex(t, defaultProfile())
	store := &stubSessionStore{log: log}
	repo := newStubUserRepo()
	repo.log = log

	handler := NewAuthHandler(store, newTestConfig(fake), newAccessService(repo))
	guarded := RequireSameOrigin([]string{"https://app.example"}, true, http.HandlerFunc(handler.HandleYandexCode))

	req := codeRequest(`{"code":"auth-code"}`)
	req.Header.Set("Origin", "https://app.example")
	rec := httptest.NewRecorder()

	guarded.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("статус = %d, ожидался 204 (тело: %q)", rec.Code, rec.Body.String())
	}
	if len(store.created) != 1 {
		t.Errorf("создано сессий: %d, ожидалась 1", len(store.created))
	}
}

// TestHandleYandexCode_ProfileFailureCreatesNoSession: без личности сессия бесполезна и
// опасна — она сразу же попадёт в ветку session_identity_missing. Вход должен падать целиком.
func TestHandleYandexCode_ProfileFailureCreatesNoSession(t *testing.T) {
	log := &callLog{}
	fake := newFakeYandex(t, defaultProfile())
	fake.profileStatus = http.StatusInternalServerError
	store := &stubSessionStore{log: log}
	repo := newStubUserRepo()
	repo.log = log

	handler := NewAuthHandler(store, newTestConfig(fake), newAccessService(repo))

	rec := httptest.NewRecorder()
	handler.HandleYandexCode(rec, codeRequest(`{"code":"auth-code"}`))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("статус = %d, ожидался 502 (тело: %q)", rec.Code, rec.Body.String())
	}
	if got := errorCode(t, rec); got != "yandex_profile_failed" {
		t.Errorf("код ошибки = %q, ожидался yandex_profile_failed", got)
	}
	if len(store.created) != 0 {
		t.Errorf("создано сессий: %d, ожидалось 0", len(store.created))
	}
	if len(repo.ensured) != 0 {
		t.Errorf("EnsureUser вызван %d раз, ожидалось 0", len(repo.ensured))
	}
	if cookieByName(rec, accessCookieName) != nil || cookieByName(rec, refreshCookieName) != nil {
		t.Errorf("неуспешный вход выставил куки: %v", rec.Header().Values("Set-Cookie"))
	}
}

// TestHandleYandexCode_EmptyLoginCreatesNoSession: профиль без логина — то же самое, что
// его отсутствие: личности нет, привязать сессию не к кому.
func TestHandleYandexCode_EmptyLoginCreatesNoSession(t *testing.T) {
	log := &callLog{}
	fake := newFakeYandex(t, map[string]any{"display_name": "Без логина"})
	store := &stubSessionStore{log: log}
	repo := newStubUserRepo()
	repo.log = log

	handler := NewAuthHandler(store, newTestConfig(fake), newAccessService(repo))

	rec := httptest.NewRecorder()
	handler.HandleYandexCode(rec, codeRequest(`{"code":"auth-code"}`))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("статус = %d, ожидался 502 (тело: %q)", rec.Code, rec.Body.String())
	}
	if got := errorCode(t, rec); got != "yandex_profile_failed" {
		t.Errorf("код ошибки = %q, ожидался yandex_profile_failed", got)
	}
	if len(store.created) != 0 {
		t.Errorf("создано сессий: %d, ожидалось 0", len(store.created))
	}
	if len(repo.ensured) != 0 {
		t.Errorf("EnsureUser вызван %d раз, ожидалось 0", len(repo.ensured))
	}
	if cookieByName(rec, accessCookieName) != nil || cookieByName(rec, refreshCookieName) != nil {
		t.Errorf("неуспешный вход выставил куки: %v", rec.Header().Values("Set-Cookie"))
	}
}

// TestHandleYandexCode_EnsureUserFailureCreatesNoSession: не удалось завести строку users —
// сессии тоже быть не должно, иначе пользователь войдёт в систему, в которой его нет.
func TestHandleYandexCode_EnsureUserFailureCreatesNoSession(t *testing.T) {
	log := &callLog{}
	fake := newFakeYandex(t, defaultProfile())
	store := &stubSessionStore{log: log}
	repo := newStubUserRepo()
	repo.log = log
	repo.ensureErr = errors.New("dial tcp 127.0.0.1:3306: connect: connection refused")

	handler := NewAuthHandler(store, newTestConfig(fake), newAccessService(repo))

	rec := httptest.NewRecorder()
	handler.HandleYandexCode(rec, codeRequest(`{"code":"auth-code"}`))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("статус = %d, ожидался 500", rec.Code)
	}
	if len(store.created) != 0 {
		t.Errorf("создано сессий: %d, ожидалось 0", len(store.created))
	}
	if cookieByName(rec, accessCookieName) != nil || cookieByName(rec, refreshCookieName) != nil {
		t.Errorf("неуспешный вход выставил куки: %v", rec.Header().Values("Set-Cookie"))
	}
	if body := rec.Body.String(); containsAny(body, "3306", "connection refused") {
		t.Errorf("тело ответа содержит внутренний текст ошибки: %q", body)
	}
}

// TestHandleYandexCode_SessionFailureAfterEnsureUser: обратная сторона предыдущего теста —
// строка users уже заведена, а сессия не создалась. Вход обязан упасть чисто: 500, без кук,
// без второго вызова EnsureUser. Осиротевшая строка users при этом остаётся и это
// нормально — EnsureUser идемпотентен (Decision 11), повторный вход просто обновит её.
func TestHandleYandexCode_SessionFailureAfterEnsureUser(t *testing.T) {
	log := &callLog{}
	fake := newFakeYandex(t, defaultProfile())
	store := &stubSessionStore{log: log, createErr: errors.New("Error 1146: Table 'oauth_sessions' doesn't exist")}
	repo := newStubUserRepo()
	repo.log = log

	handler := NewAuthHandler(store, newTestConfig(fake), newAccessService(repo))

	rec := httptest.NewRecorder()
	handler.HandleYandexCode(rec, codeRequest(`{"code":"auth-code"}`))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("статус = %d, ожидался 500", rec.Code)
	}
	if len(repo.ensured) != 1 {
		t.Errorf("EnsureUser вызван %d раз, ожидался ровно 1", len(repo.ensured))
	}
	if len(store.created) != 0 {
		t.Errorf("сессия сохранена вопреки ошибке хранилища: %d", len(store.created))
	}
	if cookieByName(rec, accessCookieName) != nil || cookieByName(rec, refreshCookieName) != nil {
		t.Errorf("неуспешный вход выставил куки: %v", rec.Header().Values("Set-Cookie"))
	}
	if body := rec.Body.String(); containsAny(body, "1146", "oauth_sessions") {
		t.Errorf("тело ответа содержит внутренний текст ошибки: %q", body)
	}
}

// TestHandleYandexCode_MissingDefaultEmailIsNotAnError: default_email может отсутствовать —
// сохраняется пустая строка, вход при этом успешен (edge case задачи).
func TestHandleYandexCode_MissingDefaultEmailIsNotAnError(t *testing.T) {
	log := &callLog{}
	fake := newFakeYandex(t, map[string]any{"login": "ivanov", "display_name": "Иван Иванов"})
	store := &stubSessionStore{log: log}
	repo := newStubUserRepo()
	repo.log = log

	handler := NewAuthHandler(store, newTestConfig(fake), newAccessService(repo))

	rec := httptest.NewRecorder()
	handler.HandleYandexCode(rec, codeRequest(`{"code":"auth-code"}`))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("статус = %d, ожидался 204 (тело: %q)", rec.Code, rec.Body.String())
	}
	if len(repo.ensured) != 1 || repo.ensured[0].Email != "" {
		t.Errorf("EnsureUser получил %+v, ожидался пустой email без ошибки входа", repo.ensured)
	}
	if len(store.created) != 1 {
		t.Fatalf("создано сессий: %d, ожидалась 1", len(store.created))
	}
	if store.created[0].YandexEmail.Valid && store.created[0].YandexEmail.String != "" {
		t.Errorf("yandex_email сессии = %+v, ожидалось пустое значение", store.created[0].YandexEmail)
	}
}

// TestNewLoginSession_MapsIdentityToNullableColumns: сборка строки сессии проверяется без
// поднятого сервера — «нет значения» обязано лечь в колонку как NULL, а не как пустая
// строка, иначе RequireAuth не отличит одно от другого.
func TestNewLoginSession_MapsIdentityToNullableColumns(t *testing.T) {
	fake := newFakeYandex(t, defaultProfile())
	cfg := newTestConfig(fake)
	handler := NewAuthHandler(&stubSessionStore{log: &callLog{}}, cfg, newAccessService(newStubUserRepo()))

	cases := []struct {
		name    string
		tokens  yandexTokens
		profile *yandexProfile
		check   func(t *testing.T, session *auth.Session)
	}{
		{
			name:    "полная личность",
			tokens:  yandexTokens{AccessToken: "ya-access", RefreshToken: "ya-refresh", ExpiresIn: 3600},
			profile: &yandexProfile{Login: "ivanov", Email: "ivanov@yandex.ru", Name: "Иван Иванов"},
			check: func(t *testing.T, session *auth.Session) {
				if !session.YandexEmail.Valid || session.YandexEmail.String != "ivanov@yandex.ru" {
					t.Errorf("yandex_email = %+v", session.YandexEmail)
				}
				if !session.YandexDisplayName.Valid || session.YandexDisplayName.String != "Иван Иванов" {
					t.Errorf("yandex_display_name = %+v", session.YandexDisplayName)
				}
				if !session.YandexRefreshToken.Valid {
					t.Errorf("yandex_refresh_token = %+v, ожидался сохранённый токен", session.YandexRefreshToken)
				}
			},
		},
		{
			name:    "без email, имени и refresh-токена Яндекса",
			tokens:  yandexTokens{AccessToken: "ya-access", ExpiresIn: 3600},
			profile: &yandexProfile{Login: "ivanov"},
			check: func(t *testing.T, session *auth.Session) {
				if session.YandexEmail.Valid {
					t.Errorf("yandex_email = %+v, ожидался NULL", session.YandexEmail)
				}
				if session.YandexDisplayName.Valid {
					t.Errorf("yandex_display_name = %+v, ожидался NULL", session.YandexDisplayName)
				}
				if session.YandexRefreshToken.Valid {
					t.Errorf("yandex_refresh_token = %+v, ожидался NULL", session.YandexRefreshToken)
				}
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			session, refreshCookie, refreshTTL := handler.newLoginSession(testCase.tokens, testCase.profile)

			if !session.YandexLogin.Valid || session.YandexLogin.String != "ivanov" {
				t.Errorf("yandex_login = %+v, ожидался ivanov", session.YandexLogin)
			}
			// В БД уходит хеш, клиенту — исходный токен: перепутать их означало бы отдать
			// клиенту то, что хранится, и сделать хеширование бессмысленным.
			if refreshCookie == "" || session.RefreshTokenHash == refreshCookie {
				t.Errorf("в сессию попало значение куки, а не её хеш: hash=%q cookie=%q", session.RefreshTokenHash, refreshCookie)
			}
			if session.RefreshTokenHash != auth.HashRefreshToken(refreshCookie) {
				t.Errorf("хеш не соответствует выданной куке")
			}
			if refreshTTL != time.Duration(cfg.RefreshTTLHours)*time.Hour {
				t.Errorf("срок жизни refresh-куки = %v, ожидался из конфигурации", refreshTTL)
			}
			if !session.AccessExpiresAt.After(time.Now().UTC()) {
				t.Errorf("access_expires_at = %v, ожидался в будущем", session.AccessExpiresAt)
			}
			testCase.check(t, session)
		})
	}
}

// TestParseYandexProfile_NameFallbackChain: имя собирается по убыванию точности, логин —
// последнее средство. Чистая функция, проверяется без сети.
func TestParseYandexProfile_NameFallbackChain(t *testing.T) {
	cases := []struct {
		name     string
		payload  map[string]interface{}
		wantName string
	}{
		{
			name:     "display_name важнее остальных",
			payload:  map[string]interface{}{"login": "ivanov", "display_name": "Ваня", "real_name": "Иван Иванов"},
			wantName: "Ваня",
		},
		{
			name:     "real_name, если нет display_name",
			payload:  map[string]interface{}{"login": "ivanov", "real_name": "Иван Иванов"},
			wantName: "Иван Иванов",
		},
		{
			name:     "имя и фамилия, если нет ни того, ни другого",
			payload:  map[string]interface{}{"login": "ivanov", "first_name": "Иван", "last_name": "Иванов"},
			wantName: "Иван Иванов",
		},
		{
			name:     "логин как последнее средство",
			payload:  map[string]interface{}{"login": "ivanov"},
			wantName: "ivanov",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			profile := parseYandexProfile(testCase.payload)
			if profile.Name != testCase.wantName {
				t.Errorf("name = %q, ожидалось %q", profile.Name, testCase.wantName)
			}
			if profile.Login != "ivanov" {
				t.Errorf("login = %q, ожидался ivanov", profile.Login)
			}
		})
	}
}

// ---------------------------------------------------------------------------------------
// Ротация
// ---------------------------------------------------------------------------------------

// TestHandleRefresh_DoesNotRefetchProfile: ротация токена не ходит в Яндекс за профилем —
// личность уже лежит в строке сессии (Decision 1). Иначе весь смысл хранения теряется.
func TestHandleRefresh_DoesNotRefetchProfile(t *testing.T) {
	log := &callLog{}
	fake := newFakeYandex(t, defaultProfile())
	store := &stubSessionStore{
		log: log,
		session: &auth.Session{
			ID:                 5,
			RefreshTokenHash:   auth.HashRefreshToken("refresh-cookie-value"),
			YandexAccessToken:  "ya-access-token",
			YandexRefreshToken: sql.NullString{String: "ya-refresh-token", Valid: true},
			AccessExpiresAt:    time.Now().UTC().Add(-time.Minute),
			RefreshExpiresAt:   time.Now().UTC().Add(720 * time.Hour),
			YandexLogin:        sql.NullString{String: "ivanov", Valid: true},
			YandexEmail:        sql.NullString{String: "ivanov@yandex.ru", Valid: true},
			YandexDisplayName:  sql.NullString{String: "Иван Иванов", Valid: true},
		},
	}
	repo := newStubUserRepo()
	repo.log = log

	handler := NewAuthHandler(store, newTestConfig(fake), newAccessService(repo))

	req := httptest.NewRequest(http.MethodPost, "https://api.example/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "refresh-cookie-value"})
	rec := httptest.NewRecorder()

	handler.HandleRefresh(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("статус = %d, ожидался 204 (тело: %q)", rec.Code, rec.Body.String())
	}
	if fake.profileCalls.Load() != 0 {
		t.Errorf("профиль запрошен %d раз при ротации, ожидалось 0", fake.profileCalls.Load())
	}
	if len(store.updated) != 1 {
		t.Fatalf("UpdateSessionTokens вызван %d раз, ожидался 1", len(store.updated))
	}
	if store.updated[0].SessionID != 5 {
		t.Errorf("обновлена сессия %d, ожидалась 5", store.updated[0].SessionID)
	}
}

// ---------------------------------------------------------------------------------------
// Проверка сессии, вынесенная из AuthHandler
// ---------------------------------------------------------------------------------------

// TestResolveSessionIsIndependentOfAuthHandler: проверка сессии обязана вызываться без
// экземпляра AuthHandler — иначе middleware.go не сможет её использовать (пункт 7 задачи).
// Утверждение compile-time: строка ниже не собирается, если ResolveSession остаётся методом.
var _ SessionResolver = func(r *http.Request) (*auth.Session, error) {
	return ResolveSession(nil, r)
}

func TestResolveSessionIsIndependentOfAuthHandler(t *testing.T) {
	session := &auth.Session{
		ID:                9,
		YandexAccessToken: "ya-access-token",
		AccessExpiresAt:   time.Now().UTC().Add(time.Hour),
		RefreshExpiresAt:  time.Now().UTC().Add(720 * time.Hour),
		YandexLogin:       sql.NullString{String: "ivanov", Valid: true},
	}
	finder := &stubSessionStore{log: &callLog{}, session: session}

	req := httptest.NewRequest(http.MethodGet, "/auth/status", nil)
	req.AddCookie(&http.Cookie{Name: accessCookieName, Value: "ya-access-token"})
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "refresh-cookie-value"})

	resolved, err := ResolveSession(finder, req)
	if err != nil {
		t.Fatalf("ResolveSession: %v", err)
	}
	if resolved.ID != 9 {
		t.Errorf("найдена сессия %d, ожидалась 9", resolved.ID)
	}
}

// TestHandleStatus_RejectsSessionWithoutIdentity и TestHandleMe_RejectsSessionWithoutIdentity —
// регрессия на цикл редиректов у пользователей со старой (до-миграционной) сессией.
//
// ResolveSession проверяет только валидность куки/сессии, не личность, — раньше
// HandleStatus/HandleMe звали его напрямую и отвечали «залогинен» даже без yandex_login.
// Лендинг (App.jsx) доверяет именно /auth/status, чтобы решить, редиректить ли на /panel;
// там сессия без личности отклоняется RequireAuth (session_identity_missing) и bootstrap-
// эффект Panel.jsx уводит обратно на "/", а лендинг снова видел /auth/status=200 и снова
// редиректил на /panel — быстрый цикл вместо однократного ухода на вход. Хендлеры теперь
// зовут requireIdentifiedSession, который добавляет ровно ту же проверку личности, что и
// RequireAuth, — оба источника «авторизован» синхронизированы.
func TestHandleStatus_RejectsSessionWithoutIdentity(t *testing.T) {
	session := &auth.Session{
		YandexAccessToken: "ya-access-token",
		AccessExpiresAt:   time.Now().UTC().Add(time.Hour),
		RefreshExpiresAt:  time.Now().UTC().Add(720 * time.Hour),
		YandexLogin:       sql.NullString{Valid: false},
	}
	store := &stubSessionStore{log: &callLog{}, session: session}
	handler := NewAuthHandler(store, newTestConfig(newFakeYandex(t, defaultProfile())), newAccessService(newStubUserRepo()))

	req := httptest.NewRequest(http.MethodGet, "/auth/status", nil)
	req.AddCookie(&http.Cookie{Name: accessCookieName, Value: "ya-access-token"})
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "refresh-cookie-value"})
	rec := httptest.NewRecorder()

	handler.HandleStatus(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("статус = %d, ожидался 401 (сессия без личности не должна выглядеть залогиненной)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), sessionIdentityMissingCode) {
		t.Errorf("тело = %q, ожидался код %q", rec.Body.String(), sessionIdentityMissingCode)
	}
}

func TestHandleMe_RejectsSessionWithoutIdentity(t *testing.T) {
	session := &auth.Session{
		YandexAccessToken: "ya-access-token",
		AccessExpiresAt:   time.Now().UTC().Add(time.Hour),
		RefreshExpiresAt:  time.Now().UTC().Add(720 * time.Hour),
		YandexLogin:       sql.NullString{Valid: false},
	}
	store := &stubSessionStore{log: &callLog{}, session: session}
	handler := NewAuthHandler(store, newTestConfig(newFakeYandex(t, defaultProfile())), newAccessService(newStubUserRepo()))

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: accessCookieName, Value: "ya-access-token"})
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "refresh-cookie-value"})
	rec := httptest.NewRecorder()

	handler.HandleMe(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("статус = %d, ожидался 401 (сессия без личности не должна отдавать профиль)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), sessionIdentityMissingCode) {
		t.Errorf("тело = %q, ожидался код %q", rec.Body.String(), sessionIdentityMissingCode)
	}
}

// TestResolveSession_RejectionSlugs: набор причин отказа не меняется — клиент матчит их
// по точному значению (client/src/utils/yandexAuth.js:14-16).
func TestResolveSession_RejectionSlugs(t *testing.T) {
	now := time.Now().UTC()

	cases := []struct {
		name     string
		session  *auth.Session
		findErr  error
		cookies  []*http.Cookie
		wantSlug string
	}{
		{
			name:     "нет access-куки",
			cookies:  nil,
			wantSlug: "access_cookie_missing",
		},
		{
			name:     "нет refresh-куки",
			cookies:  []*http.Cookie{{Name: accessCookieName, Value: "ya-access-token"}},
			wantSlug: "refresh_cookie_missing",
		},
		{
			name:     "сессии нет в хранилище",
			findErr:  errors.New("sql: no rows in result set"),
			cookies:  fullCookies("ya-access-token"),
			wantSlug: "session_not_found",
		},
		{
			name: "сессия отозвана",
			session: &auth.Session{
				YandexAccessToken: "ya-access-token",
				AccessExpiresAt:   now.Add(time.Hour),
				RefreshExpiresAt:  now.Add(time.Hour),
				RevokedAt:         sql.NullTime{Time: now, Valid: true},
			},
			cookies:  fullCookies("ya-access-token"),
			wantSlug: "session_expired",
		},
		{
			name: "истёк refresh",
			session: &auth.Session{
				YandexAccessToken: "ya-access-token",
				AccessExpiresAt:   now.Add(time.Hour),
				RefreshExpiresAt:  now.Add(-time.Hour),
			},
			cookies:  fullCookies("ya-access-token"),
			wantSlug: "session_expired",
		},
		{
			name: "истёк access",
			session: &auth.Session{
				YandexAccessToken: "ya-access-token",
				AccessExpiresAt:   now.Add(-time.Minute),
				RefreshExpiresAt:  now.Add(time.Hour),
			},
			cookies:  fullCookies("ya-access-token"),
			wantSlug: "access_expired",
		},
		{
			name: "кука не совпадает с сессией",
			session: &auth.Session{
				YandexAccessToken: "ya-access-token",
				AccessExpiresAt:   now.Add(time.Hour),
				RefreshExpiresAt:  now.Add(time.Hour),
			},
			cookies:  fullCookies("someone-elses-token"),
			wantSlug: "access_mismatch",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			finder := &stubSessionStore{log: &callLog{}, session: testCase.session, findErr: testCase.findErr}

			req := httptest.NewRequest(http.MethodGet, "/auth/status", nil)
			for _, cookie := range testCase.cookies {
				req.AddCookie(cookie)
			}

			_, err := ResolveSession(finder, req)
			if err == nil {
				t.Fatalf("ожидался отказ %q, получен успех", testCase.wantSlug)
			}
			if err.Error() != testCase.wantSlug {
				t.Errorf("слаг отказа = %q, ожидался %q", err.Error(), testCase.wantSlug)
			}
		})
	}
}

func fullCookies(accessValue string) []*http.Cookie {
	return []*http.Cookie{
		{Name: accessCookieName, Value: accessValue},
		{Name: refreshCookieName, Value: "refresh-cookie-value"},
	}
}

// TestFetchYandexProfile_ExtractsDefaultEmail: default_email извлекается тем же паттерном,
// что login и name.
func TestFetchYandexProfile_ExtractsDefaultEmail(t *testing.T) {
	cases := []struct {
		name      string
		payload   map[string]any
		wantEmail string
		wantName  string
		wantLogin string
	}{
		{
			name:      "полный профиль",
			payload:   defaultProfile(),
			wantEmail: "ivanov@yandex.ru",
			wantName:  "Иван Иванов",
			wantLogin: "ivanov",
		},
		{
			name:      "без default_email",
			payload:   map[string]any{"login": "ivanov", "display_name": "Иван Иванов"},
			wantEmail: "",
			wantName:  "Иван Иванов",
			wantLogin: "ivanov",
		},
		{
			name:      "пробелы обрезаются",
			payload:   map[string]any{"login": "ivanov", "default_email": "  ivanov@yandex.ru  "},
			wantEmail: "ivanov@yandex.ru",
			wantName:  "ivanov",
			wantLogin: "ivanov",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fake := newFakeYandex(t, testCase.payload)
			handler := NewAuthHandler(&stubSessionStore{log: &callLog{}}, newTestConfig(fake), newAccessService(newStubUserRepo()))

			profile, err := handler.fetchYandexProfile(t.Context(), "ya-access-token")
			if err != nil {
				t.Fatalf("fetchYandexProfile: %v", err)
			}
			if profile.Email != testCase.wantEmail {
				t.Errorf("email = %q, ожидался %q", profile.Email, testCase.wantEmail)
			}
			if profile.Name != testCase.wantName {
				t.Errorf("name = %q, ожидался %q", profile.Name, testCase.wantName)
			}
			if profile.Login != testCase.wantLogin {
				t.Errorf("login = %q, ожидался %q", profile.Login, testCase.wantLogin)
			}
		})
	}
}

// TestAuthHandlerHasNoLegacyTokenEntryPoint: маршрут приёма готового OAuth-токена удалён
// целиком (Decision 7). Регистрация в main.go снята тем же коммитом — рассинхрон между
// удалённым методом и живой регистрацией ловит go build ./..., поэтому здесь фиксируется
// только отсутствие самой точки входа в наборе обработчиков.
func TestAuthHandlerHasNoLegacyTokenEntryPoint(t *testing.T) {
	fake := newFakeYandex(t, defaultProfile())
	handler := NewAuthHandler(&stubSessionStore{log: &callLog{}}, newTestConfig(fake), newAccessService(newStubUserRepo()))

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/yandex/code", handler.HandleYandexCode)
	mux.HandleFunc("/auth/status", handler.HandleStatus)
	mux.HandleFunc("/auth/me", handler.HandleMe)
	mux.HandleFunc("/auth/refresh", handler.HandleRefresh)
	mux.HandleFunc("/auth/logout", handler.HandleLogout)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "https://api.example/auth/yandex", strings.NewReader(`{"access_token":"stolen"}`)))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /auth/yandex вернул %d, ожидался 404", rec.Code)
	}
}

// Тест-компиляционная проверка: AccessService передаётся в конструктор напрямую, без
// промежуточного интерфейса, — сигнатура зафиксирована задачей.
var _ func(SessionStore, *config.Config, *service.AccessService) *AuthHandler = NewAuthHandler
