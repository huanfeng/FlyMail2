package auth_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"flymail/internal/database"
	"flymail/modules/auth"

	"github.com/gin-gonic/gin"
)

func setup(t *testing.T) (*gin.Engine, *auth.Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	})
	svc := auth.NewService(auth.NewRepository(db), auth.Options{JWTSecret: "s", AccessTTLMin: 15, RefreshTTLHour: 168})
	if err := svc.SetAdminPassword("admin", "secret123"); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	auth.RegisterRoutes(r.Group("/api/v1"), svc)
	return r, svc
}

func TestLoginSuccessAndFail(t *testing.T) {
	r, _ := setup(t)

	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "secret123"})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.AccessToken == "" {
		t.Error("access_token 为空")
	}

	bad, _ := json.Marshal(map[string]string{"username": "admin", "password": "wrong"})
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(bad)))
	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("bad login status = %d, want 401", rec2.Code)
	}
}

func TestRefresh(t *testing.T) {
	r, svc := setup(t)

	// 先登录拿 token pair
	pair, err := svc.Login("admin", "secret123")
	if err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]string{"refresh_token": pair.RefreshToken})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		AccessToken string `json:"access_token"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.AccessToken == "" {
		t.Error("refresh 后 access_token 为空")
	}

	// 用错误 token refresh 应返回 401
	bad, _ := json.Marshal(map[string]string{"refresh_token": "invalid"})
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(bad)))
	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("bad refresh status = %d, want 401", rec2.Code)
	}
}

func TestLogout(t *testing.T) {
	r, _ := setup(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("logout status = %d", rec.Code)
	}
}

func TestMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	})
	svc := auth.NewService(auth.NewRepository(db), auth.Options{JWTSecret: "s", AccessTTLMin: 15, RefreshTTLHour: 168})
	if err := svc.SetAdminPassword("admin", "secret123"); err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	protected := r.Group("/api/v1", auth.Middleware(svc))
	protected.GET("/me", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"username": c.GetString(auth.ContextUsernameKey)})
	})

	// 无 token → 401
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no token status = %d, want 401", rec.Code)
	}

	// 有效 token → 200
	pair, _ := svc.Login("admin", "secret123")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("valid token status = %d body=%s", rec2.Code, rec2.Body.String())
	}
	var body struct {
		Username string `json:"username"`
	}
	json.Unmarshal(rec2.Body.Bytes(), &body)
	if body.Username != "admin" {
		t.Errorf("username = %q, want admin", body.Username)
	}
}
