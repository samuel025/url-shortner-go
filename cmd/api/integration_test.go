package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/samuel025/url-shortner-go/internal/database"
)

func setupTestApp(t *testing.T) (*application, http.Handler) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgresql://test_user:test_password@127.0.0.1:5433/shortener_url?sslmode=disable"
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("PostgreSQL open failed: %v", err)
		return nil, nil
	}
	if err := db.Ping(); err != nil {
		t.Skipf("PostgreSQL ping failed: %v", err)
		return nil, nil
	}

	models := database.NewModels(db)
	app := &application{
		port:      8080,
		jwtSecret: "test-integration-secret-key-12345",
		baseURL:   "http://localhost:8080",
		models:    models,
		db:        db,
	}

	return app, app.routes()
}

func TestFullUserWorkflowIntegration(t *testing.T) {
	_, router := setupTestApp(t)

	uniqueSuffix := time.Now().UnixNano()
	email := fmt.Sprintf("testuser_%d@example.com", uniqueSuffix)
	password := "password123"

	// 1. Register
	regPayload, _ := json.Marshal(map[string]string{
		"name":     "Integration User",
		"email":    email,
		"password": password,
	})
	regReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBuffer(regPayload))
	regReq.Header.Set("Content-Type", "application/json")
	regW := httptest.NewRecorder()
	router.ServeHTTP(regW, regReq)

	if regW.Code != http.StatusCreated {
		t.Fatalf("expected register 201 Created, got %d: %s", regW.Code, regW.Body.String())
	}

	// 2. Login
	loginPayload, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(loginPayload))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	router.ServeHTTP(loginW, loginReq)

	if loginW.Code != http.StatusOK {
		t.Fatalf("expected login 200 OK, got %d: %s", loginW.Code, loginW.Body.String())
	}

	var loginResp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(loginW.Body.Bytes(), &loginResp); err != nil || loginResp.Token == "" {
		t.Fatalf("failed to parse login token: %v", err)
	}

	// 3. Create Short URL
	createPayload, _ := json.Marshal(map[string]any{
		"url":             "https://example.com/integration-test",
		"expires_in_days": 15,
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/urls", bytes.NewBuffer(createPayload))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+loginResp.Token)
	createW := httptest.NewRecorder()
	router.ServeHTTP(createW, createReq)

	if createW.Code != http.StatusCreated {
		t.Fatalf("expected create URL 201 Created, got %d: %s", createW.Code, createW.Body.String())
	}

	var createResp struct {
		ShortCode   string `json:"short_code"`
		OriginalURL string `json:"original_url"`
	}
	if err := json.Unmarshal(createW.Body.Bytes(), &createResp); err != nil || createResp.ShortCode == "" {
		t.Fatalf("failed to parse short code from response: %v", err)
	}

	// 4. Test Redirection (HTTP 302)
	redirReq := httptest.NewRequest(http.MethodGet, "/"+createResp.ShortCode, nil)
	redirW := httptest.NewRecorder()
	router.ServeHTTP(redirW, redirReq)

	if redirW.Code != http.StatusFound {
		t.Fatalf("expected redirect 302 Found, got %d", redirW.Code)
	}
	if loc := redirW.Header().Get("Location"); loc != "https://example.com/integration-test" {
		t.Errorf("expected Location header 'https://example.com/integration-test', got %s", loc)
	}

	// 5. Test Public Metadata (HTTP 200)
	metaReq := httptest.NewRequest(http.MethodGet, "/api/v1/urls/"+createResp.ShortCode, nil)
	metaW := httptest.NewRecorder()
	router.ServeHTTP(metaW, metaReq)

	if metaW.Code != http.StatusOK {
		t.Fatalf("expected metadata 200 OK, got %d", metaW.Code)
	}

	// 6. Test Owner Statistics (click count should be 1)
	statsReq := httptest.NewRequest(http.MethodGet, "/api/v1/urls/"+createResp.ShortCode+"/stats", nil)
	statsReq.Header.Set("Authorization", "Bearer "+loginResp.Token)
	statsW := httptest.NewRecorder()
	router.ServeHTTP(statsW, statsReq)

	if statsW.Code != http.StatusOK {
		t.Fatalf("expected stats 200 OK, got %d: %s", statsW.Code, statsW.Body.String())
	}

	var statsResp struct {
		ClickCount int64 `json:"click_count"`
	}
	if err := json.Unmarshal(statsW.Body.Bytes(), &statsResp); err != nil || statsResp.ClickCount != 1 {
		t.Errorf("expected click count 1, got %d", statsResp.ClickCount)
	}

	// 7. Delete Owned URL
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/urls/"+createResp.ShortCode, nil)
	delReq.Header.Set("Authorization", "Bearer "+loginResp.Token)
	delW := httptest.NewRecorder()
	router.ServeHTTP(delW, delReq)

	if delW.Code != http.StatusOK {
		t.Fatalf("expected delete 200 OK, got %d: %s", delW.Code, delW.Body.String())
	}

	// 8. Verify 404 after deletion
	postDelReq := httptest.NewRequest(http.MethodGet, "/api/v1/urls/"+createResp.ShortCode, nil)
	postDelW := httptest.NewRecorder()
	router.ServeHTTP(postDelW, postDelReq)

	if postDelW.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found after deletion, got %d", postDelW.Code)
	}
}
