package middleware

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"backend/internal/logger"
)

// HTTPError is an interface that custom errors can implement to expose an HTTP status code.
type HTTPError interface {
	error
	StatusCode() int
}

// ErrorHandler is a Gin middleware that centralises error handling.
// It recovers from panics, logs them, and converts returned errors into
// JSON payloads with appropriate HTTP status codes.
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Recover from panics and turn them into 500 responses.
		defer func() {
			if r := recover(); r != nil {
				log := logger.Get()
				log.Error("panic recovered",
					zap.Any("panic", r),
					zap.Stack("stack"),
				)

				if !c.Writer.Written() {
					c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
						"error": "internal server error",
					})
				}
			}
		}()

		// Process request.
		c.Next()

		// If the handler added errors to the context, handle the first one.
		if len(c.Errors) > 0 {
			err := c.Errors[0].Err
			status, msg := mapError(err)

			// Log the error with request details.
			log := logger.Get()
			log.Error("request error",
				zap.Error(err),
				zap.Int("status", status),
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path),
			)

			// If the response hasn't been written yet, send a JSON error payload.
			if !c.Writer.Written() {
				c.AbortWithStatusJSON(status, gin.H{
					"error": msg,
				})
			}
		}
	}
}

// mapError converts a Go error into an HTTP status code and a user‑friendly message.
func mapError(err error) (int, string) {
	// Custom errors that implement HTTPError get their own status.
	if httpErr, ok := err.(HTTPError); ok {
		return httpErr.StatusCode(), httpErr.Error()
	}

	// Validation errors from go‑validator are client errors.
	if ve, ok := err.(validator.ValidationErrors); ok {
		return http.StatusBadRequest, ve.Error()
	}

	// GORM's record‑not‑found maps to 404.
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return http.StatusNotFound, "resource not found"
	}

	// Default to internal server error for anything else.
	return http.StatusInternalServerError, "internal server error"
}