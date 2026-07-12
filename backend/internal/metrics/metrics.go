// Package metrics owns the Prometheus registry and HTTP instrumentation
// exposed at /metrics (INFRASTRUCTURE.md §5).
package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds the app's collectors. One instance is built at startup and
// injected via DI. Labels are kept low-cardinality (route, not path) per
// INFRASTRUCTURE.md §5.
type Metrics struct {
	reg          *prometheus.Registry
	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec
}

// New builds the registry with the default Go/process collectors plus the app's
// HTTP metrics.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
	)
	factory := promauto.With(reg)

	return &Metrics{
		reg: reg,
		httpRequests: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests by route, method and status class.",
		}, []string{"route", "method", "status"}),
		httpDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency by route and method.",
			Buckets: prometheus.DefBuckets,
		}, []string{"route", "method"}),
	}
}

// Registry exposes the underlying registry so the server can mount its handler.
func (m *Metrics) Registry() *prometheus.Registry { return m.reg }

// ObserveHTTP records one request. route is the matched pattern (low
// cardinality), not the raw path.
func (m *Metrics) ObserveHTTP(route, method string, status int, dur time.Duration) {
	m.httpRequests.WithLabelValues(route, method, strconv.Itoa(status)).Inc()
	m.httpDuration.WithLabelValues(route, method).Observe(dur.Seconds())
}
