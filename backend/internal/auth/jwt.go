package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"backend/internal/secret"
)

// Claims defines the custom JWT payload used across the platform.
type Claims struct {
	UserID string `json:"uid"`  // internal user identifier
	Role   string `json:"role"` // admin | accountant | viewer
	jwt.RegisteredClaims
}

// JWTService encapsulates JWT generation and verification logic.
// It loads the HMAC signing key from Google Secret Manager once at start‑up
// and reuses it for all operations.
type JWTService struct {
	signingKey []byte
	issuer     string
	logger     *zap.Logger
}

// NewJWTService creates a new JWTService.
// It fetches the signing key from Secret Manager using the provided secret manager.
// The secret name is hard‑coded to "jwt-signing-key" but can be changed in the future
// by adjusting the call to GetSecret.
func NewJWTService(ctx context.Context, sm *secret.Manager, issuer string, logger *zap.Logger) (*JWTService, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	secretValue, err := sm.GetSecret(ctx, "jwt-signing-key")
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve JWT signing key from Secret Manager: %w", err)
	}
	if secretValue == "" {
		return nil, errors.New("jwt signing key retrieved from Secret Manager is empty")
	}
	return &JWTService{
		signingKey: []byte(secretValue),
		issuer:     issuer,
		logger:     logger,
	}, nil
}

// GenerateToken creates a signed JWT for the given user ID and role.
// ttl defines the token's lifetime (e.g., 24h). The token contains standard
// registered claims (iss, iat, exp) plus the custom uid and role fields.
func (s *JWTService) GenerateToken(userID, role string, ttl time.Duration) (string, error) {
	if userID == "" {
		return "", errors.New("userID cannot be empty")
	}
	if role == "" {
		return "", errors.New("role cannot be empty")
	}
	now := time.Now()
	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.signingKey)
	if err != nil {
		s.logger.Error("failed to sign JWT", zap.Error(err), zap.String("userID", userID), zap.String("role", role))
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}
	return signed, nil
}

// VerifyToken parses and validates the supplied JWT string.
// It returns the custom Claims if verification succeeds.
func (s *JWTService) VerifyToken(tokenString string) (*Claims, error) {
	if tokenString == "" {
		return nil, errors.New("token string is empty")
	}
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		// Ensure the token's signing method is HMAC (HS256).
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.signingKey, nil
	}, jwt.WithLeeway(5*time.Second)) // small leeway for clock skew
	if err != nil {
		s.logger.Debug("JWT parsing error", zap.Error(err))
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}
	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, errors.New("failed to cast token claims")
	}
	// Additional sanity checks (issuer, expiration) are already handled by the library,
	// but we can log them for observability.
	s.logger.Debug("JWT verified", zap.String("userID", claims.UserID), zap.String("role", claims.Role))
	return claims, nil
}

// ExtractUserInfo validates the token and returns the embedded user ID and role.
// It is a convenience wrapper used by middleware and other services.
func (s *JWTService) ExtractUserInfo(tokenString string) (userID string, role string, err error) {
	claims, err := s.VerifyToken(tokenString)
	if err != nil {
		return "", "", err
	}
	return claims.UserID, claims.Role, nil
}