package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestRateLimitMiddleware(t *testing.T) {
	app := &application{}
	router := gin.New()

	// Rate limiter: 1 token every 10 seconds, burst of 2
	limiter := app.RateLimitMiddleware(rate.Every(10*time.Second), 2)
	router.GET("/test-rate", limiter, func(c *gin.Context) {
		c.String(http.StatusOK, "success")
	})

	// First 2 requests within burst limit should succeed
	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test-rate", nil)
		req.RemoteAddr = "192.0.2.1:12345"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected request %d to succeed with 200, got %d", i, w.Code)
		}
	}

	// 3rd request should exceed burst and get 429
	req := httptest.NewRequest(http.MethodGet, "/test-rate", nil)
	req.RemoteAddr = "192.0.2.1:12345"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected request 3 to be rate limited with 429, got %d", w.Code)
	}

	if w.Header().Get("Retry-After") != "60" {
		t.Errorf("expected Retry-After header to be 60, got %s", w.Header().Get("Retry-After"))
	}
}

func TestCORSMiddleware(t *testing.T) {
	app := &application{}
	router := gin.New()
	router.Use(app.CORSMiddleware())
	router.GET("/test-cors", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// Test preflight OPTIONS request
	req := httptest.NewRequest(http.MethodOptions, "/test-cors", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected OPTIONS request to return 204 No Content, got %d", w.Code)
	}

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("expected Access-Control-Allow-Origin to be '*', got %s", w.Header().Get("Access-Control-Allow-Origin"))
	}

	if !strings.Contains(w.Header().Get("Access-Control-Allow-Methods"), "POST") {
		t.Errorf("expected Access-Control-Allow-Methods to contain POST, got %s", w.Header().Get("Access-Control-Allow-Methods"))
	}
}

func TestBodyLimitMiddleware(t *testing.T) {
	app := &application{}
	router := gin.New()
	router.Use(app.BodyLimitMiddleware(10)) // 10 bytes limit
	router.POST("/test-body-limit", func(c *gin.Context) {
		var body struct {
			Data string `json:"data"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.String(http.StatusBadRequest, "body too large or malformed")
			return
		}
		c.String(http.StatusOK, "ok")
	})

	// Exceed 10 bytes
	payload := bytes.NewBufferString(`{"data":"this string is way too large for a 10 byte limit"}`)
	req := httptest.NewRequest(http.MethodPost, "/test-body-limit", payload)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected oversized body to fail binding with 400, got %d", w.Code)
	}
}
