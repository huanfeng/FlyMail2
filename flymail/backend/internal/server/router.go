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
	"flymail/modules/system/setting"
	"flymail/web"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// Deps 路由依赖，后续里程碑在此追加 service。
type Deps struct {
	Auth    *auth.Service
	Account *account.Service
	Folder  *folder.Service
	Message *message.Service
	Sync    *syncmod.Service
	Setting *setting.Service
	Send    *send.Service
	Draft   *draft.Service
}

// New 装配 gin 并返回 http.Handler（单一真相源：server 与 desktop 共用）。
func New(deps Deps) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(cors.Default())

	api := r.Group("/api/v1")
	api.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

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
