package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"

	"flymail/internal/sse"
	"flymail/modules/auth"
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

// signAccessToken 直接签一个 access token（不经 Service.Login，避免测试依赖数据库）。
func signAccessToken(t *testing.T, secret string) string {
	t.Helper()
	claims := auth.Claims{
		Username: "admin",
		Type:     "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestEventsTicketRequiresBearer：票据签发端点必须挂在 Bearer 中间件后面。
// 它是「把长期凭据换成一次性短票」的唯一入口，一旦裸奔，SSE 就等于不鉴权。
func TestEventsTicketRequiresBearer(t *testing.T) {
	const secret = "router-test-secret"
	store := sse.NewTicketStore(time.Minute)
	h := New(Deps{
		Auth:         auth.NewService(nil, auth.Options{JWTSecret: secret, AccessTTLMin: 5, RefreshTTLHour: 1}),
		EventsTicket: sse.NewTicketHandler(store),
	})

	// 无凭证
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/events/ticket", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("无凭证 = %d, want 401", rec.Code)
	}

	// 伪造凭证
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/events/ticket", nil)
	req2.Header.Set("Authorization", "Bearer forged")
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("伪造凭证 = %d, want 401", rec2.Code)
	}

	// 合法凭证
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodPost, "/api/v1/events/ticket", nil)
	req3.Header.Set("Authorization", "Bearer "+signAccessToken(t, secret))
	h.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("合法凭证 = %d, want 200 (%s)", rec3.Code, rec3.Body.String())
	}
	var body struct {
		Ticket string `json:"ticket"`
	}
	if err := json.Unmarshal(rec3.Body.Bytes(), &body); err != nil || body.Ticket == "" {
		t.Fatalf("响应缺少 ticket: %s (%v)", rec3.Body.String(), err)
	}
	if !store.Consume(body.Ticket) {
		t.Fatal("签发的票据不被接受")
	}
}
