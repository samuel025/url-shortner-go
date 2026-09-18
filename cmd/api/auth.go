package main

import (
	"errors"
	"net/http"
	"time"
	"uuid"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
	"github.com/samuel025/url-shortner-go/internal/database"
	"golang.org/x/crypto/bcrypt"
)

type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	Name     string `json:"name" binding:"required,min=2"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

type LoginResponse struct {
	Token  string `json:"token"`
	UserId string    `json:"userId"`
}

func (app *application) registerUser(c *gin.Context) {
	var registerRequest RegisterRequest
	if err := c.ShouldBindJSON(&registerRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(registerRequest.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Error": "Something went wrong"})
		return
	}

	registerRequest.Password = string(hashedPassword)
	user := database.User{
		ID:           uuid.New(),
		Name:         registerRequest.Name,
		Email:        registerRequest.Email,
		PasswordHash: registerRequest.Password,
	}
	if err := app.models.Users.Insert(&user); err != nil {
		if errors.Is(err, database.ErrDuplicateEmail) {
			c.JSON(http.StatusConflict, gin.H{"error": "A user with this email already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "User registered successfully"})
}

func (app *application) loginUser(c *gin.Context) {
	var loginRequest LoginRequest
	if err := c.ShouldBindJSON(&loginRequest); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}
	existingUser, err := app.models.Users.GetByEmail(loginRequest.Email)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Something went wrong"})
		return
	}
	err = bcrypt.CompareHashAndPassword([]byte(existingUser.PasswordHash), []byte(loginRequest.Password))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userId": existingUser.ID,
		"exp":   time.Now().Add(time.Hour * 72).Unix(),
	})

	tokenString, err := token.SignedString([]byte(app.jwtSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error generating token"})
		return
	}
	c.JSON(http.StatusOK, LoginResponse{Token: tokenString, UserId: existingUser.ID.String()})
}
