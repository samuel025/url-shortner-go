package main

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/time/rate"
)

func (app *application) routes() http.Handler {
	g := gin.New()

	// Global Middleware Pipeline
	g.Use(gin.Recovery())
	g.Use(app.RequestIDMiddleware())
	g.Use(app.StructuredLoggerMiddleware())
	g.Use(app.MetricsMiddleware())
	g.Use(app.CORSMiddleware())
	g.Use(app.BodyLimitMiddleware(1 << 20)) // 1 MB limit

	// Rate limiters
	authLimiter := app.RateLimitMiddleware(rate.Every(6*time.Second), 5)        // 10 req/min, burst 5
	urlCreateLimiter := app.RateLimitMiddleware(rate.Every(2*time.Second), 10) // 30 req/min, burst 10

	// Prometheus Metrics Endpoint
	g.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Probes & Health Checks
	g.GET("/health", app.health)
	g.GET("/healthz", app.healthz)
	g.GET("/readyz", app.readyz)

	v1 := g.Group("api/v1")
	{
		v1.GET("/health", app.health)
		v1.POST("/auth/register", authLimiter, app.registerUser)
		v1.POST("/auth/login", authLimiter, app.loginUser)

		v1.GET("/urls/:code", app.getURLMetadata)
	}

	authGroup := v1.Group("/")
	authGroup.Use(app.AuthMiddleware())
	{
		authGroup.POST("/urls", urlCreateLimiter, app.generateURL)
		authGroup.GET("/urls", app.listURLs)
		authGroup.GET("/urls/:code/stats", app.getURLStats)
		authGroup.DELETE("/urls/:code", app.deleteURL)
	}

	g.GET("/:code", app.redirectURL)

	return g
}
