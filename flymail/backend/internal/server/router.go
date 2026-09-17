package server

import (
	"io/fs"
	"net/http"
	"strings"

	"flymail/internal/logging"
	"flymail/modules/auth"
	"flymail/modules/email/account"
	"flymail/modules/email/draft"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"
	"flymail/modules/email/rule"
	"flymail/modules/email/send"
	syncmod "flymail/modules/email/sync"
	"flymail/modules/system/monitoring"
	"flymail/modules/system/notify"
	"flymail/modules/system/privacy"
	"flymail/modules/system/setting"
	"flymail/web"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// Deps 路由依赖，后续里程碑在此追加 service。
type Deps struct {
	Auth       *auth.Service
	Account    *account.Service
	Folder     *folder.Service
	Message    *message.Service
	Sync       *syncmod.Service
	Setting    *setting.Service
	Send       *send.Service
	Draft      *draft.Service
	Notify     *notify.Service
	Monitoring *monitoring.Service
	Rule       *rule.Service
	Privacy    *privacy.Service
	// LoginLimiter 登录限流（可为 nil）
	LoginLimiter *auth.Limiter
	// TrustedProxies 允许改写客户端 IP 的反向代理；空 = 不信任任何代理（gin 默认信任所有，必须显式收紧）
	TrustedProxies []string
	Events         http.HandlerFunc
	// EventsTicket 签发 SSE 一次性连接票据；挂在 Bearer 中间件之后。
	EventsTicket http.HandlerFunc
	// VerifyAttachment 校验附件端点的令牌；fromQuery 表示凭据取自 URL，此时只接受附件令牌。
	VerifyAttachment func(token string, messageID uint, fromQuery bool) error
}

// New 装配 gin 并返回 http.Handler（单一真相源：server 与 desktop 共用）。
func New(deps Deps) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	// gin 默认 trustedCIDRs = 0.0.0.0/0：任何直连客户端带 X-Forwarded-For 都能让 ClientIP() 返回任意值，
	// 按 IP 的登录限流就形同虚设，还能伪造他人 IP 制造封禁。默认不信任任何代理，挂反代时再按配置放行。
	_ = r.SetTrustedProxies(deps.TrustedProxies)
	// request_id 必须最先注册，使后续访问日志能带上它。
	r.Use(logging.RequestID())
	// 结构化访问日志；跳过健康检查与长连接 SSE，避免噪音。
	r.Use(logging.GinLogger("/api/v1/healthz", "/api/v1/events"))
	r.Use(logging.GinRecovery())
	r.Use(cors.Default())

	api := r.Group("/api/v1")
	api.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// SSE 实时事件流：EventSource 设不了请求头，用 ?ticket= 一次性票据自鉴权，
	// 故不走 Bearer 中间件。票据由下面的签发端点在受保护组里发出。
	if deps.Events != nil {
		api.GET("/events", gin.WrapF(deps.Events))
	}

	// 票据签发：单独一个受保护子组，不与下面那个大组共命运——
	// 少了 Bearer 这道门，SSE 端点就等于完全不鉴权。
	if deps.Auth != nil && deps.EventsTicket != nil {
		ticketGroup := api.Group("")
		ticketGroup.Use(auth.Middleware(deps.Auth))
		ticketGroup.POST("/events/ticket", gin.WrapF(deps.EventsTicket))
	}

	// 附件下载/预览：支持 Bearer 头或 ?ticket=（附件令牌，img/iframe/预览新标签需要），
	// 故挂在 api 组、不走 Bearer 中间件，由 handler 自鉴权。
	if deps.Sync != nil && deps.VerifyAttachment != nil {
		api.GET("/messages/:id/attachments/:idx", syncmod.AttachmentHandler(deps.Sync, deps.VerifyAttachment))
	}

	if deps.Auth != nil {
		auth.RegisterRoutes(api, deps.Auth, deps.LoginLimiter)
	}

	// OAuth 授权回调：由服务商重定向浏览器直接访问，请求里没有 Bearer，
	// 故挂在鉴权中间件之外，来源校验由一次性 state 承担。
	if deps.Account != nil {
		account.RegisterOAuthCallbackRoute(api, deps.Account)
	}

	if deps.Auth != nil && deps.Account != nil {
		protected := api.Group("")
		protected.Use(auth.Middleware(deps.Auth))
		account.RegisterRoutes(protected, deps.Account)
		auth.RegisterProtectedRoutes(protected, deps.Auth)
		if deps.Setting != nil {
			setting.RegisterRoutes(protected, deps.Setting)
		}
		if deps.Folder != nil {
			folder.RegisterRoutes(protected, deps.Folder)
		}
		if deps.Message != nil {
			message.RegisterRoutes(protected, deps.Message)
		}
		if deps.Sync != nil {
			syncmod.RegisterRoutes(protected, deps.Sync)
		}
		if deps.Send != nil {
			send.RegisterRoutes(protected, deps.Send)
		}
		if deps.Draft != nil && deps.Send != nil {
			draft.RegisterRoutes(protected, deps.Draft, deps.Send)
		}
		if deps.Notify != nil {
			notify.RegisterRoutes(protected, deps.Notify)
		}
		if deps.Monitoring != nil {
			monitoring.RegisterRoutes(protected, deps.Monitoring)
		}
		if deps.Rule != nil {
			rule.RegisterRoutes(protected, deps.Rule)
		}
		if deps.Privacy != nil {
			privacy.RegisterRoutes(protected, deps.Privacy)
		}
	}

	// SPA 静态资源 + history fallback（非 /api 路径回退到 index.html）
	if sub, err := web.DistFS(); err == nil {
		fileServer := http.FileServer(http.FS(sub))
		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
				return
			}
			// ⚠ 缓存头必须在**确认命中真实文件之后**、按那个文件的路径设。
			// 先设头再回退的话，/assets/ 下任何**不存在**的文件都会拿到
			// index.html 的内容配上 immutable 的头——见 setStaticCacheHeaders 的说明。
			p := c.Request.URL.Path
			if st, statErr := fs.Stat(sub, strings.TrimPrefix(p, "/")); statErr == nil && !st.IsDir() {
				setStaticCacheHeaders(c.Writer.Header(), p)
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
			// /assets/ 下的是资源不是路由，找不到就该 404。喂它一份 HTML 只会让
			// 调用方拿到一段解析不了的东西，还平白多一次成功状态码。
			if strings.HasPrefix(p, "/assets/") {
				c.Status(http.StatusNotFound)
				return
			}
			c.Writer.Header().Set("Cache-Control", "no-cache")
			c.Request.URL.Path = "/"
			fileServer.ServeHTTP(c.Writer, c.Request)
		})
	}

	return r
}

// setStaticCacheHeaders 给前端静态资源装缓存头。
//
// ⚠ 不装的后果不是「慢一点」，而是**部署了新版本但用户看不到**。
//
// 资源来自 embed.FS，嵌入文件的修改时间是零值，于是 http.FileServer 既不发
// Last-Modified 也不发 ETag——一个校验器都没有。没有任何缓存指令时浏览器按
// 启发式缓存处理，index.html 与它引用的那个带 hash 的 JS 会一起留在磁盘缓存里
// 被继续复用；哪怕服务端早就换了新版本，用户刷新看到的还是旧界面，而且因为
// 没有校验器，浏览器连一次条件请求都发不出去。
// 2026-09-17 就是这么被发现的：新加的设置项确实在服务端的包里，用户却看不到。
//
// 两类资源的策略正好相反：
//
//	/assets/*   文件名带内容 hash（Vite 产出），内容一变文件名就变，
//	            可以放心长缓存 + immutable，连条件请求都省了。
//	其余        index.html 与 favicon 这些名字固定的，必须每次回源核对，
//	            否则就是上面那个「看不到新版本」的坑。
func setStaticCacheHeaders(h http.Header, urlPath string) {
	if strings.HasPrefix(urlPath, "/assets/") {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	// no-cache 不是「不缓存」，是「每次用之前都要回源核对」。
	h.Set("Cache-Control", "no-cache")
}
