package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (app *application) health(c *gin.Context) {
	c.JSON(http.StatusOK, "")
}
