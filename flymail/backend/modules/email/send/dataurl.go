package send

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

// 草稿把内联图存成 data: URI（自包含，不需要服务端暂存区），但直接发出去是坏的：
// Outlook 与 Gmail 都屏蔽 data: 图片——这正是它们防追踪的手段之一。
// 所以发送路径上必须做一次 data: → cid: 的转换，草稿直发与前端漏网的两条路一次覆盖。

// reDataImageDouble / reDataImageSingle 匹配 <img src="data:image/...;base64,...">。
// RE2 没有反向引用，双引号与单引号只能各写一条。
var (
	reDataImageDouble = regexp.MustCompile(`(?i)src\s*=\s*"(data:image/([a-z0-9.+-]+);base64,([A-Za-z0-9+/=\s]*))"`)
	reDataImageSingle = regexp.MustCompile(`(?i)src\s*=\s*'(data:image/([a-z0-9.+-]+);base64,([A-Za-z0-9+/=\s]*))'`)
)

// maxInlineDataURI 单张内联图解码后的大小上限（10 MiB）。
// 超限的整封会在总量校验处被拦下，这里只防单张畸形数据撑爆内存。
const maxInlineDataURI = 10 << 20

// InlineDataURIImages 把 HTML 里的 data: 图片替换为 cid: 引用，并返回对应的内联附件。
// 无可转换内容时原样返回，不分配额外内存。
func InlineDataURIImages(html string) (string, []Attachment, error) {
	if !strings.Contains(html, "data:image/") {
		return html, nil, nil
	}
	var (
		atts    []Attachment
		convErr error
	)
	replace := func(re *regexp.Regexp, in string) string {
		return re.ReplaceAllStringFunc(in, func(m string) string {
			if convErr != nil {
				return m
			}
			groups := re.FindStringSubmatch(m)
			if len(groups) != 4 {
				return m
			}
			subtype, payload := groups[2], groups[3]
			// base64 里允许出现换行（HTML 属性被格式化过），解码前必须先剔除空白
			raw, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(payload), ""))
			if err != nil {
				// 解不开就原样留着：宁可这张图裂，也不要整封发不出去
				return m
			}
			if len(raw) > maxInlineDataURI {
				convErr = fmt.Errorf("inline image exceeds %d bytes", maxInlineDataURI)
				return m
			}
			cid, err := newContentID()
			if err != nil {
				convErr = err
				return m
			}
			atts = append(atts, Attachment{
				Filename:    fmt.Sprintf("image-%s.%s", cid[3:11], normalizeImageExt(subtype)),
				ContentType: "image/" + strings.ToLower(subtype),
				Content:     raw,
				ContentID:   cid,
			})
			return fmt.Sprintf(`src="cid:%s"`, cid)
		})
	}
	out := replace(reDataImageDouble, html)
	out = replace(reDataImageSingle, out)
	if convErr != nil {
		return html, nil, convErr
	}
	return out, atts, nil
}

// newContentID 生成符合 ValidContentID 的随机 cid。
func newContentID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "ii_" + hex.EncodeToString(b), nil
}

// normalizeImageExt 把 MIME 子类型映射为文件扩展名（仅影响附件显示名）。
func normalizeImageExt(subtype string) string {
	switch strings.ToLower(subtype) {
	case "jpeg":
		return "jpg"
	case "svg+xml":
		return "svg"
	default:
		return strings.ToLower(subtype)
	}
}
