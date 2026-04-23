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
	for i, p := range parts {
		if i > 0 && looksLikeID(p) {
			parts[i] = ":id"
		}
	}
	return "/" + strings.Join(parts, "/")
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
