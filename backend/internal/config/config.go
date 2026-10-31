package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/go-redis/redis/v8"
	"github.com/joho/godotenv"
)

// Config holds all runtime configuration for the service.
type Config struct {
	// Server
	ServerPort string
	LogLevel   string

	// PostgreSQL
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	// Redis
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// JWT
	JWTSigningKey    string // raw key or secret name (if using Secret Manager)
	JWTIssuer        string
	JWTExpiryMinutes int

	// Google Cloud
	GCPProjectID    string
	GCPRegion      string
	CloudTasksQueue string

	// Secret Manager
	UseSecretManager bool

	// LLM Service
	LLMServiceURL string

	// Tesseract
	TesseractCmd string

	// Monitoring
	PrometheusPort string

	// GitHub Actions monitor
	GitHubToken        string
	GitHubOrg          string
	GitHubAlertWebhook string
}

// Load reads configuration from the environment, falling back to .env file for local development.
// It validates required fields and returns a populated Config instance.
func Load() (*Config, error) {
	// Load .env if present; ignore error (file may not exist in production)
	_ = godotenv.Load()

	cfg := &Config{
		ServerPort:        getEnv("SERVER_PORT", "8080"),
		LogLevel:          getEnv("LOG_LEVEL", "info"),
		DBHost:            getEnv("POSTGRES_HOST", "localhost"),
		DBPort:            getEnv("POSTGRES_PORT", "5432"),
		DBUser:            getEnv("POSTGRES_USER", "postgres"),
		DBPassword:        getEnv("POSTGRES_PASSWORD", ""),
		DBName:            getEnv("POSTGRES_DB", "invoiceflow"),
		DBSSLMode:         getEnv("POSTGRES_SSLMODE", "disable"),
		RedisAddr:         getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:     getEnv("REDIS_PASSWORD", ""),
		JWTSigningKey:     getEnv("JWT_SIGNING_KEY", ""),
		JWTIssuer:         getEnv("JWT_ISSUER", "invoiceflow"),
		LLMServiceURL:     getEnv("LLM_SERVICE_URL", "http://localhost:8000"),
		TesseractCmd:      getEnv("TESSERACT_CMD", "tesseract"),
		PrometheusPort:    getEnv("PROMETHEUS_PORT", "9090"),
		GCPProjectID:      getEnv("GCP_PROJECT_ID", ""),
		GCPRegion:         getEnv("GCP_REGION", "us-central1"),
		CloudTasksQueue:  getEnv("CLOUD_TASKS_QUEUE", ""),
		GitHubToken:       getEnv("GITHUB_TOKEN", ""),
		GitHubOrg:         getEnv("GITHUB_ORG", ""),
		GitHubAlertWebhook: getEnv("GITHUB_ALERT_WEBHOOK", ""),
	}

	// Parse integer values
	if v, err := strconv.Atoi(getEnv("REDIS_DB", "0")); err == nil {
		cfg.RedisDB = v
	} else {
		return nil, fmt.Errorf("invalid REDIS_DB: %w", err)
	}

	if v, err := strconv.Atoi(getEnv("JWT_EXPIRY_MINUTES", "60")); err == nil {
		cfg.JWTExpiryMinutes = v
	} else {
		return nil, fmt.Errorf("invalid JWT_EXPIRY_MINUTES: %w", err)
	}

	// Parse boolean values
	if v, err := strconv.ParseBool(getEnv("USE_SECRET_MANAGER", "false")); err == nil {
		cfg.UseSecretManager = v
	} else {
		return nil, fmt.Errorf("invalid USE_SECRET_MANAGER: %w", err)
	}

	// Validate required configuration
	missing := []string{}
	if cfg.JWTSigningKey == "" && !cfg.UseSecretManager {
		missing = append(missing, "JWT_SIGNING_KEY")
	}
	if cfg.GCPProjectID == "" {
		missing = append(missing, "GCP_PROJECT_ID")
	}
	if cfg.CloudTasksQueue == "" {
		missing = append(missing, "CLOUD_TASKS_QUEUE")
	}
	if cfg.LLMServiceURL == "" {
		missing = append(missing, "LLM_SERVICE_URL")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}

// getEnv returns the value of the environment variable named by key.
// If the variable is empty, fallback is returned.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// DSN builds a PostgreSQL Data Source Name from the configuration.
func (c *Config) DSN() string {
	// Example DSN: "host=... port=... user=... password=... dbname=... sslmode=..."
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.DBHost,
		c.DBPort,
		c.DBUser,
		c.DBPassword,
		c.DBName,
		c.DBSSLMode,
	)
}

// RedisOptions converts the configuration into a go‑redis Options struct.
func (c *Config) RedisOptions() *redis.Options {
	return &redis.Options{
		Addr:     c.RedisAddr,
		Password: c.RedisPassword,
		DB:       c.RedisDB,
	}
}