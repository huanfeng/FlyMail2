package notify

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册通知中心路由（调用方负责套鉴权中间件）。
func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	// 站内通知（单条标已读用 body 传 id，避免 /:id 与 /read-all 路由冲突）
	rg.GET("/notifications", h.list)
	rg.POST("/notifications/read", h.markRead)
	rg.POST("/notifications/read-all", h.markAllRead)
	rg.POST("/notifications/clear", h.clear)
	// 外发渠道
	rg.GET("/notify/channels", h.listChannels)
	rg.POST("/notify/channels", h.createChannel)
	rg.PUT("/notify/channels/:id", h.updateChannel)
	rg.DELETE("/notify/channels/:id", h.deleteChannel)
	rg.POST("/notify/channels/:id/test", h.testChannel)
	// 投递日志
	rg.GET("/notify/logs", h.listLogs)
}

type handler struct{ svc *Service }

func (h *handler) list(c *gin.Context) {
	beforeID, _ := strconv.ParseUint(c.DefaultQuery("before_id", "0"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	items, err := h.svc.List(uint(beforeID), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	unread, _ := h.svc.UnreadCount()
	c.JSON(http.StatusOK, gin.H{"notifications": items, "unread_count": unread})
}

func (h *handler) markRead(c *gin.Context) {
	var body struct {
		ID uint `json:"id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.ID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := h.svc.MarkRead(body.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) markAllRead(c *gin.Context) {
	if err := h.svc.MarkAllRead(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) clear(c *gin.Context) {
	if err := h.svc.ClearAll(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) listChannels(c *gin.Context) {
	list, err := h.svc.ListChannels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"channels": list})
}

func (h *handler) createChannel(c *gin.Context) {
	var in ChannelInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	dto, err := h.svc.CreateChannel(in)
	if err != nil {
		if errors.Is(err, ErrInvalidChannel) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "渠道参数无效"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto)
}

func (h *handler) updateChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var in ChannelInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	dto, err := h.svc.UpdateChannel(uint(id), in)
	if err != nil {
		if errors.Is(err, ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "channel not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto)
}

func (h *handler) deleteChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.DeleteChannel(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) testChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.TestChannel(uint(id)); err != nil {
		if errors.Is(err, ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "channel not found"})
			return
		}
		// 投递失败按 502（上游不可用）
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) listLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	list, err := h.svc.ListLogs(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"logs": list})
}
