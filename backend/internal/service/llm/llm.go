package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"
)

// Config holds configuration required to talk to the LLM FastAPI service.
type Config struct {
	// BaseURL is the root URL of the LLM service (e.g. http://llm-service:8000).
	BaseURL string
	// APIKey is an optional bearer token used by the LLM service for authentication.
	APIKey string
	// Timeout is the HTTP client timeout. If zero, a default of 30s is used.
	Timeout time.Duration
}

// Client is a thin HTTP wrapper around the local LLM FastAPI service.
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	logger     *zap.Logger
	validator  *validator.Validate
}

// LLMClient defines the contract used by the rest of the backend.
type LLMClient interface {
	// Extract sends raw OCR text to the LLM service and returns a validated ExtractionResult.
	Extract(ctx context.Context, rawText string) (*ExtractionResult, error)
}

// NewClient creates a new LLM client based on the supplied configuration.
// It returns an error if mandatory fields are missing.
func NewClient(cfg Config, logger *zap.Logger) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("llm service base URL is required")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Client{
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		baseURL:   cfg.BaseURL,
		apiKey:    cfg.APIKey,
		logger:    logger,
		validator: validator.New(),
	}, nil
}

// extractionRequest is the payload sent to the LLM service.
type extractionRequest struct {
	Text string `json:"text" validate:"required"`
}

// LineItem represents a single line item extracted from an invoice.
type LineItem struct {
	Description string  `json:"description" validate:"required"`
	Quantity    float64 `json:"quantity"    validate:"required,gte=0"`
	UnitPrice   float64 `json:"unit_price"  validate:"required,gte=0"`
	Total       float64 `json:"total"       validate:"required,gte=0"`
}

// ExtractionResult mirrors the JSON structure returned by the LLM service.
type ExtractionResult struct {
	VendorName    string     `json:"vendor_name"    validate:"required"`
	InvoiceNumber string     `json:"invoice_number" validate:"required"`
	InvoiceDate   string     `json:"invoice_date"   validate:"required,datetime=2006-01-02"` // ISO‑8601 date
	Currency      string     `json:"currency"      validate:"required,len=3"`               // ISO‑4217
	TotalAmount   float64    `json:"total_amount"  validate:"required,gte=0"`
	TaxAmount     float64    `json:"tax_amount"    validate:"required,gte=0"`
	LineItems     []LineItem `json:"line_items"    validate:"required,dive"` // each item validated
}

// Extract implements the LLMClient interface.
// It validates the input, calls the remote service, unmarshals the response,
// validates the response schema and performs basic business validation.
func (c *Client) Extract(ctx context.Context, rawText string) (*ExtractionResult, error) {
	if rawText == "" {
		return nil, fmt.Errorf("raw OCR text cannot be empty")
	}

	// Validate request payload.
	reqPayload := extractionRequest{Text: rawText}
	if err := c.validator.Struct(reqPayload); err != nil {
		return nil, fmt.Errorf("invalid request payload: %w", err)
	}

	body, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request payload: %w", err)
	}

	endpoint := fmt.Sprintf("%s/extract", c.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	c.logger.Debug("sending OCR text to LLM service", zap.String("url", endpoint))

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("LLM request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LLM service returned %d: %s", resp.StatusCode, string(b))
	}

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read LLM response body: %w", err)
	}

	var result ExtractionResult
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal LLM response: %w", err)
	}

	// Validate response schema.
	if err := c.validator.Struct(&result); err != nil {
		return nil, fmt.Errorf("LLM response validation failed: %w", err)
	}

	// Business‑level validation (totals vs line items).
	if err := validateTotals(&result); err != nil {
		c.logger.Warn("LLM extraction totals mismatch", zap.Error(err), zap.Any("extraction", result))
		return nil, fmt.Errorf("totals validation failed: %w", err)
	}

	return &result, nil
}

// validateTotals ensures that TotalAmount ≈ sum(LineItems.Total) + TaxAmount within a small tolerance.
func validateTotals(res *ExtractionResult) error {
	const tolerance = 0.01 // 1 cent tolerance

	var sumLineItems float64
	for _, li := range res.LineItems {
		sumLineItems += li.Total
	}
	expectedTotal := sumLineItems + res.TaxAmount
	diff := expectedTotal - res.TotalAmount
	if diff < 0 {
		diff = -diff
	}
	if diff > tolerance {
		return fmt.Errorf(
			"total_amount %.2f does not match sum(line_items)=%.2f + tax=%.2f (diff %.2f)",
			res.TotalAmount, sumLineItems, res.TaxAmount, diff,
		)
	}
	return nil
}

// Ensure *Client satisfies the LLMClient interface at compile time.
var _ LLMClient = (*Client)(nil)