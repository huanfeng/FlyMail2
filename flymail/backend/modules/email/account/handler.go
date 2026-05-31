package account

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册账户管理路由到给定分组（调用方负责套用鉴权中间件）。
func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	g := rg.Group("/accounts")
	g.GET("", h.list)
	g.POST("", h.create)
	g.GET("/:id", h.get)
	g.PUT("/:id", h.update)
	g.DELETE("/:id", h.delete)
	g.POST("/test", h.testConnection)
}

type handler struct{ svc *Service }

func parseID(c *gin.Context) (uint, bool) {
	id64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return 0, false
	}
	return uint(id64), true
}

func (h *handler) list(c *gin.Context) {
	list, err := h.svc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *handler) create(c *gin.Context) {
	var req CreateAccountRequest
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

func (h *handler) get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	resp, err := h.svc.Get(id)
	if errors.Is(err, ErrAccountNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "账户不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *handler) update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req UpdateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := h.svc.Update(id, req)
	if errors.Is(err, ErrAccountNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "账户不存在"})
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
	if err := h.svc.Delete(id); errors.Is(err, ErrAccountNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "账户不存在"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) testConnection(c *gin.Context) {
	var req TestConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, h.svc.TestConnection(req))
}
