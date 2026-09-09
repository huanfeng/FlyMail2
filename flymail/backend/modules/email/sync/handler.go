package sync

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"flymail/internal/htmlsan"
	"flymail/modules/email/message"
)

// encodeRFC5987 按 RFC 5987 对文件名做 ext-value 百分号编码：保留 attr-char，
// 其余字节一律 %XX（大写）。url.PathEscape 会漏编码 / ; = , 等在 ext-value 中非法的字符，
// 故自行实现，兼容中文等非 ASCII 文件名。
func encodeRFC5987(s string) string {
	const attrChars = "!#$&+-.^_`|~"
	var b strings.Builder
	for _, c := range []byte(s) {
		isAlnum := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
		if isAlnum || strings.IndexByte(attrChars, c) >= 0 {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// RegisterRoutes 挂载同步路由：POST /accounts/:id/sync、GET /accounts/:id/sync/status、GET /accounts/:id/stats。
func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	rg.POST("/accounts/:id/sync", h.trigger)
	rg.GET("/accounts/:id/sync/status", h.status)
	rg.GET("/accounts/:id/stats", h.stats)
	rg.GET("/messages/:id", h.detail)
	rg.POST("/messages/:id/read", h.markRead)
	rg.POST("/messages/:id/flag", h.markFlag)
	rg.POST("/messages/:id/delete", h.deleteMessage)
	rg.POST("/messages/:id/move", h.moveMessage)
	// 批量操作用独立前缀，避免与 /messages/:id 的路由参数冲突。
	rg.POST("/batch/delete", h.batchDelete)
	rg.POST("/batch/move", h.batchMove)
	rg.POST("/batch/read", h.batchRead)
	rg.POST("/batch/flag", h.batchFlag)
	// 服务端搜索兜底：与 message 模块的 GET /search/messages 同前缀，但要走 runner 连接，所以挂在这里
	rg.POST("/search/remote", h.remoteSearch)

	// 会话级操作（M10）：按 thread_id 解析成员后复用批量操作
	rg.POST("/threads/batch/delete", h.threadDelete)
	rg.POST("/threads/batch/move", h.threadMove)
	rg.POST("/threads/batch/read", h.threadRead)
	rg.POST("/threads/batch/flag", h.threadFlag)
}

// remoteSearch 用 IMAP SEARCH 在服务器上找本地没有的命中并补抓入库。
// 同步等待完成（最长 remoteSearchTimeout）再返回汇总，前端随后重跑本地搜索即可看到新命中。
func (h *handler) remoteSearch(c *gin.Context) {
	var body struct {
		Q string `json:"q"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Q) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	res, err := h.svc.RemoteSearch(c.Request.Context(), body.Q)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *handler) threadDelete(c *gin.Context) {
	var body struct {
		ThreadIDs  []string `json:"thread_ids"`
		InFolderID uint     `json:"in_folder_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.ThreadIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := h.svc.ThreadDelete(body.ThreadIDs, body.InFolderID); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) threadMove(c *gin.Context) {
	var body struct {
		ThreadIDs  []string `json:"thread_ids"`
		FolderID   uint     `json:"folder_id"`
		InFolderID uint     `json:"in_folder_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.ThreadIDs) == 0 || body.FolderID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := h.svc.ThreadMove(body.ThreadIDs, body.FolderID, body.InFolderID); err != nil {
		if errors.Is(err, ErrCrossAccountMove) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot move across accounts"})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) threadRead(c *gin.Context) {
	var body struct {
		ThreadIDs []string `json:"thread_ids"`
		Read      bool     `json:"read"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.ThreadIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := h.svc.ThreadSetRead(body.ThreadIDs, body.Read); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) threadFlag(c *gin.Context) {
	var body struct {
		ThreadIDs []string `json:"thread_ids"`
		Flagged   bool     `json:"flagged"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.ThreadIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := h.svc.ThreadSetFlagged(body.ThreadIDs, body.Flagged); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) batchDelete(c *gin.Context) {
	var body struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := h.svc.BatchDelete(body.IDs); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) batchMove(c *gin.Context) {
	var body struct {
		IDs      []uint `json:"ids"`
		FolderID uint   `json:"folder_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.IDs) == 0 || body.FolderID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := h.svc.BatchMove(body.IDs, body.FolderID); err != nil {
		if errors.Is(err, ErrCrossAccountMove) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot move across accounts"})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) batchRead(c *gin.Context) {
	var body struct {
		IDs  []uint `json:"ids"`
		Read bool   `json:"read"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := h.svc.BatchSetRead(body.IDs, body.Read); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) batchFlag(c *gin.Context) {
	var body struct {
		IDs     []uint `json:"ids"`
		Flagged bool   `json:"flagged"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := h.svc.BatchSetFlagged(body.IDs, body.Flagged); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) deleteMessage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message id"})
		return
	}
	if err := h.svc.DeleteMessage(uint(id)); err != nil {
		if errors.Is(err, message.ErrMessageNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
			return
		}
		// 删除需连 IMAP，失败按 502（上游不可用）
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) moveMessage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message id"})
		return
	}
	var body struct {
		FolderID uint `json:"folder_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.FolderID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := h.svc.MoveMessage(uint(id), body.FolderID); err != nil {
		switch {
		case errors.Is(err, message.ErrMessageNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
		case errors.Is(err, ErrCrossAccountMove):
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot move across accounts"})
		default:
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

type handler struct{ svc *Service }

func (h *handler) trigger(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid account id"})
		return
	}
	if err := h.svc.Trigger(uint(id)); err != nil {
		if errors.Is(err, ErrSyncRunning) {
			c.JSON(http.StatusConflict, gin.H{"error": "sync already running"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "started"})
}

func (h *handler) stats(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid account id"})
		return
	}
	stats, err := h.svc.AccountStats(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (h *handler) status(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid account id"})
		return
	}
	st, ok := h.svc.StatusOf(uint(id))
	if !ok {
		c.JSON(http.StatusOK, gin.H{"phase": "none"})
		return
	}
	c.JSON(http.StatusOK, st)
}

func (h *handler) detail(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message id"})
		return
	}
	d, err := h.svc.MessageDetail(uint(id))
	if err != nil {
		if errors.Is(err, message.ErrMessageNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
			return
		}
		// 抓正文需连 IMAP，失败按 502（上游不可用）
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	// 服务端净化：脚本与危险标签一律剥掉；远程资源只在用户要求（?remote=1）或发件人受信任时保留。
	// 前端 iframe 的 CSP / sandbox 是纵深防御，不再承担净化。
	allowRemote := c.Query("remote") == "1" || h.svc.trustedSender(d.FromAddr)
	res := htmlsan.Sanitize(d.HTMLBody, allowRemote)
	d.HTMLBody = res.HTML
	d.RemoteCount = res.RemoteCount
	d.RemoteAllowed = allowRemote
	if h.svc.attachmentToken != nil {
		if tok, err := h.svc.attachmentToken(d.ID); err == nil {
			d.AttachmentToken = tok
		}
	}
	c.JSON(http.StatusOK, d)
}

func (h *handler) markRead(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message id"})
		return
	}
	var body struct {
		Read bool `json:"read"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := h.svc.SetRead(uint(id), body.Read); err != nil {
		if errors.Is(err, message.ErrMessageNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// AttachmentHandler 流式返回附件。鉴权：Authorization: Bearer 头 或 ?access_token= query
// （img/iframe/预览新标签无法设头，故支持 query，见 KI-2）。默认 inline 便于图片/PDF 预览，
// ?dl=1 则强制下载。
// AttachmentHandler 附件端点。verify 接受 access token 或限定该邮件的附件令牌（详情接口签发）。
func AttachmentHandler(svc *Service, verify func(token string, messageID uint) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		mid, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message id"})
			return
		}
		token := c.Query("access_token")
		if token == "" {
			token = strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		}
		if verify(token, uint(mid)) != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		idx, err := strconv.Atoi(c.Param("idx"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid index"})
			return
		}
		res, err := svc.AttachmentContent(uint(mid), idx)
		if err != nil {
			if errors.Is(err, ErrAttachmentNotFound) || errors.Is(err, message.ErrMessageNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "attachment not found"})
				return
			}
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		// 附件与 SPA 同源：邮件自带的 Content-Type 若是 text/html 而又 inline 返回，点开预览就是
		// 在应用 origin 上执行任意脚本（能直接读走 token）。只有白名单类型才允许 inline，
		// 其余一律按二进制流下载；nosniff 让浏览器不去猜类型；CSP sandbox 再把 inline 的内容关进沙箱。
		ctype := res.ContentType
		disp := "inline"
		if c.Query("dl") == "1" || !inlineSafe(ctype) {
			disp = "attachment"
			if !inlineSafe(ctype) {
				ctype = "application/octet-stream"
			}
		}
		fn := res.Filename
		if fn == "" {
			fn = "attachment"
		}
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Content-Security-Policy", "sandbox; default-src 'none'; img-src data:; style-src 'unsafe-inline'")
		// RFC 5987 编码文件名，兼容中文等非 ASCII 及特殊字符。
		c.Header("Content-Disposition", disp+"; filename*=UTF-8''"+encodeRFC5987(fn))
		c.Data(http.StatusOK, ctype, res.Data)
	}
}

// inlineSafe 判断附件类型能否在浏览器里直接打开：图片、PDF、纯文本、音视频；
// text/html / svg / xml 这类能载脚本的一律不算。
func inlineSafe(ctype string) bool {
	mt := strings.ToLower(strings.TrimSpace(strings.SplitN(ctype, ";", 2)[0]))
	switch mt {
	case "application/pdf", "text/plain", "text/csv", "audio/mpeg", "audio/ogg", "audio/wav", "video/mp4", "video/webm":
		return true
	}
	if strings.HasPrefix(mt, "image/") && mt != "image/svg+xml" {
		return true
	}
	return false
}

func (h *handler) markFlag(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message id"})
		return
	}
	var body struct {
		Flagged bool `json:"flagged"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := h.svc.SetFlagged(uint(id), body.Flagged); err != nil {
		if errors.Is(err, message.ErrMessageNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
