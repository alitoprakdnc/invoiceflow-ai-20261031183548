package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	// Adjust the module path according to your go.mod file.
	"invoiceflow/backend/internal/repo"
)

// demoUser defines the data required to create a demo user.
type demoUser struct {
	Email    string
	Password string
	Role     string
}

// demoInvoice defines minimal data for a demo invoice.
type demoInvoice struct {
	Vendor string
	Date   time.Time
	Total  float64
}

// generateSecret creates a random 32‑byte hex string for webhook signing.
func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// hashPassword hashes a plain password using bcrypt.
func hashPassword(pw string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

// upsertUser creates a user if it does not already exist.
func upsertUser(ctx context.Context, db *gorm.DB, du demoUser) (*repo.User, error) {
	var user repo.User
	if err := db.WithContext(ctx).Where("email = ?", du.Email).First(&user).Error; err == nil {
		// User already exists.
		return &user, nil
	} else if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	hashed, err := hashPassword(du.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password for %s: %w", du.Email, err)
	}

	user = repo.User{
		Email:        du.Email,
		PasswordHash: hashed,
		Role:         du.Role,
		CreatedAt:    time.Now(),
	}
	if err := db.WithContext(ctx).Create(&user).Error; err != nil {
		return nil, fmt.Errorf("create user %s: %w", du.Email, err)
	}
	return &user, nil
}

// createDemoInvoice creates an invoice together with a placeholder document and an empty extraction result.
func createDemoInvoice(ctx context.Context, db *gorm.DB, userID uint, inv demoInvoice) (*repo.Invoice, error) {
	invoice := repo.Invoice{
		UserID:    userID,
		Vendor:    inv.Vendor,
		InvoiceDate: inv.Date,
		Total:     inv.Total,
		Status:    repo.InvoiceStatusPending,
		CreatedAt: time.Now(),
	}
	if err := db.WithContext(ctx).Create(&invoice).Error; err != nil {
		return nil, fmt.Errorf("create invoice for user %d: %w", userID, err)
	}

	// Placeholder document – in a real demo you would copy a sample PDF into the storage location.
	doc := repo.Document{
		InvoiceID: invoice.ID,
		FilePath:   "samples/sample_invoice.pdf",
		CreatedAt:  time.Now(),
	}
	if err := db.WithContext(ctx).Create(&doc).Error; err != nil {
		return nil, fmt.Errorf("create document for invoice %d: %w", invoice.ID, err)
	}

	// Empty extraction result – the processing pipeline will fill this later.
	extraction := repo.ExtractionResult{
		InvoiceID: invoice.ID,
		JSONData:  "{}",
		CreatedAt: time.Now(),
	}
	if err := db.WithContext(ctx).Create(&extraction).Error; err != nil {
		return nil, fmt.Errorf("create extraction result for invoice %d: %w", invoice.ID, err)
	}

	return &invoice, nil
}

// upsertWebhook creates a webhook endpoint for the given user if none exists.
func upsertWebhook(ctx context.Context, db *gorm.DB, userID uint) (*repo.WebhookEndpoint, error) {
	var wh repo.WebhookEndpoint
	if err := db.WithContext(ctx).Where("user_id = ?", userID).First(&wh).Error; err == nil {
		// Already exists.
		return &wh, nil
	} else if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	secret, err := generateSecret()
	if err != nil {
		return nil, fmt.Errorf("generate webhook secret: %w", err)
	}

	wh = repo.WebhookEndpoint{
		UserID:    userID,
		URL:       "https://webhook.example.com/receive",
		Secret:    secret,
		CreatedAt: time.Now(),
	}
	if err := db.WithContext(ctx).Create(&wh).Error; err != nil {
		return nil, fmt.Errorf("create webhook for user %d: %w", userID, err)
	}
	return &wh, nil
}

func main() {
	// Load .env if present – useful for local development.
	_ = godotenv.Load()

	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		log.Fatal("POSTGRES_DSN environment variable is required")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to PostgreSQL: %v", err)
	}

	// Run migrations for all core models.
	if err := db.AutoMigrate(
		&repo.User{},
		&repo.Invoice{},
		&repo.Document{},
		&repo.ExtractionResult{},
		&repo.WebhookEndpoint{},
		&repo.AuditLog{},
	); err != nil {
		log.Fatalf("auto migration failed: %v", err)
	}

	ctx := context.Background()

	// --------------------------------------------------------------------
	// Seed demo users
	// --------------------------------------------------------------------
	users := []demoUser{
		{Email: "admin@example.com", Password: "admin123", Role: "admin"},
		{Email: "accountant@example.com", Password: "accountant123", Role: "accountant"},
		{Email: "viewer@example.com", Password: "viewer123", Role: "viewer"},
	}

	var createdUsers []*repo.User
	for _, du := range users {
		u, err := upsertUser(ctx, db, du)
		if err != nil {
			log.Fatalf("error seeding user %s: %v", du.Email, err)
		}
		createdUsers = append(createdUsers, u)
		log.Printf("✅ User %s (%s) ready", u.Email, u.Role)
	}

	// --------------------------------------------------------------------
	// Seed demo invoices for each user
	// --------------------------------------------------------------------
	for _, u := range createdUsers {
		for i := 1; i <= 2; i++ {
			inv := demoInvoice{
				Vendor: fmt.Sprintf("Acme Corp %d", i),
				Date:   time.Now().AddDate(0, 0, -i*3), // 3,6 days ago
				Total:  float64(100*i) + 23.45,
			}
			_, err := createDemoInvoice(ctx, db, u.ID, inv)
			if err != nil {
				log.Fatalf("error creating invoice for user %s: %v", u.Email, err)
			}
			log.Printf("📄 Created invoice for %s: %s – $%.2f", u.Email, inv.Vendor, inv.Total)
		}
	}

	// --------------------------------------------------------------------
	// Seed webhook endpoints
	// --------------------------------------------------------------------
	for _, u := range createdUsers {
		wh, err := upsertWebhook(ctx, db, u.ID)
		if err != nil {
			log.Fatalf("error creating webhook for user %s: %v", u.Email, err)
		}
		log.Printf("🔗 Webhook for %s → %s (secret %s)", u.Email, wh.URL, wh.Secret[:8]+"...")
	}

	log.Println("🎉 Demo data seeding complete.")
}