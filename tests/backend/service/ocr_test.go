package ocr_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"invoiceflow/backend/internal/service/ocr"
)

// blankPNG is a 1×1 white pixel PNG. Tesseract should return an empty string when
// processing this image because it contains no recognizable glyphs.
var blankPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x63, 0x60, 0x00, 0x00, 0x00,
	0x02, 0x00, 0x01, 0xe2, 0x21, 0xbc, 0x33, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}

// TestOCRExtractBlankImage_ReturnsEmptyString validates that the OCR wrapper can
// invoke the Tesseract binary, handle its output correctly, and return an empty
// string (ignoring whitespace) when the source image contains no text.
func TestOCRExtractBlankImage_ReturnsEmptyString(t *testing.T) {
	t.Parallel()

	// Write the fixture PNG to a temporary location.
	imgPath := filepath.Join(t.TempDir(), "blank.png")
	if err := os.WriteFile(imgPath, blankPNG, 0o600); err != nil {
		t.Fatalf("failed to write temporary image fixture: %v", err)
	}

	// Execute the OCR extraction. The function signature is assumed to be:
	//   func ExtractText(ctx context.Context, filePath string) (string, error)
	ctx := context.Background()
	text, err := ocr.ExtractText(ctx, imgPath)
	if err != nil {
		t.Fatalf("OCR extraction returned an unexpected error: %v", err)
	}

	// Tesseract may emit a newline; trim whitespace before asserting emptiness.
	if trimmed := bytes.TrimSpace([]byte(text)); len(trimmed) != 0 {
		t.Fatalf("expected empty OCR result for a blank image, got %q", text)
	}
}