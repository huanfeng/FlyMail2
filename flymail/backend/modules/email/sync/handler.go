package sync

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 挂载同步路由：POST /accounts/:id/sync、GET /accounts/:id/sync/status。
func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	rg.POST("/accounts/:id/sync", h.trigger)
	rg.GET("/accounts/:id/sync/status", h.status)
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
