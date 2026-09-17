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

	// 校验 app_base_url：允许留空（表示不带链接），否则必须是带主机名的 http/https 绝对地址。
	// 存进去之前统一去掉结尾斜杠，拼链接时就不用两边都判断。
	if v, ok := body.Settings[KeyAppBaseURL]; ok {
		normalized, err := NormalizeBaseURL(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		body.Settings[KeyAppBaseURL] = normalized
	}

	// 校验 body_sync_mode
	if v, ok := body.Settings[KeyBodySyncMode]; ok && !ValidBodySyncMode(v) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body_sync_mode 必须是 new/recent/all 之一"})
		return
	}
	// 校验 body_sync_recent_days
	if v, ok := body.Settings[KeyBodySyncRecentDays]; ok {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 3650 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "body_sync_recent_days 必须是 1..3650 的整数"})
			return
		}
	}

	if err := h.svc.SetMany(body.Settings); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
