package ocr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"invoiceflow/internal/repo"
	"invoiceflow/internal/repo/models"
)

// Service provides OCR capabilities using the Tesseract CLI.
type Service struct {
	db     *gorm.DB
	logger *zap.Logger
	// tesseractPath allows overriding the binary location (useful for tests).
	tesseractPath string
	// timeout for the OCR process.
	timeout time.Duration
}

// New creates a new OCR service.
// tesseractPath defaults to "tesseract" which must be in the container's PATH.
// timeout defaults to 30 seconds if zero.
func New(db *gorm.DB, logger *zap.Logger, opts ...Option) *Service {
	s := &Service{
		db:            db,
		logger:        logger,
		tesseractPath: "tesseract",
		timeout:       30 * time.Second,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Option configures the OCR service.
type Option func(*Service)

// WithTesseractPath overrides the default tesseract binary location.
func WithTesseractPath(path string) Option {
	return func(s *Service) {
		s.tesseractPath = path
	}
}

// WithTimeout overrides the default OCR execution timeout.
func WithTimeout(d time.Duration) Option {
	return func(s *Service) {
		if d > 0 {
			s.timeout = d
		}
	}
}

// Process runs OCR on the supplied file, stores the raw text in the
// corresponding document record and returns the extracted text.
//
// The function expects the file to be accessible on the local filesystem.
// documentID must reference an existing Document row; the raw OCR text
// will be persisted in the `raw_text` column of that row.
//
// Errors are wrapped with context for easier debugging.
func (s *Service) Process(ctx context.Context, filePath string, documentID uint) (string, error) {
	s.logger.Debug("starting OCR process",
		zap.String("file", filePath),
		zap.Uint("document_id", documentID),
	)

	// Validate file existence early.
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			s.logger.Error("file does not exist", zap.String("path", filePath))
			return "", fmt.Errorf("ocr: file not found: %w", err)
		}
		s.logger.Error("unable to stat file", zap.String("path", filePath), zap.Error(err))
		return "", fmt.Errorf("ocr: cannot access file: %w", err)
	}

	// Ensure the document exists before we start heavy work.
	var doc models.Document
	if err := s.db.WithContext(ctx).First(&doc, documentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.logger.Error("document not found", zap.Uint("id", documentID))
			return "", fmt.Errorf("ocr: document %d not found: %w", documentID, err)
		}
		s.logger.Error("db error fetching document", zap.Uint("id", documentID), zap.Error(err))
		return "", fmt.Errorf("ocr: db error: %w", err)
	}

	// Prepare the command: tesseract <input> stdout -l eng --psm 1
	// Using stdout as the output destination.
	cmd := exec.CommandContext(ctx, s.tesseractPath, filePath, "stdout", "-l", "eng", "--psm", "1")
	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	// Apply a timeout to avoid runaway processes.
	ctxTimeout, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	cmd = exec.CommandContext(ctxTimeout, s.tesseractPath, filePath, "stdout", "-l", "eng", "--psm", "1")
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	s.logger.Debug("executing tesseract command",
		zap.String("binary", s.tesseractPath),
		zap.String("args", fmt.Sprintf("%s stdout -l eng --psm 1", filepath.Base(filePath))),
	)

	if err := cmd.Run(); err != nil {
		// Capture stderr for diagnostics.
		stderr := errBuf.String()
		s.logger.Error("tesseract execution failed",
			zap.Error(err),
			zap.String("stderr", stderr),
			zap.String("file", filePath),
		)
		return "", fmt.Errorf("ocr: tesseract execution failed: %w; stderr: %s", err, stderr)
	}

	rawText := outBuf.String()
	s.logger.Info("ocr completed successfully",
		zap.Uint("document_id", documentID),
		zap.Int("text_len", len(rawText)),
	)

	// Persist the raw OCR text atomically.
	if err := s.persistRawText(ctx, documentID, rawText); err != nil {
		s.logger.Error("failed to persist OCR result", zap.Error(err), zap.Uint("document_id", documentID))
		return "", fmt.Errorf("ocr: failed to store result: %w", err)
	}

	return rawText, nil
}

// persistRawText updates the document's raw_text column.
// It runs in its own transaction to guarantee consistency.
func (s *Service) persistRawText(ctx context.Context, documentID uint, raw string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Use Select to only update the raw_text field.
		if err := tx.Model(&models.Document{}).
			Where("id = ?", documentID).
			Update("raw_text", raw).Error; err != nil {
			return fmt.Errorf("update raw_text failed: %w", err)
		}
		// Also create an audit log entry.
		audit := models.AuditLog{
			UserID:    repo.SystemUserID, // system user for background jobs
			Action:    "ocr_processed",
			TargetID:  documentID,
			TargetType: "document",
			Detail:    fmt.Sprintf("OCR raw text stored (%d bytes)", len(raw)),
			CreatedAt: time.Now(),
		}
		if err := tx.Create(&audit).Error; err != nil {
			// Non‑critical: log but don't abort the OCR transaction.
			s.logger.Warn("failed to write audit log for OCR", zap.Error(err), zap.Uint("document_id", documentID))
		}
		return nil
	})
}

// Close releases any resources held by the service.
// Currently a no‑op but kept for future extensibility.
func (s *Service) Close() error {
	// Placeholder for potential future cleanup (e.g., temp file removal).
	return nil
}