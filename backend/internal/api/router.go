package api

import (
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/swaggo/files"
	"github.com/swaggo/gin-swagger"

	"github.com/yourorg/invoiceflow/internal/auth"
	"github.com/yourorg/invoiceflow/internal/logger"
	"github.com/yourorg/invoiceflow/internal/api/health"
	"github.com/yourorg/invoiceflow/internal/api/invoice"
	"github.com/yourorg/invoiceflow/internal/api/webhook"
)

// NewRouter creates and configures the Gin HTTP router for the API server.
// It registers global middle‑wares, health‑check, authentication routes,
// protected resource routes (invoices, webhooks) and auxiliary endpoints
// such as Swagger UI and pprof.
func NewRouter() *gin.Engine {
	// Gin defaults to Debug mode; switch to Release for production.
	gin.SetMode(gin.ReleaseMode)

	// Create a new router without any default middleware.
	r := gin.New()

	// Global middlewares ----------------------------------------------------
	// Structured request logging.
	r.Use(logger.GinLogger())

	// Recover from panics and return 500.
	r.Use(gin.Recovery())

	// Basic CORS – can be tightened per‑environment.
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * 60 * 60,
	}))

	// -----------------------------------------------------------------------
	// Public endpoints
	// -----------------------------------------------------------------------
	// Health‑check – no auth required.
	r.GET("/api/health", health.Handler)

	// Authentication endpoints (login, token refresh, etc.).
	authGroup := r.Group("/api/auth")
	{
		authGroup.POST("/login", auth.LoginHandler)
		authGroup.POST("/refresh", auth.RefreshHandler)
	}

	// Swagger UI – serves the OpenAPI spec embedded in the binary.
	r.GET("/swagger/*any", ginSwagger.WrapHandler(files.Handler))

	// pprof – useful for debugging in non‑production environments.
	pprof.Register(r)

	// -----------------------------------------------------------------------
	// Protected API – requires a valid JWT.
	// -----------------------------------------------------------------------
	protected := r.Group("/api")
	protected.Use(auth.JWTMiddleware())
	{
		// Invoice related routes.
		invoiceGroup := protected.Group("/invoices")
		{
			invoiceGroup.POST("/", invoice.UploadHandler)               // multipart upload
			invoiceGroup.GET("/", invoice.ListHandler)                // list with pagination / filters
			invoiceGroup.GET("/:id", invoice.GetHandler)              // single invoice details
			invoiceGroup.DELETE("/:id", invoice.DeleteHandler)        // soft‑delete
			invoiceGroup.PATCH("/:id/status", invoice.UpdateStatus)   // manual status override
		}

		// Webhook subscription routes.
		webhookGroup := protected.Group("/webhooks")
		{
			webhookGroup.POST("/", webhook.CreateHandler)            // register a new endpoint
			webhookGroup.GET("/", webhook.ListHandler)                // list registered endpoints
			webhookGroup.DELETE("/:id", webhook.DeleteHandler)       // remove endpoint
			webhookGroup.PATCH("/:id", webhook.UpdateHandler)        // enable/disable or edit URL
		}
	}

	// Fallback for undefined routes – returns JSON 404.
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "endpoint not found",
			"path":    c.Request.URL.Path,
			"message": "The requested resource does not exist.",
		})
	})

	return r
}