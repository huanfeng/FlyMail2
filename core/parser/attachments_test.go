package parser

import (
	"bytes"
	"io"
	"testing"

	"github.com/emersion/go-message/mail"

	"flymail-core/types"
)

// pngBytes 是一段非空的伪 PNG 内容（仅用于断言内容字节是否被完整保留）。
var pngBytes = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x01, 0x02, 0x03, 0x04}

// pdfBytes 是一段非空的伪 PDF 内容。
var pdfBytes = []byte("%PDF-1.4 fake pdf content\n%%EOF\n")

// buildTestMessage 构造一封 multipart/mixed 邮件：
// text/plain + text/html + 内联 image/png(Content-Id: <img1>) + 普通附件 application/pdf(doc.pdf)。
func buildTestMessage(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	var h mail.Header
	h.SetSubject("测试附件邮件")

	w, err := mail.CreateWriter(&buf, h)
	if err != nil {
		t.Fatalf("CreateWriter: %v", err)
	}

	// 文本部件（text/plain + text/html）
	iw, err := w.CreateInline()
	if err != nil {
		t.Fatalf("CreateInline: %v", err)
	}
	var th mail.InlineHeader
	th.Set("Content-Type", "text/plain; charset=utf-8")
	tpw, err := iw.CreatePart(th)
	if err != nil {
		t.Fatalf("CreatePart text/plain: %v", err)
	}
	if _, err := io.WriteString(tpw, "hello plain"); err != nil {
		t.Fatalf("write text/plain: %v", err)
	}
	tpw.Close()

	var hh mail.InlineHeader
	hh.Set("Content-Type", "text/html; charset=utf-8")
	hpw, err := iw.CreatePart(hh)
	if err != nil {
		t.Fatalf("CreatePart text/html: %v", err)
	}
	if _, err := io.WriteString(hpw, "<p>hello html</p>"); err != nil {
		t.Fatalf("write text/html: %v", err)
	}
	hpw.Close()
	iw.Close()

	// 内联 image/png（Content-Disposition: inline, Content-Id: <img1>）
	var imgH mail.InlineHeader
	imgH.Set("Content-Type", "image/png")
	imgH.Set("Content-Id", "<img1>")
	imgW, err := w.CreateSingleInline(imgH)
	if err != nil {
		t.Fatalf("CreateSingleInline image: %v", err)
	}
	if _, err := imgW.Write(pngBytes); err != nil {
		t.Fatalf("write png: %v", err)
	}
	imgW.Close()

	// 普通附件 application/pdf（Content-Disposition: attachment; filename=doc.pdf）
	var attH mail.AttachmentHeader
	attH.Set("Content-Type", "application/pdf")
	attH.SetFilename("doc.pdf")
	attW, err := w.CreateAttachment(attH)
	if err != nil {
		t.Fatalf("CreateAttachment pdf: %v", err)
	}
	if _, err := attW.Write(pdfBytes); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	attW.Close()

	w.Close()
	return buf.Bytes()
}

func TestExtractAttachments(t *testing.T) {
	raw := buildTestMessage(t)

	// 1) ExtractAttachments 返回 2 个元素，顺序 [png, pdf]，含内容字节。
	atts, err := ExtractAttachments(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ExtractAttachments error: %v", err)
	}
	if len(atts) != 2 {
		t.Fatalf("expected 2 attachments, got %d: %+v", len(atts), atts)
	}

	png := atts[0]
	if !png.IsInline {
		t.Errorf("png.IsInline = false, want true")
	}
	if png.ContentID != "img1" {
		t.Errorf("png.ContentID = %q, want %q", png.ContentID, "img1")
	}
	if !bytes.Equal(png.Content, pngBytes) {
		t.Errorf("png.Content = %v, want %v", png.Content, pngBytes)
	}

	pdf := atts[1]
	if pdf.IsInline {
		t.Errorf("pdf.IsInline = true, want false")
	}
	if pdf.Filename != "doc.pdf" {
		t.Errorf("pdf.Filename = %q, want %q", pdf.Filename, "doc.pdf")
	}
	if len(pdf.Content) == 0 {
		t.Errorf("pdf.Content is empty, want non-empty")
	}

	// 2) ParseBody 后 email.Attachments 同样 2 个、同序、IsInline/ContentID 一致，Size>0；
	//    且 TextBody/HTMLBody 被正确填充。
	email := &types.ParsedEmail{}
	if err := ParseBody(bytes.NewReader(raw), email, false); err != nil {
		t.Fatalf("ParseBody error: %v", err)
	}
	if email.TextBody != "hello plain" {
		t.Errorf("TextBody = %q, want %q", email.TextBody, "hello plain")
	}
	if email.HTMLBody != "<p>hello html</p>" {
		t.Errorf("HTMLBody = %q, want %q", email.HTMLBody, "<p>hello html</p>")
	}
	if len(email.Attachments) != 2 {
		t.Fatalf("expected 2 ParseBody attachments, got %d: %+v", len(email.Attachments), email.Attachments)
	}
	for i, a := range email.Attachments {
		if a.Size <= 0 {
			t.Errorf("email.Attachments[%d].Size = %d, want >0", i, a.Size)
		}
	}

	// 3) 顺序一致性：ExtractAttachments 与 ParseBody 的 (Filename, ContentID, IsInline) 序列逐一相等。
	for i := range atts {
		ea := atts[i]
		pa := email.Attachments[i]
		if ea.Filename != pa.Filename || ea.ContentID != pa.ContentID || ea.IsInline != pa.IsInline {
			t.Errorf("attachment[%d] mismatch: extract=(%q,%q,%v) parse=(%q,%q,%v)",
				i, ea.Filename, ea.ContentID, ea.IsInline, pa.Filename, pa.ContentID, pa.IsInline)
		}
	}
}
