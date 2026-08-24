package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/RoGogDBD/PoshivOn/internal/service"
)

// stubTokenSource — фиксированный или отказывающий источник токена для тестов
// HTTPRepository, не требующий настоящего metadata-сервиса.
type stubTokenSource struct {
	token string
	err   error
	calls int
}

func (s *stubTokenSource) Token(ctx context.Context) (string, error) {
	s.calls++
	return s.token, s.err
}

// capturedRequest — метод, путь, тело и заголовок Authorization последнего запроса,
// полученного тестовым сервером.
type capturedRequest struct {
	method      string
	path        string
	body        string
	auth        string
	contentType string
}

func newRecordingServer(t *testing.T, status int, responseBody string) (*httptest.Server, *capturedRequest) {
	t.Helper()
	captured := &capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		captured.method = r.Method
		captured.path = r.URL.Path
		captured.body = string(body)
		captured.auth = r.Header.Get("Authorization")
		captured.contentType = r.Header.Get("Content-Type")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, responseBody)
	}))
	return srv, captured
}

func newTestRepo(baseURL string) *HTTPRepository {
	return NewHTTPRepository(baseURL, http.DefaultClient, &stubTokenSource{token: "test-token"})
}

func assertJSONEq(t *testing.T, want, got string) {
	t.Helper()
	var w, g any
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("invalid want json %q: %v", want, err)
	}
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		t.Fatalf("invalid got json %q: %v", got, err)
	}
	if !reflect.DeepEqual(w, g) {
		t.Errorf("json mismatch:\nwant: %s\ngot:  %s", want, got)
	}
}

func assertPath(t *testing.T, captured *capturedRequest, wantPath string) {
	t.Helper()
	if captured.method != http.MethodPost {
		t.Errorf("method = %q, ожидался POST", captured.method)
	}
	if captured.path != wantPath {
		t.Errorf("path = %q, ожидался %q", captured.path, wantPath)
	}
}

var fixedTime = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

// --- UserRepository ------------------------------------------------------------------

func TestHTTPRepository_EnsureUser(t *testing.T) {
	srv, captured := newRecordingServer(t, http.StatusOK, `{}`)
	defer srv.Close()
	repo := newTestRepo(srv.URL)

	if err := repo.EnsureUser(context.Background(), "ivanov", "ivanov@example.com", "Иванов"); err != nil {
		t.Fatalf("EnsureUser: %v", err)
	}

	assertPath(t, captured, "/rpc/EnsureUser")
	assertJSONEq(t, `{"login":"ivanov","email":"ivanov@example.com","display_name":"Иванов"}`, captured.body)
	if captured.auth != "Bearer test-token" {
		t.Errorf("Authorization = %q", captured.auth)
	}
	if captured.contentType != "application/json" {
		t.Errorf("Content-Type = %q, ожидался application/json", captured.contentType)
	}
}

func TestHTTPRepository_GetUser(t *testing.T) {
	respBody := `{"login":"ivanov","display_name":"Иванов","email":"ivanov@example.com","role":"user",` +
		`"has_access":true,"request_status":"approved","requested_at":null,"created_at":"2026-08-24T12:00:00Z"}`
	srv, captured := newRecordingServer(t, http.StatusOK, respBody)
	defer srv.Close()
	repo := newTestRepo(srv.URL)

	got, err := repo.GetUser(context.Background(), "ivanov")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}

	assertPath(t, captured, "/rpc/GetUser")
	assertJSONEq(t, `{"login":"ivanov"}`, captured.body)

	want := service.UserRecord{
		Login: "ivanov", DisplayName: "Иванов", Email: "ivanov@example.com",
		Role: service.Role("user"), HasAccess: true, RequestStatus: "approved", CreatedAt: fixedTime,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetUser = %+v, ожидался %+v", got, want)
	}
}

func TestHTTPRepository_ListUsers(t *testing.T) {
	respBody := `{"items":[{"login":"ivanov","display_name":"","email":"","role":"user",` +
		`"has_access":false,"request_status":"","requested_at":null,"created_at":"2026-08-24T12:00:00Z"}]}`
	srv, captured := newRecordingServer(t, http.StatusOK, respBody)
	defer srv.Close()
	repo := newTestRepo(srv.URL)

	got, err := repo.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}

	assertPath(t, captured, "/rpc/ListUsers")
	assertJSONEq(t, `{}`, captured.body)

	if len(got) != 1 || got[0].Login != "ivanov" {
		t.Errorf("ListUsers = %+v", got)
	}
}

func TestHTTPRepository_ListUsers_Empty(t *testing.T) {
	srv, _ := newRecordingServer(t, http.StatusOK, `{"items":[]}`)
	defer srv.Close()
	repo := newTestRepo(srv.URL)

	got, err := repo.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListUsers = %+v, ожидался пустой список", got)
	}
}

func TestHTTPRepository_SetAccess(t *testing.T) {
	srv, captured := newRecordingServer(t, http.StatusOK, `{}`)
	defer srv.Close()
	repo := newTestRepo(srv.URL)

	if err := repo.SetAccess(context.Background(), "ivanov", true); err != nil {
		t.Fatalf("SetAccess: %v", err)
	}

	assertPath(t, captured, "/rpc/SetAccess")
	assertJSONEq(t, `{"login":"ivanov","granted":true}`, captured.body)
}

// --- AccessRequestRepository -----------------------------------------------------------

func TestHTTPRepository_CreateRequest(t *testing.T) {
	srv, captured := newRecordingServer(t, http.StatusOK, `{}`)
	defer srv.Close()
	repo := newTestRepo(srv.URL)

	if err := repo.CreateRequest(context.Background(), "ivanov"); err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}

	assertPath(t, captured, "/rpc/CreateRequest")
	assertJSONEq(t, `{"login":"ivanov"}`, captured.body)
}

func TestHTTPRepository_GetRequest(t *testing.T) {
	respBody := `{"user_id":"ivanov","status":"pending","created_at":"2026-08-24T12:00:00Z","decided_at":null,"decided_by":""}`
	srv, captured := newRecordingServer(t, http.StatusOK, respBody)
	defer srv.Close()
	repo := newTestRepo(srv.URL)

	got, err := repo.GetRequest(context.Background(), "ivanov")
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}

	assertPath(t, captured, "/rpc/GetRequest")
	assertJSONEq(t, `{"login":"ivanov"}`, captured.body)

	want := service.AccessRequest{UserID: "ivanov", Status: "pending", CreatedAt: fixedTime}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetRequest = %+v, ожидался %+v", got, want)
	}
}

func TestHTTPRepository_DecideRequest(t *testing.T) {
	srv, captured := newRecordingServer(t, http.StatusOK, `{}`)
	defer srv.Close()
	repo := newTestRepo(srv.URL)

	if err := repo.DecideRequest(context.Background(), "ivanov", "approved", "admin"); err != nil {
		t.Fatalf("DecideRequest: %v", err)
	}

	assertPath(t, captured, "/rpc/DecideRequest")
	assertJSONEq(t, `{"login":"ivanov","status":"approved","decided_by":"admin"}`, captured.body)
}

// --- UserSettingsRepository -------------------------------------------------------------

func TestHTTPRepository_UpsertSettings(t *testing.T) {
	srv, captured := newRecordingServer(t, http.StatusOK, `{}`)
	defer srv.Close()
	repo := newTestRepo(srv.URL)

	settings := service.DefaultUserSettings()
	if err := repo.UpsertSettings(context.Background(), "ivanov", settings); err != nil {
		t.Fatalf("UpsertSettings: %v", err)
	}

	assertPath(t, captured, "/rpc/UpsertSettings")

	// Полное сравнение тела, а не только user_id (test review): частичная проверка не
	// заметила бы потерю поля settings — реальный риск тихой потери пользовательских
	// настроек (цены, наценки, операции) при отправке.
	wantJSON, err := json.Marshal(upsertSettingsPayload{UserID: "ivanov", Settings: settings})
	if err != nil {
		t.Fatalf("marshal expected payload: %v", err)
	}
	assertJSONEq(t, string(wantJSON), captured.body)
}

func TestHTTPRepository_GetSettings(t *testing.T) {
	settings := service.DefaultUserSettings()
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	srv, captured := newRecordingServer(t, http.StatusOK, string(settingsJSON))
	defer srv.Close()
	repo := newTestRepo(srv.URL)

	got, err := repo.GetSettings(context.Background(), "ivanov")
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}

	assertPath(t, captured, "/rpc/GetSettings")
	assertJSONEq(t, `{"user_id":"ivanov"}`, captured.body)

	if !reflect.DeepEqual(got, settings) {
		t.Errorf("GetSettings вернул не то, что было отправлено сервером")
	}
}

// --- ChatRepository ----------------------------------------------------------------------

func TestHTTPRepository_CreateChat(t *testing.T) {
	chat := service.Chat{UserID: "ivanov", ID: "chat-1", Title: "Новый чат", CreatedAt: fixedTime, UpdatedAt: fixedTime}
	chatJSON, err := json.Marshal(chat)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	srv, captured := newRecordingServer(t, http.StatusOK, string(chatJSON))
	defer srv.Close()
	repo := newTestRepo(srv.URL)

	got, err := repo.CreateChat(context.Background(), chat)
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}

	assertPath(t, captured, "/rpc/CreateChat")
	assertJSONEq(t, string(chatJSON), captured.body)
	if !reflect.DeepEqual(got, chat) {
		t.Errorf("CreateChat = %+v, ожидался %+v", got, chat)
	}
}

func TestHTTPRepository_ListChats(t *testing.T) {
	respBody := `{"items":[{"user_id":"ivanov","id":"chat-1","title":"Новый чат",` +
		`"created_at":"2026-08-24T12:00:00Z","updated_at":"2026-08-24T12:00:00Z","calculations_count":2}]}`
	srv, captured := newRecordingServer(t, http.StatusOK, respBody)
	defer srv.Close()
	repo := newTestRepo(srv.URL)

	got, err := repo.ListChats(context.Background(), "ivanov")
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}

	assertPath(t, captured, "/rpc/ListChats")
	assertJSONEq(t, `{"user_id":"ivanov"}`, captured.body)

	if len(got) != 1 || got[0].ID != "chat-1" || got[0].CalculationsCount != 2 {
		t.Errorf("ListChats = %+v", got)
	}
}

func TestHTTPRepository_DeleteChat(t *testing.T) {
	srv, captured := newRecordingServer(t, http.StatusOK, `{}`)
	defer srv.Close()
	repo := newTestRepo(srv.URL)

	if err := repo.DeleteChat(context.Background(), "ivanov", "chat-1", "ivanov", true); err != nil {
		t.Fatalf("DeleteChat: %v", err)
	}

	assertPath(t, captured, "/rpc/DeleteChat")
	assertJSONEq(t, `{"user_id":"ivanov","chat_id":"chat-1","deleted_by":"ivanov","hard":true}`, captured.body)
}

func TestHTTPRepository_RestoreChat(t *testing.T) {
	srv, captured := newRecordingServer(t, http.StatusOK, `{}`)
	defer srv.Close()
	repo := newTestRepo(srv.URL)

	if err := repo.RestoreChat(context.Background(), "ivanov", "chat-1"); err != nil {
		t.Fatalf("RestoreChat: %v", err)
	}

	assertPath(t, captured, "/rpc/RestoreChat")
	assertJSONEq(t, `{"user_id":"ivanov","chat_id":"chat-1"}`, captured.body)
}

// --- ChatCalculationRepository ------------------------------------------------------------

func TestHTTPRepository_AppendCalculation(t *testing.T) {
	srv, captured := newRecordingServer(t, http.StatusOK, `{}`)
	defer srv.Close()
	repo := newTestRepo(srv.URL)

	result := service.CalculationResult{UserID: "ivanov", ChatID: "chat-1", GarmentType: "dress", Quantity: 3, CreatedAt: fixedTime}
	if err := repo.AppendCalculation(context.Background(), result); err != nil {
		t.Fatalf("AppendCalculation: %v", err)
	}

	assertPath(t, captured, "/rpc/AppendCalculation")

	// Полное сравнение тела (test review): результат уходит как есть, без DTO-обёртки,
	// частичная проверка двух полей не заметила бы потерю остальных (GarmentType, Quantity,
	// CreatedAt и т.д.) при рефакторинге.
	resultJSON, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal expected payload: %v", err)
	}
	assertJSONEq(t, string(resultJSON), captured.body)
}

func TestHTTPRepository_ListCalculations(t *testing.T) {
	respBody := `{"items":[{"user_id":"ivanov","chat_id":"chat-1","garment_type":"dress","quantity":3,` +
		`"created_at":"2026-08-24T12:00:00Z"}]}`
	srv, captured := newRecordingServer(t, http.StatusOK, respBody)
	defer srv.Close()
	repo := newTestRepo(srv.URL)

	got, err := repo.ListCalculations(context.Background(), "ivanov", "chat-1")
	if err != nil {
		t.Fatalf("ListCalculations: %v", err)
	}

	assertPath(t, captured, "/rpc/ListCalculations")
	assertJSONEq(t, `{"user_id":"ivanov","chat_id":"chat-1"}`, captured.body)

	if len(got) != 1 || got[0].GarmentType != "dress" || got[0].Quantity != 3 {
		t.Errorf("ListCalculations = %+v", got)
	}
}

// --- Error mapping и транспортные сбои ---------------------------------------------------

func TestHTTPRepository_ErrorMapping(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		wantWrap error
	}{
		{"invalid_request", http.StatusBadRequest, `{"error":"invalid_request"}`, service.ErrInvalidArgument},
		{"forbidden", http.StatusForbidden, `{"error":"forbidden"}`, service.ErrForbidden},
		{"not_found", http.StatusNotFound, `{"error":"not_found"}`, service.ErrNotFound},
		{"conflict", http.StatusConflict, `{"error":"conflict"}`, service.ErrConflict},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newRecordingServer(t, tc.status, tc.body)
			defer srv.Close()
			repo := newTestRepo(srv.URL)

			_, err := repo.GetUser(context.Background(), "ivanov")
			if err == nil {
				t.Fatal("ожидалась ошибка")
			}
			if !errors.Is(err, tc.wantWrap) {
				t.Errorf("err = %v, не оборачивает %v", err, tc.wantWrap)
			}
		})
	}
}

func TestHTTPRepository_UnknownErrorCode(t *testing.T) {
	srv, _ := newRecordingServer(t, http.StatusInternalServerError, `{"error":"internal_error"}`)
	defer srv.Close()
	repo := newTestRepo(srv.URL)

	_, err := repo.GetUser(context.Background(), "ivanov")
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	for _, sentinel := range rpcErrorSentinels {
		if errors.Is(err, sentinel) {
			t.Errorf("err = %v неожиданно оборачивает известный sentinel %v", err, sentinel)
		}
	}
}

func TestHTTPRepository_NonJSONErrorBody(t *testing.T) {
	srv, _ := newRecordingServer(t, http.StatusBadGateway, `<html>502</html>`)
	defer srv.Close()
	repo := newTestRepo(srv.URL)

	if _, err := repo.GetUser(context.Background(), "ivanov"); err == nil {
		t.Fatal("ожидалась ошибка на не-JSON теле ошибки")
	}
}

func TestHTTPRepository_MalformedSuccessBody(t *testing.T) {
	srv, _ := newRecordingServer(t, http.StatusOK, `not json`)
	defer srv.Close()
	repo := newTestRepo(srv.URL)

	if _, err := repo.GetUser(context.Background(), "ivanov"); err == nil {
		t.Fatal("ожидалась ошибка декодирования ответа")
	}
}

func TestHTTPRepository_TokenSourceError(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	repo := NewHTTPRepository(srv.URL, http.DefaultClient, &stubTokenSource{err: errors.New("iam недоступен")})

	if _, err := repo.GetUser(context.Background(), "ivanov"); err == nil {
		t.Fatal("ожидалась ошибка получения токена")
	}
	if called {
		t.Error("запрос к db-service не должен был уйти без токена")
	}
}

// unmarshalablePayload — тип, который json.Marshal гарантированно не умеет кодировать
// (channel), нужен только чтобы дойти до ветки encode-error в callRPC (test review: она
// не была покрыта ни одним из пятнадцати реальных DTO, которые всегда кодируются успешно).
type unmarshalablePayload struct {
	C chan int
}

func TestHTTPRepository_EncodeRequestError(t *testing.T) {
	repo := newTestRepo("http://127.0.0.1:1") // до сети дела не дойдёт — упадёт на marshal

	_, err := callRPC[unmarshalablePayload, emptyRPCResponse](context.Background(), repo, "X", unmarshalablePayload{C: make(chan int)})
	if err == nil {
		t.Fatal("ожидалась ошибка кодирования запроса")
	}
}

func TestHTTPRepository_TransportFailure(t *testing.T) {
	repo := newTestRepo("http://127.0.0.1:1") // заведомо ничего не слушает

	if _, err := repo.GetUser(context.Background(), "ivanov"); err == nil {
		t.Fatal("ожидалась ошибка транспорта")
	}
}
