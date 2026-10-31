package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"

	"golang.org/x/crypto/subtle"
)

// HeaderName is the HTTP header used to transport the HMAC signature.
const HeaderName = "X-Webhook-Signature"

// ErrMissingSignature is returned when the signature header is absent.
var ErrMissingSignature = errors.New("missing webhook signature header")

// ErrInvalidSignature is returned when the provided signature does not match the payload.
var ErrInvalidSignature = errors.New("invalid webhook signature")

// Sign generates a constant‑time HMAC‑SHA256 signature for the given payload using the supplied secret.
// The returned value is a hex‑encoded string (e.g. "a3f5...").
func Sign(payload []byte, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	// Write never returns an error for the hash implementations.
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify checks that the signature supplied in the HTTP header matches the HMAC of the payload.
// The signatureHeader may be either a raw hex string or prefixed with "sha256=" (common convention).
// It returns nil on success or a descriptive error on failure.
func Verify(payload []byte, signatureHeader string, secret []byte) error {
	if signatureHeader == "" {
		return ErrMissingSignature
	}
	// Allow the "sha256=" prefix (e.g., GitHub style) but be tolerant of whitespace.
	sig := strings.TrimSpace(signatureHeader)
	if strings.HasPrefix(sig, "sha256=") {
		sig = strings.TrimPrefix(sig, "sha256=")
	}

	expected := Sign(payload, secret)

	// Constant‑time comparison to mitigate timing attacks.
	if subtle.ConstantTimeCompare([]byte(sig), []byte(expected)) != 1 {
		return ErrInvalidSignature
	}
	return nil
}

// VerifyRequestMiddleware returns a Gin‑compatible (or standard net/http) middleware that
// validates incoming webhook requests using the provided secret. It reads the request body,
// verifies the signature, and restores the body for downstream handlers.
// If verification fails, it aborts the request with an appropriate HTTP status code.
func VerifyRequestMiddleware(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Read the raw body.
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "unable to read request body", http.StatusBadRequest)
				return
			}
			// Restore the body so downstream handlers can read it.
			r.Body = io.NopCloser(bytes.NewBuffer(body))

			sig := r.Header.Get(HeaderName)
			if err := Verify(body, sig, secret); err != nil {
				if errors.Is(err, ErrMissingSignature) {
					http.Error(w, err.Error(), http.StatusBadRequest)
				} else {
					http.Error(w, err.Error(), http.StatusUnauthorized)
				}
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}