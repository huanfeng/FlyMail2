package translate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flymail/internal/ai"

	"github.com/gin-gonic/gin"
)

func newRouter(t *testing.T, svc *Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterRoutes(r.Group("/"), svc)
	return r
}

func do(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	return res
}

// 打开邮件时界面要先问一句"这封翻过没有"，这一问不能花钱。
func TestGetTranslationNeverCallsAI(t *testing.T) {
	chat := &fakeChat{}
	svc := newSvc(t, htmlDetail(), chat)
	r := newRouter(t, svc)

	res := do(t, r, http.MethodGet, "/messages/7/translation?lang=zh", "")
	if res.Code != http.StatusNoContent {
		t.Fatalf("没翻过应当回 204（而不是 404，那会让前端弹「邮件不存在」），得到 %d", res.Code)
	}
	if chat.calls != 0 {
		t.Errorf("只查缓存却调了 %d 次 AI", chat.calls)
	}
}

func TestPostThenGet(t *testing.T) {
	svc := newSvc(t, htmlDetail(), &fakeChat{})
	r := newRouter(t, svc)

	res := do(t, r, http.MethodPost, "/messages/7/translate", `{"lang":"zh"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("翻译失败：%d %s", res.Code, res.Body.String())
	}
	var got response
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Cached {
		t.Error("首次翻译不该报告命中缓存")
	}
	if !strings.Contains(got.HTMLBody, "[译]Hello") {
		t.Errorf("译文不对：%s", got.HTMLBody)
	}

	res = do(t, r, http.MethodGet, "/messages/7/translation?lang=zh", "")
	if res.Code != http.StatusOK {
		t.Fatalf("翻过之后应当查得到：%d", res.Code)
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Cached {
		t.Error("从缓存读出来的要标明 cached")
	}
}

// 译文和原文必须走同一道净化：译文里的远程图同样是追踪像素。
func TestTranslationHTMLIsSanitized(t *testing.T) {
	d := htmlDetail()
	d.HTMLBody = `<p>Hello</p><img src="https://tracker.example.com/open.gif">`
	svc := newSvc(t, d, &fakeChat{})
	r := newRouter(t, svc)

	res := do(t, r, http.MethodPost, "/messages/7/translate", `{"lang":"zh"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("翻译失败：%d %s", res.Code, res.Body.String())
	}
	var got response
	_ = json.Unmarshal(res.Body.Bytes(), &got)
	if strings.Contains(got.HTMLBody, "tracker.example.com") {
		t.Errorf("远程引用没被拦住：%s", got.HTMLBody)
	}
	if got.RemoteCount == 0 {
		t.Error("远程引用个数没数出来，界面上的拦截横幅就不会出现")
	}
	if got.RemoteAllowed {
		t.Error("默认不该放行远程引用")
	}
}

// 用户点「显示图片」之后，译文也要能还原出原图地址。
//
// 这条钉的是"净化在出站时做、缓存里存未净化的原样"这个决定：反过来的话，
// 第一次请求把占位符存进了缓存，此后再点显示图片也换不回来。
func TestRemoteCanBeAllowedAfterCaching(t *testing.T) {
	d := htmlDetail()
	d.HTMLBody = `<p>Hello</p><img src="https://cdn.example.com/logo.png">`
	svc := newSvc(t, d, &fakeChat{})
	r := newRouter(t, svc)

	// 先按默认（拦住）翻一次并落缓存
	if res := do(t, r, http.MethodPost, "/messages/7/translate", `{"lang":"zh"}`); res.Code != http.StatusOK {
		t.Fatalf("翻译失败：%d %s", res.Code, res.Body.String())
	}
	// 再按"用户已同意显示图片"读同一份缓存
	res := do(t, r, http.MethodGet, "/messages/7/translation?lang=zh&remote=1", "")
	if res.Code != http.StatusOK {
		t.Fatalf("读缓存失败：%d", res.Code)
	}
	var got response
	_ = json.Unmarshal(res.Body.Bytes(), &got)
	if !strings.Contains(got.HTMLBody, "cdn.example.com/logo.png") {
		t.Errorf("放行后拿不回原图地址，说明缓存里存的是净化过的版本：%s", got.HTMLBody)
	}
	if !got.RemoteAllowed {
		t.Error("remote=1 时应当报告已放行")
	}

	// ⚠ 反过来再读一次：上一趟的净化不能污染缓存对象
	res = do(t, r, http.MethodGet, "/messages/7/translation?lang=zh", "")
	_ = json.Unmarshal(res.Body.Bytes(), &got)
	if strings.Contains(got.HTMLBody, "cdn.example.com/logo.png") {
		t.Error("默认请求拿到了放行版本，缓存被上一次请求污染了")
	}
}

// 400 与 502 的区别对界面是有意义的：前者"改配置去"，后者"待会再试"。
func TestErrorStatusCodes(t *testing.T) {
	t.Run("未配置", func(t *testing.T) {
		svc := newSvc(t, htmlDetail(), &fakeChat{})
		svc.settings = func() Settings { return Settings{DefaultTarget: "zh"} }
		res := do(t, newRouter(t, svc), http.MethodPost, "/messages/7/translate", `{}`)
		if res.Code != http.StatusBadRequest {
			t.Errorf("应当回 400，得到 %d %s", res.Code, res.Body.String())
		}
	})

	t.Run("密钥错", func(t *testing.T) {
		chat := &fakeChat{err: &ai.APIError{Status: http.StatusUnauthorized, Msg: "bad key"}}
		res := do(t, newRouter(t, newSvc(t, htmlDetail(), chat)), http.MethodPost, "/messages/7/translate", `{}`)
		if res.Code != http.StatusBadRequest {
			t.Errorf("401 重试多少次都一样，应当回 400，得到 %d", res.Code)
		}
	})

	t.Run("上游故障", func(t *testing.T) {
		chat := &fakeChat{raw: "模型没按格式回话"}
		res := do(t, newRouter(t, newSvc(t, htmlDetail(), chat)), http.MethodPost, "/messages/7/translate", `{}`)
		if res.Code != http.StatusBadGateway {
			t.Errorf("上游给不出可用结果应当回 502（可重试），得到 %d", res.Code)
		}
	})

	t.Run("语言代码非法", func(t *testing.T) {
		res := do(t, newRouter(t, newSvc(t, htmlDetail(), &fakeChat{})), http.MethodPost,
			"/messages/7/translate", `{"lang":"zzz"}`)
		if res.Code != http.StatusBadRequest {
			t.Errorf("应当回 400，得到 %d", res.Code)
		}
	})
}

// 请求体可以整个省略：只翻成默认语言时前端不该被迫拼一个 JSON。
func TestPostWithoutBody(t *testing.T) {
	res := do(t, newRouter(t, newSvc(t, htmlDetail(), &fakeChat{})), http.MethodPost, "/messages/7/translate", "")
	if res.Code != http.StatusOK {
		t.Errorf("空请求体应当按默认语言翻译，得到 %d %s", res.Code, res.Body.String())
	}
}

func TestLanguagesEndpoint(t *testing.T) {
	svc := newSvc(t, htmlDetail(), &fakeChat{})
	res := do(t, newRouter(t, svc), http.MethodGet, "/translate/languages", "")
	if res.Code != http.StatusOK {
		t.Fatalf("%d", res.Code)
	}
	var got struct {
		Languages []struct {
			Code, Name, Native string
		} `json:"languages"`
		DefaultTarget string `json:"default_target"`
		Enabled       bool   `json:"enabled"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Languages) == 0 || got.DefaultTarget != "zh" || !got.Enabled {
		t.Errorf("清单接口回的内容不对：%+v", got)
	}
	// 自称名是给下拉框直接用的，缺了就得在前端再编一套 i18n
	for _, l := range got.Languages {
		if l.Native == "" {
			t.Errorf("%s 缺自称名", l.Code)
		}
	}
}

func TestBadMessageID(t *testing.T) {
	r := newRouter(t, newSvc(t, htmlDetail(), &fakeChat{}))
	if res := do(t, r, http.MethodGet, "/messages/abc/translation", ""); res.Code != http.StatusBadRequest {
		t.Errorf("非法 id 应当回 400，得到 %d", res.Code)
	}
}
