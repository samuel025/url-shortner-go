package main

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

func (app *application) routes() http.Handler {
	g := gin.Default()

	authLimiter := app.RateLimitMiddleware(rate.Every(6*time.Second), 5)    // 10 req/min, burst 5
	urlCreateLimiter := app.RateLimitMiddleware(rate.Every(2*time.Second), 10) // 30 req/min, burst 10

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

	g.GET("/health", app.health)
	g.GET("/:code", app.redirectURL)

	return g
}
