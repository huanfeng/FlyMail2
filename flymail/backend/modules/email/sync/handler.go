package sync

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

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
func AttachmentHandler(svc *Service, verify func(token string) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.Query("access_token")
		if token == "" {
			token = strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		}
		if verify(token) != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		mid, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message id"})
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
		disp := "inline"
		if c.Query("dl") == "1" {
			disp = "attachment"
		}
		fn := res.Filename
		if fn == "" {
			fn = "attachment"
		}
		// RFC 5987 编码文件名，兼容中文等非 ASCII 及特殊字符。
		c.Header("Content-Disposition", disp+"; filename*=UTF-8''"+encodeRFC5987(fn))
		c.Data(http.StatusOK, res.ContentType, res.Data)
	}
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
