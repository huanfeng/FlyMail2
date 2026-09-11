package account

import (
	"errors"
	"fmt"
	"html"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RegisterOAuthCallbackRoute 注册免鉴权的授权回调端点。
//
// 必须挂在鉴权中间件之外：这个地址由服务商重定向用户浏览器直接访问，请求里没有 JWT。
// 来源校验由 state 承担（一次性高熵随机值，用后即弃）。
// 仅在配置了固定回调地址时才有意义；loopback 模式下浏览器打的是本机临时端口，不经此处。
func RegisterOAuthCallbackRoute(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	rg.GET(CallbackPath, h.oauthCallback)
}

func (h *handler) oauthCallback(c *gin.Context) {
	q := c.Request.URL.Query()
	err := h.svc.HandleCallback(q.Get("state"), q.Get("code"), q.Get("error"), q.Get("error_description"))

	// 回调 URL 里带着授权码，禁止缓存或经 Referer 外泄。
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	if err != nil {
		writeCallbackPage(c, http.StatusBadRequest, "授权未完成", err.Error())
		return
	}
	writeCallbackPage(c, http.StatusOK, "授权成功", "已完成授权，可以关闭本页并回到 FlyMail。")
}

// writeCallbackPage 输出一个无外部依赖的极简结果页。
//
// message 可能源自服务商回调里的 error_description（外部可控），必须转义。
func writeCallbackPage(c *gin.Context, status int, title, message string) {
	title, message = html.EscapeString(title), html.EscapeString(message)
	c.Data(status, "text/html; charset=utf-8", []byte(fmt.Sprintf(
		`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">`+
			`<meta name="viewport" content="width=device-width,initial-scale=1"><title>%s</title>`+
			`<style>body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;`+
			`font:16px/1.6 system-ui,-apple-system,"Segoe UI",sans-serif;background:#f6f7f9;color:#1f2328}`+
			`.card{background:#fff;padding:40px 48px;border-radius:12px;box-shadow:0 1px 3px rgba(0,0,0,.08);`+
			`text-align:center;max-width:420px}h1{margin:0 0 12px;font-size:20px}p{margin:0;color:#5b6470}</style>`+
			`</head><body><div class="card"><h1>%s</h1><p>%s</p></div></body></html>`,
		title, title, message)))
}

// registerOAuthRoutes 注册 OAuth 授权相关路由（由 RegisterRoutes 调用）。
func registerOAuthRoutes(g *gin.RouterGroup, h *handler) {
	o := g.Group("/oauth")
	o.GET("/providers", h.oauthProviders)
	o.POST("/start", h.oauthStart)
	o.GET("/flows/:flow_id", h.oauthFlowStatus)
	o.DELETE("/flows/:flow_id", h.oauthCancel)
	o.POST("/complete", h.oauthComplete)
}

func (h *handler) oauthProviders(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.OAuthProviders())
}

func (h *handler) oauthStart(c *gin.Context) {
	var req StartOAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	res, err := h.svc.StartOAuth(req)
	if err != nil {
		// 未配置凭据是部署方的事，用 501 与「参数错误」区分开，前端据此提示去配置。
		if errors.Is(err, ErrOAuthNotConfigured) {
			c.JSON(http.StatusNotImplemented, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, ErrAccountNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "账户不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *handler) oauthFlowStatus(c *gin.Context) {
	res, err := h.svc.OAuthFlowStatus(c.Param("flow_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *handler) oauthCancel(c *gin.Context) {
	h.svc.CancelOAuthFlow(c.Param("flow_id"))
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) oauthComplete(c *gin.Context) {
	var req CompleteOAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	resp, err := h.svc.CompleteOAuth(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}
