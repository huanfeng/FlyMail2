package message

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 挂载邮件列表路由：GET /folders/:fid/messages?before_uid=&limit=。
func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	rg.GET("/folders/:fid/messages", h.list)
}

type handler struct{ svc *Service }

func (h *handler) list(c *gin.Context) {
	folderID, err := strconv.ParseUint(c.Param("fid"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid folder id"})
		return
	}
	beforeUID, _ := strconv.ParseUint(c.DefaultQuery("before_uid", "0"), 10, 32)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	items, err := h.svc.List(uint(folderID), uint32(beforeUID), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"messages": items})
}
