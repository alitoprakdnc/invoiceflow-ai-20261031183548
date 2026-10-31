package repo

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// Role constants for User.Role
const (
	RoleAdmin     = "admin"
	RoleAccountant = "accountant"
	RoleViewer    = "viewer"
)

// InvoiceStatus constants for Invoice.Status
const (
	InvoiceStatusPending    = "pending"
	InvoiceStatusProcessing = "processing"
	InvoiceStatusCompleted  = "completed"
	InvoiceStatusFailed     = "failed"
)

// User represents an application user.
// It includes role‑based access information and is linked to the
// resources a user can own (invoices, webhook endpoints, audit logs).
type User struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
	Email        string         `gorm:"size:255;uniqueIndex;not null" json:"email"`
	PasswordHash string        `gorm:"size:255;not null" json:"-"`
	Name         string         `gorm:"size:100" json:"name,omitempty"`
	Role         string         `gorm:"size:20;not null" json:"role"` // admin, accountant, viewer

	// Relationships
	Invoices          []Invoice          `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"-"`
	WebhookEndpoints  []WebhookEndpoint  `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"-"`
	AuditLogs         []AuditLog         `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"-"`
}

// TableName overrides the default table name for User.
func (User) TableName() string {
	return "users"
}

// Invoice represents a financial document uploaded by a user.
// The processing pipeline updates its status and eventually stores
// the extracted structured data.
type Invoice struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	UserID    uint   `gorm:"not null;index" json:"user_id"`
	Status    string `gorm:"size:20;not null;default:'pending'" json:"status"` // pending, processing, completed, failed
	Vendor    string `gorm:"size:255" json:"vendor,omitempty"`
	InvoiceDate *time.Time `json:"invoice_date,omitempty"`
	TotalAmount float64   `json:"total_amount,omitempty"`
	Currency    string    `gorm:"size:10" json:"currency,omitempty"`

	ProcessedAt *time.Time `json:"processed_at,omitempty"`

	// Relationships
	Documents        []Document        `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"documents,omitempty"`
	ExtractionResult ExtractionResult  `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"extraction_result,omitempty"`
}

// TableName overrides the default table name for Invoice.
func (Invoice) TableName() string {
	return "invoices"
}

// Document stores metadata about an uploaded file and the OCR raw text.
type Document struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	InvoiceID uint   `gorm:"not null;index" json:"invoice_id"`
	FileName  string `gorm:"size:255;not null" json:"file_name"`
	FilePath  string `gorm:"size:1024;not null" json:"file_path"` // path inside the storage bucket / volume
	MIMEType  string `gorm:"size:100;not null" json:"mime_type"`
	Size      int64  `gorm:"not null" json:"size"` // bytes

	// OCR raw text – populated after the OCR step.
	RawText   string `gorm:"type:text" json:"raw_text,omitempty"`
}

// TableName overrides the default table name for Document.
func (Document) TableName() string {
	return "documents"
}

// ExtractionResult holds the structured data produced by the LLM after OCR.
// The entire payload is stored as JSON for flexibility, while key fields are
// also materialised for easy querying.
type ExtractionResult struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	InvoiceID uint `gorm:"uniqueIndex;not null" json:"invoice_id"`

	// Core fields extracted from the document.
	Vendor      string     `gorm:"size:255" json:"vendor,omitempty"`
	InvoiceDate *time.Time `json:"invoice_date,omitempty"`
	TotalAmount float64    `json:"total_amount,omitempty"`
	TaxAmount   float64    `json:"tax_amount,omitempty"`
	Currency    string     `gorm:"size:10" json:"currency,omitempty"`

	// Raw JSON returned by the LLM – includes line items and any additional metadata.
	RawJSON json.RawMessage `gorm:"type:jsonb" json:"raw_json,omitempty"`

	// Convenience field for line‑items – stored as JSON array.
	LineItems json.RawMessage `gorm:"type:jsonb" json:"line_items,omitempty"`
}

// TableName overrides the default table name for ExtractionResult.
func (ExtractionResult) TableName() string {
	return "extraction_results"
}

// WebhookEndpoint defines a user‑configured outbound webhook.
// The Secret is used to generate a constant‑time HMAC signature for each payload.
type WebhookEndpoint struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	UserID uint   `gorm:"not null;index" json:"user_id"`
	URL    string `gorm:"size:1024;not null" json:"url"`
	Secret string `gorm:"size:255;not null" json:"-"` // HMAC secret; never exposed via API

	Enabled    bool   `gorm:"default:true" json:"enabled"`
	// EventTypes is a JSON array of strings (e.g., ["invoice.completed","invoice.failed"])
	EventTypes json.RawMessage `gorm:"type:jsonb;default:'[]'" json:"event_types,omitempty"`

	// Optional description for UI purposes.
	Description string `gorm:"size:255" json:"description,omitempty"`
}

// TableName overrides the default table name for WebhookEndpoint.
func (WebhookEndpoint) TableName() string {
	return "webhook_endpoints"
}

// AuditLog records every significant action performed in the system.
// Details can store arbitrary JSON for richer context (e.g., request payloads,
// validation errors, webhook responses).
type AuditLog struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`

	UserID uint   `gorm:"index" json:"user_id,omitempty"`
	Action string `gorm:"size:100;not null" json:"action"` // e.g., "upload", "extraction", "webhook_sent"
	Entity string `gorm:"size:50" json:"entity,omitempty"` // e.g., "invoice", "document"
	EntityID uint `json:"entity_id,omitempty"`

	Details json.RawMessage `gorm:"type:jsonb" json:"details,omitempty"`
}

// TableName overrides the default table name for AuditLog.
func (AuditLog) TableName() string {
	return "audit_logs"
}