package dbservice

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RoGogDBD/PoshivOn/internal/service"
)

// postJSON шлёт POST-запрос с JSON-телом в mux и декодирует JSON-ответ в dst (если dst не
// nil). Общий хелпер вместо копирования одного и того же httptest-ритуала в пятнадцати
// тестах — сама логика каждого теста ниже в том, ЧТО отправляется и ЧТО должно записаться
// в fakeRepository, а не в механике HTTP-вызова.
func postJSON(t *testing.T, mux http.Handler, path string, body any, dst any) *httptest.ResponseRecorder {
	t.Helper()

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if dst != nil && rec.Code == http.StatusOK {
		if err := json.NewDecoder(rec.Body).Decode(dst); err != nil {
			t.Fatalf("decode response body: %v (body=%s)", err, rec.Body.String())
		}
	}
	return rec
}

func TestHealth(t *testing.T) {
	mux := Routes(newTestDeps(&fakeRepository{}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestEnsureUser(t *testing.T) {
	repo := &fakeRepository{}
	mux := Routes(newTestDeps(repo))

	rec := postJSON(t, mux, "/rpc/EnsureUser", ensureUserRequest{
		Login: "ivanov", Email: "ivanov@example.com", DisplayName: "Иванов",
	}, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if repo.lastLogin != "ivanov" || repo.lastEmail != "ivanov@example.com" || repo.lastDisplayName != "Иванов" {
		t.Errorf("EnsureUser вызван с login=%q email=%q display_name=%q, часть значений не дошла",
			repo.lastLogin, repo.lastEmail, repo.lastDisplayName)
	}
}

func TestGetUser(t *testing.T) {
	repo := &fakeRepository{userRecord: service.UserRecord{Login: "ivanov", Role: service.RoleAdmin}}
	mux := Routes(newTestDeps(repo))

	var got service.UserRecord
	rec := postJSON(t, mux, "/rpc/GetUser", loginRequest{Login: "ivanov"}, &got)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if repo.lastLogin != "ivanov" {
		t.Errorf("GetUser вызван с login=%q, ожидался ivanov", repo.lastLogin)
	}
	if got.Login != "ivanov" || got.Role != service.RoleAdmin {
		t.Errorf("ответ = %+v, ожидался UserRecord{Login: ivanov, Role: admin}", got)
	}
}

func TestGetUser_NotFound(t *testing.T) {
	repo := &fakeRepository{err: service.ErrNotFound}
	mux := Routes(newTestDeps(repo))

	rec := postJSON(t, mux, "/rpc/GetUser", loginRequest{Login: "unknown"}, nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}

func TestListUsers(t *testing.T) {
	repo := &fakeRepository{userRecords: []service.UserRecord{{Login: "a"}, {Login: "b"}}}
	mux := Routes(newTestDeps(repo))

	var got itemsResponse[service.UserRecord]
	rec := postJSON(t, mux, "/rpc/ListUsers", emptyRequest{}, &got)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if len(got.Items) != 2 {
		t.Errorf("items = %d, want 2", len(got.Items))
	}
}

func TestSetAccess(t *testing.T) {
	repo := &fakeRepository{}
	mux := Routes(newTestDeps(repo))

	rec := postJSON(t, mux, "/rpc/SetAccess", setAccessRequest{Login: "ivanov", Granted: true}, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if repo.lastLogin != "ivanov" || !repo.lastGranted {
		t.Errorf("SetAccess вызван с login=%q granted=%v, ожидался ivanov/true", repo.lastLogin, repo.lastGranted)
	}
}

func TestCreateRequest(t *testing.T) {
	repo := &fakeRepository{}
	mux := Routes(newTestDeps(repo))

	rec := postJSON(t, mux, "/rpc/CreateRequest", loginRequest{Login: "ivanov"}, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if repo.lastLogin != "ivanov" {
		t.Errorf("CreateRequest вызван с login=%q, ожидался ivanov", repo.lastLogin)
	}
}

func TestCreateRequest_Conflict(t *testing.T) {
	repo := &fakeRepository{err: service.ErrConflict}
	mux := Routes(newTestDeps(repo))

	rec := postJSON(t, mux, "/rpc/CreateRequest", loginRequest{Login: "ivanov"}, nil)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetRequest(t *testing.T) {
	repo := &fakeRepository{accessReq: service.AccessRequest{UserID: "ivanov", Status: "pending"}}
	mux := Routes(newTestDeps(repo))

	var got service.AccessRequest
	rec := postJSON(t, mux, "/rpc/GetRequest", loginRequest{Login: "ivanov"}, &got)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if repo.lastLogin != "ivanov" {
		t.Errorf("GetRequest вызван с login=%q, ожидался ivanov", repo.lastLogin)
	}
	if got.Status != "pending" {
		t.Errorf("Status = %q, want pending", got.Status)
	}
}

func TestDecideRequest(t *testing.T) {
	repo := &fakeRepository{}
	mux := Routes(newTestDeps(repo))

	rec := postJSON(t, mux, "/rpc/DecideRequest", decideRequestRequest{
		Login: "ivanov", Status: "approved", DecidedBy: "admin1",
	}, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if repo.lastLogin != "ivanov" || repo.lastStatus != "approved" || repo.lastDecidedBy != "admin1" {
		t.Errorf("DecideRequest вызван с login=%q status=%q decided_by=%q, часть значений не дошла",
			repo.lastLogin, repo.lastStatus, repo.lastDecidedBy)
	}
}

// TestDecideRequest_EmptyDecidedBy — та же граница атрибуции, что и у DeleteChat выше.
func TestDecideRequest_EmptyDecidedBy(t *testing.T) {
	repo := &fakeRepository{}
	mux := Routes(newTestDeps(repo))

	rec := postJSON(t, mux, "/rpc/DecideRequest", decideRequestRequest{Login: "ivanov", Status: "approved"}, nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if repo.lastLogin != "" {
		t.Error("репозиторий не должен был вызываться при пустом decided_by")
	}
}

func TestUpsertSettings(t *testing.T) {
	repo := &fakeRepository{}
	mux := Routes(newTestDeps(repo))

	settings := service.DefaultUserSettings()
	rec := postJSON(t, mux, "/rpc/UpsertSettings", upsertSettingsRequest{
		UserID: "ivanov", Settings: settings,
	}, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if repo.lastUserID != "ivanov" {
		t.Errorf("UpsertSettings вызван с user_id=%q, ожидался ivanov", repo.lastUserID)
	}
	if len(repo.lastSettings.Garments) != len(settings.Garments) {
		t.Errorf("настройки дошли не полностью: Garments len=%d, want %d",
			len(repo.lastSettings.Garments), len(settings.Garments))
	}
}

func TestGetSettings(t *testing.T) {
	repo := &fakeRepository{settings: service.DefaultUserSettings()}
	mux := Routes(newTestDeps(repo))

	var got service.UserSettings
	rec := postJSON(t, mux, "/rpc/GetSettings", userIDRequest{UserID: "ivanov"}, &got)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if repo.lastUserID != "ivanov" {
		t.Errorf("GetSettings вызван с user_id=%q, ожидался ivanov", repo.lastUserID)
	}
	if len(got.Garments) == 0 {
		t.Error("ответ не содержит Garments — сериализация настроек сломана")
	}
}

func TestCreateChat(t *testing.T) {
	repo := &fakeRepository{chat: service.Chat{ID: "chat-1", UserID: "ivanov", Title: "Новый чат"}}
	mux := Routes(newTestDeps(repo))

	input := service.Chat{ID: "chat-1", UserID: "ivanov", Title: "Новый чат", CreatedAt: time.Now().UTC()}
	var got service.Chat
	rec := postJSON(t, mux, "/rpc/CreateChat", input, &got)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if repo.lastChat.ID != "chat-1" || repo.lastChat.UserID != "ivanov" {
		t.Errorf("CreateChat вызван с chat=%+v, ожидался ID=chat-1 UserID=ivanov", repo.lastChat)
	}
	if got.ID != "chat-1" {
		t.Errorf("ответ ID = %q, want chat-1", got.ID)
	}
}

func TestListChats(t *testing.T) {
	repo := &fakeRepository{chats: []service.Chat{{ID: "c1"}, {ID: "c2"}}}
	mux := Routes(newTestDeps(repo))

	var got itemsResponse[service.Chat]
	rec := postJSON(t, mux, "/rpc/ListChats", userIDRequest{UserID: "ivanov"}, &got)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if repo.lastUserID != "ivanov" {
		t.Errorf("ListChats вызван с user_id=%q, ожидался ivanov", repo.lastUserID)
	}
	if len(got.Items) != 2 {
		t.Errorf("items = %d, want 2", len(got.Items))
	}
}

// TestDeleteChat — значения намеренно различны и ненулевые (UserID != DeletedBy, Hard=true,
// не zero-value bool), чтобы тест реально ловил перепутанные местами параметры в проводке
// handlers.go, а не проходил случайно из-за совпадающих/дефолтных значений (litmus-пробел,
// найденный ревью тестов).
func TestDeleteChat(t *testing.T) {
	repo := &fakeRepository{}
	mux := Routes(newTestDeps(repo))

	rec := postJSON(t, mux, "/rpc/DeleteChat", deleteChatRequest{
		UserID: "ivanov", ChatID: "chat-1", DeletedBy: "moderator-1", Hard: true,
	}, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if repo.lastUserID != "ivanov" {
		t.Errorf("UserID = %q, want ivanov", repo.lastUserID)
	}
	if repo.lastChatID != "chat-1" {
		t.Errorf("ChatID = %q, want chat-1", repo.lastChatID)
	}
	if repo.lastDeletedBy != "moderator-1" {
		t.Errorf("DeletedBy = %q, want moderator-1 (не должен совпасть с UserID)", repo.lastDeletedBy)
	}
	if !repo.lastHard {
		t.Error("Hard = false, want true — значение не дошло или подменено дефолтом")
	}
}

func TestDeleteChat_NotFound(t *testing.T) {
	repo := &fakeRepository{err: service.ErrNotFound}
	mux := Routes(newTestDeps(repo))

	rec := postJSON(t, mux, "/rpc/DeleteChat", deleteChatRequest{
		UserID: "ivanov", ChatID: "missing", DeletedBy: "ivanov",
	}, nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}

// TestDeleteChat_EmptyDeletedBy — граница атрибуции, добавленная поверх исходного набора
// эндпоинтов: пустой deleted_by отклоняется до обращения к репозиторию (security audit
// db-service, находка про потерю атрибуции при обходе AccessService/CostingService).
func TestDeleteChat_EmptyDeletedBy(t *testing.T) {
	repo := &fakeRepository{}
	mux := Routes(newTestDeps(repo))

	rec := postJSON(t, mux, "/rpc/DeleteChat", deleteChatRequest{UserID: "ivanov", ChatID: "chat-1"}, nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if repo.lastChatID != "" {
		t.Error("репозиторий не должен был вызываться при пустом deleted_by")
	}
}

func TestRestoreChat(t *testing.T) {
	repo := &fakeRepository{}
	mux := Routes(newTestDeps(repo))

	rec := postJSON(t, mux, "/rpc/RestoreChat", chatRefRequest{UserID: "ivanov", ChatID: "chat-1"}, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if repo.lastUserID != "ivanov" || repo.lastChatID != "chat-1" {
		t.Errorf("RestoreChat вызван с user_id=%q chat_id=%q, ожидался ivanov/chat-1",
			repo.lastUserID, repo.lastChatID)
	}
}

func TestAppendCalculation(t *testing.T) {
	repo := &fakeRepository{}
	mux := Routes(newTestDeps(repo))

	input := service.CalculationResult{UserID: "ivanov", ChatID: "chat-1", GarmentType: "Пиджак", Total: 5000}
	rec := postJSON(t, mux, "/rpc/AppendCalculation", input, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if repo.lastCalculation.UserID != "ivanov" || repo.lastCalculation.Total != 5000 {
		t.Errorf("AppendCalculation вызван с %+v, часть значений не дошла", repo.lastCalculation)
	}
}

func TestListCalculations(t *testing.T) {
	repo := &fakeRepository{calculations: []service.CalculationResult{{Total: 1}, {Total: 2}}}
	mux := Routes(newTestDeps(repo))

	var got itemsResponse[service.CalculationResult]
	rec := postJSON(t, mux, "/rpc/ListCalculations", chatRefRequest{UserID: "ivanov", ChatID: "chat-1"}, &got)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if repo.lastUserID != "ivanov" || repo.lastChatID != "chat-1" {
		t.Errorf("ListCalculations вызван с user_id=%q chat_id=%q, ожидался ivanov/chat-1",
			repo.lastUserID, repo.lastChatID)
	}
	if len(got.Items) != 2 {
		t.Errorf("items = %d, want 2", len(got.Items))
	}
}

// TestRoutes_UnknownPath — на неизвестном пути мультиплексор отвечает штатным net/http 404,
// не паникует и не отдаёт 200. Не находка сама по себе, а защита от будущей регрессии в
// самой сборке роутов (например, опечатка в паттерне).
func TestRoutes_UnknownPath(t *testing.T) {
	mux := Routes(newTestDeps(&fakeRepository{}))

	req := httptest.NewRequest(http.MethodPost, "/rpc/DoesNotExist", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 для неизвестного пути", rec.Code)
	}
}
