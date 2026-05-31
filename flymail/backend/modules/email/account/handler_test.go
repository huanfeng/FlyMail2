package account_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"flymail/modules/email/account"

	"github.com/gin-gonic/gin"
)

func newRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc, _, _ := newSvc(t) // 复用 testhelpers_test.go
	r := gin.New()
	account.RegisterRoutes(r.Group("/api/v1"), svc)
	return r
}

func TestCreateListDelete(t *testing.T) {
	r := newRouter(t)

	body, _ := json.Marshal(map[string]any{
		"name": "Work", "email": "u@example.com", "password": "p@ss",
		"imap_host": "imap.example.com", "imap_port": 993, "imap_security": "ssl",
		"smtp_host": "smtp.example.com", "smtp_port": 465, "smtp_security": "ssl",
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("p@ss")) {
		t.Errorf("响应不应含明文密码: %s", rec.Body.String())
	}
	var created account.AccountResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil))
	if rec2.Code != http.StatusOK {
		t.Fatalf("list status=%d", rec2.Code)
	}

	rec3 := httptest.NewRecorder()
	r.ServeHTTP(rec3, httptest.NewRequest(http.MethodDelete, "/api/v1/accounts/"+strconv.FormatUint(uint64(created.ID), 10), nil))
	if rec3.Code != http.StatusOK {
		t.Fatalf("delete status=%d", rec3.Code)
	}
}
