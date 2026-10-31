package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"invoiceflow/internal/queue"
)

// QueueChecker abstracts the queue health check.
// The concrete implementation lives in internal/queue/google_tasks.go.
type QueueChecker interface {
	// Depth returns the current number of pending tasks in the queue.
	Depth(ctx context.Context) (int64, error)
}

// HealthHandler aggregates the dependencies required for the health endpoint.
type HealthHandler struct {
	db          *gorm.DB
	queueClient QueueChecker
}

// NewHealthHandler creates a new HealthHandler.
// It should be instantiated in the server bootstrap code and injected into the router.
func NewHealthHandler(db *gorm.DB, queueClient QueueChecker) *HealthHandler {
	return &HealthHandler{
		db:          db,
		queueClient: queueClient,
	}
}

// healthResponse defines the JSON payload returned by /api/health.
type healthResponse struct {
	Timestamp time.Time `json:"timestamp"`

	// Liveness indicates that the process is up and able to respond to requests.
	Liveness bool `json:"liveness"`

	// Readiness indicates that the service is ready to serve traffic.
	// It is false when any critical dependency (DB, queue) is unhealthy.
	Readiness bool `json:"readiness"`

	DB struct {
		Connected bool   `json:"connected"`
		Error     string `json:"error,omitempty"`
	} `json:"db"`

	Queue struct {
		Connected bool   `json:"connected"`
		Depth     int64  `json:"depth,omitempty"`
		Error     string `json:"error,omitempty"`
	} `json:"queue"`
}

// Register registers the health endpoint on the provided Gin router group.
func (h *HealthHandler) Register(rg *gin.RouterGroup) {
	rg.GET("/health", h.handle)
}

// handle is the Gin handler for GET /api/health.
func (h *HealthHandler) handle(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	resp := healthResponse{
		Timestamp: time.Now().UTC(),
		Liveness:  true, // if we are executing code, the process is alive
		Readiness: true, // will be set to false if any check fails
	}

	// ---- Database health check ----
	if err := h.pingDB(ctx); err != nil {
		resp.DB.Connected = false
		resp.DB.Error = err.Error()
		resp.Readiness = false
	} else {
		resp.DB.Connected = true
	}

	// ---- Queue health check ----
	if depth, err := h.checkQueue(ctx); err != nil {
		resp.Queue.Connected = false
		resp.Queue.Error = err.Error()
		resp.Readiness = false
	} else {
		resp.Queue.Connected = true
		resp.Queue.Depth = depth
	}

	// Choose appropriate HTTP status
	status := http.StatusOK
	if !resp.Readiness {
		status = http.StatusServiceUnavailable
	}

	c.JSON(status, resp)
}

// pingDB attempts a lightweight query to verify the DB connection.
func (h *HealthHandler) pingDB(ctx context.Context) error {
	// GORM does not expose Ping directly; a simple SELECT works.
	sqlDB, err := h.db.DB()
	if err != nil {
		return err
	}
	// Use the context with timeout.
	if err := sqlDB.PingContext(ctx); err != nil {
		return err
	}
	// Additionally, run a trivial query to ensure the connection is usable.
	var one int
	if err := h.db.WithContext(ctx).Raw("SELECT 1").Scan(&one).Error; err != nil {
		return err
	}
	return nil
}

// checkQueue queries the queue client for its current depth.
func (h *HealthHandler) checkQueue(ctx context.Context) (int64, error) {
	if h.queueClient == nil {
		// Queue is optional in some test environments; treat as healthy but unknown depth.
		return 0, nil
	}
	return h.queueClient.Depth(ctx)
}

// Ensure that the concrete Google Tasks client satisfies QueueChecker.
var _ QueueChecker = (*queue.Client)(nil)