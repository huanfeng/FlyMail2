package draft

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册草稿相关路由到给定路由组（调用方负责鉴权中间件）。
func RegisterRoutes(rg *gin.RouterGroup, svc *Service, sender Sender) {
	h := &handler{svc: svc, sender: sender}

	// 列出账户草稿
	rg.GET("/accounts/:id/drafts", h.list)

	g := rg.Group("/drafts")
	g.POST("", h.create)
	g.PUT("/:id", h.update)
	g.DELETE("/:id", h.delete)
	g.POST("/:id/send", h.sendDraft)
}

type handler struct {
	svc    *Service
	sender Sender
}

func parseID(c *gin.Context) (uint, bool) {
	id64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return 0, false
	}
	return uint(id64), true
}

func (h *handler) list(c *gin.Context) {
	accountID, ok := parseID(c)
	if !ok {
		return
	}
	list, err := h.svc.List(accountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *handler) create(c *gin.Context) {
	var req DraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := h.svc.Create(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建失败"})
		return
	}
	c.JSON(http.StatusCreated, resp)
}

func (h *handler) update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req DraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := h.svc.Update(id, req)
	if errors.Is(err, ErrDraftNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "草稿不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *handler) delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(id); errors.Is(err, ErrDraftNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "草稿不存在"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) sendDraft(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.svc.SendDraft(id, h.sender); errors.Is(err, ErrDraftNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "草稿不存在"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
