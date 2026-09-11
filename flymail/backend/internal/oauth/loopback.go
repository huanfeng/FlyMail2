package oauth

import (
	"fmt"
	"html"
	"net"
	"net/http"
	"sync"
)

// CallbackPath 是 loopback 回调的固定路径。
const CallbackPath = "/oauth/callback"

// CallbackResult 是浏览器回调带回的结果，Err 非空表示用户拒绝或提供方报错。
type CallbackResult struct {
	Code string
	Err  error
}

// LoopbackServer 在 127.0.0.1 的随机端口上等待一次授权回调。
//
// 为什么用 loopback 而不是固定 redirect_uri：FlyMail 同时以自部署 Web 和 Wails 桌面端
// 运行，桌面端没有稳定的公网回调地址，而要求每个部署方去服务商后台登记自己的域名会把
// 接入门槛拉高一个数量级。loopback 是 RFC 8252 为原生应用给出的标准答案，
// 配合 PKCE 后无需向客户端内嵌任何机密。
//
// 端口取 0 由内核分配：固定端口在多实例或端口被占时会直接失败，而 Google 与 Microsoft
// 对 http://127.0.0.1 的重定向均不校验端口，正是为此场景设计。
type LoopbackServer struct {
	listener net.Listener
	srv      *http.Server
	results  chan CallbackResult
	state    string
	once     sync.Once
}

// StartLoopback 启动回调监听。调用方拿到 RedirectURI() 后用于构造授权地址，
// 并必须在流程结束（成功、失败或超时）时调用 Close 释放端口。
func StartLoopback(state string) (*LoopbackServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("启动本地回调监听失败: %w", err)
	}
	s := &LoopbackServer{
		listener: ln,
		// 缓冲 1：回调处理函数不应因为无人接收而阻塞在 HTTP 请求里。
		results: make(chan CallbackResult, 1),
		state:   state,
	}
	mux := http.NewServeMux()
	mux.HandleFunc(CallbackPath, s.handle)
	s.srv = &http.Server{Handler: mux}
	go func() { _ = s.srv.Serve(ln) }()
	return s, nil
}

// RedirectURI 返回本次流程应登记的重定向地址。
func (s *LoopbackServer) RedirectURI() string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", s.listener.Addr().(*net.TCPAddr).Port, CallbackPath)
}

// Results 返回回调结果通道，最多产出一个元素。
func (s *LoopbackServer) Results() <-chan CallbackResult { return s.results }

// Close 关闭监听端口，可重复调用。
func (s *LoopbackServer) Close() {
	s.once.Do(func() {
		_ = s.srv.Close()
	})
}

func (s *LoopbackServer) handle(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// state 比对必须在读取 code 之前：它是这条回调唯一的来源凭证，
	// 不校验就等于允许任意本机页面向该端口投递伪造的授权码。
	if q.Get("state") != s.state {
		s.deliver(CallbackResult{Err: fmt.Errorf("state 校验失败，可能是伪造的回调")})
		writePage(w, http.StatusBadRequest, "授权失败", "回调校验未通过，请回到 FlyMail 重新发起授权。")
		return
	}
	if errCode := q.Get("error"); errCode != "" {
		desc := q.Get("error_description")
		if desc == "" {
			desc = errCode
		}
		s.deliver(CallbackResult{Err: fmt.Errorf("授权被拒绝: %s", desc)})
		writePage(w, http.StatusOK, "授权未完成", desc)
		return
	}
	code := q.Get("code")
	if code == "" {
		s.deliver(CallbackResult{Err: fmt.Errorf("回调缺少授权码")})
		writePage(w, http.StatusBadRequest, "授权失败", "回调地址中没有授权码。")
		return
	}
	s.deliver(CallbackResult{Code: code})
	writePage(w, http.StatusOK, "授权成功", "已完成授权，可以关闭本页并回到 FlyMail。")
}

// deliver 非阻塞投递结果：重复回调（用户刷新回调页）不应卡住 HTTP 处理。
func (s *LoopbackServer) deliver(res CallbackResult) {
	select {
	case s.results <- res:
	default:
	}
}

// writePage 输出一个无外部依赖的极简结果页。
//
// 刻意不引用任何外部资源：这个页面在用户的默认浏览器里打开，而回调地址是一次性的
// 本地端口，任何外链都会在端口关闭后变成死链或泄露流程发生的时间点。
func writePage(w http.ResponseWriter, status int, title, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// 回调 URL 里带着授权码，禁止被浏览器缓存或经 Referer 外泄。
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(status)
	// message 可能源自提供方回调 URL 里的 error_description（外部可控），必须转义后再拼进 HTML。
	title, message = html.EscapeString(title), html.EscapeString(message)
	fmt.Fprintf(w, `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>%s</title>
<style>body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;
font:16px/1.6 system-ui,-apple-system,"Segoe UI",sans-serif;background:#f6f7f9;color:#1f2328}
.card{background:#fff;padding:40px 48px;border-radius:12px;box-shadow:0 1px 3px rgba(0,0,0,.08);text-align:center;max-width:420px}
h1{margin:0 0 12px;font-size:20px}p{margin:0;color:#5b6470}</style></head>
<body><div class="card"><h1>%s</h1><p>%s</p></div></body></html>`, title, title, message)
}
