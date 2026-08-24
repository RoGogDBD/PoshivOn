package dbservice

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RoGogDBD/PoshivOn/internal/service"
)

type rpcTestReq struct {
	Value string `json:"value"`
}

type rpcTestResp struct {
	Echo string `json:"echo"`
}

func TestRPC_Success(t *testing.T) {
	handler := rpc(func(ctx context.Context, req rpcTestReq) (rpcTestResp, error) {
		return rpcTestResp{Echo: req.Value}, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/rpc/Test", strings.NewReader(`{"value":"hello"}`))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got rpcTestResp
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Echo != "hello" {
		t.Errorf("Echo = %q, want hello", got.Echo)
	}
}

// TestRPC_InvalidJSON — неизвестное поле в теле запроса отклоняется до вызова fn, а не
// подхватывается молча: тот же DisallowUnknownFields, что и decodeJSON браузерного API
// (server/internal/handler/http.go).
func TestRPC_InvalidJSON(t *testing.T) {
	called := false
	handler := rpc(func(ctx context.Context, req rpcTestReq) (rpcTestResp, error) {
		called = true
		return rpcTestResp{}, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/rpc/Test", strings.NewReader(`{"unknown_field":1}`))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if called {
		t.Error("fn был вызван при невалидном JSON — декодирование должно отсекать до вызова")
	}
}

// TestRPC_ErrorMapping — доменные сентинелы service.Err* транслируются в HTTP-статусы тем
// же принципом, что и в browser-facing handler.classifyDomainError.
func TestRPC_ErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"invalid argument", service.ErrInvalidArgument, http.StatusBadRequest, "invalid_request"},
		{"forbidden", service.ErrForbidden, http.StatusForbidden, "forbidden"},
		{"not found", service.ErrNotFound, http.StatusNotFound, "not_found"},
		{"conflict", service.ErrConflict, http.StatusConflict, "conflict"},
		{"неизвестная ошибка", errors.New("boom"), http.StatusInternalServerError, "internal_error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := rpc(func(ctx context.Context, req rpcTestReq) (rpcTestResp, error) {
				return rpcTestResp{}, tc.err
			})

			req := httptest.NewRequest(http.MethodPost, "/rpc/Test", strings.NewReader(`{}`))
			rec := httptest.NewRecorder()
			handler(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			var body map[string]string
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode error body: %v", err)
			}
			if body["error"] != tc.wantCode {
				t.Errorf("error code = %q, want %q", body["error"], tc.wantCode)
			}
		})
	}
}

// TestRPC_DomainErrorNotLeaked — наружу уходит только категория ошибки, не её текст: текст
// доменной/SQL-ошибки может содержать внутренние детали (DSN, значения параметров запроса),
// а db-service — новый доверительный периметр, вызываемый по сети (см. Фазу 2 плана и
// раздел «История вопроса» про db-gateway — IDOR/утечка деталей там прямо в списке рисков).
func TestRPC_DomainErrorNotLeaked(t *testing.T) {
	sensitive := errors.New("dial tcp 127.0.0.1:3306: connection refused (password=s3cret)")
	handler := rpc(func(ctx context.Context, req rpcTestReq) (rpcTestResp, error) {
		return rpcTestResp{}, sensitive
	})

	req := httptest.NewRequest(http.MethodPost, "/rpc/Test", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if strings.Contains(rec.Body.String(), "s3cret") {
		t.Errorf("тело ответа раскрывает внутренние детали ошибки: %s", rec.Body.String())
	}
}

// TestRPC_MultipleJSONObjects — тело с несколькими JSON-объектами подряд отклоняется, а не
// молча использует только первый (тот же паттерн decodeJSON, что и в handler-пакете).
func TestRPC_MultipleJSONObjects(t *testing.T) {
	handler := rpc(func(ctx context.Context, req rpcTestReq) (rpcTestResp, error) {
		t.Fatal("fn не должен вызываться при нескольких JSON-объектах в теле")
		return rpcTestResp{}, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/rpc/Test", strings.NewReader(`{"value":"a"}{"value":"b"}`))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
