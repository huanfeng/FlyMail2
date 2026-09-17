package server

import (
	"net/http"
	"testing"
)

// 前端静态资源的缓存策略。
//
// ── 缘起（2026-09-17） ───────────────────────────────────────────────────────
//
// 用户说新加的设置项在界面上看不到。查下来代码确实在服务端的包里，问题出在缓存：
// 资源来自 embed.FS，嵌入文件的修改时间是零值，http.FileServer 于是既不发
// Last-Modified 也不发 ETag，而我们也没设 Cache-Control——**一个校验器和一条指令
// 都没有**。浏览器只能按启发式缓存，index.html 和它引用的那个 JS 一起被留在
// 磁盘缓存里继续用，服务端换了新版本也看不到，而且连条件请求都发不出去。
//
// 这不是「慢一点」的问题，是每次部署都可能白部署。
func TestStaticCacheHeaders(t *testing.T) {
	cases := []struct {
		path string
		want string
		why  string
	}{
		{
			path: "/assets/index-BpsPOWP3.js",
			want: "public, max-age=31536000, immutable",
			why:  "文件名带内容 hash，内容一变文件名就变，可以长缓存",
		},
		{
			path: "/assets/index-abc123.css",
			want: "public, max-age=31536000, immutable",
			why:  "同上",
		},
		{
			path: "/",
			want: "no-cache",
			why:  "index.html 名字固定，必须每次回源核对，否则用户永远停在旧版本",
		},
		{
			path: "/settings",
			want: "no-cache",
			why:  "SPA 的任意路由都会回退到 index.html",
		},
		{
			path: "/favicon.ico",
			want: "no-cache",
			why:  "名字固定的资源一律回源核对",
		},
	}
	for _, tc := range cases {
		h := http.Header{}
		setStaticCacheHeaders(h, tc.path)
		if got := h.Get("Cache-Control"); got != tc.want {
			t.Errorf("%s：想要 %q，拿到 %q\n  （%s）", tc.path, tc.want, got, tc.why)
		}
	}
}

// ⚠ 带 hash 的资源不能被当成 no-cache，否则每次打开都要重下整个包。
// 反过来 index.html 不能被当成 immutable，那等于永久锁死在旧版本。
// 这条把两个方向都钉住——只钉一边的话，另一边的错误写法照样能过。
func TestStaticCacheDoesNotMixUpTheTwoKinds(t *testing.T) {
	assets := http.Header{}
	setStaticCacheHeaders(assets, "/assets/app-deadbeef.js")
	if assets.Get("Cache-Control") == "no-cache" {
		t.Error("带 hash 的资源被设成了 no-cache，每次打开都要重下整个包")
	}

	index := http.Header{}
	setStaticCacheHeaders(index, "/")
	if cc := index.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("index.html 的缓存策略是 %q，一旦允许长缓存，用户会被永久锁在旧版本", cc)
	}
}
