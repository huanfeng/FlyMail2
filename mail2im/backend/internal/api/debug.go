package api

import (
	"flymail-core/httputil"
	"mail2im/internal/core"

	"github.com/gin-gonic/gin"
)

func GetDebugStats(c *gin.Context) {
	statuses := core.Debug.GetAllStatuses()
	httputil.Success(c, gin.H{
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
	httputil.NoContent(c, "event triggered")
}
