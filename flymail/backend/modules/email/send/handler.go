package send

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// maxAttachmentTotal 单封邮件附件总大小上限（25 MiB），防止内存被超大上传撑爆。
const maxAttachmentTotal = 25 << 20

// RegisterRoutes 注册发送相关路由到给定的路由组。
func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	rg.POST("/send", func(c *gin.Context) {
		req, err := parseSendRequest(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := svc.Send(req); err != nil {
			if err == ErrNoRecipient {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
}

// parseSendRequest 根据 Content-Type 解析发送请求：
//   - multipart/form-data：payload 字段为 JSON，attachments 字段为文件（带附件场景）
//   - 其他（JSON）：整个 body 即 SendRequest（无附件场景，向后兼容）
func parseSendRequest(c *gin.Context) (SendRequest, error) {
	if strings.HasPrefix(c.ContentType(), "multipart/form-data") {
		return parseMultipart(c)
	}
	var req SendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return SendRequest{}, err
	}
	return req, nil
}

// parseMultipart 从 multipart/form-data 中解析 payload(JSON) 与 attachments(文件)。
func parseMultipart(c *gin.Context) (SendRequest, error) {
	form, err := c.MultipartForm()
	if err != nil {
		return SendRequest{}, fmt.Errorf("parse multipart form: %w", err)
	}

	payloads := form.Value["payload"]
	if len(payloads) == 0 {
		return SendRequest{}, fmt.Errorf("missing payload field")
	}
	var req SendRequest
	if err := json.Unmarshal([]byte(payloads[0]), &req); err != nil {
		return SendRequest{}, fmt.Errorf("invalid payload json: %w", err)
	}

	var total int64
	for _, fh := range form.File["attachments"] {
		total += fh.Size
		if total > maxAttachmentTotal {
			return SendRequest{}, fmt.Errorf("attachments exceed %d bytes", maxAttachmentTotal)
		}
		att, err := readAttachment(fh)
		if err != nil {
			return SendRequest{}, err
		}
		req.Attachments = append(req.Attachments, att)
	}
	return req, nil
}

// readAttachment 读取单个上传文件为 Attachment。
func readAttachment(fh *multipart.FileHeader) (Attachment, error) {
	f, err := fh.Open()
	if err != nil {
		return Attachment{}, fmt.Errorf("open attachment %q: %w", fh.Filename, err)
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		return Attachment{}, fmt.Errorf("read attachment %q: %w", fh.Filename, err)
	}

	return Attachment{
		Filename:    fh.Filename,
		ContentType: fh.Header.Get("Content-Type"),
		Content:     content,
	}, nil
}
