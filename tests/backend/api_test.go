package tests

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"invoiceflow/backend/internal/api"
	"invoiceflow/backend/internal/auth"
	"invoiceflow/backend/internal/config"
	"invoiceflow/backend/internal/logger"
	"invoiceflow/backend/internal/repo"
	"invoiceflow/backend/internal/secret"
	"invoiceflow/backend/internal/webhook"
)

// testEnv holds resources needed for the integration tests.
type testEnv struct {
	Router      *gin.Engine
	DB          *sql.DB
	RedisServer *testcontainers.Container
	Cleanup     func()
	TestUserID  uuid.UUID
	JWTToken    string
}

// startPostgresContainer launches a PostgreSQL container for testing.
func startPostgresContainer(t *testing.T) (string, func()) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "postgres:15-alpine",
		Env:          map[string]string{"POSTGRES_PASSWORD": "password", "POSTGRES_USER": "test", "POSTGRES_DB": "invoiceflow_test"},
		ExposedPorts: []string{"5432/tcp"},
		WaitingFor:   wait.ForListeningPort("5432/tcp").WithStartupTimeout(60 * time.Second),
	}
	postgresC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	host, err := postgresC.Host(ctx)
	require.NoError(t, err)
	p, err := postgresC.MappedPort(ctx, "5432")
	require.NoError(t, err)

	dsn := "host=" + host + " port=" + p.Port() + " user=test password=password dbname=invoiceflow_test sslmode=disable"

	cleanup := func() {
		postgresC.Terminate(ctx) //nolint:errcheck
	}
	return dsn, cleanup
}

// startRedisContainer launches a Redis container for testing.
func startRedisContainer(t *testing.T) (string, func()) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "redis:7-alpine",
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForListeningPort("6379/tcp").WithStartupTimeout(30 * time.Second),
	}
	redisC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	host, err := redisC.Host(ctx)
	require.NoError(t, err)
	p, err := redisC.MappedPort(ctx, "6379")
	require.NoError(t, err)

	addr := host + ":" + p.Port()
	cleanup := func() {
		redisC.Terminate(ctx) //nolint:errcheck
	}
	return addr, cleanup
}

// applyMigrations runs the SQL migration files against the provided DB.
func applyMigrations(t *testing.T, db *sql.DB) {
	migrationFiles := []string{
		"migrations/001_init.sql",
		"migrations/002_invoice_table.sql",
	}
	for _, mf := range migrationFiles {
		content, err := os.ReadFile(mf)
		require.NoError(t, err, "reading migration %s", mf)

		_, err = db.Exec(string(content))
		require.NoError(t, err, "executing migration %s", mf)
	}
}

// createTestUser inserts a user and returns its ID.
func createTestUser(t *testing.T, db *sql.DB) uuid.UUID {
	userID := uuid.New()
	passwordHash, err := auth.HashPassword("Password123!")
	require.NoError(t, err)

	_, err = db.Exec(`
		INSERT INTO users (id, email, password_hash, role, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
	`, userID, "test@example.com", passwordHash, "admin")
	require.NoError(t, err)
	return userID
}

// initTestEnv builds the full application stack for integration testing.
func initTestEnv(t *testing.T) *testEnv {
	// Load base config (will be overridden by env vars below)
	cfg, err := config.Load()
	require.NoError(t, err)

	// Start dependent containers
	postgresDSN, pgCleanup := startPostgresContainer(t)
	redisAddr, redisCleanup := startRedisContainer(t)

	// Override config for test environment
	cfg.Database.DSN = postgresDSN
	cfg.Redis.Addr = redisAddr
	cfg.JWT.Secret = "test-secret-key" // simple static secret for JWT signing
	cfg.SecretManager.Enabled = false   // disable real GSM calls

	// Initialise logger (discard output)
	logger.Init(logger.Config{Level: "error", Development: false})

	// Initialise DB connection
	db, err := repo.NewPostgres(cfg.Database.DSN)
	require.NoError(t, err)

	// Apply migrations
	applyMigrations(t, db)

	// Initialise Redis client (repo package expects a *redis.Client)
	redisClient, err := repo.NewRedis(cfg.Redis)
	require.NoError(t, err)

	// Initialise secret manager (no-op because disabled)
	secretMgr := secret.NewNoopManager()

	// Initialise repositories
	userRepo := repo.NewUserRepo(db)
	invoiceRepo := repo.NewInvoiceRepo(db)

	// Initialise services that depend on secret manager
	jwtService := auth.NewService(cfg.JWT.Secret)

	// Build Gin router with all middlewares and routes
	router := gin.New()
	router.Use(gin.Recovery())
	api.RegisterRoutes(router, api.Dependencies{
		Config:       cfg,
		Logger:       logger.Get(),
		DB:           db,
		Redis:        redisClient,
		SecretMgr:    secretMgr,
		UserRepo:     userRepo,
		InvoiceRepo:  invoiceRepo,
		JWTService:   jwtService,
		WebhookSigner: webhook.NewSigner([]byte(cfg.JWT.Secret)),
	})

	// Create a test user and JWT token
	userID := createTestUser(t, db)
	token, err := jwtService.GenerateToken(userID.String(), "admin")
	require.NoError(t, err)

	cleanup := func() {
		db.Close()
		redisClient.Close()
		pgCleanup()
		redisCleanup()
	}

	return &testEnv{
		Router:      router,
		DB:          db,
		Cleanup:     cleanup,
		TestUserID:  userID,
		JWTToken:    token,
	}
}

// TestHealthEndpoint verifies that the health check returns expected JSON fields.
func TestHealthEndpoint(t *testing.T) {
	env := initTestEnv(t)
	defer env.Cleanup()

	req, _ := http.NewRequest(http.MethodGet, "/api/health", nil)
	resp := httptest.NewRecorder()
	env.Router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)

	var payload struct {
		Liveness  bool   `json:"liveness"`
		Readiness bool   `json:"readiness"`
		DB        string `json:"db"`
		Queue     int    `json:"queue_depth"`
	}
	err := json.NewDecoder(resp.Body).Decode(&payload)
	require.NoError(t, err)

	require.True(t, payload.Liveness, "service should be live")
	require.True(t, payload.Readiness, "service should be ready")
	require.Equal(t, "ok", payload.DB)
	require.GreaterOrEqual(t, payload.Queue, 0)
}

// TestInvoiceUploadEndpoint checks that a multipart upload is accepted and a task is queued.
func TestInvoiceUploadEndpoint(t *testing.T) {
	env := initTestEnv(t)
	defer env.Cleanup()

	// Prepare a tiny PDF payload (valid enough for Tesseract to accept)
	pdfBytes := []byte("%PDF-1.4\n%âãÏÓ\n1 0 obj\n<< /Type /Catalog >>\nendobj\nxref\n0 1\n0000000000 65535 f \ntrailer\n<< /Root 1 0 R >>\nstartxref\n9\n%%EOF")
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "sample.pdf")
	require.NoError(t, err)
	_, err = io.Copy(part, bytes.NewReader(pdfBytes))
	require.NoError(t, err)
	// Add optional fields if the API expects them (e.g., vendor_id)
	_ = writer.WriteField("vendor_id", "12345")
	require.NoError(t, writer.Close())

	req, _ := http.NewRequest(http.MethodPost, "/api/invoices", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+env.JWTToken)

	resp := httptest.NewRecorder()
	env.Router.ServeHTTP(resp, req)

	// The upload handler is expected to return 202 Accepted
	require.Equal(t, http.StatusAccepted, resp.Code)

	var respBody struct {
		InvoiceID string `json:"invoice_id"`
		Status    string `json:"status"`
	}
	err = json.NewDecoder(resp.Body).Decode(&respBody)
	require.NoError(t, err)
	require.NotEmpty(t, respBody.InvoiceID)
	require.Equal(t, "queued", respBody.Status)

	// Verify the invoice record exists in DB with status = "queued"
	var status string
	err = env.DB.QueryRow(`SELECT status FROM invoices WHERE id = $1`, respBody.InvoiceID).Scan(&status)
	require.NoError(t, err)
	require.Equal(t, "queued", status)
}

// TestWebhookSignatureGeneration ensures constant‑time HMAC signatures are produced correctly.
func TestWebhookSignatureGeneration(t *testing.T) {
	secretKey := []byte("super-secret-key")
	payload := []byte(`{"event":"invoice.processed","invoice_id":"abc-123"}`)

	sig := webhook.GenerateSignature(payload, secretKey)
	require.NotEmpty(t, sig)

	// Verify using constant‑time compare
	valid := webhook.VerifySignature(payload, sig, secretKey)
	require.True(t, valid)

	// Tamper with payload – verification must fail
	invalid := webhook.VerifySignature(append(payload, 'x'), sig, secretKey)
	require.False(t, invalid)
}

// Helper to locate the project root for migration file paths.
func init() {
	// Ensure the working directory is the repository root when `go test ./...` runs.
	wd, _ := os.Getwd()
	if !filepath.IsAbs(wd) {
		wd, _ = filepath.Abs(wd)
	}
	// Change to repo root if we are inside a sub‑directory like tests.
	if filepath.Base(wd) == "tests" {
		_ = os.Chdir(filepath.Dir(wd))
	}
}