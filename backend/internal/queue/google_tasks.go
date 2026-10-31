package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"cloud.google.com/go/cloudtasks/apiv2"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"github.com/yourorg/invoiceflow/internal/config"
	"github.com/yourorg/invoiceflow/internal/metrics"

	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TaskQueue encapsulates interactions with Google Cloud Tasks.
type TaskQueue struct {
	client          *cloudtasks.Client
	queuePath       string
	targetURL       string
	logger          *zap.Logger
	queueDepthGauge prometheus.Gauge
}

// NewTaskQueue creates a new TaskQueue helper.
// It registers a Prometheus gauge for the queue depth and starts a background
// goroutine that periodically updates the gauge.
func NewTaskQueue(ctx context.Context, cfg *config.Config, logger *zap.Logger) (*TaskQueue, error) {
	// Initialise Cloud Tasks client.
	client, err := cloudtasks.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("cloudtasks.NewClient: %w", err)
	}

	// Build the fully‑qualified queue name.
	queuePath := fmt.Sprintf(
		"projects/%s/locations/%s/queues/%s",
		cfg.GCPProjectID,
		cfg.CloudTasksLocation,
		cfg.CloudTasksQueue,
	)

	// Register Prometheus gauge for queue depth.
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "invoiceflow",
		Subsystem: "queue",
		Name:      "depth",
		Help:      "Current number of tasks pending in the Cloud Tasks queue",
	})
	if err := prometheus.Register(gauge); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			gauge = are.ExistingCollector.(prometheus.Gauge)
		} else {
			logger.Error("failed to register queue depth gauge", zap.Error(err))
		}
	}

	tq := &TaskQueue{
		client:          client,
		queuePath:       queuePath,
		targetURL:       cfg.CloudTasksTargetURL,
		logger:          logger,
		queueDepthGauge: gauge,
	}

	// Start background refresh of the gauge.
	go tq.refreshQueueDepthLoop(ctx, 30*time.Second)

	// Expose the gauge through the central metrics package (optional, depending on project layout).
	metrics.RegisterGauge(gauge)

	return tq, nil
}

// Enqueue creates a new task in the configured Cloud Tasks queue.
// payload is marshalled to JSON and sent as the HTTP request body.
// If scheduleTime is non‑nil the task will be delayed until that time.
func (t *TaskQueue) Enqueue(ctx context.Context, payload interface{}, scheduleTime *time.Time) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("json.Marshal payload: %w", err)
	}

	httpReq := &taskspb.HttpRequest{
		HttpMethod: taskspb.HttpMethod_POST,
		Url:        t.targetURL,
		Body:       body,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	}

	task := &taskspb.Task{
		MessageType: &taskspb.Task_HttpRequest{
			HttpRequest: httpReq,
		},
	}

	if scheduleTime != nil {
		task.ScheduleTime = timestamppb.New(*scheduleTime)
	}

	req := &taskspb.CreateTaskRequest{
		Parent: t.queuePath,
		Task:   task,
	}

	_, err = t.client.CreateTask(ctx, req)
	if err != nil {
		return fmt.Errorf("cloudtasks.CreateTask: %w", err)
	}

	t.logger.Debug("task enqueued", zap.Any("payload", payload), zap.String("queue", t.queuePath))
	return nil
}

// GetQueueDepth returns the approximate number of pending tasks in the queue.
func (t *TaskQueue) GetQueueDepth(ctx context.Context) (int64, error) {
	req := &taskspb.GetQueueRequest{
		Name: t.queuePath,
	}
	queue, err := t.client.GetQueue(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("cloudtasks.GetQueue: %w", err)
	}
	if queue.GetStats() == nil {
		return 0, nil
	}
	return queue.GetStats().GetApproximateNumberOfTasks(), nil
}

// refreshQueueDepthLoop periodically updates the Prometheus gauge with the current queue depth.
func (t *TaskQueue) refreshQueueDepthLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			depth, err := t.GetQueueDepth(ctx)
			if err != nil {
				t.logger.Error("failed to fetch queue depth", zap.Error(err))
				continue
			}
			t.queueDepthGauge.Set(float64(depth))
		}
	}
}

// Close releases the underlying Cloud Tasks client resources.
func (t *TaskQueue) Close() error {
	return t.client.Close()
}