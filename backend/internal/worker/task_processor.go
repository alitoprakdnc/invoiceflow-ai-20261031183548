package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"backend/internal/logger"
	"backend/internal/metrics"
	"backend/internal/repo"
	"backend/internal/repo/models"
	"backend/internal/service/llm"
	"backend/internal/service/ocr"
	"backend/internal/webhook"
	"backend/internal/validation"
)

// TaskPayload is the JSON payload sent by Cloud Tasks.
type TaskPayload struct {
	InvoiceID  uint `json:"invoice_id" binding:"required"`
	DocumentID uint `json:"document_id" binding:"required"`
}

// HandleTask is the HTTP handler invoked by a Cloud Tasks push request.
// It runs the OCR → LLM pipeline, stores the results, validates the data,
// updates the invoice status and dispatches any configured webhooks.
func HandleTask(c *gin.Context) {
	var payload TaskPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		logger.Get().Error("invalid task payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task payload"})
		return
	}

	db := repo.GetDB()
	l := logger.Get()
	m := metrics.Get()

	// ----------------------------------------------------------------------
	// Load invoice
	// ----------------------------------------------------------------------
	var inv models.Invoice
	if err := db.First(&inv, payload.InvoiceID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			l.Error("invoice not found", zap.Uint("invoice_id", payload.InvoiceID))
			c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
			return
		}
		l.Error("failed to query invoice", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// Idempotency: if already processed, acknowledge the task.
	if inv.Status == models.InvoiceStatusProcessed {
		c.Status(http.StatusOK)
		return
	}

	// ----------------------------------------------------------------------
	// Load document
	// ----------------------------------------------------------------------
	var doc models.Document
	if err := db.First(&doc, payload.DocumentID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			l.Error("document not found", zap.Uint("document_id", payload.DocumentID))
			c.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
			return
		}
		l.Error("failed to query document", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// ----------------------------------------------------------------------
	// OCR processing
	// ----------------------------------------------------------------------
	rawText, err := ocr.ExtractText(doc.FilePath)
	if err != nil {
		l.Error("OCR processing failed",
			zap.Uint("document_id", doc.ID),
			zap.Error(err),
		)
		updateInvoiceStatus(db, &inv, models.InvoiceStatusOCRFailed)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ocr failed"})
		return
	}

	// ----------------------------------------------------------------------
	// LLM extraction
	// ----------------------------------------------------------------------
	structured, err := llm.ExtractEntities(rawText)
	if err != nil {
		l.Error("LLM extraction failed",
			zap.Uint("invoice_id", inv.ID),
			zap.Error(err),
		)
		updateInvoiceStatus(db, &inv, models.InvoiceStatusLLMFailed)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "llm extraction failed"})
		return
	}

	// ----------------------------------------------------------------------
	// Validation
	// ----------------------------------------------------------------------
	if err := validation.ValidateExtraction(structured); err != nil {
		l.Warn("extracted data validation failed",
			zap.Uint("invoice_id", inv.ID),
			zap.Error(err),
		)
		// Continue – we still store the result but mark the invoice as validation_failed.
		updateInvoiceStatus(db, &inv, models.InvoiceStatusValidationFailed)
	} else {
		updateInvoiceStatus(db, &inv, models.InvoiceStatusProcessed)
	}

	// ----------------------------------------------------------------------
	// Persist extraction result and audit log inside a transaction
	// ----------------------------------------------------------------------
	if err := db.Transaction(func(tx *gorm.DB) error {
		// Marshal structured data for storage (JSONB column)
		structuredJSON, err := json.Marshal(structured)
		if err != nil {
			return fmt.Errorf("marshal structured data: %w", err)
		}

		extraction := models.ExtractionResult{
			InvoiceID:      inv.ID,
			RawText:        rawText,
			StructuredData: structuredJSON,
			CreatedAt:      time.Now(),
		}
		if err := tx.Create(&extraction).Error; err != nil {
			return fmt.Errorf("store extraction result: %w", err)
		}

		// Update invoice with processing timestamps
		if err := tx.Model(&inv).Updates(map[string]interface{}{
			"status":        inv.Status,
			"processed_at": time.Now(),
		}).Error; err != nil {
			return fmt.Errorf("update invoice status: %w", err)
		}

		// Audit log entry
		audit := models.AuditLog{
			UserID:    inv.UserID,
			Action:    "invoice_processed",
			Detail:    fmt.Sprintf("Invoice %d processed, status=%s", inv.ID, inv.Status),
			CreatedAt: time.Now(),
		}
		if err := tx.Create(&audit).Error; err != nil {
			return fmt.Errorf("store audit log: %w", err)
		}
		return nil
	}); err != nil {
		l.Error("transaction failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "processing failed"})
		return
	}

	// ----------------------------------------------------------------------
	// Dispatch webhooks (fire‑and‑forget)
	// ----------------------------------------------------------------------
	go dispatchWebhooks(context.Background(), db, inv, structured)

	// ----------------------------------------------------------------------
	// Metrics
	// ----------------------------------------------------------------------
	m.TaskProcessed.Inc()
	c.Status(http.StatusOK)
}

// updateInvoiceStatus updates the invoice status field in the DB.
// It does not commit the change; callers may wrap it in a transaction.
func updateInvoiceStatus(db *gorm.DB, inv *models.Invoice, status string) {
	inv.Status = status
	// The actual DB write is performed by the caller's transaction or a separate update.
}

// dispatchWebhooks fetches all webhook endpoints belonging to the invoice owner
// and sends the extracted payload to each endpoint with a constant‑time HMAC signature.
func dispatchWebhooks(ctx context.Context, db *gorm.DB, inv models.Invoice, payload interface{}) {
	var endpoints []models.WebhookEndpoint
	if err := db.Where("user_id = ?", inv.UserID).Find(&endpoints).Error; err != nil {
		logger.Get().Error("failed to load webhook endpoints",
			zap.Uint("user_id", inv.UserID),
			zap.Error(err),
		)
		return
	}

	body, err := json.Marshal(payload)
	if err != nil {
		logger.Get().Error("failed to marshal webhook payload", zap.Error(err))
		return
	}

	for _, ep := range endpoints {
		go func(ep models.WebhookEndpoint) {
			if err := sendWebhook(ctx, ep, body); err != nil {
				logger.Get().Error("webhook dispatch failed",
					zap.String("url", ep.URL),
					zap.Error(err),
				)
				metrics.Get().WebhookFailure.Inc()
			} else {
				metrics.Get().WebhookSuccess.Inc()
			}
		}(ep)
	}
}

// sendWebhook performs a single HTTP POST with HMAC signature.
// It respects the endpoint's configured secret and retries with exponential back‑off.
func sendWebhook(ctx context.Context, ep models.WebhookEndpoint, body []byte) error {
	const maxAttempts = 3
	var backoff = 500 * time.Millisecond

	signature := webhook.GenerateSignature(ep.Secret, body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", signature)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, err := client.Do(req)
		if err != nil {
			if attempt == maxAttempts {
				return fmt.Errorf("request error after %d attempts: %w", attempt, err)
			}
			time.Sleep(backoff)
			backoff *= 2
			continue
		}
		_ = resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			logger.Get().Info("webhook delivered",
				zap.String("url", ep.URL),
				zap.Int("status", resp.StatusCode),
			)
			return nil
		}

		// Non‑2xx response – treat as failure and retry.
		if attempt == maxAttempts {
			return fmt.Errorf("non‑2xx response after %d attempts: %d", attempt, resp.StatusCode)
		}
		time.Sleep(backoff)
		backoff *= 2
	}
	return fmt.Errorf("unreachable code reached in sendWebhook")
}