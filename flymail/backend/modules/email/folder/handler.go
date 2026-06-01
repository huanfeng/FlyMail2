package folder

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 在受保护组下挂载文件夹路由：GET /accounts/:id/folders。
func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	rg.GET("/accounts/:id/folders", h.list)
}

type handler struct{ svc *Service }

func (h *handler) list(c *gin.Context) {
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid account id"})
		return
	}
	folders, err := h.svc.List(uint(accountID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]FolderResponse, 0, len(folders))
	for i := range folders {
		out = append(out, toResponse(&folders[i]))
	}
	c.JSON(http.StatusOK, gin.H{"folders": out})
}
