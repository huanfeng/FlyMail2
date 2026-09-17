package setting_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flymail/modules/system/setting"

	"github.com/gin-gonic/gin"
)

// 对外访问地址的校验必须挂在**接口**上，而不只是那个纯函数上。
//
// ── 为什么单测 NormalizeBaseURL 不够 ─────────────────────────────────────────
//
// baseurl_test.go 把这个函数覆盖得很扎实，但把 handler 里调用它的那几行整个删掉，
// 那批测试**照样全绿**。真正要守的行为是「接口会拒绝非法地址」，而不是
// 「有这么一个函数能判断地址合不合法」。
func TestPutSettingsRejectsBadBaseURL(t *testing.T) {
	r, _ := newSettingRouter(t)

	bad := []struct{ in, why string }{
		{"mail.example.com", "没有 scheme：url.Parse 不报错，拼出来是死链"},
		{"ftp://mail.example.com", "不是 http/https"},
		{"http://", "缺少主机名"},
	}
	for _, tc := range bad {
		res := putSettings(t, r, map[string]string{"app_base_url": tc.in})
		if res.Code != http.StatusBadRequest {
			t.Errorf("%q 应当被拒（%s），接口回了 %d", tc.in, tc.why, res.Code)
			continue
		}
		// 错误原因要带回前端：这是个 400，重试一万次都一样，
		// 只说「保存失败请稍后重试」的话用户会一直卡在那儿。
		var body map[string]string
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil || strings.TrimSpace(body["error"]) == "" {
			t.Errorf("%q 被拒了却没给出原因：%s", tc.in, res.Body.String())
		}
	}
}

// 合法地址要存进去，并且存的是**规整后**的值。
func TestPutSettingsNormalizesBaseURL(t *testing.T) {
	r, svc := newSettingRouter(t)

	if res := putSettings(t, r, map[string]string{"app_base_url": "http://192.168.5.11:8086/"}); res.Code != http.StatusOK {
		t.Fatalf("合法地址被拒了：%d %s", res.Code, res.Body.String())
	}
	// 结尾斜杠必须在入库前去掉，否则拼出来是 .../ /?account=1 这种双斜杠
	if got := svc.GetString("app_base_url", ""); got != "http://192.168.5.11:8086" {
		t.Errorf("存进去的是 %q，想要去掉结尾斜杠的形式", got)
	}
}

// 留空是合法的：表示不配对外地址，通知照发只是不带链接。
func TestPutSettingsAcceptsEmptyBaseURL(t *testing.T) {
	r, svc := newSettingRouter(t)

	if res := putSettings(t, r, map[string]string{"app_base_url": ""}); res.Code != http.StatusOK {
		t.Fatalf("留空被拒了：%d %s", res.Code, res.Body.String())
	}
	// ⚠ 用 All() 而不是 GetString：GetString 把空串当成「没配」返回默认值，
	// 这里要验的恰恰是「空串被如实存下来了」。
	if got, ok := svc.All()["app_base_url"]; !ok || got != "" {
		t.Errorf("留空没被如实存下来：值 %q，存在 %v", got, ok)
	}
}

func newSettingRouter(t *testing.T) (*gin.Engine, *setting.Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := newSvc(t)
	r := gin.New()
	setting.RegisterRoutes(r.Group("/"), svc)
	return r, svc
}

func putSettings(t *testing.T, r *gin.Engine, settings map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"settings": settings})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/settings", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	return res
}
