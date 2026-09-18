package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"uuid"

	"github.com/gin-gonic/gin"
	"github.com/samuel025/url-shortner-go/internal/database"
)

const shortCodeCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
const shortCodeLength = 6
const maxShortCodeRetries = 5

type CreateURLRequest struct {
	URL           string `json:"url" binding:"required"`
	ExpiresInDays *int   `json:"expires_in_days"`
}

type CreateURLResponse struct {
	ShortCode   string     `json:"short_code"`
	ShortURL    string     `json:"short_url"`
	OriginalURL string     `json:"original_url"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

func generateShortCode(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	result := make([]byte, length)
	charsetLen := byte(len(shortCodeCharset))
	for i, b := range bytes {
		result[i] = shortCodeCharset[b%charsetLen]
	}
	return string(result), nil
}

func isValidURL(rawURL string) bool {
	if len(rawURL) == 0 || len(rawURL) > 2048 {
		return false
	}

	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return false
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}

	if parsed.Host == "" {
		return false
	}

	return true
}

func (app *application) buildShortURL(c *gin.Context, shortCode string) string {
	baseURL := app.baseURL
	if baseURL == "" {
		scheme := "http"
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		baseURL = fmt.Sprintf("%s://%s", scheme, c.Request.Host)
	}
	return fmt.Sprintf("%s/%s", strings.TrimRight(baseURL, "/"), shortCode)
}

func (app *application) generateURL(c *gin.Context) {
	userVal, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{
				"code":    "UNAUTHORIZED",
				"message": "Authentication required",
			},
		})
		return
	}

	user, ok := userVal.(*database.User)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "INTERNAL_SERVER_ERROR",
				"message": "Failed to resolve authenticated user",
			},
		})
		return
	}

	var req CreateURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": "Request body must contain a valid 'url'",
			},
		})
		return
	}

	req.URL = strings.TrimSpace(req.URL)
	if !isValidURL(req.URL) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "INVALID_URL",
				"message": "The supplied URL is invalid. It must begin with http:// or https:// and be well-formed.",
			},
		})
		return
	}

	var expiresAt *time.Time
	if req.ExpiresInDays != nil {
		if *req.ExpiresInDays <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": gin.H{
					"code":    "INVALID_EXPIRATION",
					"message": "expires_in_days must be greater than 0",
				},
			})
			return
		}
		exp := time.Now().Add(time.Duration(*req.ExpiresInDays) * 24 * time.Hour)
		expiresAt = &exp
	}

	var createdURL *database.URL
	for attempt := 0; attempt < maxShortCodeRetries; attempt++ {
		code, err := generateShortCode(shortCodeLength)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{
					"code":    "INTERNAL_SERVER_ERROR",
					"message": "Failed to generate short code",
				},
			})
			return
		}

		newURL := &database.URL{
			ID:          uuid.New(),
			UserID:      user.ID,
			ShortCode:   code,
			OriginalURL: req.URL,
			ExpiresAt:   expiresAt,
		}

		if err := app.models.URLs.Insert(newURL); err != nil {
			if errors.Is(err, database.ErrDuplicateShortCode) {
				continue
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{
					"code":    "INTERNAL_SERVER_ERROR",
					"message": "Failed to store URL",
				},
			})
			return
		}

		createdURL = newURL
		break
	}

	if createdURL == nil {
		c.JSON(http.StatusConflict, gin.H{
			"error": gin.H{
				"code":    "SHORT_CODE_COLLISION",
				"message": "Unable to generate unique short code after multiple attempts. Please try again.",
			},
		})
		return
	}

	response := CreateURLResponse{
		ShortCode:   createdURL.ShortCode,
		ShortURL:    app.buildShortURL(c, createdURL.ShortCode),
		OriginalURL: createdURL.OriginalURL,
		ExpiresAt:   createdURL.ExpiresAt,
	}

	c.JSON(http.StatusCreated, response)
}