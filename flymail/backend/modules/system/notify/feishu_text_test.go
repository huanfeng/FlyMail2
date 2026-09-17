package notify

import (
	"strings"
	"testing"
)

// 飞书是纯文本消息，链接必须单独起一行。
//
// 聊天客户端靠「整行是 URL」来识别可点区域，塞在句子中间多半点不开——
// 那就等于白带了链接。
func TestFeishuTextPutsLinkOnItsOwnLine(t *testing.T) {
	text := feishuText(Event{
		Title: "新邮件 · Alice",
		Body:  "会议纪要\n下周三上午十点，会议室 A，请准备各自的进度",
		URL:   "https://mail.example.com/?account=1&folder=7&message=42",
	})

	if !strings.Contains(text, "新邮件 · Alice") {
		t.Errorf("标题丢了：%q", text)
	}
	if !strings.Contains(text, "请准备各自的进度") {
		t.Errorf("正文摘要丢了——这正是「不点进去也知道要不要处理」的那部分：%q", text)
	}
	if !strings.Contains(text, "\nhttps://mail.example.com/?account=1&folder=7&message=42") {
		t.Errorf("链接没有单独成行，聊天客户端不会把它识别成可点的：%q", text)
	}
}

// 没有链接时不要留一行空白。
func TestFeishuTextWithoutLink(t *testing.T) {
	text := feishuText(Event{Title: "同步失败", Body: "连接超时"})
	if text != "同步失败\n连接超时" {
		t.Errorf("没有链接时多了东西：%q", text)
	}
	only := feishuText(Event{Title: "账户状态变化"})
	if only != "账户状态变化" {
		t.Errorf("只有标题时多了东西：%q", only)
	}
}
