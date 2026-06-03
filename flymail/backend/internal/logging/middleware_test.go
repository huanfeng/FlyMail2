package logging

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestID_GeneratesAndSetsHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	var seen string
	r.GET("/x", func(c *gin.Context) {
		v, _ := c.Get(RequestIDKey)
		seen, _ = v.(string)
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.ServeHTTP(w, req)

	if seen == "" {
		t.Fatal("context 未注入 request_id")
	}
	if w.Header().Get(RequestIDHeader) != seen {
		t.Fatalf("响应头 X-Request-ID(%q) 与 context(%q) 不一致", w.Header().Get(RequestIDHeader), seen)
	}
}

func TestRequestID_PassthroughIncoming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(RequestIDHeader, "abc123")
	r.ServeHTTP(w, req)

	if got := w.Header().Get(RequestIDHeader); got != "abc123" {
		t.Fatalf("应透传传入的 X-Request-ID，got=%q", got)
	}
}
