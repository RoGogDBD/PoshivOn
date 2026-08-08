package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/RoGogDBD/PoshivOn/internal/metrics"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func normalizePath(path string) string {
	// Collapse /api/v1/users/<id>/... → /api/v1/users/:id/...
	// to avoid high-cardinality label values.
	parts := strings.Split(strings.Trim(path, "/"), "/")
	normalized := make([]string, len(parts))
	copy(normalized, parts)

	for i, p := range parts {
		if i > 0 && (looksLikeID(p) || isAdminUserLogin(parts, i)) {
			normalized[i] = ":id"
		}
	}
	// Смотрим на parts, а пишем в normalized: иначе условие ниже читало бы уже
	// переписанные соседние сегменты и зависело бы от порядка обхода.
	return "/" + strings.Join(normalized, "/")
}

// isAdminUserLogin — третье правило свёртки (Decision 18): сегмент сразу после
// /api/v1/admin/users/ схлопывается независимо от формы значения.
//
// Своего условия ему мало: WithMetrics оборачивает mux снаружи авторизации (порядок
// CORS → Metrics → mux), поэтому путь попадает в метку Prometheus раньше, чем RequireAdmin
// успевает отклонить запрос. Логин администратора короткий и без дефиса, то есть под
// looksLikeID не подходит, — и без этого правила анонимный вызывающий раздувал бы
// кардинальность метки произвольными значениями, получая на каждое из них 401 или 403.
//
// Привязка к позиции 4 (а не просто «предыдущие два сегмента — admin/users» где угодно в
// пути) — иначе путь вида /foo/admin/users/bar с любым префиксом тоже схлопывался бы,
// хотя Decision 18 описывает ровно один маршрут.
func isAdminUserLogin(parts []string, i int) bool {
	return i == 4 && len(parts) > 4 &&
		parts[0] == "api" && parts[1] == "v1" && parts[2] == "admin" && parts[3] == "users"
}

func looksLikeID(s string) bool {
	if len(s) == 0 {
		return false
	}
	_, errInt := strconv.ParseInt(s, 10, 64)
	if errInt == nil {
		return true
	}
	// UUID-like: xxxxxxxx-xxxx-...
	return len(s) > 8 && strings.Contains(s, "-")
}

// WithMetrics wraps an HTTP handler and records Prometheus RED metrics.
func WithMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()

		next.ServeHTTP(rec, r)

		path := normalizePath(r.URL.Path)
		status := fmt.Sprintf("%d", rec.status)
		elapsed := time.Since(start).Seconds()

		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, path, status).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(r.Method, path).Observe(elapsed)
	})
}
