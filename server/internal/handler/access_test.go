package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/RoGogDBD/PoshivOn/internal/auth"
	"github.com/RoGogDBD/PoshivOn/internal/config"
	"github.com/RoGogDBD/PoshivOn/internal/repository"
	"github.com/RoGogDBD/PoshivOn/internal/service"
)

// ---------------------------------------------------------------------------------------
// Обвязка: маршруты собираются ТОЙ ЖЕ функцией, что и в main.go (BuildRoutes).
//
// Копия сборки в тесте проверяла бы копию: рассинхрон между тем, какие middleware реально
// навешены в проде, и тем, какие навешаны в тесте, — ровно та ошибка, которую эти тесты
// обязаны ловить (Task 4 оставила middleware написанными, но ни к чему не подключёнными).
//
// Хранилище доступа — стабы из middleware_test.go, а не MemoryRepository: роль admin в
// MemoryRepository невыразима (EnsureUser всегда пишет role='user', экспортированного пути
// проставить роль нет), а без администратора не проверить ни один админский маршрут.
// Контракт самих реализаций хранилища закрыт контрактным набором Task 3.
// Хранилище калькулятора при этом настоящее (MemoryRepository): маршруты /api/v1/users/
// должны отвечать осмысленным 200, иначе «прошёл RequireAccess» и «упал на nil-сервисе»
// неразличимы.
// ---------------------------------------------------------------------------------------

const (
	fixtureHost         = "app.example"
	fixtureContactEmail = "help@poshivon.example"
	// sessionWithoutLogin — значение куки, на которое стаб резолвера отдаёт сессию без
	// yandex_login: строка, созданная до миграции 004 (Decision 2). Через живой резолвер
	// такой случай недостижим без БД.
	sessionWithoutLogin = "session-without-login"
)

type fixtureOptions struct {
	allowedOrigins []string
	cookieSecure   bool
	contactEmail   string
	users          []service.UserRecord
}

type routeFixture struct {
	t        *testing.T
	mux      *http.ServeMux
	users    *stubUserRepo
	requests *stubRequestRepo
	// costing — то же хранилище калькулятора, что подключено к маршрутам. Нужно тестам
	// /api/v1/users/**: предусловие («у Петрова есть чат») и утверждение («чат Петрова не
	// тронут») не должны проходить через тот самый обработчик, который тест и проверяет, —
	// иначе согласованно неверная реализация выглядела бы согласованно верной.
	costing *repository.MemoryRepository
	origin  string
	scheme  string
}

func newRouteFixture(t *testing.T, options fixtureOptions) *routeFixture {
	t.Helper()

	// writeAPIDomainError пишет исходную ошибку в лог по каждому отказу; в выводе тестов
	// это шум. Сам факт логирования проверяет отдельный тест в http_test.go.
	t.Cleanup(silenceLog(t))

	userRepo := newStubUserRepo(options.users...)
	requestRepo := newStubRequestRepo()
	// Заявка и пользователь связаны так же, как в хранилище: request_status в строке
	// пользователя — это join с заявкой, а не независимое поле.
	userRepo.requests = requestRepo

	accessService := service.NewAccessService(userRepo, requestRepo)
	costingRepo := repository.NewMemoryRepository()
	cfg := &config.Config{CookiePath: "/", CookieSameSite: "Lax", RefreshTTLHours: 720}

	contactEmail := options.contactEmail
	if contactEmail == "" {
		contactEmail = fixtureContactEmail
	}

	scheme := "http"
	if options.cookieSecure {
		scheme = "https"
	}

	origin := scheme + "://" + fixtureHost
	if len(options.allowedOrigins) > 0 {
		origin = options.allowedOrigins[0]
	}

	mux := BuildRoutes(RouteDeps{
		Auth:           NewAuthHandler(auth.NewStore(nil), cfg, accessService),
		API:            NewAPIHandler(service.NewCostingService(costingRepo, costingRepo, costingRepo), nil),
		Access:         NewAccessHandler(accessService, contactEmail),
		AccessService:  accessService,
		ResolveSession: fixtureResolver(),
		AllowedOrigins: options.allowedOrigins,
		CookieSecure:   options.cookieSecure,
	})

	return &routeFixture{
		t:        t,
		mux:      mux,
		users:    userRepo,
		requests: requestRepo,
		costing:  costingRepo,
		origin:   origin,
		scheme:   scheme,
	}
}

// fixtureResolver — стаб проверки сессии: значение access-куки трактуется как логин.
// Интерфейсом резолвер сделала Task 4 именно ради этого — auth.Store требует живого *sql.DB.
func fixtureResolver() SessionResolver {
	return func(r *http.Request) (*auth.Session, error) {
		cookie, err := r.Cookie(accessCookieName)
		if err != nil || strings.TrimSpace(cookie.Value) == "" {
			return nil, errAccessCookieMissing
		}
		if cookie.Value == sessionWithoutLogin {
			return &auth.Session{ID: 1}, nil
		}
		return &auth.Session{
			ID:                2,
			YandexLogin:       sql.NullString{String: cookie.Value, Valid: true},
			YandexEmail:       sql.NullString{String: cookie.Value + "@yandex.ru", Valid: true},
			YandexDisplayName: sql.NullString{String: cookie.Value, Valid: true},
		}, nil
	}
}

// apiRequest описывает запрос к собранным маршрутам. Origin по умолчанию — разрешённый:
// в негативных случаях он подменяется явно, чтобы в тесте было видно, что именно нарушено.
type apiRequest struct {
	method   string
	path     string
	body     string
	as       string // логин, от чьего имени идёт запрос; пусто — запрос без кук
	origin   string // пусто — подставляется разрешённый
	noOrigin bool   // заголовок Origin не отправляется вовсе
}

func (f *routeFixture) do(request apiRequest) *httptest.ResponseRecorder {
	f.t.Helper()

	var body io.Reader
	if request.body != "" {
		body = strings.NewReader(request.body)
	}

	httpRequest := httptest.NewRequest(request.method, f.scheme+"://"+fixtureHost+request.path, body)
	httpRequest.Host = fixtureHost
	if request.body != "" {
		httpRequest.Header.Set("Content-Type", "application/json")
	}
	if !request.noOrigin {
		origin := request.origin
		if origin == "" {
			origin = f.origin
		}
		httpRequest.Header.Set("Origin", origin)
	}
	if request.as != "" {
		httpRequest.AddCookie(&http.Cookie{Name: accessCookieName, Value: request.as})
	}

	recorder := httptest.NewRecorder()
	f.mux.ServeHTTP(recorder, httpRequest)
	return recorder
}

// accessSnapshot — состояние флагов доступа всех пользователей. Негативные случаи проверяют
// не только код ответа, но и то, что состояние не изменилось: отказ, случившийся ПОСЛЕ
// записи, отличался бы от отказа до неё только этим.
func (f *routeFixture) accessSnapshot() map[string]bool {
	snapshot := make(map[string]bool, len(f.users.users))
	for login, record := range f.users.users {
		snapshot[login] = record.HasAccess
	}
	return snapshot
}

func (f *routeFixture) assertAccessUnchanged(before map[string]bool) {
	f.t.Helper()

	after := f.accessSnapshot()
	if len(before) != len(after) {
		f.t.Fatalf("изменился состав пользователей: было %v, стало %v", before, after)
	}
	for login, hadAccess := range before {
		got, ok := after[login]
		if !ok {
			f.t.Fatalf("пользователь %q исчез из хранилища", login)
		}
		if got != hadAccess {
			f.t.Fatalf("флаг доступа %q изменился: было %v, стало %v", login, hadAccess, got)
		}
	}
}

func (f *routeFixture) hasAccess(login string) bool {
	f.t.Helper()

	record, ok := f.users.users[login]
	if !ok {
		f.t.Fatalf("пользователя %q нет в хранилище", login)
	}
	return record.HasAccess
}

func (f *routeFixture) requestStatus(login string) string {
	f.t.Helper()

	request, ok := f.requests.requests[login]
	if !ok {
		return ""
	}
	return request.Status
}

func decodeBody(t *testing.T, recorder *httptest.ResponseRecorder, dst any) {
	t.Helper()

	if err := json.Unmarshal(recorder.Body.Bytes(), dst); err != nil {
		t.Fatalf("тело ответа не разбирается: %v (%q)", err, recorder.Body.String())
	}
}

// accessMeResponsePayload фиксирует форму ответа GET /api/v1/access/me из таблицы
// «Контракт API» техспека. Имена полей — часть контракта с клиентом, поэтому проверяются
// именно они, а не структура Go.
type accessMeResponsePayload struct {
	Login         string `json:"login"`
	DisplayName   string `json:"display_name"`
	Email         string `json:"email"`
	Role          string `json:"role"`
	HasAccess     bool   `json:"has_access"`
	RequestStatus string `json:"request_status"`
	ContactEmail  string `json:"contact_email"`
}

type adminUsersPayload struct {
	Items []struct {
		Login         string `json:"login"`
		DisplayName   string `json:"display_name"`
		Email         string `json:"email"`
		Role          string `json:"role"`
		HasAccess     bool   `json:"has_access"`
		RequestStatus string `json:"request_status"`
	} `json:"items"`
}

func userRecord(login string, role service.Role, hasAccess bool) service.UserRecord {
	return service.UserRecord{
		Login:       login,
		DisplayName: login,
		Email:       login + "@yandex.ru",
		Role:        role,
		HasAccess:   hasAccess,
	}
}

// ---------------------------------------------------------------------------------------
// GET /api/v1/access/me
// ---------------------------------------------------------------------------------------

// TestAccessMe_NoCookies401: /api/v1/access/ закрыт RequireAuth, хотя и не закрыт
// RequireAccess — туда обращается именно тот, у кого доступа ещё нет, но не тот, кто вообще
// не вошёл.
func TestAccessMe_NoCookies401(t *testing.T) {
	fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
		userRecord("ivanov", service.RoleUser, false),
	}})

	recorder := fixture.do(apiRequest{method: http.MethodGet, path: "/api/v1/access/me"})

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("статус = %d, ожидался 401", recorder.Code)
	}
	if got := errorCode(t, recorder); got != "access_cookie_missing" {
		t.Errorf("код ошибки = %q, ожидался access_cookie_missing", got)
	}
}

// TestAccessMe_SessionWithoutYandexLogin401: сессия домиграционной эпохи отвергается с
// отдельным кодом (Decision 2) — на него клиент не запускает повтор с refresh.
func TestAccessMe_SessionWithoutYandexLogin401(t *testing.T) {
	fixture := newRouteFixture(t, fixtureOptions{})

	recorder := fixture.do(apiRequest{
		method: http.MethodGet,
		path:   "/api/v1/access/me",
		as:     sessionWithoutLogin,
	})

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("статус = %d, ожидался 401", recorder.Code)
	}
	if got := errorCode(t, recorder); got != "session_identity_missing" {
		t.Errorf("код ошибки = %q, ожидался session_identity_missing", got)
	}
}

// TestAccessMe_UserWithoutAccess200: пользователь без доступа получает не отказ, а состояние —
// именно на этом ответе клиент решает, показать рабочий интерфейс или плашку.
func TestAccessMe_UserWithoutAccess200(t *testing.T) {
	fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
		userRecord("ivanov", service.RoleUser, false),
	}})

	recorder := fixture.do(apiRequest{method: http.MethodGet, path: "/api/v1/access/me", as: "ivanov"})

	if recorder.Code != http.StatusOK {
		t.Fatalf("статус = %d, ожидался 200 (тело: %s)", recorder.Code, recorder.Body.String())
	}

	var payload accessMeResponsePayload
	decodeBody(t, recorder, &payload)

	if payload.Login != "ivanov" {
		t.Errorf("login = %q, ожидался ivanov", payload.Login)
	}
	if payload.HasAccess {
		t.Errorf("has_access = true у пользователя без доступа")
	}
	if payload.Role != string(service.RoleUser) {
		t.Errorf("role = %q, ожидалась user", payload.Role)
	}
	if payload.RequestStatus != "" {
		t.Errorf("request_status = %q, ожидалась пустая строка (заявки нет)", payload.RequestStatus)
	}
	if payload.Email != "ivanov@yandex.ru" {
		t.Errorf("email = %q, ожидался ivanov@yandex.ru", payload.Email)
	}
	if payload.ContactEmail != fixtureContactEmail {
		t.Errorf("contact_email = %q, ожидался %q", payload.ContactEmail, fixtureContactEmail)
	}
}

// TestAccessMe_ContactEmailComesFromConfig: значение плашки приходит из CONTACT_EMAIL, а не
// зашито в код. Два разных значения — потому что одно проходило бы и на константе.
func TestAccessMe_ContactEmailComesFromConfig(t *testing.T) {
	for _, contactEmail := range []string{"support@poshivon.example", "admin@example.org", ""} {
		t.Run(contactEmail, func(t *testing.T) {
			fixture := newRouteFixture(t, fixtureOptions{
				contactEmail: contactEmail,
				users:        []service.UserRecord{userRecord("ivanov", service.RoleUser, false)},
			})
			// Пустое значение CONTACT_EMAIL — штатная конфигурация по умолчанию, и оно тоже
			// обязано доехать как есть, а не подмениться дефолтом фикстуры.
			want := contactEmail
			if contactEmail == "" {
				want = fixtureContactEmail
			}

			recorder := fixture.do(apiRequest{method: http.MethodGet, path: "/api/v1/access/me", as: "ivanov"})

			if recorder.Code != http.StatusOK {
				t.Fatalf("статус = %d, ожидался 200", recorder.Code)
			}
			var payload accessMeResponsePayload
			decodeBody(t, recorder, &payload)
			if payload.ContactEmail != want {
				t.Errorf("contact_email = %q, ожидался %q", payload.ContactEmail, want)
			}
		})
	}
}

// TestAccessMe_AdminWithoutFlagReportsAccess: has_access в ответе — итоговое право
// (Decision 10), а не сырая колонка: администратор со снятой галочкой обязан увидеть
// рабочий интерфейс, а не плашку (US-14).
func TestAccessMe_AdminWithoutFlagReportsAccess(t *testing.T) {
	fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
		userRecord("RoGogDBD", service.RoleAdmin, false),
	}})

	recorder := fixture.do(apiRequest{method: http.MethodGet, path: "/api/v1/access/me", as: "RoGogDBD"})

	if recorder.Code != http.StatusOK {
		t.Fatalf("статус = %d, ожидался 200", recorder.Code)
	}
	var payload accessMeResponsePayload
	decodeBody(t, recorder, &payload)
	if !payload.HasAccess {
		t.Errorf("has_access = false у администратора — плашка вместо интерфейса (Decision 10)")
	}
	if payload.Role != string(service.RoleAdmin) {
		t.Errorf("role = %q, ожидалась admin", payload.Role)
	}
}

// ---------------------------------------------------------------------------------------
// POST /api/v1/access/requests
// ---------------------------------------------------------------------------------------

// TestAccessRequests_DuplicateConflict: повторная подача при заявке на рассмотрении —
// 409 (Decision 5), а не молчаливое обновление даты обращения.
func TestAccessRequests_DuplicateConflict(t *testing.T) {
	fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
		userRecord("ivanov", service.RoleUser, false),
	}})

	first := fixture.do(apiRequest{method: http.MethodPost, path: "/api/v1/access/requests", as: "ivanov"})
	if first.Code != http.StatusCreated {
		t.Fatalf("первая подача: статус = %d, ожидался 201 (тело: %s)", first.Code, first.Body.String())
	}
	if got := fixture.requestStatus("ivanov"); got != "pending" {
		t.Fatalf("статус заявки = %q, ожидался pending", got)
	}

	second := fixture.do(apiRequest{method: http.MethodPost, path: "/api/v1/access/requests", as: "ivanov"})
	if second.Code != http.StatusConflict {
		t.Fatalf("повторная подача: статус = %d, ожидался 409", second.Code)
	}
	if got := errorCode(t, second); got != "conflict" {
		t.Errorf("код ошибки = %q, ожидался conflict", got)
	}

	// Заявка после отказа второй подачи осталась той же самой, а не была перезаписана.
	if got := fixture.requestStatus("ivanov"); got != "pending" {
		t.Errorf("статус заявки = %q, ожидался pending", got)
	}
	if fixture.hasAccess("ivanov") {
		t.Errorf("подача заявки выдала доступ")
	}
}

// TestAccessRequests_AlreadyGrantedConflict: пользователю с доступом заявка не нужна —
// 409 без обращения к хранилищу заявок (правило техспека, ветка в AccessService).
func TestAccessRequests_AlreadyGrantedConflict(t *testing.T) {
	fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
		userRecord("ivanov", service.RoleUser, true),
		userRecord("RoGogDBD", service.RoleAdmin, false),
	}})

	for _, login := range []string{"ivanov", "RoGogDBD"} {
		t.Run(login, func(t *testing.T) {
			recorder := fixture.do(apiRequest{method: http.MethodPost, path: "/api/v1/access/requests", as: login})

			if recorder.Code != http.StatusConflict {
				t.Fatalf("статус = %d, ожидался 409", recorder.Code)
			}
			if got := fixture.requestStatus(login); got != "" {
				t.Errorf("заявка создана (%q), хотя доступ уже есть", got)
			}
		})
	}
}

// TestAccessRequests_AfterRejectionAllowed: подача после отказа разрешена и возвращает
// заявку в pending (Decision 5) — иначе отказ становился бы пожизненным.
func TestAccessRequests_AfterRejectionAllowed(t *testing.T) {
	fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
		userRecord("ivanov", service.RoleUser, false),
	}})

	if recorder := fixture.do(apiRequest{method: http.MethodPost, path: "/api/v1/access/requests", as: "ivanov"}); recorder.Code != http.StatusCreated {
		t.Fatalf("первая подача: статус = %d, ожидался 201", recorder.Code)
	}
	fixture.requests.decide("ivanov", "rejected", "RoGogDBD")

	recorder := fixture.do(apiRequest{method: http.MethodPost, path: "/api/v1/access/requests", as: "ivanov"})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("подача после отказа: статус = %d, ожидался 201", recorder.Code)
	}
	if got := fixture.requestStatus("ivanov"); got != "pending" {
		t.Errorf("статус заявки = %q, ожидался pending", got)
	}
}

// TestAccessRequests_NoOriginRejected: заявка — изменяющий запрос, и без заголовка Origin
// он отклоняется до всякой записи (Decision 8).
func TestAccessRequests_NoOriginRejected(t *testing.T) {
	fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
		userRecord("ivanov", service.RoleUser, false),
	}})

	recorder := fixture.do(apiRequest{
		method:   http.MethodPost,
		path:     "/api/v1/access/requests",
		as:       "ivanov",
		noOrigin: true,
	})

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("статус = %d, ожидался 403", recorder.Code)
	}
	if got := fixture.requestStatus("ivanov"); got != "" {
		t.Errorf("заявка создана (%q) несмотря на отказ по Origin", got)
	}
}

// ---------------------------------------------------------------------------------------
// GET /api/v1/admin/users
// ---------------------------------------------------------------------------------------

// TestAdminUsers_NoCookies401AndNonAdmin403: два разных отказа на два разных входа —
// неопознанный вызывающий получает 401, опознанный без роли — 403.
func TestAdminUsers_NoCookies401AndNonAdmin403(t *testing.T) {
	fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
		userRecord("ivanov", service.RoleUser, true),
		userRecord("RoGogDBD", service.RoleAdmin, true),
	}})

	t.Run("без кук 401", func(t *testing.T) {
		recorder := fixture.do(apiRequest{method: http.MethodGet, path: "/api/v1/admin/users"})

		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("статус = %d, ожидался 401", recorder.Code)
		}
		if strings.Contains(recorder.Body.String(), "RoGogDBD") {
			t.Errorf("тело отказа содержит список пользователей: %q", recorder.Body.String())
		}
	})

	t.Run("не-администратором 403", func(t *testing.T) {
		// Доступ у ivanov есть — то есть отклоняет именно RequireAdmin, а не RequireAccess.
		recorder := fixture.do(apiRequest{method: http.MethodGet, path: "/api/v1/admin/users", as: "ivanov"})

		if recorder.Code != http.StatusForbidden {
			t.Fatalf("статус = %d, ожидался 403", recorder.Code)
		}
		if got := errorCode(t, recorder); got != "forbidden" {
			t.Errorf("код ошибки = %q, ожидался forbidden", got)
		}
		if strings.Contains(recorder.Body.String(), "RoGogDBD") {
			t.Errorf("тело отказа содержит список пользователей: %q", recorder.Body.String())
		}
	})
}

// TestAdminUsers_AsAdmin200: список содержит всех, включая тех, кто заявку не подавал —
// администратор должен видеть каждого, кто когда-либо входил (US-7).
func TestAdminUsers_AsAdmin200(t *testing.T) {
	fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
		userRecord("ivanov", service.RoleUser, false),
		userRecord("petrov", service.RoleUser, true),
		userRecord("RoGogDBD", service.RoleAdmin, false),
	}})
	if recorder := fixture.do(apiRequest{method: http.MethodPost, path: "/api/v1/access/requests", as: "ivanov"}); recorder.Code != http.StatusCreated {
		t.Fatalf("подготовка заявки: статус = %d, ожидался 201", recorder.Code)
	}

	recorder := fixture.do(apiRequest{method: http.MethodGet, path: "/api/v1/admin/users", as: "RoGogDBD"})

	if recorder.Code != http.StatusOK {
		t.Fatalf("статус = %d, ожидался 200 (тело: %s)", recorder.Code, recorder.Body.String())
	}

	var payload adminUsersPayload
	decodeBody(t, recorder, &payload)

	logins := make([]string, 0, len(payload.Items))
	for _, item := range payload.Items {
		logins = append(logins, item.Login)
	}
	sort.Strings(logins)
	want := []string{"RoGogDBD", "ivanov", "petrov"}
	if strings.Join(logins, ",") != strings.Join(want, ",") {
		t.Fatalf("список = %v, ожидался %v", logins, want)
	}

	byLogin := make(map[string]int, len(payload.Items))
	for i, item := range payload.Items {
		byLogin[item.Login] = i
	}
	if item := payload.Items[byLogin["petrov"]]; !item.HasAccess {
		t.Errorf("has_access у petrov = false, ожидался true")
	}
	if item := payload.Items[byLogin["ivanov"]]; item.HasAccess {
		t.Errorf("has_access у ivanov = true, ожидался false")
	}
	if item := payload.Items[byLogin["ivanov"]]; item.RequestStatus != "pending" {
		t.Errorf("request_status у ivanov = %q, ожидался pending", item.RequestStatus)
	}
	if item := payload.Items[byLogin["RoGogDBD"]]; item.Role != string(service.RoleAdmin) {
		t.Errorf("role у RoGogDBD = %q, ожидалась admin", item.Role)
	}
	// Сырой флаг, а не итоговый: галочку администратора клиент рисует по роли, и подмена
	// сырого значения итоговым скрыла бы от администратора реальное состояние колонки.
	if item := payload.Items[byLogin["RoGogDBD"]]; item.HasAccess {
		t.Errorf("has_access у администратора = true, ожидался сырой флаг false")
	}
}

// ---------------------------------------------------------------------------------------
// POST /api/v1/admin/users/{login}/access
// ---------------------------------------------------------------------------------------

// TestSetAccess_NonAdminOtherLogin403Unchanged: US-13 — не-администратор не выдаёт доступ
// никому. Проверяется не только код ответа, но и то, что ни один флаг не изменился.
func TestSetAccess_NonAdminOtherLogin403Unchanged(t *testing.T) {
	fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
		userRecord("ivanov", service.RoleUser, false),
		userRecord("petrov", service.RoleUser, false),
		userRecord("RoGogDBD", service.RoleAdmin, false),
	}})
	before := fixture.accessSnapshot()

	recorder := fixture.do(apiRequest{
		method: http.MethodPost,
		path:   "/api/v1/admin/users/petrov/access",
		body:   `{"granted":true}`,
		as:     "ivanov",
	})

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("статус = %d, ожидался 403", recorder.Code)
	}
	if got := errorCode(t, recorder); got != "forbidden" {
		t.Errorf("код ошибки = %q, ожидался forbidden", got)
	}
	fixture.assertAccessUnchanged(before)
}

// TestSetAccess_NonAdminSelfLogin403Unchanged: US-13 «даже для себя» — попытка выдать
// доступ самому себе ничем не отличается от попытки выдать его другому.
func TestSetAccess_NonAdminSelfLogin403Unchanged(t *testing.T) {
	fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
		userRecord("ivanov", service.RoleUser, false),
		userRecord("RoGogDBD", service.RoleAdmin, false),
	}})
	before := fixture.accessSnapshot()

	recorder := fixture.do(apiRequest{
		method: http.MethodPost,
		path:   "/api/v1/admin/users/ivanov/access",
		body:   `{"granted":true}`,
		as:     "ivanov",
	})

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("статус = %d, ожидался 403", recorder.Code)
	}
	if fixture.hasAccess("ivanov") {
		t.Errorf("пользователь выдал доступ сам себе")
	}
	fixture.assertAccessUnchanged(before)
}

// TestSetAccess_NoCookies401Unchanged: без кук отклоняет RequireAuth — 401, а не 403.
// Заголовок Origin здесь корректный: цепочка RequireSameOrigin → RequireAuth даёт ровно
// два разных кода на два разных негативных входа, и этот тест закрепляет первый из них.
func TestSetAccess_NoCookies401Unchanged(t *testing.T) {
	fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
		userRecord("ivanov", service.RoleUser, false),
	}})
	before := fixture.accessSnapshot()

	recorder := fixture.do(apiRequest{
		method: http.MethodPost,
		path:   "/api/v1/admin/users/ivanov/access",
		body:   `{"granted":true}`,
	})

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("статус = %d, ожидался 401 (Origin корректный, отклоняет RequireAuth)", recorder.Code)
	}
	fixture.assertAccessUnchanged(before)
}

// TestSetAccess_NoOriginIs403NotUnauthorized: тот же запрос вовсе без Origin отклоняется
// раньше — RequireSameOrigin стоит первым в цепочке, до всякой проверки кук.
func TestSetAccess_NoOriginIs403NotUnauthorized(t *testing.T) {
	fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
		userRecord("ivanov", service.RoleUser, false),
	}})
	before := fixture.accessSnapshot()

	recorder := fixture.do(apiRequest{
		method:   http.MethodPost,
		path:     "/api/v1/admin/users/ivanov/access",
		body:     `{"granted":true}`,
		noOrigin: true,
	})

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("статус = %d, ожидался 403 (отклоняет RequireSameOrigin до проверки кук)", recorder.Code)
	}
	fixture.assertAccessUnchanged(before)
}

// TestSetAccess_UnknownLogin404NoChange: незнакомый логин — 404, а не 500, независимо от
// того, как сервис различает «нет пользователя» и «нет заявки».
func TestSetAccess_UnknownLogin404NoChange(t *testing.T) {
	fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
		userRecord("ivanov", service.RoleUser, false),
		userRecord("RoGogDBD", service.RoleAdmin, false),
	}})
	before := fixture.accessSnapshot()

	recorder := fixture.do(apiRequest{
		method: http.MethodPost,
		path:   "/api/v1/admin/users/ghost/access",
		body:   `{"granted":true}`,
		as:     "RoGogDBD",
	})

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("статус = %d, ожидался 404", recorder.Code)
	}
	if got := errorCode(t, recorder); got != "not_found" {
		t.Errorf("код ошибки = %q, ожидался not_found", got)
	}
	fixture.assertAccessUnchanged(before)
	if _, ok := fixture.users.users["ghost"]; ok {
		t.Errorf("несуществующий пользователь заведён отказавшим запросом")
	}
}

// TestSetAccess_GrantAndRevoke204: выдача и отзыв — один маршрут, два значения granted.
// Отзыв проверяется отдельно: реализация, игнорирующая granted=false, прошла бы только
// первую половину (снятая галочка не имела бы эффекта — US-10).
func TestSetAccess_GrantAndRevoke204(t *testing.T) {
	fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
		userRecord("ivanov", service.RoleUser, false),
		userRecord("RoGogDBD", service.RoleAdmin, false),
	}})
	if recorder := fixture.do(apiRequest{method: http.MethodPost, path: "/api/v1/access/requests", as: "ivanov"}); recorder.Code != http.StatusCreated {
		t.Fatalf("подготовка заявки: статус = %d, ожидался 201", recorder.Code)
	}

	granted := fixture.do(apiRequest{
		method: http.MethodPost,
		path:   "/api/v1/admin/users/ivanov/access",
		body:   `{"granted":true}`,
		as:     "RoGogDBD",
	})
	if granted.Code != http.StatusNoContent {
		t.Fatalf("выдача: статус = %d, ожидался 204 (тело: %s)", granted.Code, granted.Body.String())
	}
	if granted.Body.Len() != 0 {
		t.Errorf("204 с непустым телом: %q", granted.Body.String())
	}
	if !fixture.hasAccess("ivanov") {
		t.Fatalf("флаг доступа не выставлен")
	}
	if got := fixture.requestStatus("ivanov"); got != "approved" {
		t.Errorf("статус заявки = %q, ожидался approved", got)
	}
	if got := fixture.requests.requests["ivanov"].DecidedBy; got != "RoGogDBD" {
		t.Errorf("decided_by = %q, ожидался RoGogDBD (личность из сессии, а не из тела запроса)", got)
	}

	revoked := fixture.do(apiRequest{
		method: http.MethodPost,
		path:   "/api/v1/admin/users/ivanov/access",
		body:   `{"granted":false}`,
		as:     "RoGogDBD",
	})
	if revoked.Code != http.StatusNoContent {
		t.Fatalf("отзыв: статус = %d, ожидался 204", revoked.Code)
	}
	if fixture.hasAccess("ivanov") {
		t.Fatalf("флаг доступа не снят")
	}
	if got := fixture.requestStatus("ivanov"); got != "rejected" {
		t.Errorf("статус заявки = %q, ожидался rejected", got)
	}
}

// assertInvalidRequest: 400 отдаёт фиксированный слаг, а не текст ошибки разбора JSON.
// Отдельная проверка, потому что ошибки decodeJSON не проходят через writeAPIDomainError —
// это самостоятельный источник 400, и конвенция http.go для него (writeAPIError с
// err.Error()) утекла бы клиенту имя неизвестного поля и внутренности сообщения парсера.
func assertInvalidRequest(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("статус = %d, ожидался 400", recorder.Code)
	}
	if got := errorCode(t, recorder); got != "invalid_request" {
		t.Errorf("тело = %q, ожидался фиксированный текст invalid_request", got)
	}
	if containsAny(recorder.Body.String(), "unknown field", "invalid json", "json:", "cannot unmarshal", "EOF") {
		t.Errorf("тело ответа содержит текст ошибки разбора: %q", recorder.Body.String())
	}
}

// TestSetAccess_UnknownField400: decodeJSON отвергает неизвестные поля
// (DisallowUnknownFields) — тело с посторонним ключом не должно молча применяться.
func TestSetAccess_UnknownField400(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "лишнее поле рядом с granted", body: `{"granted":true,"role":"admin"}`},
		{name: "только лишнее поле", body: `{"is_admin":true}`},
		{name: "тело не объект", body: `[{"granted":true}]`},
		{name: "два объекта подряд", body: `{"granted":true}{"granted":false}`},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
				userRecord("ivanov", service.RoleUser, false),
				userRecord("RoGogDBD", service.RoleAdmin, false),
			}})
			before := fixture.accessSnapshot()

			recorder := fixture.do(apiRequest{
				method: http.MethodPost,
				path:   "/api/v1/admin/users/ivanov/access",
				body:   testCase.body,
				as:     "RoGogDBD",
			})

			assertInvalidRequest(t, recorder)
			fixture.assertAccessUnchanged(before)
		})
	}
}

// TestSetAccess_MissingGrantedField400: пустое тело не должно означать «снять доступ».
// Значение по умолчанию у bool — false, и без явной проверки `{}` молча отзывал бы доступ.
func TestSetAccess_MissingGrantedField400(t *testing.T) {
	fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
		userRecord("ivanov", service.RoleUser, true),
		userRecord("RoGogDBD", service.RoleAdmin, false),
	}})
	before := fixture.accessSnapshot()

	for _, body := range []string{`{}`, `{"granted":null}`} {
		t.Run(body, func(t *testing.T) {
			recorder := fixture.do(apiRequest{
				method: http.MethodPost,
				path:   "/api/v1/admin/users/ivanov/access",
				body:   body,
				as:     "RoGogDBD",
			})

			assertInvalidRequest(t, recorder)
			fixture.assertAccessUnchanged(before)
		})
	}
}

// TestSetAccess_EmptyBody400: тело вообще отсутствует — тоже 400, а не паника и не отзыв.
func TestSetAccess_EmptyBody400(t *testing.T) {
	fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
		userRecord("ivanov", service.RoleUser, true),
		userRecord("RoGogDBD", service.RoleAdmin, false),
	}})
	before := fixture.accessSnapshot()

	recorder := fixture.do(apiRequest{
		method: http.MethodPost,
		path:   "/api/v1/admin/users/ivanov/access",
		as:     "RoGogDBD",
	})

	assertInvalidRequest(t, recorder)
	fixture.assertAccessUnchanged(before)
}

// ---------------------------------------------------------------------------------------
// Обвязка маршрутов целиком
// ---------------------------------------------------------------------------------------

// TestUsersRoutesAreClosedByAccessChain — главный смысл этой задачи: Task 4 оставила
// middleware написанными, но ни к чему не подключёнными, и /api/v1/users/ до сих пор
// открыт. Проверяются все три состояния сразу: без кук 401, с куками без доступа 403,
// с доступом — обычный ответ маршрута (Decision 6, US-16).
//
// Адрес — новой формы, без сегмента владельца (Task 6). Ветка «пользователь с доступом»
// проверяет не только 200, но и содержимое ответа: 404 «route not found» здесь возможен по
// совершенно другой причине (адрес не разобран), и утверждение «доступ выдан — маршрут
// работает» без проверки данных выродилось бы в «сервер что-то ответил».
func TestUsersRoutesAreClosedByAccessChain(t *testing.T) {
	fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
		userRecord("ivanov", service.RoleUser, false),
		userRecord("petrov", service.RoleUser, true),
	}})
	fixture.seedChat("petrov", "chat-petrov", "чат петрова")

	cases := []struct {
		name       string
		as         string
		wantStatus int
	}{
		{name: "без кук", as: "", wantStatus: http.StatusUnauthorized},
		{name: "сессия без личности", as: sessionWithoutLogin, wantStatus: http.StatusUnauthorized},
		{name: "пользователь без доступа", as: "ivanov", wantStatus: http.StatusForbidden},
		{name: "пользователь с доступом", as: "petrov", wantStatus: http.StatusOK},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := fixture.do(apiRequest{
				method: http.MethodGet,
				path:   "/api/v1/users/chats",
				as:     testCase.as,
			})

			if recorder.Code != testCase.wantStatus {
				t.Fatalf("статус = %d, ожидался %d (тело: %s)", recorder.Code, testCase.wantStatus, recorder.Body.String())
			}
			if testCase.wantStatus != http.StatusOK {
				return
			}

			var payload chatsPayload
			decodeBody(t, recorder, &payload)
			if len(payload.Items) != 1 || payload.Items[0].UserID != "petrov" || payload.Items[0].ID != "chat-petrov" {
				t.Fatalf("маршрут ответил 200, но не данными владельца сессии: %s", recorder.Body.String())
			}
		})
	}
}

// TestUsersMutatingRoutesRequireSameOrigin: изменяющий запрос к существующим маршрутам
// тоже закрыт проверкой источника — цепочка навешена целиком, а не наполовину.
func TestUsersMutatingRoutesRequireSameOrigin(t *testing.T) {
	fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
		userRecord("petrov", service.RoleUser, true),
	}})

	recorder := fixture.do(apiRequest{
		method:   http.MethodPost,
		path:     "/api/v1/users/chats",
		body:     `{"title":"новый чат"}`,
		as:       "petrov",
		noOrigin: true,
	})

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("статус = %d, ожидался 403", recorder.Code)
	}
	if titles := fixture.storedChatTitles("petrov"); len(titles) != 0 {
		t.Errorf("чат создан несмотря на отказ по Origin: %v", titles)
	}
}

// TestAuthMutatingRoutesRequireSameOrigin: Decision 8 закрывает и /auth/*-маршруты,
// меняющие состояние, — иначе чужая страница подставляет свой код авторизации в куки
// жертвы (login CSRF). GET-маршруты /auth/ при этом не затронуты.
func TestAuthMutatingRoutesRequireSameOrigin(t *testing.T) {
	fixture := newRouteFixture(t, fixtureOptions{})

	mutating := []string{"/auth/yandex/code", "/auth/refresh", "/auth/logout"}
	for _, path := range mutating {
		t.Run("POST "+path, func(t *testing.T) {
			recorder := fixture.do(apiRequest{
				method:   http.MethodPost,
				path:     path,
				body:     `{"code":"attacker-code"}`,
				noOrigin: true,
			})

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("статус = %d, ожидался 403", recorder.Code)
			}
		})

		t.Run("POST "+path+" с чужого origin", func(t *testing.T) {
			recorder := fixture.do(apiRequest{
				method: http.MethodPost,
				path:   path,
				body:   `{"code":"attacker-code"}`,
				origin: "http://evil.example",
			})

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("статус = %d, ожидался 403", recorder.Code)
			}
		})
	}

	safe := []string{"/auth/status", "/auth/me"}
	for _, path := range safe {
		t.Run("GET "+path, func(t *testing.T) {
			// Метод GET из проверки исключён: 401 (сессии нет) — это ответ самого
			// обработчика, а не отказ по источнику.
			recorder := fixture.do(apiRequest{method: http.MethodGet, path: path, noOrigin: true})

			if recorder.Code == http.StatusForbidden {
				t.Fatalf("GET %s отклонён по Origin — безопасный метод блокировать нельзя", path)
			}
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("статус = %d, ожидался 401", recorder.Code)
			}
		})
	}
}

// TestBuildRoutes_LegacyYandexTokenRouteIsGone: приём готового OAuth-токена удалён вместе
// с маршрутом (Decision 7, US-17) — 404 от ServeMux, а не 401/403 от middleware.
func TestBuildRoutes_LegacyYandexTokenRouteIsGone(t *testing.T) {
	fixture := newRouteFixture(t, fixtureOptions{})

	recorder := fixture.do(apiRequest{
		method: http.MethodPost,
		path:   "/auth/yandex",
		body:   `{"access_token":"stolen"}`,
	})

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("статус = %d, ожидался 404 (маршрут не зарегистрирован)", recorder.Code)
	}
}

// TestBuildRoutes_HealthAndMetricsStayOpen: /health и /metrics обвязкой не закрываются —
// иначе healthcheck контейнера и сбор метрик перестанут работать.
func TestBuildRoutes_HealthAndMetricsStayOpen(t *testing.T) {
	fixture := newRouteFixture(t, fixtureOptions{})

	for _, path := range []string{"/health", "/metrics"} {
		t.Run(path, func(t *testing.T) {
			recorder := fixture.do(apiRequest{method: http.MethodGet, path: path, noOrigin: true})

			if recorder.Code != http.StatusOK {
				t.Fatalf("статус = %d, ожидался 200", recorder.Code)
			}
		})
	}
}

// leakText изображает то, что реально приходит из хранилища: обёртка репозитория с текстом
// SQL и куском DSN внутри.
const leakText = `get user: SELECT * FROM users WHERE id = ? -- dsn=poshivon:poshivon@tcp(db:3306)`

func assertNoLeak(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("статус = %d, ожидался 500", recorder.Code)
	}
	if got := errorCode(t, recorder); got != "internal_error" {
		t.Errorf("тело = %q, ожидался фиксированный текст internal_error", got)
	}
	if containsAny(recorder.Body.String(), "SELECT", "dsn=", "poshivon", "3306", "get user", "list users", "set access") {
		t.Errorf("тело ответа содержит внутренний текст ошибки: %q", recorder.Body.String())
	}
}

// TestWriteAPIDomainError_NoInternalTextLeak: ошибка хранилища с SQL-текстом внутри не
// уходит клиенту ни через один маршрут нового контура (Decision 17). Ветки самой функции
// перечислены в http_test.go; здесь проверяется, что маршруты действительно ходят через
// неё, а не пишут ошибку в ответ напрямую.
//
// Ошибка здесь безусловная, поэтому на закрытых маршрутах отвечает middleware, а не хендлер:
// это отдельное — и тоже нужное — утверждение «отказ по недоступному хранилищу не течёт и
// не пропускает». Утечку из кода самого AccessHandler ловит тест ниже, где ошибка адресная.
func TestWriteAPIDomainError_NoInternalTextLeak(t *testing.T) {
	cases := []struct {
		name    string
		request apiRequest
	}{
		{
			name:    "GET /api/v1/access/me",
			request: apiRequest{method: http.MethodGet, path: "/api/v1/access/me", as: "ivanov"},
		},
		{
			name:    "POST /api/v1/access/requests",
			request: apiRequest{method: http.MethodPost, path: "/api/v1/access/requests", as: "ivanov"},
		},
		{
			name:    "GET /api/v1/admin/users",
			request: apiRequest{method: http.MethodGet, path: "/api/v1/admin/users", as: "RoGogDBD"},
		},
		{
			name: "POST /api/v1/admin/users/{login}/access",
			request: apiRequest{
				method: http.MethodPost,
				path:   "/api/v1/admin/users/ivanov/access",
				body:   `{"granted":true}`,
				as:     "RoGogDBD",
			},
		},
		{
			name:    "GET /api/v1/users/chats",
			request: apiRequest{method: http.MethodGet, path: "/api/v1/users/chats", as: "ivanov"},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
				userRecord("ivanov", service.RoleUser, true),
				userRecord("RoGogDBD", service.RoleAdmin, true),
			}})
			fixture.users.getErr = errors.New(leakText)

			assertNoLeak(t, fixture.do(testCase.request))
		})
	}
}

// TestAccessHandler_RepositoryFailureAfterGatePasses: сбой хранилища ВНУТРИ хендлера, когда
// авторизация уже пройдена.
//
// Разница с тестом выше принципиальна: там ошибка срабатывает на любом логине, и первым же
// GetUser падает сама проверка прав (RequireAdmin/RequireAccess зовут её на логине
// вызывающего) — до кода AccessHandler запрос не доходит вовсе. Здесь ошибка адресная:
// вызывающий-администратор читается успешно, гейт пропускает, и падает уже тот вызов,
// который делает хендлер. Без этого разделения обработка ошибок в ListUsers и SetAccess не
// покрыта ничем: замена writeAPIDomainError на запись err.Error() в тело осталась бы
// незамеченной.
func TestAccessHandler_RepositoryFailureAfterGatePasses(t *testing.T) {
	admins := []service.UserRecord{
		userRecord("ivanov", service.RoleUser, true),
		userRecord("RoGogDBD", service.RoleAdmin, true),
	}

	t.Run("ListUsers падает, гейт пройден", func(t *testing.T) {
		fixture := newRouteFixture(t, fixtureOptions{users: admins})
		// Проверка роли вызывающего проходит: GetUser("RoGogDBD") работает.
		fixture.users.listErr = errors.New("list users: " + leakText)

		assertNoLeak(t, fixture.do(apiRequest{method: http.MethodGet, path: "/api/v1/admin/users", as: "RoGogDBD"}))
	})

	t.Run("GetUser целевого логина падает, гейт пройден", func(t *testing.T) {
		fixture := newRouteFixture(t, fixtureOptions{users: admins})
		fixture.users.getErrByLogin = map[string]error{"ivanov": errors.New(leakText)}

		assertNoLeak(t, fixture.do(apiRequest{
			method: http.MethodPost,
			path:   "/api/v1/admin/users/ivanov/access",
			body:   `{"granted":true}`,
			as:     "RoGogDBD",
		}))
	})

	t.Run("SetAccess падает, гейт пройден", func(t *testing.T) {
		fixture := newRouteFixture(t, fixtureOptions{users: admins})
		fixture.users.setAccessErr = errors.New("set access: " + leakText)

		assertNoLeak(t, fixture.do(apiRequest{
			method: http.MethodPost,
			path:   "/api/v1/admin/users/ivanov/access",
			body:   `{"granted":true}`,
			as:     "RoGogDBD",
		}))
	})

	t.Run("CreateRequest падает, гейт пройден", func(t *testing.T) {
		fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
			userRecord("ivanov", service.RoleUser, false),
		}})
		fixture.requests.createErr = errors.New("create access request: " + leakText)

		assertNoLeak(t, fixture.do(apiRequest{method: http.MethodPost, path: "/api/v1/access/requests", as: "ivanov"}))
	})
}
