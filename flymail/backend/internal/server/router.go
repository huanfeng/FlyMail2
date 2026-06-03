package server

import (
	"io/fs"
	"net/http"
	"strings"

	"flymail/modules/auth"
	"flymail/modules/email/account"
	"flymail/modules/email/draft"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"
	"flymail/modules/email/send"
	syncmod "flymail/modules/email/sync"
	"flymail/modules/system/monitoring"
	"flymail/modules/system/notify"
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
	Events     http.HandlerFunc
	// VerifyToken 校验 access token（供 SSE/附件等无法走 Bearer 中间件的端点自鉴权）。
	VerifyToken func(token string) error
}

// New 装配 gin 并返回 http.Handler（单一真相源：server 与 desktop 共用）。
func New(deps Deps) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	// 请求日志写入统一日志（gin.DefaultWriter 已在 app 装配时指向轮转文件）；
	// 跳过健康检查与长连接 SSE，避免噪音。
	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/api/v1/healthz", "/api/v1/events"},
	}))
	r.Use(gin.Recovery())
	r.Use(cors.Default())

	api := r.Group("/api/v1")
	api.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// SSE 实时事件流：自带 query-param access_token 鉴权，不走 Bearer 中间件。
	if deps.Events != nil {
		api.GET("/events", gin.WrapF(deps.Events))
	}

	// 附件下载/预览：支持 Bearer 头或 ?access_token= query（img/iframe/预览新标签需要），
	// 故挂在 api 组、不走 Bearer 中间件，由 handler 自鉴权。
	if deps.Sync != nil && deps.VerifyToken != nil {
		api.GET("/messages/:id/attachments/:idx", syncmod.AttachmentHandler(deps.Sync, deps.VerifyToken))
	}

	if deps.Auth != nil {
		auth.RegisterRoutes(api, deps.Auth)
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
	}

	// SPA 静态资源 + history fallback（非 /api 路径回退到 index.html）
	if sub, err := web.DistFS(); err == nil {
		fileServer := http.FileServer(http.FS(sub))
		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
				return
			}
			if _, statErr := fs.Stat(sub, strings.TrimPrefix(c.Request.URL.Path, "/")); statErr == nil && c.Request.URL.Path != "/" {
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
			c.Request.URL.Path = "/"
			fileServer.ServeHTTP(c.Writer, c.Request)
		})
	}

	return r
}
