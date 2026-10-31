package logger

import (
	"context"
	"os"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type ctxKey string

const requestIDKey ctxKey = "request_id"

// InitLogger configures a global structured logger based on the supplied
// log level (e.g. "debug", "info", "warn", "error"). It is safe to call
// multiple times; the logger will be initialized only once.
func InitLogger(level string) error {
	var initErr error
	once.Do(func() {
		// Determine log level; default to INFO.
		zapLevel := zap.InfoLevel
		if level != "" {
			if err := zapLevel.UnmarshalText([]byte(strings.ToLower(level))); err != nil {
				initErr = err
				return
			}
		}

		// Production‑oriented JSON encoder.
		encoderCfg := zap.NewProductionEncoderConfig()
		encoderCfg.TimeKey = "timestamp"
		encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		encoderCfg.CallerKey = "caller"
		encoderCfg.MessageKey = "msg"
		encoderCfg.StacktraceKey = "stacktrace"

		cfg := zap.Config{
			Level:            zap.NewAtomicLevelAt(zapLevel),
			Development:       false,
			Encoding:         "json",
			EncoderConfig:    encoderCfg,
			OutputPaths:      []string{"stdout"},
			ErrorOutputPaths: []string{"stderr"},
		}

		var baseLogger *zap.Logger
		baseLogger, initErr = cfg.Build(zap.AddCallerSkip(1))
		if initErr != nil {
			return
		}
		sugaredLogger = baseLogger.Sugar()
		// Replace the global zap logger so other packages that use zap.L()
		// get the same configuration.
		zap.ReplaceGlobals(baseLogger)
	})
	return initErr
}

// Logger returns the global sugared logger. If the logger has not been
// initialized yet, it falls back to zap's default production logger.
func Logger() *zap.SugaredLogger {
	if sugaredLogger == nil {
		// best‑effort fallback – should never happen in a correctly
		// bootstrapped application.
		fallback, _ := zap.NewProduction()
		sugaredLogger = fallback.Sugar()
	}
	return sugaredLogger
}

// FromContext extracts the request ID (if present) from the supplied
// context and returns a logger enriched with that field. This enables
// request‑level correlation across all log statements.
func FromContext(ctx context.Context) *zap.SugaredLogger {
	if ctx == nil {
		return Logger()
	}
	if rid := ctx.Value(requestIDKey); rid != nil {
		return Logger().With("request_id", rid)
	}
	return Logger()
}

// WithFields creates a logger that includes the provided key/value pairs.
// The map is flattened into zap's variadic field format.
func WithFields(fields map[string]interface{}) *zap.SugaredLogger {
	if len(fields) == 0 {
		return Logger()
	}
	args := make([]interface{}, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}
	return Logger().With(args...)
}

// SetRequestID returns a new context containing the given request ID.
// Middleware can use this to propagate the ID downstream.
func SetRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// Sync flushes any buffered log entries. It should be called
// from defer in main before the application exits.
func Sync() error {
	if sugaredLogger == nil {
		return nil
	}
	// zap.SugaredLogger.Sync may return an error on some platforms;
	// ignore ErrInvalidArgument which indicates nothing to sync.
	if err := sugaredLogger.Sync(); err != nil && err != zapcore.ErrInvalidArgument {
		return err
	}
	return nil
}

var (
	sugaredLogger *zap.SugaredLogger
	once          sync.Once
)