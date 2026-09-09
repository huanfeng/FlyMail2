package send

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
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
			if errors.Is(err, ErrNoRecipient) || errors.Is(err, ErrAliasNotAllowed) {
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
	add := func(fh *multipart.FileHeader, cid string) error {
		total += fh.Size
		if total > maxAttachmentTotal {
			return fmt.Errorf("attachments exceed %d bytes", maxAttachmentTotal)
		}
		att, err := readAttachment(fh)
		if err != nil {
			return err
		}
		att.ContentID = cid
		req.Attachments = append(req.Attachments, att)
		return nil
	}

	// 内联资源：inline 文件字段与 payload 里的 inline_cids 按下标一一对应。
	// 数量对不上说明前端拼错了表单，此时宁可整封拒收也不能错配 cid——
	// 错配的结果是正文引用不到图，收件方看到一堆裂图外加莫名附件。
	inlineFiles := form.File["inline"]
	if len(inlineFiles) != len(req.InlineCIDs) {
		return SendRequest{}, fmt.Errorf("inline files (%d) and inline_cids (%d) count mismatch",
			len(inlineFiles), len(req.InlineCIDs))
	}
	for i, fh := range inlineFiles {
		cid := req.InlineCIDs[i]
		if !ValidContentID(cid) {
			return SendRequest{}, fmt.Errorf("%w: %q", ErrInvalidContentID, cid)
		}
		if err := add(fh, cid); err != nil {
			return SendRequest{}, err
		}
		// 内联图的 Content-Type 决定收件方是否把它当图片渲染，不能只信客户端：
		// 表单上传经常只给 application/octet-stream，那样发出去就是一张裂图。
		last := &req.Attachments[len(req.Attachments)-1]
		last.ContentType = resolveInlineContentType(last.ContentType, last.Filename, last.Content)
	}

	for _, fh := range form.File["attachments"] {
		if err := add(fh, ""); err != nil {
			return SendRequest{}, err
		}
	}
	return req, nil
}

// resolveInlineContentType 定内联资源的 Content-Type。
// 先嗅探再看扩展名：嗅探对 png/jpeg/gif/webp 准确且骗不过去，
// svg 这类文本格式嗅探不出来，才回退到扩展名。
func resolveInlineContentType(declared, filename string, content []byte) string {
	if declared != "" && !strings.EqualFold(declared, "application/octet-stream") {
		return declared
	}
	if sniffed := http.DetectContentType(content); strings.HasPrefix(sniffed, "image/") {
		return sniffed
	}
	if ext := filepath.Ext(filename); ext != "" {
		if byExt := mime.TypeByExtension(ext); byExt != "" {
			return byExt
		}
	}
	return "application/octet-stream"
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
