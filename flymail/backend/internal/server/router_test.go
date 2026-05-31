package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthz(t *testing.T) {
	h := New(Deps{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestSPAFallback(t *testing.T) {
	h := New(Deps{})

	// 非 API 的未知路径 → 回退 index.html，应 200
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/some/spa/route", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("SPA 路由 = %d, want 200", rec.Code)
	}

	// 未知 API 路径 → 404 JSON
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/v1/nope", nil))
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("未知 API = %d, want 404", rec2.Code)
	}
}
