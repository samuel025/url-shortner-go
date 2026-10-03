package main

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (app *application) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
	})
}

func (app *application) healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "alive",
	})
}

func (app *application) readyz(c *gin.Context) {
	if app.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unready",
			"error":  "database handle is nil",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if err := app.db.PingContext(ctx); err != nil {
		if app.logger != nil {
			app.logger.Error("Readiness probe database ping failed", "error", err)
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":   "unready",
			"database": "down",
		})
		return
	}

	redisStatus := "disabled"
	if app.cache != nil {
		if err := app.cache.Ping(ctx); err != nil {
			if app.logger != nil {
				app.logger.Error("Readiness probe redis ping failed", "error", err)
			}
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":   "unready",
				"database": "up",
				"redis":    "down",
			})
			return
		}
		redisStatus = "up"
	}

	drmqStatus := "disabled"
	if app.queue != nil {
		drmqStatus = "up"
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "ready",
		"database": "up",
		"redis":    redisStatus,
		"drmq":     drmqStatus,
	})
}
