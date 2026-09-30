package main

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
	"uuid"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
	"golang.org/x/time/rate"
	"github.com/samuel025/url-shortner-go/internal/database"
)

func (app *application) RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.GetHeader("X-Request-ID")
		if reqID == "" {
			reqID = uuid.New().String()
		}
		c.Set("request_id", reqID)
		c.Writer.Header().Set("X-Request-ID", reqID)
		c.Next()
	}
}

func (app *application) StructuredLoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery
		if raw != "" {
			path = path + "?" + raw
		}

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		reqID := c.GetString("request_id")
		clientIP := c.ClientIP()
		method := c.Request.Method
		userAgent := c.Request.UserAgent()

		var userID string
		if u, exists := c.Get("user"); exists {
			if user, ok := u.(*database.User); ok {
				userID = user.ID.String()
			}
		}

		attrs := []slog.Attr{
			slog.String("request_id", reqID),
			slog.String("method", method),
			slog.String("path", path),
			slog.Int("status", status),
			slog.Duration("latency", latency),
			slog.Float64("latency_ms", float64(latency.Microseconds())/1000.0),
			slog.String("client_ip", clientIP),
			slog.String("user_agent", userAgent),
		}

		if userID != "" {
			attrs = append(attrs, slog.String("user_id", userID))
		}

		if len(c.Errors) > 0 {
			attrs = append(attrs, slog.String("errors", c.Errors.String()))
		}

		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		} else if status >= 400 {
			level = slog.LevelWarn
		}

		if app.logger != nil {
			app.logger.LogAttrs(c.Request.Context(), level, "HTTP request", attrs...)
		} else {
			slog.Default().LogAttrs(c.Request.Context(), level, "HTTP request", attrs...)
		}
	}
}

func (app *application) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is required"})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Bearer token is required"})
			c.Abort()
			return
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(app.jwtSecret), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		userId := claims["userId"].(string)

		user, err := app.models.Users.GetById(userId)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized access"})
			c.Abort()
			return
		}
		c.Set("user", user)
		c.Next()
	}
}

type clientEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type ipRateLimiter struct {
	mu      sync.Mutex
	clients map[string]*clientEntry
	limit   rate.Limit
	burst   int
}

func newIPRateLimiter(r rate.Limit, b int) *ipRateLimiter {
	limiter := &ipRateLimiter{
		clients: make(map[string]*clientEntry),
		limit:   r,
		burst:   b,
	}

	go func() {
		for {
			time.Sleep(time.Minute)
			limiter.mu.Lock()
			for ip, client := range limiter.clients {
				if time.Since(client.lastSeen) > 3*time.Minute {
					delete(limiter.clients, ip)
				}
			}
			limiter.mu.Unlock()
		}
	}()

	return limiter
}

func (l *ipRateLimiter) getLimiter(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, exists := l.clients[ip]
	if !exists {
		limiter := rate.NewLimiter(l.limit, l.burst)
		l.clients[ip] = &clientEntry{
			limiter:  limiter,
			lastSeen: time.Now(),
		}
		return limiter
	}

	entry.lastSeen = time.Now()
	return entry.limiter
}

func (app *application) RateLimitMiddleware(r rate.Limit, burst int) gin.HandlerFunc {
	limiter := newIPRateLimiter(r, burst)

	return func(c *gin.Context) {
		ip := c.ClientIP()
		clientLimiter := limiter.getLimiter(ip)

		if !clientLimiter.Allow() {
			rateLimitedTotal.Inc()
			c.Header("Retry-After", "60")
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{
					"code":    "RATE_LIMITED",
					"message": "Too many requests. Please try again later.",
				},
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func (app *application) CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, Origin")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func (app *application) BodyLimitMiddleware(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}