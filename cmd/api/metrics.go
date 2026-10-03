package main

import (
	"database/sql"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests processed, partitioned by method, route, and status.",
		},
		[]string{"method", "route", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Histogram of HTTP request latencies in seconds.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"method", "route"},
	)

	httpRequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Current number of in-flight HTTP requests.",
		},
	)

	urlsCreatedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "url_shortener_urls_created_total",
			Help: "Total number of short URLs created.",
		},
	)

	redirectsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "url_shortener_redirects_total",
			Help: "Total number of redirects processed successfully.",
		},
	)

	collisionsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "url_shortener_collisions_total",
			Help: "Total number of short code collisions encountered during generation.",
		},
	)

	rateLimitedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "url_shortener_rate_limited_total",
			Help: "Total number of requests throttled due to rate limits.",
		},
	)

	cacheHitsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "url_shortener_cache_hits_total",
			Help: "Total number of URL cache hits.",
		},
	)

	cacheMissesTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "url_shortener_cache_misses_total",
			Help: "Total number of URL cache misses.",
		},
	)
)

func (app *application) MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		httpRequestsInFlight.Inc()
		start := time.Now()

		c.Next()

		httpRequestsInFlight.Dec()
		duration := time.Since(start).Seconds()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}

		status := strconv.Itoa(c.Writer.Status())

		httpRequestsTotal.WithLabelValues(c.Request.Method, route, status).Inc()
		httpRequestDuration.WithLabelValues(c.Request.Method, route).Observe(duration)
	}
}

func startDBStatsCollector(db *sql.DB) {
	if db == nil {
		return
	}

	dbOpenConns := promauto.NewGauge(prometheus.GaugeOpts{
		Name: "db_connections_open",
		Help: "The number of established connections both in use and idle.",
	})
	dbInUseConns := promauto.NewGauge(prometheus.GaugeOpts{
		Name: "db_connections_in_use",
		Help: "The number of connections currently in use.",
	})
	dbIdleConns := promauto.NewGauge(prometheus.GaugeOpts{
		Name: "db_connections_idle",
		Help: "The number of idle connections.",
	})

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		for range ticker.C {
			stats := db.Stats()
			dbOpenConns.Set(float64(stats.OpenConnections))
			dbInUseConns.Set(float64(stats.InUse))
			dbIdleConns.Set(float64(stats.Idle))
		}
	}()
}
