package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/yourorg/InvoiceFlow/backend/internal/config"
	"github.com/yourorg/InvoiceFlow/backend/internal/logger"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// InitPostgres creates a GORM DB instance, configures the connection pool,
// runs automatic migrations for all core models and returns the ready‑to‑use DB.
// It should be called once at application start‑up.
func InitPostgres(cfg *config.Config) (*gorm.DB, error) {
	// Build DSN – PostgreSQL expects a space‑separated list of key=value pairs.
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=UTC",
		cfg.DBHost,
		cfg.DBPort,
		cfg.DBUser,
		cfg.DBPassword,
		cfg.DBName,
		cfg.DBSSLMode,
	)

	// Choose GORM logger level based on the global log level.
	var gormLogLevel gormlogger.LogLevel
	switch cfg.LogLevel {
	case "debug":
		gormLogLevel = gormlogger.Info
	case "warn":
		gormLogLevel = gormlogger.Warn
	case "error":
		gormLogLevel = gormlogger.Error
	default:
		gormLogLevel = gormlogger.Silent
	}

	gormCfg := &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			// Use snake_case for tables/columns, keep plural form.
			SingularTable: false,
		},
		Logger: gormlogger.New(
			logger.NewZapAdapter(),
			gormlogger.Config{
				SlowThreshold: time.Second,
				LogLevel:      gormLogLevel,
				Colorful:     false,
			},
		),
		PrepareStmt: true,
	}

	db, err := gorm.Open(postgres.Open(dsn), gormCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	// Configure low‑level sql.DB connection pool.
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get generic database object: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.DBMaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.DBMaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.DBConnMaxLifetime) * time.Second)

	// Run migrations – keep them in code for the core models.
	if err := db.AutoMigrate(
		&User{},
		&Invoice{},
		&Document{},
		&ExtractionResult{},
		&WebhookEndpoint{},
		&AuditLog{},
	); err != nil {
		return nil, fmt.Errorf("auto migration failed: %w", err)
	}

	logger.Info("PostgreSQL connection established and migrations applied")
	return db, nil
}

// WithTransaction runs the supplied function inside a GORM transaction.
// The transaction inherits the provided context, ensuring request‑scoped
// cancellation and tracing work correctly.
//
// Example:
//
//	err := repo.WithTransaction(ctx, db, func(tx *gorm.DB) error {
//	    // use tx for all DB operations
//	    return nil
//	})
//
func WithTransaction(ctx context.Context, db *gorm.DB, fn func(tx *gorm.DB) error) error {
	tx := db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	// Ensure we rollback on panic.
	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback()
			panic(r) // re‑throw after rollback
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback().Error; rbErr != nil {
			return fmt.Errorf("transaction rollback error: %v (original error: %w)", rbErr, err)
		}
		return err
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("transaction commit failed: %w", err)
	}
	return nil
}