package main

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
	"golang.org/x/time/rate"
)

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