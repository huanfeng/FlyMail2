package setting_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"flymail/internal/crypto"
	"flymail/modules/system/setting"

	"github.com/gin-gonic/gin"
)

// 地址在**进门时**就归一成可直接请求的完整地址。
//
// 放在这里而不是让服务那侧每次调用前再拼一遍：拼接规则只该有一处实现，
// 否则"用户填的是哪种形状"这件事会在每个调用点各判一遍，迟早判得不一样。
func TestPutSettingsNormalizesAIBaseURL(t *testing.T) {
	r, svc := newSettingRouter(t)

	cases := map[string]string{
		"https://api.openai.com":               "https://api.openai.com/v1/chat/completions",
		"https://api.deepseek.com/v1":          "https://api.deepseek.com/v1/chat/completions",
		"http://127.0.0.1:11434/v1/":           "http://127.0.0.1:11434/v1/chat/completions",
		"https://open.bigmodel.cn/api/paas/v4": "https://open.bigmodel.cn/api/paas/v4/chat/completions",
		"https://x.com/v1/chat/completions":    "https://x.com/v1/chat/completions",
	}
	for in, want := range cases {
		if res := putSettings(t, r, map[string]string{"ai_base_url": in}); res.Code != http.StatusOK {
			t.Fatalf("%q 被拒了：%d %s", in, res.Code, res.Body.String())
		}
		if got := svc.GetString("ai_base_url", ""); got != want {
			t.Errorf("%q 存成了 %q，想要 %q", in, got, want)
		}
	}
}

func TestPutSettingsRejectsBadAIBaseURL(t *testing.T) {
	r, _ := newSettingRouter(t)

	// 少写 scheme 是最常见的手误。不拦住的话，拼出来的地址发不出请求，
	// 而用户看到的会是一个跟"地址填错了"毫无关系的错误。
	for _, bad := range []string{"api.openai.com/v1", "ftp://x/v1"} {
		res := putSettings(t, r, map[string]string{"ai_base_url": bad})
		if res.Code != http.StatusBadRequest {
			t.Errorf("%q 应当被拒，接口回了 %d", bad, res.Code)
			continue
		}
		var body map[string]string
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil || body["error"] == "" {
			t.Errorf("%q 被拒了却没说原因：%s", bad, res.Body.String())
		}
	}
}

// 留空 = 不启用翻译。必须如实存成空串，不能被当成"没填所以别动"。
func TestPutSettingsAcceptsEmptyAIBaseURL(t *testing.T) {
	r, svc := newSettingRouter(t)

	if res := putSettings(t, r, map[string]string{"ai_base_url": ""}); res.Code != http.StatusOK {
		t.Fatalf("留空被拒了：%d %s", res.Code, res.Body.String())
	}
	if got, ok := svc.All()["ai_base_url"]; !ok || got != "" {
		t.Errorf("留空没被如实存下来：值 %q，存在 %v", got, ok)
	}
}

// 模型名与密钥从文档里复制时极易带上空白，而带空白的值只会换来一个
// 与"填错了"无关的 404 / 401。
func TestPutSettingsTrimsModelAndKey(t *testing.T) {
	r, svc := newSettingRouter(t)
	enc, err := crypto.New("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	svc.SetEncryptor(enc)

	if res := putSettings(t, r, map[string]string{
		"ai_model":   "  gpt-4o-mini\n",
		"ai_api_key": "  sk-abc123  ",
	}); res.Code != http.StatusOK {
		t.Fatalf("保存被拒：%d %s", res.Code, res.Body.String())
	}
	if got := svc.GetString("ai_model", ""); got != "gpt-4o-mini" {
		t.Errorf("模型名 = %q，空白没裁掉", got)
	}
	if got := svc.GetSecret("ai_api_key"); got != "sk-abc123" {
		t.Errorf("密钥 = %q，空白没裁掉", got)
	}
}

// 密钥是密文键：GET /settings 只该回报「配没配」。
//
// 这条与 secret_test.go 里对 OAuth secret 的那条是同一个要求，但必须
// 对 ai_api_key 再验一遍——漏把新键加进 secretKeys 名单，是这里唯一
// 会犯的错，而它的后果是一个本该加密的凭据以明文形式出网。
func TestAIKeyNeverEchoed(t *testing.T) {
	r, svc := newSettingRouter(t)
	enc, err := crypto.New("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	svc.SetEncryptor(enc)

	if res := putSettings(t, r, map[string]string{"ai_api_key": "sk-secret-value"}); res.Code != http.StatusOK {
		t.Fatalf("保存被拒：%d %s", res.Code, res.Body.String())
	}

	all := svc.All()
	if _, leaked := all["ai_api_key"]; leaked {
		t.Fatal("密钥出现在 All() 里，等于把加密存储退化成多编码了一层的明文")
	}
	if all["ai_api_key_set"] != "true" {
		t.Errorf("应当回报已配置，得到 %q", all["ai_api_key_set"])
	}
}

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
