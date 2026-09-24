package aiprovider

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 挂载 AI 配置路由（调用方负责套用鉴权中间件）：
//
//	GET    /ai/providers          全部配置（按使用顺序）
//	POST   /ai/providers          新建
//	PUT    /ai/providers/order    按完整 ID 顺序重排
//	PUT    /ai/providers/:id      修改
//	DELETE /ai/providers/:id      删除
//	POST   /ai/providers/:id/test 测试连接（结果记入健康状态）
//	POST   /ai/providers/:id/reset 手动解除冷却
func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	g := rg.Group("/ai/providers")
	g.GET("", h.list)
	g.POST("", h.create)
	g.PUT("/order", h.reorder)
	g.PUT("/:id", h.update)
	g.DELETE("/:id", h.remove)
	g.POST("/:id/test", h.test)
	g.POST("/:id/reset", h.reset)
}

func (h *handler) test(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.svc.Test(c.Request.Context(), id)
	if errors.Is(err, context.Canceled) {
		// 客户端已经走了，回什么都没人看；别落进 writeError 回一句「保存失败」
		c.Abort()
		return
	}
	if err != nil {
		writeError(c, err)
		return
	}
	// 测试失败也回 200：请求本身成功了，失败是测试的「结果」，在 body 里。
	c.JSON(http.StatusOK, res)
}

func (h *handler) reset(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	v, err := h.svc.Reset(id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}

type handler struct{ svc *Service }

func (h *handler) list(c *gin.Context) {
	list, err := h.svc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"providers": list})
}

func (h *handler) create(c *gin.Context) {
	var in Input
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}
	v, err := h.svc.Create(in)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}

func (h *handler) update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in Input
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}
	v, err := h.svc.Update(id, in)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}

func (h *handler) remove(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(id); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) reorder(c *gin.Context) {
	var body struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}
	if err := h.svc.Reorder(body.IDs); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func parseID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider id"})
		return 0, false
	}
	return uint(id), true
}

// writeError 把服务层错误映射成状态码。
func writeError(c *gin.Context, err error) {
	var invalid *InvalidError
	switch {
	case errors.As(err, &invalid):
		// 校验失败的消息是写给人看的中文，界面原样显示。
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, ErrOrderMismatch):
		// 409：请求本身没毛病，是客户端手里的列表过时了（与账户排序一致）。
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, ErrNoEncryptor):
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败"})
	}
}
