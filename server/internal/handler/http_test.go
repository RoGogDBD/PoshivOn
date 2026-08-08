package handler

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

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
