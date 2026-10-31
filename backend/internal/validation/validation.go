package validation

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/pkg/errors"

	"backend/internal/repo"
)

var (
	// validator instance shared across the package
	validate *validator.Validate

	// tolerance used for floating‑point comparisons (e.g. totals, taxes)
	floatTolerance = 0.01
)

// init registers all custom validators and struct‑level checks.
func init() {
	validate = validator.New()

	// Register custom field‑level validators
	_ = validate.RegisterValidation("notfuture", notFutureDate)

	// Register struct‑level validators for business rules
	validate.RegisterStructValidation(extractionResultStructLevel, repo.ExtractionResult{})
	validate.RegisterStructValidation(lineItemStructLevel, repo.LineItem{})
}

// notFutureDate validates that a time.Time value is not in the future
// (allowing a small grace period of 24h to accommodate timezone differences).
func notFutureDate(fl validator.FieldLevel) bool {
	t, ok := fl.Field().Interface().(time.Time)
	if !ok {
		return false
	}
	// Allow up to 24h in the future to tolerate clock skew.
	return !t.After(time.Now().Add(24 * time.Hour))
}

// extractionResultStructLevel performs cross‑field validation on ExtractionResult.
func extractionResultStructLevel(sl validator.StructLevel) {
	er, ok := sl.Current().Interface().(repo.ExtractionResult)
	if !ok {
		// Should never happen because we registered the struct type explicitly.
		return
	}

	// 1️⃣ Validate total amount = sum(line items amount) + tax amount (within tolerance)
	var lineItemsSum float64
	var taxSum float64
	for _, li := range er.LineItems {
		lineItemsSum += li.Amount
		taxSum += li.TaxAmount
	}
	expectedTotal := lineItemsSum + taxSum
	if math.Abs(er.TotalAmount-expectedTotal) > floatTolerance {
		sl.ReportError(er.TotalAmount, "TotalAmount", "TotalAmount", "totalmatch", fmt.Sprintf("expected %.2f", expectedTotal))
	}

	// 2️⃣ Validate tax amount matches sum of line‑item taxes
	if math.Abs(er.TaxAmount-taxSum) > floatTolerance {
		sl.ReportError(er.TaxAmount, "TaxAmount", "TaxAmount", "taxmatch", fmt.Sprintf("expected %.2f", taxSum))
	}

	// 3️⃣ Invoice date must not be in the future (handled by field validator, but we double‑check for safety)
	if er.InvoiceDate.After(time.Now().Add(24 * time.Hour)) {
		sl.ReportError(er.InvoiceDate, "InvoiceDate", "InvoiceDate", "notfuture", "")
	}
}

// lineItemStructLevel validates that the Amount field of a LineItem equals Quantity * UnitPrice.
func lineItemStructLevel(sl validator.StructLevel) {
	li, ok := sl.Current().Interface().(repo.LineItem)
	if !ok {
		return
	}
	expected := float64(li.Quantity) * li.UnitPrice
	if math.Abs(li.Amount-expected) > floatTolerance {
		sl.ReportError(li.Amount, "Amount", "Amount", "amountmatch", fmt.Sprintf("expected %.2f", expected))
	}
}

// ValidationError aggregates field‑level validation failures.
type ValidationError struct {
	Field string `json:"field"`
	Tag   string `json:"tag"`
	Value any    `json:"value,omitempty"`
	Msg   string `json:"msg,omitempty"`
}

// Error implements the error interface.
func (ve ValidationError) Error() string {
	if ve.Msg != "" {
		return fmt.Sprintf("field %s failed %s validation: %s", ve.Field, ve.Tag, ve.Msg)
	}
	return fmt.Sprintf("field %s failed %s validation", ve.Field, ve.Tag)
}

// ValidateExtraction validates an ExtractionResult according to the
// business rules defined above. It returns a slice of ValidationError
// (wrapped in a single error) when any rule is violated.
func ValidateExtraction(er *repo.ExtractionResult) error {
	if er == nil {
		return errors.New("extraction result is nil")
	}

	if err := validate.Struct(er); err != nil {
		if ve, ok := err.(validator.ValidationErrors); ok {
			var errs []ValidationError
			for _, fe := range ve {
				ve := ValidationError{
					Field: fe.Field(),
					Tag:   fe.Tag(),
					Value: fe.Value(),
				}
				// Use custom error messages when provided via ReportError
				if fe.Param() != "" {
					ve.Msg = fe.Param()
				}
				errs = append(errs, ve)
			}
			// Combine into a single error for callers.
			var sb strings.Builder
			for i, e := range errs {
				if i > 0 {
					sb.WriteString("; ")
				}
				sb.WriteString(e.Error())
			}
			return errors.New(sb.String())
		}
		// Non‑validation error (should be rare)
		return err
	}
	return nil
}