package api

import (
	"mail2im/internal/core"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

func DownloadAttachment(c *gin.Context) {
	filename := c.Param("filename")
	path := core.Attachments.GetPath(filename)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Attachment not found"})
		return
	}

	c.File(path)
}
