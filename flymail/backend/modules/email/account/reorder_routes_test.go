package account_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flymail/modules/email/account"

	"github.com/gin-gonic/gin"
)

func newOrderRouter(t *testing.T) (*gin.Engine, *account.Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc, _, _ := newSvc(t)
	r := gin.New()
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("路由注册 panic（很可能是 PUT /accounts/order 与 /accounts/:id 冲突）: %v", rec)
		}
	}()
	account.RegisterRoutes(r.Group("/api/v1"), svc)
	return r, svc
}

func putJSON(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// PUT /accounts/order 与 PUT /accounts/:id 是同一层的静态段与通配符。
// gin 支持这种兄弟关系，但它历来是这类路由的翻车点：一旦被 :id 吃掉，
// "order" 会被当成账户 ID 去解析，报出来的是「无效的 ID」——
// 一个和排序毫无关系、极难往路由上联想的错误。
func TestReorderRoute_NotShadowedByIDParam(t *testing.T) {
	r, svc := newOrderRouter(t)

	ids := createAccounts(t, svc, "a", "b")
	body, _ := json.Marshal(map[string][]uint{"ids": {ids[1], ids[0]}})

	w := putJSON(t, r, http.MethodPut, "/api/v1/accounts/order", string(body))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT /accounts/order 返回 %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "无效的 ID") {
		t.Fatal("路由被 /:id 吃掉了：\"order\" 被当成账户 ID 解析")
	}

	// 顺序确实落库了，而不只是返回了 200。
	list, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list[0].ID != ids[1] || list[1].ID != ids[0] {
		t.Errorf("顺序未生效：got [%d %d], want [%d %d]", list[0].ID, list[1].ID, ids[1], ids[0])
	}
}

// 列表与库中账户对不上时回 409 而不是 400/500：请求本身没毛病，
// 是客户端手里的列表过时了，前端据此重取并让用户重来。
func TestReorderRoute_MismatchIs409(t *testing.T) {
	r, svc := newOrderRouter(t)
	ids := createAccounts(t, svc, "a", "b")

	body, _ := json.Marshal(map[string][]uint{"ids": {ids[0]}})
	w := putJSON(t, r, http.MethodPut, "/api/v1/accounts/order", string(body))
	if w.Code != http.StatusConflict {
		t.Fatalf("应返回 409，实际 %d: %s", w.Code, w.Body.String())
	}
}

func TestReorderRoute_RejectsMalformedBody(t *testing.T) {
	r, _ := newOrderRouter(t)
	w := putJSON(t, r, http.MethodPut, "/api/v1/accounts/order", `{"ids": "not-an-array"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("应返回 400，实际 %d: %s", w.Code, w.Body.String())
	}
}

func createAccounts(t *testing.T, svc *account.Service, names ...string) []uint {
	t.Helper()
	ids := make([]uint, 0, len(names))
	for i, n := range names {
		resp, err := svc.Create(account.CreateAccountRequest{
			Name: n, Email: fmt.Sprintf("%s@example.com", n), Password: "pw",
			IMAPHost: "imap.example.com", IMAPPort: 993,
			SMTPHost: "smtp.example.com", SMTPPort: 465,
		})
		if err != nil {
			t.Fatalf("Create %s (#%d): %v", n, i, err)
		}
		ids = append(ids, resp.ID)
	}
	return ids
}
