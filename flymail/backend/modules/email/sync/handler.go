package sync

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"flymail/modules/email/message"
)

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
