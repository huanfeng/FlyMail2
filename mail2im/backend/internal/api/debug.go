package api

import (
	"mail2im/internal/core"
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetDebugStats(c *gin.Context) {
	statuses := core.Debug.GetAllStatuses()
	c.JSON(http.StatusOK, gin.H{
		"workers": statuses,
		"total":   len(statuses),
	})
}

func TriggerTestEvent(c *gin.Context) {
	core.Bus.Publish(core.Event{
		Type:     core.EventEmailReceived,
		Priority: core.PriorityNormal,
		Source:   "debug:test",
		Payload:  map[string]interface{}{"message": "Hello from test event"},
	})
	c.JSON(http.StatusOK, gin.H{"message": "Event triggered"})
}
