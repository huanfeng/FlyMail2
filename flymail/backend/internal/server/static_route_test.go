package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flymail/web"
)

// 缓存头装在**真实的路由**上是什么效果。
//
// ── 为什么单测 setStaticCacheHeaders 不够 ────────────────────────────────────
//
// static_cache_test.go 直接调那个纯函数，输入是凭空造的 Header。它永远碰不到
// NoRoute handler，于是看不见真正的缺陷——缺陷不在函数里，而在**谁在什么时机
// 调它、调完之后路径又被改成了什么**。
//
// 具体来说：原先的写法先按请求路径设头，再去 stat；stat 失败就把路径改成 "/"
// 回退到 index.html，而头已经设好了不会重来。于是 /assets/ 下任何**不存在**的
// 文件都会拿到「一段 HTML + Cache-Control: immutable」。
//
// ⚠ 这个组合特别毒：immutable 意味着浏览器在有效期内**根本不发条件请求**，
// 所以即使下次部署把文件补回去，那个 URL 在该浏览器里仍然是坏的——服务端
// 没有任何办法远程挽救，只能请用户自己清缓存。
//
// 而它命中的恰好是最需要被救的那批人：浏览器里压着旧 index.html 的用户，
// 部署后会去请求上一版的 hash 文件名，那个文件服务端已经没有了。
func TestAssetsRouteDoesNotServeHTMLAsImmutable(t *testing.T) {
	h := New(Deps{})

	res := doGet(t, h, "/assets/index-OLDHASH.js")
	if res.Code == http.StatusOK {
		t.Errorf("不存在的资源回了 200（内容是 %q…），"+
			"旧页面会把一段 HTML 当 JS 解析", first(res.Body.String(), 40))
	}
	if cc := res.Header().Get("Cache-Control"); strings.Contains(cc, "immutable") {
		t.Errorf("不存在的资源被标成了 immutable（%q）——"+
			"这个 URL 从此在该浏览器里毒化，服务端无法远程修复", cc)
	}
	if ct := res.Header().Get("Content-Type"); strings.Contains(ct, "text/html") {
		t.Errorf("不存在的 .js 回了 HTML（Content-Type=%q）", ct)
	}
}

// 反向：确实存在的带 hash 资源仍然要长缓存，否则每次打开都重下整个包。
func TestExistingAssetKeepsImmutable(t *testing.T) {
	name := anyAsset(t)
	res := doGet(t, New(Deps{}), "/assets/"+name)
	if res.Code != http.StatusOK {
		t.Fatalf("%s 应当能取到，拿到 %d", name, res.Code)
	}
	if cc := res.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("存在的带 hash 资源没有长缓存（%q），每次打开都要重下整个包", cc)
	}
}

// SPA 路由照旧回退到 index.html，并且必须是 no-cache。
//
// 这条守住修复的另一半：/assets/ 之外的未知路径仍然是前端路由，不能被上面那个
// 404 分支误伤。
func TestSPARouteFallsBackToIndexWithNoCache(t *testing.T) {
	h := New(Deps{})
	for _, p := range []string{"/", "/settings", "/mail/inbox"} {
		res := doGet(t, h, p)
		if res.Code != http.StatusOK {
			t.Errorf("%s 应当回退到 index.html，拿到 %d", p, res.Code)
			continue
		}
		if cc := res.Header().Get("Cache-Control"); cc != "no-cache" {
			t.Errorf("%s 的缓存策略是 %q，一旦允许长缓存，用户会被永久锁在旧版本", p, cc)
		}
	}
}

// /api/ 下的未知路径仍归 API，回 JSON 而不是 HTML。
func TestUnknownAPIPathStaysJSON(t *testing.T) {
	res := doGet(t, New(Deps{}), "/api/v1/nope")
	if res.Code != http.StatusNotFound {
		t.Errorf("未知 API 路径应当 404，拿到 %d", res.Code)
	}
	if ct := res.Header().Get("Content-Type"); !strings.Contains(ct, "json") {
		t.Errorf("未知 API 路径回了 %q，前端的错误处理按 JSON 解析会炸", ct)
	}
}

func doGet(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
	return res
}

// anyAsset 从嵌入产物里挑一个真实的 assets 文件名，避免把构建 hash 写死在测试里。
func anyAsset(t *testing.T) string {
	t.Helper()
	sub, err := web.DistFS()
	if err != nil {
		t.Skipf("没有嵌入前端产物：%v", err)
	}
	entries, err := fs.ReadDir(sub, "assets")
	if err != nil || len(entries) == 0 {
		t.Skipf("嵌入产物里没有 assets 目录：%v", err)
	}
	return entries[0].Name()
}

func first(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
