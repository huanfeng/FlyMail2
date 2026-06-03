package monitoring

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册监控只读路由（调用方负责套鉴权中间件）。
func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	rg.GET("/monitoring/overview", h.overview)
	rg.GET("/monitoring/accounts", h.accounts)
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
