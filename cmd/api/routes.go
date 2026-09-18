package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (app *application) routes() http.Handler {
	g := gin.Default()

	v1 := g.Group("api/v1")
	{
		v1.GET("/health", app.health)
		v1.POST("/auth/register", app.registerUser)
		v1.POST("/auth/login", app.loginUser)
	}

	authGroup := v1.Group("/")
	authGroup.Use(app.AuthMiddleware())
	{
		authGroup.POST("/urls", app.generateURL)
	}

	return g
}
