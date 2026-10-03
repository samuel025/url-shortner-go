package main

import (
	"database/sql"
	"log/slog"
	"os"

	_ "github.com/lib/pq"
	"github.com/samuel025/url-shortner-go/internal/cache"
	"github.com/samuel025/url-shortner-go/internal/database"
	"github.com/samuel025/url-shortner-go/internal/env"
	"github.com/samuel025/url-shortner-go/internal/queue"
)

type application struct {
	port      int
	jwtSecret string
	baseURL   string
	models    database.Models
	db        *sql.DB
	cache     *cache.Client
	queue     *queue.Client
	logger    *slog.Logger
}

func main() {
	var handler slog.Handler
	if env.GetEnvString("ENV", "development") == "production" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)

	db, err := sql.Open("postgres", env.GetEnvString("DATABASE_URL", ""))
	if err != nil {
		logger.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		logger.Error("Database ping failed", "error", err)
		os.Exit(1)
	}

	startDBStatsCollector(db)

	var redisCache *cache.Client
	redisURL := env.GetEnvString("REDIS_URL", "")
	if redisURL != "" {
		c, err := cache.New(redisURL)
		if err != nil {
			logger.Warn("Failed to connect to Redis cache, continuing without cache", "error", err)
		} else {
			redisCache = c
			logger.Info("Connected to Redis cache", "addr", redisURL)
			defer redisCache.Close()
		}
	}

	models := database.NewModels(db)

	var drmqQueue *queue.Client
	drmqServers := env.GetEnvString("DRMQ_BOOTSTRAP_SERVERS", "")
	if drmqServers != "" {
		topic := env.GetEnvString("DRMQ_CLICK_TOPIC", "url-clicks")
		q, err := queue.New(drmqServers, topic, "url-shortener-workers", models, logger)
		if err != nil {
			logger.Warn("Failed to connect to DRMQ broker, continuing without async queue", "error", err)
		} else {
			drmqQueue = q
			logger.Info("Connected to DRMQ broker", "servers", drmqServers, "topic", topic)
			defer drmqQueue.Close()
		}
	}

	app := &application{
		port:      env.GetEnvInt("PORT", 8080),
		jwtSecret: env.GetEnvString("JWT_SECRET", "some-secret-123456"),
		baseURL:   env.GetEnvString("BASE_URL", ""),
		models:    models,
		db:        db,
		cache:     redisCache,
		queue:     drmqQueue,
		logger:    logger,
	}

	if err := app.serve(); err != nil {
		logger.Error("Server stopped", "error", err)
		os.Exit(1)
	}
}
