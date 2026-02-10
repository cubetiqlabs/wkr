package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ── Invocation: success vs error (the #1 signal for observers) ──

	InvocationsSuccessTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "cubis",
		Name:      "invocations_success_total",
		Help:      "Total successful worker invocations",
	}, []string{"worker", "runtime", "region", "node_id"})

	InvocationsFailedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "cubis",
		Name:      "invocations_failed_total",
		Help:      "Total failed worker invocations",
	}, []string{"worker", "runtime", "error_type", "region", "node_id"})

	InvocationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "cubis",
		Name:      "invocations_total",
		Help:      "Total worker invocations (all outcomes)",
	}, []string{"worker", "runtime", "status", "region", "node_id"})

	InvocationDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "cubis",
		Name:      "invocation_duration_seconds",
		Help:      "Worker invocation duration in seconds",
		Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30},
	}, []string{"worker", "runtime", "region"})

	// ── Error breakdown (for alerting rules) ──

	InvocationTimeouts = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "cubis",
		Name:      "invocation_timeouts_total",
		Help:      "Worker invocations that timed out",
	}, []string{"worker", "runtime", "region"})

	InvocationCompileErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "cubis",
		Name:      "invocation_compile_errors_total",
		Help:      "Go worker compilation failures",
	}, []string{"worker", "node_id"})

	InvocationRuntimeErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "cubis",
		Name:      "invocation_runtime_errors_total",
		Help:      "Uncaught exceptions / panics in worker code",
	}, []string{"worker", "runtime", "region"})

	// ── Go binary cache ──

	GoCacheHits = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "cubis",
		Name:      "go_cache_hits_total",
		Help:      "Go binary cache hits",
	}, []string{"node_id"})

	GoCacheMisses = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "cubis",
		Name:      "go_cache_misses_total",
		Help:      "Go binary cache misses (compilation required)",
	}, []string{"node_id"})

	GoCompileDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "cubis",
		Name:      "go_compile_duration_seconds",
		Help:      "Time to compile Go worker binary",
		Buckets:   []float64{.1, .25, .5, 1, 2, 5, 10},
	}, []string{"node_id"})

	// ── Pool ──

	ActiveWorkers = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "cubis",
		Name:      "active_workers",
		Help:      "Currently executing workers",
	}, []string{"node_id", "region"})

	PoolCapacity = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "cubis",
		Name:      "pool_capacity",
		Help:      "Max concurrent workers allowed",
	}, []string{"node_id"})

	PoolQueueWait = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "cubis",
		Name:      "pool_queue_wait_seconds",
		Help:      "Time spent waiting for pool slot",
		Buckets:   []float64{.001, .005, .01, .05, .1, .5, 1, 5},
	}, []string{"node_id"})

	PoolRejected = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "cubis",
		Name:      "pool_rejected_total",
		Help:      "Requests rejected due to pool full / context cancelled",
	}, []string{"node_id"})

	// ── HTTP ──

	HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "cubis",
		Name:      "http_requests_total",
		Help:      "Total HTTP requests",
	}, []string{"method", "path", "status"})

	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "cubis",
		Name:      "http_request_duration_seconds",
		Help:      "HTTP request duration",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "path"})

	// ── Bandwidth ──

	RequestBytesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "cubis",
		Name:      "request_bytes_total",
		Help:      "Total inbound bytes",
	}, []string{"node_id"})

	ResponseBytesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "cubis",
		Name:      "response_bytes_total",
		Help:      "Total outbound bytes",
	}, []string{"node_id"})

	// ── Auth & Quota ──

	AuthSuccessTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "cubis",
		Name:      "auth_success_total",
		Help:      "Successful authentications",
	})

	AuthFailedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "cubis",
		Name:      "auth_failed_total",
		Help:      "Failed authentication attempts",
	})

	QuotaRejectedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "cubis",
		Name:      "quota_rejected_total",
		Help:      "Requests rejected by quota enforcement",
	}, []string{"reason", "node_id"})

	// ── Edge / Cluster ──

	EdgeNodeStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "cubis",
		Name:      "edge_node_status",
		Help:      "Edge node status (1=online, 0=offline)",
	}, []string{"node_id", "region", "role"})

	EdgeSyncDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "cubis",
		Name:      "edge_sync_duration_seconds",
		Help:      "Time to sync with control plane",
		Buckets:   []float64{.01, .05, .1, .5, 1, 5},
	}, []string{"node_id"})

	EdgeFailovers = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "cubis",
		Name:      "edge_failovers_total",
		Help:      "Total failover events",
	}, []string{"from_node", "to_node", "region"})

	// ── Security ──

	SecurityBlocked = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "cubis",
		Name:      "security_blocked_total",
		Help:      "Blocked execution attempts",
	}, []string{"reason", "worker", "node_id"})

	// ── System ──

	UptimeSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "cubis",
		Name:      "uptime_seconds",
		Help:      "Seconds since process start",
	})

	WorkersRegistered = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "cubis",
		Name:      "workers_registered",
		Help:      "Total number of registered workers",
	})
)
