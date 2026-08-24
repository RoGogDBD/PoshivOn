package dbservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/RoGogDBD/PoshivOn/internal/service"
)

// maxRequestBodyBytes ограничивает тело запроса сверху. Без этого лимита json.Decoder
// читает r.Body целиком в память независимо от размера — раньше это было унаследованным
// пробелом browser-facing decodeJSON (handler/http.go), терпимым за CDN/nginx перед ним;
// здесь та же дыра становится майорной находкой (security audit db-service) именно из-за
// co-location: исчерпание памяти на этом HTTP-слое роняет и единственный процесс MariaDB
// того же инстанса, а не просто один запрос. Лимит с запасом над крупнейшим легитимным
// payload — AppendCalculation/UpsertSettings с полным набором конфигурации.
const maxRequestBodyBytes = 1 << 20 // 1 MiB

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	defer r.Body.Close()

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}
	if decoder.More() {
		return errors.New("invalid json: multiple objects in body")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

// classifyError отображает доменную ошибку сервисного слоя на HTTP-статус — тот же принцип,
// что server/internal/handler/http.go применяет для browser-facing API (Decision 17: наружу
// уходит только категория, не текст ошибки — он может нести детали SQL-запроса). Отдельная
// копия, а не общая функция: classifyDomainError в handler не экспортирован, а db-service —
// самостоятельный доверительный периметр (см. план миграции, раздел «Фаза 2»), которому не
// нужен остальной browser-facing слой handler (CORS, cookies, DeepSeek-специфичные ветки).
//
// err всегда не nil здесь: единственный вызывающий, rpc() ниже, проверяет это перед вызовом.
func classifyError(err error) (status int, code string) {
	switch {
	case errors.Is(err, service.ErrInvalidArgument):
		return http.StatusBadRequest, "invalid_request"
	case errors.Is(err, service.ErrForbidden):
		return http.StatusForbidden, "forbidden"
	case errors.Is(err, service.ErrNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, service.ErrConflict):
		return http.StatusConflict, "conflict"
	default:
		return http.StatusInternalServerError, "internal_error"
	}
}

// rpc оборачивает один метод репозитория как HTTP-эндпоинт: декодирует JSON-запрос,
// вызывает fn на существующем репозитории, кодирует результат или ошибку в ответ. Один
// generic-хелпер вместо пятнадцати почти одинаковых обработчиков — разница между ними
// только в типах Req/Resp и самом вызове метода (см. handlers.go).
func rpc[Req any, Resp any](fn func(ctx context.Context, req Req) (Resp, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req Req
		if err := decodeJSON(w, r, &req); err != nil {
			log.Printf("dbservice rpc: path=%s status=%d code=invalid_request err=%v", r.URL.Path, http.StatusBadRequest, err)
			writeError(w, http.StatusBadRequest, "invalid_request")
			return
		}

		resp, err := fn(r.Context(), req)
		if err != nil {
			status, code := classifyError(err)
			log.Printf("dbservice rpc: path=%s status=%d code=%s err=%v", r.URL.Path, status, code, err)
			writeError(w, status, code)
			return
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// audited добавляет лог об успешном вызове поверх уже собранного rpc-хендлера. Обычные
// хендлеры такого следа не получают — иначе лог наполнился бы шумом на каждый ListChats/
// GetSettings. Только для точек, где отсутствие сетевого следа об успехе было бы находкой
// security audit само по себе — сейчас это SetAccess и DecideRequest (см. handlers.go):
// изменение прав доступа и решение по заявке должны оставлять след независимо от того, что
// записано в самой БД, чтобы при расследовании инцидента был операционный контекст
// (когда, через какой путь), а не только итоговая строка в таблице.
func audited(name string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next(rec, r)
		if rec.status == http.StatusOK {
			log.Printf("dbservice audit: endpoint=%s status=%d", name, rec.status)
		}
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
