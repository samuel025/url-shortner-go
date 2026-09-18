package main

import (
	"database/sql"
	"log"

	_ "github.com/lib/pq"
	"github.com/samuel025/url-shortner-go/internal/database"
	"github.com/samuel025/url-shortner-go/internal/env"
)

type application struct {
	port      int
	jwtSecret string
	baseURL   string
	models    database.Models
}

func main() {
	db, err := sql.Open("postgres", env.GetEnvString("DATABASE_URL", ""))
	if err != nil {
		log.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}

	models := database.NewModels(db)
	app := &application{
		port:      env.GetEnvInt("PORT", 8080),
		jwtSecret: env.GetEnvString("JWT_SECRET", "some-secret-123456"),
		baseURL:   env.GetEnvString("BASE_URL", ""),
		models:    models,
	}
	if err := app.serve(); err != nil {
		log.Fatal(err)
	}
}
