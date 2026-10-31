package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"backend/internal/auth"
	"backend/internal/logger"
	"backend/internal/metrics"
	"backend/internal/queue"
	"backend/internal/repo"
	"backend/internal/validation"
)

// RegisterInvoiceRoutes registers all invoice‑related routes.
// It is called from the main router setup.
func RegisterInvoiceRoutes(rg *gin.RouterGroup) {
	invoices := rg.Group("/invoices")
	invoices.Use(auth.JWTMiddleware())
	{
		invoices.POST("/", uploadInvoiceHandler)
		invoices.GET("/", listInvoicesHandler)
		invoices.GET("/:id", getInvoiceHandler)
		invoices.DELETE("/:id", deleteInvoiceHandler)

		invoices.GET("/:id/status", getInvoiceStatusHandler)

		// Webhook subscription management
		invoices.POST("/:id/webhooks", createWebhookHandler)
		invoices.GET("/:id/webhooks", listWebhooksHandler)
		invoices.DELETE("/:id/webhooks/:webhookId", deleteWebhookHandler)
	}
}

// ---------- Helper utilities ----------

// getUserInfo extracts the authenticated user ID and role from the Gin context.
func getUserInfo(c *gin.Context) (userID uint, role string, err error) {
	uid, ok := c.Get(auth.ContextUserIDKey)
	if !ok {
		err = errors.New("user id missing in context")
		return
	}
	uidUint, ok := uid.(uint)
	if !ok {
		err = errors.New("invalid user id type")
		return
	}
	userID = uidUint

	r, ok := c.Get(auth.ContextUserRoleKey)
	if !ok {
		err = errors.New("user role missing in context")
		return
	}
	roleStr, ok := r.(string)
	if !ok {
		err = errors.New("invalid role type")
		return
	}
	role = roleStr
	return
}

// isAdmin returns true if the supplied role grants admin privileges.
func isAdmin(role string) bool {
	return role == "admin"
}

// writeAudit creates an audit log entry.
func writeAudit(db *gorm.DB, userID uint, action string, details string) {
	a := repo.AuditLog{
		UserID:    userID,
		Action:    action,
		Details:   details,
		Timestamp: time.Now(),
	}
	// Errors on audit logging should not break the main flow.
	_ = db.Create(&a).Error
}

// ---------- Request / Response structs ----------

type uploadResponse struct {
	InvoiceID uint   `json:"invoice_id"`
	Status    string `json:"status"`
}

type invoiceListResponse struct {
	Total    int64           `json:"total"`
	Page     int             `json:"page"`
	Limit    int             `json:"limit"`
	Invoices []repo.Invoice  `json:"invoices"`
}

type webhookCreateRequest struct {
	URL    string `json:"url" binding:"required"`
	Secret string `json:"secret,omitempty"`
}

// ---------- Handlers ----------

// uploadInvoiceHandler handles multipart file upload, creates an invoice record
// and enqueues the OCR/LLM processing task.
func uploadInvoiceHandler(c *gin.Context) {
	log := logger.FromContext(c)
	db := repo.GetDB()

	userID, role, err := getUserInfo(c)
	if err != nil {
		log.Error("failed to get user info", zap.Error(err))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	_ = role // upload is allowed for any authenticated role

	file, err := c.FormFile("file")
	if err != nil {
		log.Error("missing file in request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}

	ext := filepath.Ext(file.Filename)
	allowed := map[string]bool{".pdf": true, ".png": true, ".jpg": true, ".jpeg": true}
	if !allowed[ext] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported file type"})
		return
	}

	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "./uploads"
	}
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		log.Error("failed to create upload directory", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	newFileName := uuid.New().String() + ext
	destPath := filepath.Join(uploadDir, newFileName)

	if err := c.SaveUploadedFile(file, destPath); err != nil {
		log.Error("failed to save uploaded file", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not store file"})
		return
	}

	// Create invoice record
	invoice := repo.Invoice{
		UserID:    userID,
		Status:    "pending",
		CreatedAt: time.Now(),
	}
	if err := db.Create(&invoice).Error; err != nil {
		log.Error("failed to create invoice record", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create invoice"})
		return
	}

	// Store document metadata
	doc := repo.Document{
		InvoiceID:   invoice.ID,
		FilePath:   destPath,
		FileName:   file.Filename,
		UploadedAt: time.Now(),
	}
	if err := db.Create(&doc).Error; err != nil {
		log.Error("failed to store document metadata", zap.Error(err))
		// attempt to clean up invoice record
		_ = db.Delete(&invoice).Error
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not store document info"})
		return
	}

	// Enqueue background processing task
	if err := queue.EnqueueInvoiceTask(c.Request.Context(), invoice.ID, destPath); err != nil {
		log.Error("failed to enqueue processing task", zap.Error(err))
		// Cleanup DB entries – best‑effort
		_ = db.Delete(&doc).Error
		_ = db.Delete(&invoice).Error
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not schedule processing"})
		return
	}

	// Audit
	writeAudit(db, userID, "upload_invoice", fmt.Sprintf("invoice_id=%d file=%s", invoice.ID, file.Filename))

	metrics.IncInvoiceUploaded()

	c.JSON(http.StatusAccepted, uploadResponse{
		InvoiceID: invoice.ID,
		Status:    invoice.Status,
	})
}

// listInvoicesHandler returns a paginated list of invoices visible to the caller.
func listInvoicesHandler(c *gin.Context) {
	log := logger.FromContext(c)
	db := repo.GetDB()

	userID, role, err := getUserInfo(c)
	if err != nil {
		log.Error("auth extraction failed", zap.Error(err))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	var total int64
	query := db.Model(&repo.Invoice{})
	if !isAdmin(role) {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.Count(&total).Error; err != nil {
		log.Error("failed to count invoices", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not retrieve invoices"})
		return
	}

	var invoices []repo.Invoice
	if err := query.
		Preload("Document").
		Preload("ExtractionResult").
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&invoices).Error; err != nil {
		log.Error("failed to fetch invoices", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not retrieve invoices"})
		return
	}

	c.JSON(http.StatusOK, invoiceListResponse{
		Total:    total,
		Page:     page,
		Limit:    limit,
		Invoices: invoices,
	})
}

// getInvoiceHandler returns detailed information for a single invoice.
func getInvoiceHandler(c *gin.Context) {
	log := logger.FromContext(c)
	db := repo.GetDB()

	userID, role, err := getUserInfo(c)
	if err != nil {
		log.Error("auth extraction failed", zap.Error(err))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	invoiceID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid invoice id"})
		return
	}

	var invoice repo.Invoice
	query := db.Preload("Document").
		Preload("ExtractionResult").
		Preload("WebhookEndpoints")
	if isAdmin(role) {
		query = query.First(&invoice, invoiceID)
	} else {
		query = query.First(&invoice, "id = ? AND user_id = ?", invoiceID, userID)
	}
	if errors.Is(query.Error, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
		return
	}
	if query.Error != nil {
		log.Error("failed to fetch invoice", zap.Error(query.Error))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not retrieve invoice"})
		return
	}

	c.JSON(http.StatusOK, invoice)
}

// deleteInvoiceHandler removes an invoice and all related data.
func deleteInvoiceHandler(c *gin.Context) {
	log := logger.FromContext(c)
	db := repo.GetDB()

	userID, role, err := getUserInfo(c)
	if err != nil {
		log.Error("auth extraction failed", zap.Error(err))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	invoiceID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid invoice id"})
		return
	}

	// Verify ownership / permission
	var invoice repo.Invoice
	query := db.First(&invoice, "id = ?", invoiceID)
	if errors.Is(query.Error, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
		return
	}
	if query.Error != nil {
		log.Error("db error while fetching invoice", zap.Error(query.Error))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not process request"})
		return
	}
	if invoice.UserID != userID && !isAdmin(role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "permission denied"})
		return
	}

	// Transactional delete
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&repo.WebhookEndpoint{}, "invoice_id = ?", invoice.ID).Error; err != nil {
			return err
		}
		if err := tx.Delete(&repo.ExtractionResult{}, "invoice_id = ?", invoice.ID).Error; err != nil {
			return err
		}
		if err := tx.Delete(&repo.Document{}, "invoice_id = ?", invoice.ID).Error; err != nil {
			return err
		}
		if err := tx.Delete(&invoice).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		log.Error("failed to delete invoice transactionally", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete invoice"})
		return
	}

	writeAudit(db, userID, "delete_invoice", fmt.Sprintf("invoice_id=%d", invoice.ID))
	metrics.IncInvoiceDeleted()

	c.Status(http.StatusNoContent)
}

// getInvoiceStatusHandler returns the current processing status of an invoice.
func getInvoiceStatusHandler(c *gin.Context) {
	log := logger.FromContext(c)
	db := repo.GetDB()

	userID, role, err := getUserInfo(c)
	if err != nil {
		log.Error("auth extraction failed", zap.Error(err))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	invoiceID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid invoice id"})
		return
	}

	var invoice repo.Invoice
	query := db.Select("id", "status", "error_message").
		First(&invoice, "id = ?", invoiceID)
	if errors.Is(query.Error, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
		return
	}
	if query.Error != nil {
		log.Error("db error while fetching status", zap.Error(query.Error))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not retrieve status"})
		return
	}
	// Permission check
	if invoice.UserID != userID && !isAdmin(role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "permission denied"})
		return
	}

	resp := gin.H{
		"id":            invoice.ID,
		"status":        invoice.Status,
		"error_message": invoice.ErrorMessage,
	}
	c.JSON(http.StatusOK, resp)
}

// ---------- Webhook subscription handlers ----------

// createWebhookHandler registers a new outbound webhook for a given invoice.
func createWebhookHandler(c *gin.Context) {
	log := logger.FromContext(c)
	db := repo.GetDB()

	userID, role, err := getUserInfo(c)
	if err != nil {
		log.Error("auth extraction failed", zap.Error(err))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	invoiceID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid invoice id"})
		return
	}

	// Verify invoice ownership
	var invoice repo.Invoice
	if err := db.First(&invoice, "id = ?", invoiceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
			return
		}
		log.Error("db error while fetching invoice", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify invoice"})
		return
	}
	if invoice.UserID != userID && !isAdmin(role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "permission denied"})
		return
	}

	var req webhookCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
		return
	}
	// Validate URL
	if _, err := url.ParseRequestURI(req.URL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook URL"})
		return
	}
	if err := validation.Validate.Struct(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation failed", "details": err.Error()})
		return
	}

	// Store secret securely – for demo we store plain, but in production hash it.
	webhook := repo.WebhookEndpoint{
		InvoiceID:   invoice.ID,
		URL:         req.URL,
		Secret:     req.Secret,
		CreatedAt:   time.Now(),
		CreatedByID: userID,
	}
	if err := db.Create(&webhook).Error; err != nil {
		log.Error("failed to create webhook endpoint", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not register webhook"})
		return
	}

	writeAudit(db, userID, "create_webhook", fmt.Sprintf("invoice_id=%d webhook_id=%d", invoice.ID, webhook.ID))
	metrics.IncWebhookCreated()

	c.JSON(http.StatusCreated, webhook)
}

// listWebhooksHandler returns all webhook endpoints configured for an invoice.
func listWebhooksHandler(c *gin.Context) {
	log := logger.FromContext(c)
	db := repo.GetDB()

	userID, role, err := getUserInfo(c)
	if err != nil {
		log.Error("auth extraction failed", zap.Error(err))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	invoiceID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid invoice id"})
		return
	}

	// Verify ownership
	var invoice repo.Invoice
	if err := db.First(&invoice, "id = ?", invoiceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
			return
		}
		log.Error("db error while fetching invoice", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify invoice"})
		return
	}
	if invoice.UserID != userID && !isAdmin(role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "permission denied"})
		return
	}

	var hooks []repo.WebhookEndpoint
	if err := db.Where("invoice_id = ?", invoiceID).Find(&hooks).Error; err != nil {
		log.Error("failed to list webhooks", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not retrieve webhooks"})
		return
	}

	c.JSON(http.StatusOK, hooks)
}

// deleteWebhookHandler removes a specific webhook endpoint from an invoice.
func deleteWebhookHandler(c *gin.Context) {
	log := logger.FromContext(c)
	db := repo.GetDB()

	userID, role, err := getUserInfo(c)
	if err != nil {
		log.Error("auth extraction failed", zap.Error(err))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	invoiceID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid invoice id"})
		return
	}
	webhookID, err := strconv.ParseUint(c.Param("webhookId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook id"})
		return
	}

	// Verify invoice ownership
	var invoice repo.Invoice
	if err := db.First(&invoice, "id = ?", invoiceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
			return
		}
		log.Error("db error while fetching invoice", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify invoice"})
		return
	}
	if invoice.UserID != userID && !isAdmin(role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "permission denied"})
		return
	}

	// Delete webhook
	if err := db.Delete(&repo.WebhookEndpoint{}, "id = ? AND invoice_id = ?", webhookID, invoiceID).Error; err != nil {
		log.Error("failed to delete webhook", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete webhook"})
		return
	}

	writeAudit(db, userID, "delete_webhook", fmt.Sprintf("invoice_id=%d webhook_id=%d", invoiceID, webhookID))
	metrics.IncWebhookDeleted()

	c.Status(http.StatusNoContent)
}

// ---------- JSON Marshal helpers (optional) ----------

// MarshalInvoice safely hides sensitive fields when returning JSON.
func MarshalInvoice(inv repo.Invoice) ([]byte, error) {
	// Remove secret fields from webhook endpoints before marshalling.
	type safeWebhook struct {
		ID        uint   `json:"id"`
		URL       string `json:"url"`
		CreatedAt string `json:"created_at"`
	}
	type safeInvoice struct {
		ID               uint            `json:"id"`
		UserID           uint            `json:"user_id"`
		Status           string          `json:"status"`
		CreatedAt        string          `json:"created_at"`
		Document         *repo.Document  `json:"document,omitempty"`
		ExtractionResult *repo.ExtractionResult `json:"extraction_result,omitempty"`
		Webhooks         []safeWebhook   `json:"webhooks,omitempty"`
	}
	safe := safeInvoice{
		ID:        inv.ID,
		UserID:    inv.UserID,
		Status:    inv.Status,
		CreatedAt: inv.CreatedAt.Format(time.RFC3339),
		Document:  inv.Document,
		ExtractionResult: inv.ExtractionResult,
	}
	for _, wh := range inv.WebhookEndpoints {
		safe.Webhooks = append(safe.Webhooks, safeWebhook{
			ID:        wh.ID,
			URL:       wh.URL,
			CreatedAt: wh.CreatedAt.Format(time.RFC3339),
		})
	}
	return json.Marshal(safe)
}