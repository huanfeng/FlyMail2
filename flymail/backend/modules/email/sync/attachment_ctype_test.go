package sync

import "testing"

// TestInlineSafe：能载脚本的类型一律不允许 inline（text/html 附件在应用 origin 上预览 = 任意脚本执行）。
func TestInlineSafe(t *testing.T) {
	yes := []string{"image/png", "IMAGE/JPEG; name=a.jpg", "application/pdf", "text/plain; charset=utf-8", "video/mp4"}
	no := []string{"text/html", "text/html; charset=utf-8", "image/svg+xml", "application/xhtml+xml", "text/xml",
		"application/octet-stream", "application/javascript", "", "multipart/mixed"}
	for _, ct := range yes {
		if !inlineSafe(ct) {
			t.Errorf("%q should be inline-safe", ct)
		}
	}
	for _, ct := range no {
		if inlineSafe(ct) {
			t.Errorf("%q must not be inline-safe", ct)
		}
	}
}
