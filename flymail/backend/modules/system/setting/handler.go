package setting

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册设置路由到给定分组（调用方负责套用鉴权中间件）。
func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	rg.GET("/settings", h.getAll)
	rg.PUT("/settings", h.setAll)
}

type handler struct{ svc *Service }

func (h *handler) getAll(c *gin.Context) {
	m := h.svc.All()
	// 补充缺省值
	if _, ok := m[KeySyncDepth]; !ok {
		m[KeySyncDepth] = DefaultSyncDepth
	}
	if _, ok := m[KeySyncMaxConcurrent]; !ok {
		m[KeySyncMaxConcurrent] = DefaultSyncMaxConcurrent
	}
	if _, ok := m[KeySyncMaxIdleConns]; !ok {
		m[KeySyncMaxIdleConns] = DefaultSyncMaxIdleConns
	}
	c.JSON(http.StatusOK, gin.H{"settings": m})
}

func (h *handler) setAll(c *gin.Context) {
	var body struct {
		Settings map[string]string `json:"settings"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}

	// 校验 sync_depth
	if v, ok := body.Settings[KeySyncDepth]; ok {
		n, err := strconv.Atoi(v)
		if err != nil || n < 100 || n > 5000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sync_depth 必须是 100..5000 的整数"})
			return
		}
	}
	// 校验 sync_max_concurrent
	if v, ok := body.Settings[KeySyncMaxConcurrent]; ok {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 64 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sync_max_concurrent 必须是 1..64 的整数"})
			return
		}
	}
	// 校验 sync_max_idle_conns
	if v, ok := body.Settings[KeySyncMaxIdleConns]; ok {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 1000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sync_max_idle_conns 必须是 0..1000 的整数"})
			return
		}
	}

	if err := h.svc.SetMany(body.Settings); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
