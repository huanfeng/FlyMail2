package api

import (
	"fmt"
	"flymail-core/httputil"
	"mail2im/internal/core"
	"time"

	"github.com/gin-gonic/gin"
)

func GetSettings(c *gin.Context) {
	// Get system settings
	timezone := core.GetSystemSettingWithDefault("timezone", "UTC")
	quietEnabled := core.ParseBoolSetting("quiet_enabled", false)
	quietStart := core.GetSystemSettingWithDefault("quiet_start", "")
	quietEnd := core.GetSystemSettingWithDefault("quiet_end", "")
	nightEnabled := core.ParseBoolSetting("night_enabled", false)
	nightStart := core.GetSystemSettingWithDefault("night_start", "")
	nightEnd := core.GetSystemSettingWithDefault("night_end", "")

	response := gin.H{
		"timezone":      timezone,
		"server_time":   core.NowInSystemTZ().Format(time.RFC3339),
		"timezones":     core.ListTimezones(),
		"quiet_enabled": quietEnabled,
		"quiet_start":   quietStart,
		"quiet_end":     quietEnd,
		"night_enabled": nightEnabled,
		"night_start":   nightStart,
		"night_end":     nightEnd,
	}

	httputil.Success(c, response)
}

func UpdateSettings(c *gin.Context) {
	var input map[string]interface{}
	if err := c.ShouldBindJSON(&input); err != nil {
		httputil.BadRequest(c, err.Error(), err)
		return
	}

	// Update system settings
	if v, ok := input["timezone"]; ok {
		core.SetSystemSetting("timezone", v.(string))
	}
	if v, ok := input["quiet_enabled"]; ok {
		core.SetSystemSetting("quiet_enabled", fmt.Sprintf("%v", v))
	}
	if v, ok := input["quiet_start"]; ok {
		core.SetSystemSetting("quiet_start", v.(string))
	}
	if v, ok := input["quiet_end"]; ok {
		core.SetSystemSetting("quiet_end", v.(string))
	}
	if v, ok := input["night_enabled"]; ok {
		core.SetSystemSetting("night_enabled", fmt.Sprintf("%v", v))
	}
	if v, ok := input["night_start"]; ok {
		core.SetSystemSetting("night_start", v.(string))
	}
	if v, ok := input["night_end"]; ok {
		core.SetSystemSetting("night_end", v.(string))
	}

	httputil.NoContent(c, "ok")
}
