package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "poshivon_http_requests_total",
			Help: "Total HTTP requests by method, path, and status code",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "poshivon_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5},
		},
		[]string{"method", "path"},
	)

	AIRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "poshivon_ai_requests_total",
			Help: "Total DeepSeek API calls by status (success/error)",
		},
		[]string{"status"},
	)

	AITokensTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "poshivon_ai_tokens_total",
			Help: "Total AI tokens consumed, by type (prompt/completion)",
		},
		[]string{"type"},
	)

	AIRequestDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "poshivon_ai_request_duration_seconds",
			Help:    "DeepSeek API call duration in seconds",
			Buckets: []float64{0.5, 1, 2, 5, 10, 20, 30, 45, 60},
		},
	)
)
