package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/RoGogDBD/PoshivOn/internal/service"
)

// internalDetail изображает то, что сегодня утекает в ответ: обёртку репозитория с текстом
// SQL и именем таблицы. Ни одна ветка writeAPIDomainError не имеет права показать это клиенту
// (Decision 17).
const internalDetail = `get user: SELECT * FROM users WHERE id = 'RoGogDBD' -- dsn=poshivon:poshivon@tcp(db:3306)`

type domainErrorCase struct {
	name       string
	err        error
	wantStatus int
	wantBody   string
}

// domainErrorCases перечисляет все ветки функции, а не только новые: Decision 17 применяется
// к 400/404/429/503/504 ровно так же, как к 403/409.
func domainErrorCases() []domainErrorCase {
	return []domainErrorCase{
		{
			name:       "400 invalid argument",
			err:        fmt.Errorf("%s: %w", internalDetail, service.ErrInvalidArgument),
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid_request",
		},
		{
			name:       "403 forbidden",
			err:        fmt.Errorf("%s: %w", internalDetail, service.ErrForbidden),
			wantStatus: http.StatusForbidden,
			wantBody:   "forbidden",
		},
		{
			name:       "404 not found",
			err:        fmt.Errorf("%s: %w", internalDetail, service.ErrNotFound),
			wantStatus: http.StatusNotFound,
			wantBody:   "not_found",
		},
		{
			name:       "409 conflict",
			err:        fmt.Errorf("%s: %w", internalDetail, service.ErrConflict),
			wantStatus: http.StatusConflict,
			wantBody:   "conflict",
		},
		{
			name:       "429 rate limited",
			err:        fmt.Errorf("deepseek: rate_limit_exceeded: %s", internalDetail),
			wantStatus: http.StatusTooManyRequests,
			wantBody:   "rate_limited",
		},
		{
			name:       "503 service unavailable",
			err:        fmt.Errorf("deepseek: service_unavailable: %s", internalDetail),
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   "service_unavailable",
		},
		{
			name:       "504 timeout",
			err:        fmt.Errorf("deepseek: timeout while waiting: %s", internalDetail),
			wantStatus: http.StatusGatewayTimeout,
			wantBody:   "timeout",
		},
		{
			name:       "500 unclassified",
			err:        errors.New(internalDetail),
			wantStatus: http.StatusInternalServerError,
			wantBody:   "internal_error",
		},
	}
}

// TestWriteAPIDomainError_NoInternalTextInAnyBranch: ни один ответ не содержит текста
// исходной ошибки, и каждая ветка отдаёт фиксированный текст своей категории.
func TestWriteAPIDomainError_NoInternalTextInAnyBranch(t *testing.T) {
	// Логи функции не должны сыпаться в вывод тестов; сам факт логирования проверяет
	// отдельный тест ниже.
	restore := silenceLog(t)
	defer restore()

	for _, testCase := range domainErrorCases() {
		t.Run(testCase.name, func(t *testing.T) {
			rec := httptest.NewRecorder()

			writeAPIDomainError(rec, testCase.err)

			if rec.Code != testCase.wantStatus {
				t.Fatalf("статус = %d, ожидался %d", rec.Code, testCase.wantStatus)
			}
			if got := errorCode(t, rec); got != testCase.wantBody {
				t.Errorf("тело = %q, ожидался фиксированный текст %q", got, testCase.wantBody)
			}
			body := rec.Body.String()
			for _, leak := range []string{"SELECT", "users", "RoGogDBD", "dsn=", "poshivon", "deepseek", "get user"} {
				if strings.Contains(body, leak) {
					t.Errorf("тело ответа содержит внутренний фрагмент %q: %q", leak, body)
				}
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, ожидался application/json", got)
			}
		})
	}
}

// TestWriteAPIDomainError_LogsOriginalError: текст, убранный из ответа, обязан остаться
// на сервере — иначе диагностика 500-х пропадает вместе с утечкой.
func TestWriteAPIDomainError_LogsOriginalError(t *testing.T) {
	for _, testCase := range domainErrorCases() {
		t.Run(testCase.name, func(t *testing.T) {
			var buffer bytes.Buffer
			restore := captureLog(t, &buffer)
			defer restore()

			writeAPIDomainError(httptest.NewRecorder(), testCase.err)

			if !strings.Contains(buffer.String(), internalDetail) {
				t.Errorf("исходная ошибка не попала в лог: %q", buffer.String())
			}
		})
	}
}

// TestWriteAPIDomainError_NilErrorDoesNotPanic: через эту функцию проходит каждый ответ об
// ошибке API, и ветки сопоставления по тексту вызывают err.Error() — на nil это была бы
// паника вместо ответа.
func TestWriteAPIDomainError_NilErrorDoesNotPanic(t *testing.T) {
	restore := silenceLog(t)
	defer restore()

	rec := httptest.NewRecorder()
	writeAPIDomainError(rec, nil)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("статус = %d, ожидался 500", rec.Code)
	}
	if got := errorCode(t, rec); got != "internal_error" {
		t.Errorf("тело = %q, ожидался internal_error", got)
	}
}

// =======================================================================================
// /api/v1/users/** — владелец берётся из сессии, а не из адреса (Decision 6, US-15/US-16).
//
// Маршруты собираются той же функцией, что и в проде (BuildRoutes через newRouteFixture),
// а не руками: утверждение здесь — не «обработчик умеет», а «по этому адресу в проде
// отвечает именно он и именно за этим владельцем».
//
// Предусловия негативных тестов заводятся прямо в хранилище (fixture.costing), а не через
// проверяемые же маршруты: иначе тест «чужие данные недостижимы» опирался бы на тот самый
// обработчик, чью корректность он и доказывает.
// =======================================================================================

const (
	usersSettingsRoute       = "/api/v1/users/settings"
	usersChatsRoute          = "/api/v1/users/chats"
	usersMarketFeedbackRoute = "/api/v1/users/market-feedback"
)

func usersChatRoute(chatID string) string         { return usersChatsRoute + "/" + chatID }
func usersRestoreRoute(chatID string) string      { return usersChatRoute(chatID) + "/restore" }
func usersCalculateRoute(chatID string) string    { return usersChatRoute(chatID) + "/calculate" }
func usersCalculationsRoute(chatID string) string { return usersChatRoute(chatID) + "/calculations" }
func legacyUsersRoute(login, rest string) string  { return "/api/v1/users/" + login + "/" + rest }

// Формы ответов проверяются по именам JSON-полей, а не по структурам Go: с клиентом
// согласован именно контракт по проводу.
type chatPayload struct {
	UserID string `json:"user_id"`
	ID     string `json:"id"`
	Title  string `json:"title"`
}

type chatsPayload struct {
	Items []chatPayload `json:"items"`
}

type calculationPayload struct {
	UserID string `json:"user_id"`
	ChatID string `json:"chat_id"`
}

type calculationsPayload struct {
	Items []calculationPayload `json:"items"`
}

// settingsPayload читает единственное поле, которым настройки одного владельца отличаются
// от настроек другого в этих тестах, — по нему видно, чьи настройки вернулись.
type settingsPayload struct {
	PricingRules struct {
		LaborMinuteRate int64 `json:"labor_minute_rate"`
	} `json:"pricing_rules"`
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("не сериализуется тело запроса: %v", err)
	}
	return string(raw)
}

// sampleOrderBody — минимальный валидный заказ на значениях из DefaultUserSettings.
func sampleOrderBody(t *testing.T) string {
	t.Helper()

	return mustJSON(t, service.OrderInput{
		GarmentType:   "Платье",
		MaterialType:  "Хлопок",
		Quantity:      2,
		Urgency:       "Стандарт",
		Fittings:      1,
		MarketSegment: "Средний",
	})
}

func requireStatus(t *testing.T, recorder *httptest.ResponseRecorder, want int) {
	t.Helper()

	if recorder.Code != want {
		t.Fatalf("статус = %d, ожидался %d (тело: %s)", recorder.Code, want, recorder.Body.String())
	}
}

// --- посев состояния в обход маршрутов ------------------------------------------------

// seedSettings кладёт настройки владельца прямо в хранилище. laborMinuteRate служит меткой
// владельца: по нему в ответе видно, чьи настройки вернулись.
func (f *routeFixture) seedSettings(login string, laborMinuteRate int64) {
	f.t.Helper()

	settings := service.DefaultUserSettings()
	settings.PricingRules.LaborMinuteRate = laborMinuteRate
	if err := f.costing.UpsertSettings(context.Background(), login, settings); err != nil {
		f.t.Fatalf("посев настроек %q: %v", login, err)
	}
}

func (f *routeFixture) seedChat(login, chatID, title string) {
	f.t.Helper()

	now := time.Now().UTC()
	if _, err := f.costing.CreateChat(context.Background(), service.Chat{
		UserID:    login,
		ID:        chatID,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		f.t.Fatalf("посев чата %q/%q: %v", login, chatID, err)
	}
}

func (f *routeFixture) seedCalculation(login, chatID string) {
	f.t.Helper()

	if err := f.costing.AppendCalculation(context.Background(), service.CalculationResult{
		UserID:    login,
		ChatID:    chatID,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		f.t.Fatalf("посев расчёта %q/%q: %v", login, chatID, err)
	}
}

// --- чтение состояния в обход маршрутов -----------------------------------------------

func (f *routeFixture) storedChatTitles(login string) []string {
	f.t.Helper()

	chats, err := f.costing.ListChats(context.Background(), login)
	if err != nil {
		f.t.Fatalf("чтение чатов %q: %v", login, err)
	}
	titles := make([]string, 0, len(chats))
	for _, chat := range chats {
		titles = append(titles, chat.Title)
	}
	return titles
}

func (f *routeFixture) storedCalculationCount(login, chatID string) int {
	f.t.Helper()

	items, err := f.costing.ListCalculations(context.Background(), login, chatID)
	if err != nil {
		f.t.Fatalf("чтение расчётов %q/%q: %v", login, chatID, err)
	}
	return len(items)
}

// assertUntouched — «данные владельца не изменились ни одним из способов». Негативный тест
// без этой проверки не отличает отказ ДО записи от отказа ПОСЛЕ неё.
func (f *routeFixture) assertUntouched(login, chatID string, wantTitles []string, wantCalculations int) {
	f.t.Helper()

	if got := strings.Join(f.storedChatTitles(login), "|"); got != strings.Join(wantTitles, "|") {
		f.t.Fatalf("чаты %q = %v, ожидались %v", login, got, wantTitles)
	}
	if got := f.storedCalculationCount(login, chatID); got != wantCalculations {
		f.t.Fatalf("расчётов у %q в чате %q = %d, ожидалось %d", login, chatID, got, wantCalculations)
	}
}

// usersFixture — общая расстановка: у Петрова есть доступ и данные, у Иванова доступа нет.
func usersFixture(t *testing.T) *routeFixture {
	t.Helper()

	fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
		userRecord("petrov", service.RoleUser, true),
		userRecord("ivanov", service.RoleUser, false),
	}})
	fixture.seedSettings("petrov", 18)
	fixture.seedChat("petrov", "chat-petrov", "секрет петрова")
	fixture.seedCalculation("petrov", "chat-petrov")
	return fixture
}

// ---------------------------------------------------------------------------------------
// 401 / 403
// ---------------------------------------------------------------------------------------

// TestHandleUsers_NoCookies_401: без кук ни один маршрут владельца не отвечает данными.
// Проверяется на НОВЫХ адресах (без сегмента логина): цепочка навешена на префикс, но после
// смены формы адресов утверждение «эти конкретные адреса закрыты» нужно закрепить заново.
func TestHandleUsers_NoCookies_401(t *testing.T) {
	fixture := usersFixture(t)
	before := fixture.storedChatTitles("petrov")

	cases := []apiRequest{
		{method: http.MethodGet, path: usersChatsRoute},
		{method: http.MethodGet, path: usersSettingsRoute},
		{method: http.MethodPost, path: usersCalculateRoute("chat-petrov"), body: sampleOrderBody(t)},
	}

	for _, request := range cases {
		t.Run(request.method+" "+request.path, func(t *testing.T) {
			recorder := fixture.do(request)

			requireStatus(t, recorder, http.StatusUnauthorized)
			if got := errorCode(t, recorder); got != "access_cookie_missing" {
				t.Errorf("код ошибки = %q, ожидался access_cookie_missing", got)
			}
			if strings.Contains(recorder.Body.String(), "секрет петрова") {
				t.Errorf("в теле отказа чужие данные: %q", recorder.Body.String())
			}
		})
	}

	fixture.assertUntouched("petrov", "chat-petrov", before, 1)
}

// TestHandleUsers_NoAccess_403: US-16 — снятый доступ должен иметь эффект именно здесь, за
// маршрутами владельца лежат все данные продукта. Логин Иванова заведён и опознан, отказ
// даёт RequireAccess, а не проверка сессии.
func TestHandleUsers_NoAccess_403(t *testing.T) {
	fixture := usersFixture(t)
	before := fixture.storedChatTitles("petrov")

	cases := []apiRequest{
		{method: http.MethodGet, path: usersChatsRoute, as: "ivanov"},
		{method: http.MethodGet, path: usersSettingsRoute, as: "ivanov"},
		{method: http.MethodPost, path: usersCalculateRoute("chat-petrov"), body: sampleOrderBody(t), as: "ivanov"},
	}

	for _, request := range cases {
		t.Run(request.method+" "+request.path, func(t *testing.T) {
			recorder := fixture.do(request)

			requireStatus(t, recorder, http.StatusForbidden)
			if got := errorCode(t, recorder); got != "forbidden" {
				t.Errorf("код ошибки = %q, ожидался forbidden", got)
			}
		})
	}

	// Отказ случился ДО записи: у Петрова ничего не прибавилось и не убавилось.
	fixture.assertUntouched("petrov", "chat-petrov", before, 1)
	// И в собственном пространстве Иванова тоже ничего не появилось: реализация, которая
	// сначала пишет от имени вызывающего, а отказывает потом, отличалась бы только этим.
	fixture.assertUntouched("ivanov", "chat-petrov", nil, 0)
}

// ---------------------------------------------------------------------------------------
// Полный проход по всем семи формам маршрута от лица владельца с доступом
// ---------------------------------------------------------------------------------------

// TestHandleUsers_WithAccess_200_OwnerScoped: каждая из форм маршрута отвечает по-своему
// штатно, и во всех ответах владелец — логин из сессии.
//
// Сегмента владельца в адресе больше нет, поэтому единственный способ ошибиться — взять
// владельца не оттуда; тест смотрит на user_id в теле ответа, а не только на статус.
func TestHandleUsers_WithAccess_200_OwnerScoped(t *testing.T) {
	fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
		userRecord("petrov", service.RoleUser, true),
	}})

	t.Run("POST /settings 204", func(t *testing.T) {
		recorder := fixture.do(apiRequest{
			method: http.MethodPost,
			path:   usersSettingsRoute,
			body:   mustJSON(t, service.DefaultUserSettings()),
			as:     "petrov",
		})

		requireStatus(t, recorder, http.StatusNoContent)
		// Настройки легли именно Петрову: без этого «сохранил кому-то» и «сохранил тому»
		// неразличимы по коду ответа.
		if _, err := fixture.costing.GetSettings(context.Background(), "petrov"); err != nil {
			t.Fatalf("настройки Петрова не сохранены: %v", err)
		}
	})

	t.Run("GET /settings 200", func(t *testing.T) {
		recorder := fixture.do(apiRequest{method: http.MethodGet, path: usersSettingsRoute, as: "petrov"})

		requireStatus(t, recorder, http.StatusOK)
		var payload settingsPayload
		decodeBody(t, recorder, &payload)
		if payload.PricingRules.LaborMinuteRate != 18 {
			t.Errorf("labor_minute_rate = %d, ожидался 18 (настройки Петрова)", payload.PricingRules.LaborMinuteRate)
		}
	})

	var chatID string

	t.Run("POST /chats 201", func(t *testing.T) {
		recorder := fixture.do(apiRequest{
			method: http.MethodPost,
			path:   usersChatsRoute,
			body:   `{"title":"чат петрова"}`,
			as:     "petrov",
		})

		requireStatus(t, recorder, http.StatusCreated)
		var payload chatPayload
		decodeBody(t, recorder, &payload)
		if payload.UserID != "petrov" {
			t.Fatalf("user_id созданного чата = %q, ожидался petrov (логин из сессии)", payload.UserID)
		}
		if payload.ID == "" {
			t.Fatalf("создан чат без идентификатора: %s", recorder.Body.String())
		}
		chatID = payload.ID
	})

	if chatID == "" {
		t.Fatalf("дальнейшие формы маршрута без идентификатора чата непроверяемы")
	}

	// Чужой логин в query string — единственный оставшийся после удаления сегмента канал,
	// куда его вообще можно подставить. Владелец обязан остаться прежним.
	t.Run("GET /chats 200 и чужой логин в query не действует", func(t *testing.T) {
		recorder := fixture.do(apiRequest{
			method: http.MethodGet,
			path:   usersChatsRoute + "?user_id=ivanov",
			as:     "petrov",
		})

		requireStatus(t, recorder, http.StatusOK)
		var payload chatsPayload
		decodeBody(t, recorder, &payload)
		if len(payload.Items) != 1 {
			t.Fatalf("чатов в ответе = %d, ожидался 1: %s", len(payload.Items), recorder.Body.String())
		}
		if payload.Items[0].UserID != "petrov" || payload.Items[0].ID != chatID {
			t.Errorf("вернулся чат %+v, ожидался собственный чат Петрова %q", payload.Items[0], chatID)
		}
	})

	t.Run("POST /chats/{id}/calculate 200", func(t *testing.T) {
		recorder := fixture.do(apiRequest{
			method: http.MethodPost,
			path:   usersCalculateRoute(chatID),
			body:   sampleOrderBody(t),
			as:     "petrov",
		})

		requireStatus(t, recorder, http.StatusOK)
		var payload calculationPayload
		decodeBody(t, recorder, &payload)
		if payload.UserID != "petrov" {
			t.Errorf("user_id расчёта = %q, ожидался petrov", payload.UserID)
		}
		if payload.ChatID != chatID {
			t.Errorf("chat_id расчёта = %q, ожидался %q", payload.ChatID, chatID)
		}
	})

	t.Run("GET /chats/{id}/calculations 200", func(t *testing.T) {
		recorder := fixture.do(apiRequest{method: http.MethodGet, path: usersCalculationsRoute(chatID), as: "petrov"})

		requireStatus(t, recorder, http.StatusOK)
		var payload calculationsPayload
		decodeBody(t, recorder, &payload)
		if len(payload.Items) != 1 {
			t.Fatalf("расчётов в ответе = %d, ожидался 1: %s", len(payload.Items), recorder.Body.String())
		}
		if payload.Items[0].UserID != "petrov" {
			t.Errorf("user_id расчёта = %q, ожидался petrov", payload.Items[0].UserID)
		}
	})

	t.Run("DELETE /chats/{id} 204", func(t *testing.T) {
		recorder := fixture.do(apiRequest{method: http.MethodDelete, path: usersChatRoute(chatID), as: "petrov"})

		requireStatus(t, recorder, http.StatusNoContent)
		if titles := fixture.storedChatTitles("petrov"); len(titles) != 0 {
			t.Errorf("чат Петрова не удалён: %v", titles)
		}
	})

	t.Run("POST /chats/{id}/restore 204", func(t *testing.T) {
		recorder := fixture.do(apiRequest{method: http.MethodPost, path: usersRestoreRoute(chatID), as: "petrov"})

		requireStatus(t, recorder, http.StatusNoContent)
		if titles := fixture.storedChatTitles("petrov"); len(titles) != 1 {
			t.Errorf("чат Петрова не восстановлен: %v", titles)
		}
	})

	// В фикстуре DeepSeek не сконфигурирован, поэтому штатный ответ этой формы — 503 из
	// ветки h.deepseek == nil. Существенно здесь другое: 503, а не 404, — значит адрес без
	// сегмента владельца разобран и до обработчика дошёл.
	t.Run("POST /market-feedback 503, а не 404", func(t *testing.T) {
		recorder := fixture.do(apiRequest{
			method: http.MethodPost,
			path:   usersMarketFeedbackRoute,
			body:   `{"garment_type":"Платье"}`,
			as:     "petrov",
		})

		requireStatus(t, recorder, http.StatusServiceUnavailable)
	})
}

// ---------------------------------------------------------------------------------------
// US-15: чужого владельца не достать ничем
// ---------------------------------------------------------------------------------------

// TestHandleUsers_ForeignOwnerUnreachable: после удаления сегмента адреса остаются ровно два
// места, куда вызывающий может вписать чужой логин, — тело и query string. Ни одно из них не
// должно менять владельца ни на чтении, ни на записи.
func TestHandleUsers_ForeignOwnerUnreachable(t *testing.T) {
	newFixture := func(t *testing.T) *routeFixture {
		t.Helper()

		fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
			userRecord("petrov", service.RoleUser, true),
			userRecord("ivanov", service.RoleUser, true),
		}})
		// Метка владельца: 18 у Петрова против 99 у Иванова. Одинаковые настройки прошли бы
		// проверку и при подмене владельца.
		fixture.seedSettings("petrov", 18)
		fixture.seedSettings("ivanov", 99)
		fixture.seedChat("petrov", "chat-petrov", "секрет петрова")
		fixture.seedCalculation("petrov", "chat-petrov")
		return fixture
	}

	t.Run("чужой логин в query на чтении настроек", func(t *testing.T) {
		fixture := newFixture(t)

		recorder := fixture.do(apiRequest{
			method: http.MethodGet,
			path:   usersSettingsRoute + "?user_id=petrov",
			as:     "ivanov",
		})

		requireStatus(t, recorder, http.StatusOK)
		var payload settingsPayload
		decodeBody(t, recorder, &payload)
		if payload.PricingRules.LaborMinuteRate != 99 {
			t.Errorf("labor_minute_rate = %d, ожидался 99 — вернулись настройки чужого владельца",
				payload.PricingRules.LaborMinuteRate)
		}
	})

	t.Run("чужой логин в query на списке чатов", func(t *testing.T) {
		fixture := newFixture(t)

		recorder := fixture.do(apiRequest{
			method: http.MethodGet,
			path:   usersChatsRoute + "?user_id=petrov",
			as:     "ivanov",
		})

		requireStatus(t, recorder, http.StatusOK)
		var payload chatsPayload
		decodeBody(t, recorder, &payload)
		if len(payload.Items) != 0 {
			t.Fatalf("у Иванова своих чатов нет, а вернулось %d: %s", len(payload.Items), recorder.Body.String())
		}
		if strings.Contains(recorder.Body.String(), "секрет петрова") {
			t.Errorf("в ответе чужой чат: %q", recorder.Body.String())
		}
	})

	// Поле владельца в теле отвергается разбором (DisallowUnknownFields) — то есть канала
	// «подставить владельца телом» не существует, а не «он есть, но игнорируется».
	t.Run("чужой логин в теле создания чата", func(t *testing.T) {
		fixture := newFixture(t)
		before := fixture.storedChatTitles("petrov")

		recorder := fixture.do(apiRequest{
			method: http.MethodPost,
			path:   usersChatsRoute,
			body:   `{"title":"подделка","user_id":"petrov"}`,
			as:     "ivanov",
		})

		requireStatus(t, recorder, http.StatusBadRequest)
		fixture.assertUntouched("petrov", "chat-petrov", before, 1)
		if titles := fixture.storedChatTitles("ivanov"); len(titles) != 0 {
			t.Errorf("чат создан несмотря на отказ: %v", titles)
		}
	})

	// Идентификатор чужого чата в адресе — это идентификатор в пространстве владельца из
	// сессии, а не ключ к чужим данным.
	t.Run("идентификатор чужого чата на удалении", func(t *testing.T) {
		fixture := newFixture(t)
		before := fixture.storedChatTitles("petrov")

		recorder := fixture.do(apiRequest{
			method: http.MethodDelete,
			path:   usersChatRoute("chat-petrov"),
			as:     "ivanov",
		})

		requireStatus(t, recorder, http.StatusNotFound)
		fixture.assertUntouched("petrov", "chat-petrov", before, 1)
	})

	t.Run("идентификатор чужого чата на чтении расчётов", func(t *testing.T) {
		fixture := newFixture(t)

		recorder := fixture.do(apiRequest{
			method: http.MethodGet,
			path:   usersCalculationsRoute("chat-petrov"),
			as:     "ivanov",
		})

		requireStatus(t, recorder, http.StatusOK)
		var payload calculationsPayload
		decodeBody(t, recorder, &payload)
		if len(payload.Items) != 0 {
			t.Fatalf("вернулись расчёты чужого чата: %s", recorder.Body.String())
		}
	})

	// Запись по чужому идентификатору чата ложится в пространство вызывающего, а история
	// Петрова остаётся ровно той же.
	t.Run("расчёт по идентификатору чужого чата не пишет чужому", func(t *testing.T) {
		fixture := newFixture(t)
		before := fixture.storedChatTitles("petrov")

		recorder := fixture.do(apiRequest{
			method: http.MethodPost,
			path:   usersCalculateRoute("chat-petrov"),
			body:   sampleOrderBody(t),
			as:     "ivanov",
		})

		requireStatus(t, recorder, http.StatusOK)
		var payload calculationPayload
		decodeBody(t, recorder, &payload)
		if payload.UserID != "ivanov" {
			t.Errorf("user_id расчёта = %q, ожидался ivanov", payload.UserID)
		}
		fixture.assertUntouched("petrov", "chat-petrov", before, 1)
		if got := fixture.storedCalculationCount("ivanov", "chat-petrov"); got != 1 {
			t.Errorf("расчёт Иванова = %d, ожидался 1 (запись в его собственном пространстве)", got)
		}
	})
}

// ---------------------------------------------------------------------------------------
// Старая форма адреса
// ---------------------------------------------------------------------------------------

// TestRegisterRoutes_LegacyUserIDSegment_NotFound: адрес прежней формы
// /api/v1/users/{login}/... больше не существует. Требование сильнее, чем «404»: лишний
// сегмент не должен ни разбираться как владелец, ни случайно попадать в другую ветку switch.
func TestRegisterRoutes_LegacyUserIDSegment_NotFound(t *testing.T) {
	cases := []struct {
		name    string
		request apiRequest
	}{
		{
			name:    "GET чужие чаты",
			request: apiRequest{method: http.MethodGet, path: legacyUsersRoute("petrov", "chats"), as: "ivanov"},
		},
		{
			name:    "GET собственные чаты старой формой",
			request: apiRequest{method: http.MethodGet, path: legacyUsersRoute("ivanov", "chats"), as: "ivanov"},
		},
		{
			name:    "GET чужие настройки",
			request: apiRequest{method: http.MethodGet, path: legacyUsersRoute("petrov", "settings"), as: "ivanov"},
		},
		{
			name: "POST чат чужому владельцу",
			request: apiRequest{
				method: http.MethodPost,
				path:   legacyUsersRoute("petrov", "chats"),
				body:   `{"title":"подделка"}`,
				as:     "ivanov",
			},
		},
		{
			name: "POST чужие настройки",
			request: apiRequest{
				method: http.MethodPost,
				path:   legacyUsersRoute("petrov", "settings"),
				body:   `{}`,
				as:     "ivanov",
			},
		},
		{
			name:    "DELETE чужой чат",
			request: apiRequest{method: http.MethodDelete, path: legacyUsersRoute("petrov", "chats/chat-petrov"), as: "ivanov"},
		},
		{
			name: "POST восстановление чужого чата",
			request: apiRequest{
				method: http.MethodPost,
				path:   legacyUsersRoute("petrov", "chats/chat-petrov/restore"),
				as:     "ivanov",
			},
		},
		{
			name: "POST расчёт в чужом чате",
			request: apiRequest{
				method: http.MethodPost,
				path:   legacyUsersRoute("petrov", "chats/chat-petrov/calculate"),
				body:   sampleOrderBody(t),
				as:     "ivanov",
			},
		},
		{
			name:    "GET расчёты чужого чата",
			request: apiRequest{method: http.MethodGet, path: legacyUsersRoute("petrov", "chats/chat-petrov/calculations"), as: "ivanov"},
		},
		{
			name: "POST market-feedback от чужого имени",
			request: apiRequest{
				method: http.MethodPost,
				path:   legacyUsersRoute("petrov", "market-feedback"),
				body:   `{"garment_type":"Платье"}`,
				as:     "ivanov",
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newRouteFixture(t, fixtureOptions{users: []service.UserRecord{
				userRecord("petrov", service.RoleUser, true),
				userRecord("ivanov", service.RoleUser, true),
			}})
			fixture.seedSettings("petrov", 18)
			fixture.seedSettings("ivanov", 99)
			fixture.seedChat("petrov", "chat-petrov", "секрет петрова")
			fixture.seedCalculation("petrov", "chat-petrov")
			before := fixture.storedChatTitles("petrov")

			recorder := fixture.do(testCase.request)

			requireStatus(t, recorder, http.StatusNotFound)
			if got := errorCode(t, recorder); got != "route not found" {
				t.Errorf("тело = %q, ожидалось route not found", got)
			}
			if strings.Contains(recorder.Body.String(), "секрет петрова") {
				t.Errorf("в теле ответа чужие данные: %q", recorder.Body.String())
			}
			// Ни один вариант старой формы не должен ничего изменить у Петрова.
			fixture.assertUntouched("petrov", "chat-petrov", before, 1)
		})
	}
}

func captureLog(t *testing.T, buffer *bytes.Buffer) func() {
	t.Helper()

	previousFlags := log.Flags()
	log.SetOutput(buffer)
	log.SetFlags(0)
	return func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(previousFlags)
	}
}

func silenceLog(t *testing.T) func() {
	t.Helper()
	return captureLog(t, &bytes.Buffer{})
}
