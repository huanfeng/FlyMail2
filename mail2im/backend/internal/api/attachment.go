package api

import (
	"flymail-core/httputil"
	"mail2im/internal/core"
	"os"

	"github.com/gin-gonic/gin"
)

func DownloadAttachment(c *gin.Context) {
	filename := c.Param("filename")
	path := core.Attachments.GetPath(filename)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		httputil.NotFound(c, "Attachment not found", nil)
		return
	}

	c.File(path)
}
