package sync

import (
	"strings"
	"testing"
	"time"

	"flymail/modules/email/folder"
	"flymail/modules/email/message"
)

type notice struct {
	Type      string
	AccountID uint
	MessageID uint
	Title     string
	Body      string
}

// captureNotices 装一个假的 emit，把发出去的通知收集起来。
func captureNotices(m *Manager) *[]notice {
	got := &[]notice{}
	m.emit = func(eventType string, accountID, messageID uint, title, body string) {
		*got = append(*got, notice{eventType, accountID, messageID, title, body})
	}
	return got
}

func inbox() *folder.Folder { return &folder.Folder{ID: 1, AccountID: 1, Path: "INBOX", Type: "inbox"} }
func label() *folder.Folder {
	return &folder.Folder{ID: 2, AccountID: 1, Path: "工作", Type: "custom"}
}

func unseen(n int) []message.Message {
	out := make([]message.Message, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, message.Message{
			ID: uint(100 + i), AccountID: 1, FolderID: 1,
			Subject: "主题" + string(rune('A'+i)), FromName: "发件人" + string(rune('A'+i)),
			Date: time.Now().UTC(),
		})
	}
	return out
}

// 一轮里来了几封，就发几条通知。
//
// ── 缘起（用户提的） ─────────────────────────────────────────────────────────
//
// 原先只要一轮不止一封就合并成「收到 N 封新邮件」。那条通知既没有 message id、
// 也没有发件人和主题：点开跳不到任何地方，推到飞书上也只是一句没有信息量的话——
// 想知道是谁来的信还得自己切回应用翻一遍。
// 一次轮询通常就一两封，逐封发才是常态。
func TestNewMailNoticesSplitPerMessage(t *testing.T) {
	m, _, _ := newBodyManager(t)
	got := captureNotices(m)

	list := unseen(3)
	m.emitNewMailNotices(1, inbox(), &message.NewMail{Unseen: list, UnseenTotal: 3})

	if len(*got) != 3 {
		t.Fatalf("想要 3 条通知，拿到 %d 条：%+v", len(*got), *got)
	}
	for i, n := range *got {
		if n.MessageID != list[i].ID {
			t.Errorf("第 %d 条没带上对应的 message id：%d（想要 %d）——点开跳不到那封邮件", i, n.MessageID, list[i].ID)
		}
		if !strings.Contains(n.Title, list[i].FromName) {
			t.Errorf("第 %d 条标题里没有发件人：%q", i, n.Title)
		}
		if !strings.Contains(n.Body, list[i].Subject) {
			t.Errorf("第 %d 条正文里没有主题：%q", i, n.Body)
		}
	}
}

func TestSingleNewMailStillOneNotice(t *testing.T) {
	m, _, _ := newBodyManager(t)
	got := captureNotices(m)

	list := unseen(1)
	m.emitNewMailNotices(1, inbox(), &message.NewMail{Unseen: list, UnseenTotal: 1})

	if len(*got) != 1 || (*got)[0].MessageID != list[0].ID {
		t.Fatalf("单封应当发一条带 id 的通知：%+v", *got)
	}
}

// ⚠ 不能无上限地拆。
//
// 离线一夜再上线可能一次收进几十封，逐封发会把通知中心和飞书群直接刷爆。
// 明细本身也只取前 newMailUnseenCap 封，超过就退回合并，并在文案里给出总数。
func TestTooManyNewMailsMergeIntoOne(t *testing.T) {
	m, _, _ := newBodyManager(t)
	got := captureNotices(m)

	// 明细只有 3 封，实际来了 47 封
	m.emitNewMailNotices(1, inbox(), &message.NewMail{Unseen: unseen(3), UnseenTotal: 47})

	if len(*got) != 1 {
		t.Fatalf("一次来 47 封时应当合并成一条，拿到 %d 条", len(*got))
	}
	n := (*got)[0]
	if !strings.Contains(n.Title, "47") {
		t.Errorf("合并的通知没说清有多少封：%q", n.Title)
	}
	if n.MessageID != 0 {
		t.Errorf("合并的通知不该指向某一封邮件，却带了 id=%d", n.MessageID)
	}
}

// 首次同步整合成一条，并且说清楚是什么。
//
// 新账户接进来时收件箱里本来就堆着几百上千封未读。逐封提醒会直接刷爆，
// 而报成「收到 N 封新邮件」又会让人以为出事了——那些邮件是账户接入前就有的。
func TestBaselineSyncEmitsOneClearNotice(t *testing.T) {
	m, _, _ := newBodyManager(t)
	got := captureNotices(m)

	m.emitNewMailNotices(1, inbox(), &message.NewMail{Baseline: true, Unseen: unseen(3), UnseenTotal: 862})

	if len(*got) != 1 {
		t.Fatalf("首次同步应当只发一条，拿到 %d 条：%+v", len(*got), *got)
	}
	n := (*got)[0]
	if !strings.Contains(n.Body, "862") {
		t.Errorf("没说清有多少封未读：%q", n.Body)
	}
	// 要能看出这不是「刚刚收到 862 封新邮件」
	if !strings.Contains(n.Title+n.Body, "首次同步") {
		t.Errorf("文案没点明这是首次同步，用户会以为刚收到 862 封新邮件：%q / %q", n.Title, n.Body)
	}
	if n.MessageID != 0 {
		t.Errorf("整合通知不该指向某一封邮件")
	}
}

// ⚠ 首次同步只在收件箱发一条。
//
// 基线会对**每个**文件夹各触发一次，而 Gmail 把标签映射成文件夹——一个账户
// 十几个标签就是十几条「首次同步完成」。claimNotify 拦不住：各文件夹的未读明细
// 不同，去重键天然不一样。
func TestBaselineDoesNotFireOncePerLabel(t *testing.T) {
	m, _, _ := newBodyManager(t)
	got := captureNotices(m)

	nm := &message.NewMail{Baseline: true, Unseen: unseen(2), UnseenTotal: 300}
	m.emitNewMailNotices(1, inbox(), nm)
	m.emitNewMailNotices(1, label(), nm) // 同一轮基线里的另一个标签文件夹
	m.emitNewMailNotices(1, label(), nm)

	if len(*got) != 1 {
		t.Fatalf("首次同步应当只发一条，标签文件夹不该各发一条，拿到 %d 条：%+v", len(*got), *got)
	}
}
