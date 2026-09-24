package translate

import (
	"errors"
	"net/http"
	"strconv"

	"flymail/internal/ai"
	"flymail/internal/htmlsan"
	"flymail/internal/lang"
	"flymail/modules/email/message"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 挂载翻译路由：
//   - GET  /translate/languages                 可选目标语言 + 默认值 + AI 是否已配置
//   - GET  /messages/:id/translation?lang=      只查缓存（没有就是 204，不花钱）
//   - POST /messages/:id/translate              翻译（缓存优先，body.force 可强制重译）
//
// 查与写分成两个接口，是为了让界面能先问一句"这封有没有翻过"而不触发任何
// AI 调用——打开邮件时就要知道该显示"翻译"还是"显示译文"。
func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	rg.GET("/translate/languages", h.languages)
	rg.GET("/messages/:id/translation", h.get)
	rg.POST("/messages/:id/translate", h.create)
}

type handler struct{ svc *Service }

// response 是译文的对外表示：缓存行 + 这次请求下的远程图状态。
type response struct {
	*Translation
	// Cached 为真表示这次没有调用 AI。
	Cached        bool `json:"cached"`
	RemoteCount   int  `json:"remote_count"`
	RemoteAllowed bool `json:"remote_allowed"`
}

func (h *handler) languages(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"languages":      lang.Supported,
		"default_target": h.svc.DefaultTarget(),
		"enabled":        h.svc.Enabled(),
	})
}

func (h *handler) get(c *gin.Context) {
	id, target, ok := h.args(c)
	if !ok {
		return
	}
	t, err := h.svc.Cached(id, target)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if t == nil {
		// 204 而不是 404：这封信存在，只是还没翻过。
		// 404 会让前端的通用错误处理弹一句"邮件不存在"。
		c.Status(http.StatusNoContent)
		return
	}
	c.JSON(http.StatusOK, h.render(c, t, true))
}

func (h *handler) create(c *gin.Context) {
	id, ok := messageID(c)
	if !ok {
		return
	}
	var body struct {
		Lang  string `json:"lang"`
		Force bool   `json:"force"`
	}
	// 请求体可以整个省略（只翻成默认语言时），所以绑定失败不当错误。
	_ = c.ShouldBindJSON(&body)
	if body.Lang == "" {
		body.Lang = c.Query("lang")
	}
	target, err := h.svc.ResolveTarget(body.Lang)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	t, cached, err := h.svc.Translate(c.Request.Context(), id, target, body.Force)
	if err != nil {
		writeTranslateError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.render(c, t, cached))
}

// render 把缓存行组装成响应：HTML 译文在**出站时**才净化。
//
// 净化不能在入库前做：那样存下来的会是"远程图已换成占位符"的版本，
// 用户此后再点「显示图片」也换不回来——一次性的选择被缓存成了永久的。
func (h *handler) render(c *gin.Context, t *Translation, cached bool) response {
	allowRemote := c.Query("remote") == "1" || h.svc.AllowRemoteFor(t.MessageID)
	// 拷一份再改：t 可能是缓存对象，改了它会污染同进程内的后续请求。
	out := *t
	res := htmlsan.Sanitize(out.HTMLBody, allowRemote)
	out.HTMLBody = res.HTML
	return response{
		Translation:   &out,
		Cached:        cached,
		RemoteCount:   res.RemoteCount,
		RemoteAllowed: allowRemote,
	}
}

// args 取邮件 id 与目标语言；不合法时已写好响应并返回 ok=false。
func (h *handler) args(c *gin.Context) (id uint, target string, ok bool) {
	id, ok = messageID(c)
	if !ok {
		return 0, "", false
	}
	target, err := h.svc.ResolveTarget(c.Query("lang"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return 0, "", false
	}
	return id, target, true
}

func messageID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message id"})
		return 0, false
	}
	return uint(id), true
}

// writeTranslateError 把翻译失败翻译成合适的状态码。
//
// 状态码在这里是有意义的：400 意味着"这么重试一万次都一样，去改配置"，
// 502 意味着"上游的问题，稍后可以再试"。前端据此决定是否给重试按钮。
func writeTranslateError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ai.ErrNotConfigured):
		// 「没配」和「配了但全停用」对用户是同一件事：去设置页的 AI 翻译里处理
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有可用的 AI 配置，请到「设置 → AI 翻译」里添加或启用"})
	case errors.Is(err, ErrNoContent):
		c.JSON(http.StatusBadRequest, gin.H{"error": ErrNoContent.Error()})
	case errors.Is(err, message.ErrMessageNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
	default:
		var all *AllFailedError
		if errors.As(err, &all) {
			if all.ConfigOnly() {
				c.JSON(http.StatusBadRequest, gin.H{"error": all.Error()})
			} else {
				c.JSON(http.StatusBadGateway, gin.H{"error": all.Error()})
			}
			return
		}
		var apiErr *ai.APIError
		if errors.As(err, &apiErr) && !apiErr.Retryable() {
			c.JSON(http.StatusBadRequest, gin.H{"error": apiErr.Error()})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
	}
}
