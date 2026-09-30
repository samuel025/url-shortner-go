package main

import (
	"database/sql"
	"log/slog"
	"os"

	_ "github.com/lib/pq"
	"github.com/samuel025/url-shortner-go/internal/database"
	"github.com/samuel025/url-shortner-go/internal/env"
)

type application struct {
	port      int
	jwtSecret string
	baseURL   string
	models    database.Models
	db        *sql.DB
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

	models := database.NewModels(db)
	app := &application{
		port:      env.GetEnvInt("PORT", 8080),
		jwtSecret: env.GetEnvString("JWT_SECRET", "some-secret-123456"),
		baseURL:   env.GetEnvString("BASE_URL", ""),
		models:    models,
		db:        db,
		logger:    logger,
	}

	if err := app.serve(); err != nil {
		logger.Error("Server stopped", "error", err)
		os.Exit(1)
	}
}
