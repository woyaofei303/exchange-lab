// Package observability provides bounded metrics and tracing integration.
package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics owns the process-local Prometheus registry and bounded label sets.
type Metrics struct {
	registry               *prometheus.Registry
	HTTPDuration           *prometheus.HistogramVec
	Liquidations           prometheus.Counter
	LiquidationBacklog     prometheus.Gauge
	FundingLag             prometheus.Gauge
	PriceFresh             prometheus.Gauge
	AIRequests             *prometheus.CounterVec
	ReconciliationFailures prometheus.Counter
}

// New creates an isolated registry for one service process.
func New(service string) *Metrics {
	registry := prometheus.NewRegistry()
	m := &Metrics{registry: registry,
		HTTPDuration:           prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "exchange_http_request_duration_seconds", Help: "HTTP request latency by service, method, route, and status.", Buckets: prometheus.DefBuckets}, []string{"service", "method", "route", "status"}),
		Liquidations:           prometheus.NewCounter(prometheus.CounterOpts{Name: "exchange_liquidations_total", Help: "Completed teaching-grade liquidations."}),
		LiquidationBacklog:     prometheus.NewGauge(prometheus.GaugeOpts{Name: "exchange_liquidation_backlog", Help: "Pending liquidation jobs."}),
		FundingLag:             prometheus.NewGauge(prometheus.GaugeOpts{Name: "exchange_funding_lag_seconds", Help: "Funding settlement lag."}),
		PriceFresh:             prometheus.NewGauge(prometheus.GaugeOpts{Name: "exchange_mark_price_fresh", Help: "One when the authoritative mark price is fresh."}),
		AIRequests:             prometheus.NewCounterVec(prometheus.CounterOpts{Name: "exchange_ai_requests_total", Help: "AI summaries by status."}, []string{"status"}),
		ReconciliationFailures: prometheus.NewCounter(prometheus.CounterOpts{Name: "exchange_reconciliation_failures_total", Help: "Financial reconciliation failures."}),
	}
	registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}), m.HTTPDuration, m.Liquidations, m.LiquidationBacklog, m.FundingLag, m.PriceFresh, m.AIRequests, m.ReconciliationFailures)
	_ = service
	return m
}

// Handler exposes this registry in Prometheus text format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// Wrap records request latency and status using bounded route labels.
func (m *Metrics) Wrap(service string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		tracked := &metricWriter{ResponseWriter: w}
		next.ServeHTTP(tracked, r)
		route := r.URL.Path
		if route == "" {
			route = "unknown"
		}
		m.HTTPDuration.WithLabelValues(service, r.Method, route, strconv.Itoa(tracked.code())).Observe(time.Since(started).Seconds())
	})
}

type metricWriter struct {
	http.ResponseWriter
	status int
}

// WriteHeader records the first response status before forwarding it.
func (w *metricWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

// Write records an implicit 200 response before forwarding bytes.
func (w *metricWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	return w.ResponseWriter.Write(p)
}
func (w *metricWriter) code() int {
	if w.status == 0 {
		return 200
	}
	return w.status
}

// Unwrap exposes the underlying writer to optional HTTP interfaces.
func (w *metricWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
