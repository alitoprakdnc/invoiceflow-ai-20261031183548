package metrics

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	namespace = "invoiceflow"
)

// Prometheus metric collectors.
var (
	// RequestDuration records the latency of HTTP requests, labeled by handler name,
	// HTTP method and response status code.
	RequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "api",
			Name:      "request_duration_seconds",
			Help:      "Histogram of request latency (seconds) per handler, method and status code.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"handler", "method", "code"},
	)

	// RequestErrors counts HTTP responses with status >= 400.
	RequestErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "api",
			Name:      "request_errors_total",
			Help:      "Total number of error responses per handler, method and status code.",
		},
		[]string{"handler", "method", "code"},
	)

	// QueueDepth reflects the current depth of a Google Cloud Tasks queue.
	QueueDepth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "queue",
			Name:      "depth",
			Help:      "Current depth of a Cloud Tasks queue.",
		},
		[]string{"queue"},
	)

	// WebhookSuccess counts successful outbound webhook dispatches.
	WebhookSuccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "webhook",
			Name:      "success_total",
			Help:      "Total successful webhook dispatches.",
		},
		[]string{"endpoint"},
	)

	// WebhookFailure counts failed outbound webhook dispatches.
	WebhookFailure = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "webhook",
			Name:      "failure_total",
			Help:      "Total failed webhook dispatches.",
		},
		[]string{"endpoint"},
	)
)

// init registers all metric collectors with the default Prometheus registry.
func init() {
	prometheus.MustRegister(
		RequestDuration,
		RequestErrors,
		QueueDepth,
		WebhookSuccess,
		WebhookFailure,
	)
}

// InstrumentHandler returns a Gin middleware that records request latency and error counts.
// The caller should provide a stable handler name (e.g., "invoice.upload").
func InstrumentHandler(handlerName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		elapsed := time.Since(start).Seconds()

		status := c.Writer.Status()
		method := c.Request.Method
		codeLabel := http.StatusText(status)

		RequestDuration.WithLabelValues(handlerName, method, codeLabel).Observe(elapsed)

		if status >= http.StatusBadRequest {
			RequestErrors.WithLabelValues(handlerName, method, codeLabel).Inc()
		}
	}
}

// SetQueueDepth updates the gauge for a specific Cloud Tasks queue.
func SetQueueDepth(queueName string, depth int) {
	QueueDepth.WithLabelValues(queueName).Set(float64(depth))
}

// RecordWebhookSuccess increments the success counter for the given endpoint URL.
func RecordWebhookSuccess(endpoint string) {
	WebhookSuccess.WithLabelValues(endpoint).Inc()
}

// RecordWebhookFailure increments the failure counter for the given endpoint URL.
func RecordWebhookFailure(endpoint string) {
	WebhookFailure.WithLabelValues(endpoint).Inc()
}

// Handler returns a Gin-compatible handler that serves the /metrics endpoint.
func Handler() gin.HandlerFunc {
	h := promhttp.Handler()
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}