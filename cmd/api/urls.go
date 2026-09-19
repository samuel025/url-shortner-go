package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
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
	for range maxShortCodeRetries {
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

func isValidShortCode(code string) bool {
	if len(code) == 0 || len(code) > 16 {
		return false
	}
	for _, char := range code {
		if !strings.ContainsRune(shortCodeCharset, char) {
			return false
		}
	}
	return true
}

func (app *application) redirectURL(c *gin.Context) {
	code := c.Param("code")
	if !isValidShortCode(code) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "INVALID_CODE",
				"message": "The short code format is invalid.",
			},
		})
		return
	}

	urlRecord, err := app.models.URLs.GetByShortCode(code)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"code":    "NOT_FOUND",
					"message": "Short URL not found",
				},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "INTERNAL_SERVER_ERROR",
				"message": "Failed to look up URL",
			},
		})
		return
	}

	if !urlRecord.IsActive || (urlRecord.ExpiresAt != nil && time.Now().After(*urlRecord.ExpiresAt)) {
		c.JSON(http.StatusGone, gin.H{
			"error": gin.H{
				"code":    "EXPIRED_URL",
				"message": "This short link has expired or has been deactivated",
			},
		})
		return
	}

	if err := app.models.URLs.IncrementClickCount(urlRecord.ID); err != nil {
		log.Printf("Failed to increment click count for short code %s (id: %s): %v", code, urlRecord.ID, err)
	}

	c.Redirect(http.StatusFound, urlRecord.OriginalURL)
}

type URLResponse struct {
	ID          uuid.UUID  `json:"id"`
	ShortCode   string     `json:"short_code"`
	ShortURL    string     `json:"short_url"`
	OriginalURL string     `json:"original_url"`
	ClickCount  int64      `json:"click_count"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
	IsActive    bool       `json:"is_active"`
}

type PaginationResponse struct {
	Page         int `json:"page"`
	Limit        int `json:"limit"`
	TotalRecords int `json:"total_records"`
	TotalPages   int `json:"total_pages"`
}

type ListURLsResponse struct {
	URLs       []URLResponse      `json:"urls"`
	Pagination PaginationResponse `json:"pagination"`
}

func (app *application) listURLs(c *gin.Context) {
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

	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit < 1 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}

	offset := (page - 1) * limit

	records, totalRecords, err := app.models.URLs.GetByUserID(user.ID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "INTERNAL_SERVER_ERROR",
				"message": "Failed to retrieve URLs",
			},
		})
		return
	}

	urls := make([]URLResponse, 0, len(records))
	for _, r := range records {
		urls = append(urls, URLResponse{
			ID:          r.ID,
			ShortCode:   r.ShortCode,
			ShortURL:    app.buildShortURL(c, r.ShortCode),
			OriginalURL: r.OriginalURL,
			ClickCount:  r.ClickCount,
			CreatedAt:   r.CreatedAt,
			ExpiresAt:   r.ExpiresAt,
			IsActive:    r.IsActive,
		})
	}

	totalPages := 0
	if totalRecords > 0 {
		totalPages = (totalRecords + limit - 1) / limit
	}

	c.JSON(http.StatusOK, ListURLsResponse{
		URLs: urls,
		Pagination: PaginationResponse{
			Page:         page,
			Limit:        limit,
			TotalRecords: totalRecords,
			TotalPages:   totalPages,
		},
	})
}