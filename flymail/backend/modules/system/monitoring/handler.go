package monitoring

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册监控只读路由（调用方负责套鉴权中间件）。
func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	rg.GET("/monitoring/overview", h.overview)
	rg.GET("/monitoring/accounts", h.accounts)
	rg.GET("/monitoring/accounts/:id/diagnostics", h.diagnostics)
}

type handler struct{ svc *Service }

func (h *handler) overview(c *gin.Context) {
	ov, err := h.svc.Overview()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ov)
}

func (h *handler) accounts(c *gin.Context) {
	list, err := h.svc.Accounts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"accounts": list})
}

func (h *handler) diagnostics(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid account id"})
		return
	}
	diag, ok := h.svc.Diagnostics(uint(id))
	if !ok {
		// 账户停用或 runner 尚未拉起：返回 204 让前端显示"无运行时"。
		c.JSON(http.StatusOK, gin.H{"running": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"running": true, "diagnostics": diag})
}
