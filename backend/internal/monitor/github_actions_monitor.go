package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// Config holds configuration for the GitHub Actions monitor.
// All fields are read from environment variables by the caller
// (or from the central config package) and passed to NewGitHubActionsMonitor.
type Config struct {
	// GitHub organization name (e.g. "my-company")
	Org string
	// Personal access token with `repo` and `admin:org` scopes.
	// The token is sent as a Bearer token in the Authorization header.
	Token string
	// How often the monitor polls the GitHub API.
	Interval time.Duration
	// Optional Slack Incoming Webhook URL for alerts.
	SlackWebhookURL string
	// Queue length at which an alert should be sent.
	AlertThreshold int
	// Minimum time between two consecutive Slack alerts.
	AlertCooldown time.Duration
}

// GitHubActionsMonitor periodically queries the GitHub Actions API for the
// number of queued workflow runs in an organization. The current queue length
// is exposed as a Prometheus gauge. When the length exceeds a configurable
// threshold an optional Slack webhook is invoked (subject to a cooldown).
type GitHubActionsMonitor struct {
	org              string
	token            string
	interval         time.Duration
	slackWebhookURL  string
	alertThreshold   int
	alertCooldown    time.Duration
	client           *http.Client
	logger           *zap.Logger
	queueGauge       prometheus.Gauge
	lastAlertSent    atomic.Int64 // unix seconds of the last alert
}

// NewGitHubActionsMonitor creates a new monitor instance, registers the
// Prometheus metric and returns the monitor ready to be started.
func NewGitHubActionsMonitor(cfg Config, logger *zap.Logger) (*GitHubActionsMonitor, error) {
	if cfg.Org == "" {
		return nil, fmt.Errorf("github organization must be provided")
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("github token must be provided")
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 30 * time.Second
	}
	if cfg.AlertThreshold <= 0 {
		cfg.AlertThreshold = 100 // sensible default
	}
	if cfg.AlertCooldown <= 0 {
		cfg.AlertCooldown = 5 * time.Minute
	}

	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "github_actions_queue_length",
		Help: "Current number of queued GitHub Actions workflow runs for the configured organization.",
	})
	// Register the gauge; ignore error if already registered (e.g., in tests).
	_ = prometheus.Register(gauge)

	monitor := &GitHubActionsMonitor{
		org:             cfg.Org,
		token:           cfg.Token,
		interval:        cfg.Interval,
		slackWebhookURL: cfg.SlackWebhookURL,
		alertThreshold:  cfg.AlertThreshold,
		alertCooldown:   cfg.AlertCooldown,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
		logger:     logger,
		queueGauge: gauge,
	}
	return monitor, nil
}

// Run starts the monitoring loop. It blocks until the supplied context is
// cancelled. Errors are logged but do not stop the loop; the monitor will
// continue attempting to poll on the next tick.
func (m *GitHubActionsMonitor) Run(ctx context.Context) {
	m.logger.Info("starting GitHub Actions queue monitor",
		zap.String("org", m.org),
		zap.Duration("interval", m.interval),
		zap.Int("alert_threshold", m.alertThreshold),
		zap.String("slack_webhook", m.slackWebhookURL),
	)

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	// Perform an immediate check before waiting for the first tick.
	m.checkAndReport(ctx)

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("GitHub Actions monitor stopped")
			return
		case <-ticker.C:
			m.checkAndReport(ctx)
		}
	}
}

// checkAndReport fetches the current queue length, updates the gauge and
// triggers an alert if needed.
func (m *GitHubActionsMonitor) checkAndReport(ctx context.Context) {
	queueLen, err := m.fetchQueueLength(ctx)
	if err != nil {
		m.logger.Error("failed to fetch GitHub Actions queue length", zap.Error(err))
		// Do not update the gauge on error; keep the last known good value.
		return
	}

	m.queueGauge.Set(float64(queueLen))
	m.logger.Debug("GitHub Actions queue length", zap.Int("queued", queueLen))

	if m.slackWebhookURL != "" && queueLen >= m.alertThreshold {
		last := time.Unix(m.lastAlertSent.Load(), 0)
		if time.Since(last) >= m.alertCooldown {
			if err := m.sendSlackAlert(queueLen); err != nil {
				m.logger.Error("failed to send Slack alert", zap.Error(err))
			} else {
				m.lastAlertSent.Store(time.Now().Unix())
				m.logger.Info("sent Slack alert for high GitHub Actions queue", zap.Int("queued", queueLen))
			}
		}
	}
}

// fetchQueueLength queries the GitHub REST API for the number of queued workflow
// runs. The endpoint used is:
//   GET https://api.github.com/orgs/{org}/actions/runs?status=queued&per_page=1
// The response contains a `total_count` field that represents the full queue size.
func (m *GitHubActionsMonitor) fetchQueueLength(ctx context.Context) (int, error) {
	url := fmt.Sprintf("https://api.github.com/orgs/%s/actions/runs?status=queued&per_page=1", m.org)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "InvoiceFlow-AI-Monitor/1.0")

	resp, err := m.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Attempt to read error body for better diagnostics.
		var errBody struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		if errBody.Message == "" {
			errBody.Message = resp.Status
		}
		return 0, fmt.Errorf("github api error: %s (status %d)", errBody.Message, resp.StatusCode)
	}

	var payload struct {
		TotalCount int `json:"total_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, fmt.Errorf("decoding response: %w", err)
	}
	return payload.TotalCount, nil
}

// sendSlackAlert posts a simple JSON payload to the configured Slack webhook.
// The payload follows Slack's Incoming Webhook format.
func (m *GitHubActionsMonitor) sendSlackAlert(queueLen int) error {
	if m.slackWebhookURL == "" {
		return nil
	}

	message := fmt.Sprintf("*Alert*: GitHub Actions queue length for organization `%s` is *%d* (threshold %d).",
		m.org, queueLen, m.alertThreshold)

	payload := map[string]string{
		"text": message,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal slack payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, m.slackWebhookURL, nil)
	if err != nil {
		return fmt.Errorf("create slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Body = http.NoBody
	// Replace Body with a ReadCloser that yields our JSON.
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	req.ContentLength = int64(len(body))

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("slack request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("slack webhook returned status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// LoadConfigFromEnv builds a Config struct from environment variables.
// This helper is convenient for the main package but can be used elsewhere.
func LoadConfigFromEnv() (Config, error) {
	var cfg Config
	var missing []string

	if org := os.Getenv("GITHUB_ORG"); org != "" {
		cfg.Org = org
	} else {
		missing = append(missing, "GITHUB_ORG")
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		cfg.Token = token
	} else {
		missing = append(missing, "GITHUB_TOKEN")
	}
	if intervalStr := os.Getenv("GITHUB_MONITOR_INTERVAL"); intervalStr != "" {
		if d, err := time.ParseDuration(intervalStr); err == nil {
			cfg.Interval = d
		} else {
			return cfg, fmt.Errorf("invalid GITHUB_MONITOR_INTERVAL: %w", err)
		}
	}
	if webhook := os.Getenv("SLACK_WEBHOOK_URL"); webhook != "" {
		cfg.SlackWebhookURL = webhook
	}
	if threshStr := os.Getenv("GITHUB_QUEUE_ALERT_THRESHOLD"); threshStr != "" {
		if v, err := strconv.Atoi(threshStr); err == nil && v > 0 {
			cfg.AlertThreshold = v
		} else {
			return cfg, fmt.Errorf("invalid GITHUB_QUEUE_ALERT_THRESHOLD: %w", err)
		}
	}
	if cooldownStr := os.Getenv("GITHUB_QUEUE_ALERT_COOLDOWN"); cooldownStr != "" {
		if d, err := time.ParseDuration(cooldownStr); err == nil {
			cfg.AlertCooldown = d
		} else {
			return cfg, fmt.Errorf("invalid GITHUB_QUEUE_ALERT_COOLDOWN: %w", err)
		}
	}

	if len(missing) > 0 {
		return cfg, fmt.Errorf("missing required env vars: %v", missing)
	}
	return cfg, nil
}