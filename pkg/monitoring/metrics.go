package monitoring

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Prometheus metrics for the ad processing system
var (
	// HTTP metrics
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "endpoint", "status"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint", "status"},
	)

	// Queue metrics
	QueueSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "queue_size",
			Help: "Current number of ads in queue by priority",
		},
		[]string{"priority", "shard"},
	)

	QueueProcessingRate = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "queue_processing_total",
			Help: "Total number of ads processed from queue",
		},
		[]string{"priority", "status"},
	)

	QueueWaitTime = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "queue_wait_time_seconds",
			Help:    "Time ads spend waiting in queue",
			Buckets: []float64{1, 5, 10, 30, 60, 300, 600, 1800, 3600},
		},
		[]string{"priority"},
	)

	// Processing metrics
	AdProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ad_processing_duration_seconds",
			Help:    "Time taken to process individual ads",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30},
		},
		[]string{"worker_id", "game_family"},
	)

	ActiveWorkers = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "active_workers",
			Help: "Number of currently active workers",
		},
		[]string{"service"},
	)

	WorkerUtilization = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "worker_utilization_percent",
			Help: "Worker utilization percentage",
		},
		[]string{"worker_id"},
	)

	// AI Agent metrics
	AICommandsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ai_commands_total",
			Help: "Total number of AI commands processed",
		},
		[]string{"intent", "status"},
	)

	AICommandDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ai_command_duration_seconds",
			Help:    "Time taken to process AI commands",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
		},
		[]string{"intent"},
	)

	AIAPICallsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ai_api_calls_total",
			Help: "Total number of calls to AI API",
		},
		[]string{"provider", "status"},
	)

	// Database metrics
	DatabaseConnections = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "database_connections",
			Help: "Current number of database connections",
		},
		[]string{"state", "db"},
	)

	DatabaseQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "database_query_duration_seconds",
			Help:    "Database query execution time",
			Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5},
		},
		[]string{"query_type", "table"},
	)

	DatabaseErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "database_errors_total",
			Help: "Total number of database errors",
		},
		[]string{"error_type", "table"},
	)

	// Redis metrics
	RedisConnections = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "redis_connections",
			Help: "Current number of Redis connections",
		},
		[]string{"state"},
	)

	RedisCommandDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "redis_command_duration_seconds",
			Help:    "Redis command execution time",
			Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
		},
		[]string{"command"},
	)

	RedisErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "redis_errors_total",
			Help: "Total number of Redis errors",
		},
		[]string{"error_type"},
	)

	// Business metrics
	AdsCreatedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ads_created_total",
			Help: "Total number of ads created",
		},
		[]string{"game_family", "priority"},
	)

	AdsProcessedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ads_processed_total",
			Help: "Total number of ads processed",
		},
		[]string{"game_family", "status"},
	)

	PriorityBoostsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "priority_boosts_total",
			Help: "Total number of priority boosts applied",
		},
		[]string{"reason"},
	)

	// System metrics
	SystemResourceUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "system_resource_usage_percent",
			Help: "System resource usage percentage",
		},
		[]string{"resource", "service"},
	)

	SystemErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "system_errors_total",
			Help: "Total number of system errors",
		},
		[]string{"component", "severity"},
	)
)

// MetricsMiddleware creates a Gin middleware for collecting HTTP metrics
func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		// Process request
		c.Next()

		// Record metrics
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())

		// Normalize path to avoid high cardinality
		normalizedPath := normalizePath(path)

		HTTPRequestsTotal.WithLabelValues(method, normalizedPath, status).Inc()
		HTTPRequestDuration.WithLabelValues(method, normalizedPath, status).Observe(duration)
	}
}

// normalizePath reduces cardinality by grouping similar paths
func normalizePath(path string) string {
	// Common path patterns
	patterns := map[string]string{
		"/api/ads/":     "/api/ads/{id}",
		"/health":       "/health",
		"/metrics":      "/metrics",
		"/api/agent/":   "/api/agent/command",
	}

	for pattern, normalized := range patterns {
		if len(path) >= len(pattern) && path[:len(pattern)] == pattern {
			if pattern == "/api/ads/" && len(path) > len(pattern) {
				return "/api/ads/{id}"
			}
			return normalized
		}
	}

	// Default normalization
	if path == "/" {
		return "/"
	}
	return "/other"
}

// PrometheusHandler returns the Prometheus metrics handler
func PrometheusHandler() http.Handler {
	return promhttp.Handler()
}

// RecordQueueMetrics updates queue-related metrics
func RecordQueueMetrics(priority int, shard string, size float64) {
	QueueSize.WithLabelValues(strconv.Itoa(priority), shard).Set(size)
}

// RecordProcessingTime records the time taken to process an ad
func RecordProcessingTime(workerID, gameFamily string, duration time.Duration) {
	AdProcessingDuration.WithLabelValues(workerID, gameFamily).Observe(duration.Seconds())
}

// RecordQueueWaitTime records how long an ad waited in the queue
func RecordQueueWaitTime(priority int, waitTime time.Duration) {
	QueueWaitTime.WithLabelValues(strconv.Itoa(priority)).Observe(waitTime.Seconds())
}

// RecordAICommand records AI command metrics
func RecordAICommand(intent, status string, duration time.Duration) {
	AICommandsTotal.WithLabelValues(intent, status).Inc()
	AICommandDuration.WithLabelValues(intent).Observe(duration.Seconds())
}

// RecordDatabaseQuery records database query metrics
func RecordDatabaseQuery(queryType, table string, duration time.Duration, err error) {
	DatabaseQueryDuration.WithLabelValues(queryType, table).Observe(duration.Seconds())
	if err != nil {
		DatabaseErrorsTotal.WithLabelValues("query_error", table).Inc()
	}
}

// RecordRedisCommand records Redis command metrics
func RecordRedisCommand(command string, duration time.Duration, err error) {
	RedisCommandDuration.WithLabelValues(command).Observe(duration.Seconds())
	if err != nil {
		RedisErrorsTotal.WithLabelValues("command_error").Inc()
	}
}

// RecordAdCreated records when a new ad is created
func RecordAdCreated(gameFamily string, priority int) {
	AdsCreatedTotal.WithLabelValues(gameFamily, strconv.Itoa(priority)).Inc()
}

// RecordAdProcessed records when an ad is processed
func RecordAdProcessed(gameFamily, status string) {
	AdsProcessedTotal.WithLabelValues(gameFamily, status).Inc()
}

// RecordPriorityBoost records when a priority boost is applied
func RecordPriorityBoost(reason string) {
	PriorityBoostsTotal.WithLabelValues(reason).Inc()
}

// UpdateActiveWorkers updates the number of active workers
func UpdateActiveWorkers(service string, count int) {
	ActiveWorkers.WithLabelValues(service).Set(float64(count))
}

// UpdateWorkerUtilization updates worker utilization metrics
func UpdateWorkerUtilization(workerID string, utilizationPercent float64) {
	WorkerUtilization.WithLabelValues(workerID).Set(utilizationPercent)
}

// RecordSystemError records system errors
func RecordSystemError(component, severity string) {
	SystemErrors.WithLabelValues(component, severity).Inc()
}

// UpdateSystemResourceUsage updates system resource usage metrics
func UpdateSystemResourceUsage(resource, service string, usagePercent float64) {
	SystemResourceUsage.WithLabelValues(resource, service).Set(usagePercent)
}