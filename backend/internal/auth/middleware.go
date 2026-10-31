package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// context keys used to store authentication data in gin.Context.
// Using a distinct type prevents collisions with other keys.
type contextKey string

const (
	userIDKey   contextKey = "authUserID"
	userRoleKey contextKey = "authUserRole"
)

// AuthMiddleware validates a JWT from the Authorization header,
// extracts the user ID and role, and stores them in the request context.
// If validation fails the request is aborted with HTTP 401.
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Expect header in the form: Authorization: Bearer <token>
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "authorization header missing",
			})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid authorization header format",
			})
			return
		}

		tokenStr := parts[1]

		claims, err := VerifyToken(tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid or expired token",
			})
			return
		}

		// Store the extracted values in the Gin context for downstream handlers.
		c.Set(string(userIDKey), claims.UserID)
		c.Set(string(userRoleKey), claims.Role)

		c.Next()
	}
}

// UserID returns the authenticated user ID from the context.
// The second return value indicates whether the value was present and of the correct type.
func UserID(c *gin.Context) (string, bool) {
	val, exists := c.Get(string(userIDKey))
	if !exists {
		return "", false
	}
	uid, ok := val.(string)
	return uid, ok
}

// UserRole returns the authenticated user's role from the context.
// The second return value indicates whether the value was present and of the correct type.
func UserRole(c *gin.Context) (string, bool) {
	val, exists := c.Get(string(userRoleKey))
	if !exists {
		return "", false
	}
	role, ok := val.(string)
	return role, ok
}

// RequireRole returns a middleware that ensures the authenticated user has one of the allowed roles.
// If the user does not have a permitted role, the request is aborted with HTTP 403.
func RequireRole(allowedRoles ...string) gin.HandlerFunc {
	roleSet := make(map[string]struct{}, len(allowedRoles))
	for _, r := range allowedRoles {
		roleSet[r] = struct{}{}
	}

	return func(c *gin.Context) {
		role, ok := UserRole(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "role information missing",
			})
			return
		}
		if _, allowed := roleSet[role]; !allowed {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "insufficient permissions",
			})
			return
		}
		c.Next()
	}
}