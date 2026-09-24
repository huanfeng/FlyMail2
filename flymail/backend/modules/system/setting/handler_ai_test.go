package setting_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"flymail/modules/system/setting"

	"github.com/gin-gonic/gin"
)

func TestPutSettingsValidatesTargetLang(t *testing.T) {
	r, svc := newSettingRouter(t)

	if res := putSettings(t, r, map[string]string{"translate_target_lang": "ja"}); res.Code != http.StatusOK {
		t.Fatalf("合法语言被拒：%d %s", res.Code, res.Body.String())
	}
	if got := svc.GetString("translate_target_lang", ""); got != "ja" {
		t.Errorf("目标语言 = %q", got)
	}

	// 清单外的代码会一路传进提示词，模型只会自由发挥——必须在进门时拦住
	if res := putSettings(t, r, map[string]string{"translate_target_lang": "zzz"}); res.Code != http.StatusBadRequest {
		t.Errorf("未知语言代码应当被拒，接口回了 %d", res.Code)
	}
}

// 没配过的时候，GET 也要给出默认目标语言，前端下拉框才有初值可选中。
func TestGetSettingsFillsDefaultTargetLang(t *testing.T) {
	r, _ := newSettingRouter(t)

	req := getSettings(t, r)
	if req.Code != http.StatusOK {
		t.Fatalf("GET 失败：%d", req.Code)
	}
	var body struct {
		Settings map[string]string `json:"settings"`
	}
	if err := json.Unmarshal(req.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Settings["translate_target_lang"] != setting.DefaultTranslateTargetLang {
		t.Errorf("默认目标语言 = %q，想要 %q",
			body.Settings["translate_target_lang"], setting.DefaultTranslateTargetLang)
	}
}

func getSettings(t *testing.T, r *gin.Engine) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	return res
}

// 旧版页面（升级后没刷新）还会提交 ai_* 键。必须拒绝：ai_api_key 已不在密文名单里，
// 收下就是明文落库、再从 GET /settings 原样回显。
func TestPutSettingsRejectsRetiredAIKeys(t *testing.T) {
	r, svc := newSettingRouter(t)
	for _, k := range []string{"ai_base_url", "ai_api_key", "ai_model"} {
		res := putSettings(t, r, map[string]string{k: "sk-plaintext"})
		if res.Code != http.StatusBadRequest {
			t.Errorf("%s 应被拒绝，得到 %d", k, res.Code)
		}
		if _, stored := svc.All()[k]; stored {
			t.Errorf("%s 被写进了 settings 表", k)
		}
	}
}
