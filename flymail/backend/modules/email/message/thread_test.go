package message_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"flymail-core/types"

	"flymail/modules/email/account"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"
)

// put 模拟同步入库的一步：upsert 后立刻做线程归属（与 Service.upsertBatch 同序）。
func put(t *testing.T, repo *message.Repository, m *message.Message) *message.Message {
	t.Helper()
	if err := repo.Upsert(m); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	rows, err := repo.LoadByFolderUIDs(m.FolderID, []uint32{m.UID})
	if err != nil || len(rows) != 1 {
		t.Fatalf("load: %v (%d rows)", err, len(rows))
	}
	if err := repo.AssignThreads(rows); err != nil {
		t.Fatalf("assign: %v", err)
	}
	return &rows[0]
}

func threadOf(t *testing.T, repo *message.Repository, folderID uint, uid uint32) string {
	t.Helper()
	m, err := repo.GetByFolderUID(folderID, uid)
	if err != nil {
		t.Fatalf("get %d/%d: %v", folderID, uid, err)
	}
	return m.ThreadID
}

func seedThreadFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	accts := []account.Account{{ID: 1, Name: "工作邮箱", Email: "me@work.com"}, {ID: 2, Name: "personal", Email: "me@home.org"}}
	if err := db.Create(&accts).Error; err != nil {
		t.Fatal(err)
	}
	folders := []folder.Folder{
		{ID: 1, AccountID: 1, Path: "INBOX", DisplayName: "收件箱", Type: "inbox", Selectable: true},
		{ID: 2, AccountID: 1, Path: "Sent", DisplayName: "已发送", Type: "sent", Selectable: true},
		{ID: 3, AccountID: 1, Path: "[Gmail]/All Mail", DisplayName: "所有邮件", Type: "archive", Selectable: true},
		{ID: 4, AccountID: 2, Path: "INBOX", DisplayName: "Inbox", Type: "inbox", Selectable: true},
	}
	if err := db.Create(&folders).Error; err != nil {
		t.Fatal(err)
	}
}

var t0 = time.Date(2026, 3, 1, 9, 0, 0, 0, time.Local)

func at(h int) time.Time { return t0.Add(time.Duration(h) * time.Hour) }

func TestAssignThreadsForwardAndReverse(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedThreadFixture(t, db)

	// 原信 A → 回复 B（In-Reply-To A）→ 回复 C（References A B）：正向沿用
	a := put(t, repo, &message.Message{AccountID: 1, FolderID: 1, UID: 1, MessageID: "a@x", Subject: "hi", Date: at(0)})
	b := put(t, repo, &message.Message{AccountID: 1, FolderID: 2, UID: 1, MessageID: "b@x", InReplyTo: "a@x", References: "a@x", Subject: "Re: hi", Date: at(1)})
	c := put(t, repo, &message.Message{AccountID: 1, FolderID: 1, UID: 2, MessageID: "c@x", InReplyTo: "b@x", References: "a@x b@x", Subject: "Re: hi", Date: at(2)})
	if a.ThreadID != "1:a@x" || b.ThreadID != a.ThreadID || c.ThreadID != a.ThreadID {
		t.Fatalf("forward chain: %q %q %q", a.ThreadID, b.ThreadID, c.ThreadID)
	}

	// 回复先到（Sent 先同步）：E 回复 D，D 后入库 → 反向认领 E 的线程
	e := put(t, repo, &message.Message{AccountID: 1, FolderID: 2, UID: 2, MessageID: "e@x", InReplyTo: "d@x", Subject: "Re: later", Date: at(4)})
	if e.ThreadID != "1:e@x" {
		t.Fatalf("orphan reply should open its own thread: %q", e.ThreadID)
	}
	d := put(t, repo, &message.Message{AccountID: 1, FolderID: 1, UID: 3, MessageID: "d@x", Subject: "later", Date: at(3)})
	if d.ThreadID != "1:e@x" {
		t.Errorf("root should adopt reply's thread: %q", d.ThreadID)
	}

	// 分叉合并：F、G 各自回复缺失的原信 Z（各开线程），Z 到达后两条并成一条
	f := put(t, repo, &message.Message{AccountID: 1, FolderID: 1, UID: 4, MessageID: "f@x", InReplyTo: "z@x", Subject: "Re: z", Date: at(5)})
	g := put(t, repo, &message.Message{AccountID: 1, FolderID: 1, UID: 5, MessageID: "g@x", InReplyTo: "z@x", Subject: "Re: z", Date: at(6)})
	if f.ThreadID == g.ThreadID {
		t.Fatalf("without root the two replies cannot be linked online: %q", f.ThreadID)
	}
	z := put(t, repo, &message.Message{AccountID: 1, FolderID: 1, UID: 6, MessageID: "z@x", Subject: "z", Date: at(4)})
	if threadOf(t, repo, 1, 4) != z.ThreadID || threadOf(t, repo, 1, 5) != z.ThreadID {
		t.Errorf("root arrival should merge both branches: f=%q g=%q z=%q", threadOf(t, repo, 1, 4), threadOf(t, repo, 1, 5), z.ThreadID)
	}

	// 账户隔离：账户 2 收到同一封 A，不并入账户 1 的线程
	a2 := put(t, repo, &message.Message{AccountID: 2, FolderID: 4, UID: 1, MessageID: "a@x", Subject: "hi", Date: at(0)})
	if a2.ThreadID == a.ThreadID || !strings.HasPrefix(a2.ThreadID, "2:") {
		t.Errorf("threads must be account-scoped: %q", a2.ThreadID)
	}

	// 在线归属之后整库重建：分组不变，且**沿用既有 id**（d 认领了 e 的线程 "1:e@x"，重建不改成 "1:d@x"）
	before := map[string]string{}
	for _, k := range [][2]uint{{1, 1}, {2, 1}, {1, 2}, {1, 3}, {2, 2}, {1, 4}, {1, 5}, {1, 6}, {4, 1}} {
		before[fmt.Sprint(k)] = threadOf(t, repo, k[0], uint32(k[1]))
	}
	if _, err := message.RebuildThreads(db); err != nil {
		t.Fatal(err)
	}
	for k, tid := range before {
		var f, u uint
		fmt.Sscanf(k, "[%d %d]", &f, &u)
		if got := threadOf(t, repo, f, uint32(u)); got != tid {
			t.Errorf("rebuild renamed thread of %s: %q -> %q", k, tid, got)
		}
	}
}

func TestAssignThreadsGmailCopyAndIdempotent(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedThreadFixture(t, db)
	a := put(t, repo, &message.Message{AccountID: 1, FolderID: 1, UID: 1, MessageID: "a@x", Subject: "hi", Date: at(0), Size: 10})
	// 「所有邮件」里的副本：同 Message-ID → 同线程
	cp := put(t, repo, &message.Message{AccountID: 1, FolderID: 3, UID: 9, MessageID: "a@x", Subject: "hi", Date: at(0), Size: 10})
	if cp.ThreadID != a.ThreadID {
		t.Errorf("gmail copy should share thread: %q vs %q", cp.ThreadID, a.ThreadID)
	}
	// 重复 upsert（全量同步每次都会重抓最近 N 封）不改线程
	b := put(t, repo, &message.Message{AccountID: 1, FolderID: 1, UID: 2, MessageID: "b@x", InReplyTo: "a@x", Subject: "Re: hi", Date: at(1)})
	for i := 0; i < 2; i++ {
		put(t, repo, &message.Message{AccountID: 1, FolderID: 1, UID: 1, MessageID: "a@x", Subject: "hi", Date: at(0), Size: 10})
		put(t, repo, &message.Message{AccountID: 1, FolderID: 1, UID: 2, MessageID: "b@x", InReplyTo: "a@x", Subject: "Re: hi", Date: at(1)})
	}
	if threadOf(t, repo, 1, 1) != a.ThreadID || threadOf(t, repo, 1, 2) != b.ThreadID || a.ThreadID != b.ThreadID {
		t.Errorf("re-upsert changed threads: %q %q", threadOf(t, repo, 1, 1), threadOf(t, repo, 1, 2))
	}
	// 没有 Message-ID 的邮件：用 (folder, uid) 兜底，不会跟别的空 id 邮件撞到一起
	n1 := put(t, repo, &message.Message{AccountID: 1, FolderID: 1, UID: 7, Subject: "no id", Date: at(2)})
	n2 := put(t, repo, &message.Message{AccountID: 1, FolderID: 1, UID: 8, Subject: "no id", Date: at(3)})
	if n1.ThreadID == "" || n1.ThreadID == n2.ThreadID {
		t.Errorf("messages without Message-ID must not collide: %q %q", n1.ThreadID, n2.ThreadID)
	}
}

// TestStoreParsedBodyBackfillsThreadHeaders：元数据阶段没拿到线程头（HEADER.FIELDS 回空的服务器）时，
// 正文落库要把头补上并把这封归并进原信的线程，连带它已有的回复一起挪过去。
func TestStoreParsedBodyBackfillsThreadHeaders(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedThreadFixture(t, db)
	svc := message.NewService(repo, message.NewBodyRepository(db))
	root := put(t, repo, &message.Message{AccountID: 1, FolderID: 1, UID: 1, MessageID: "a@x", Subject: "hi", Date: at(0)})
	// 回复入库时没有头 → 自己开线程；再来一封回复它的（头指向 b）→ 挂在 b 的线程上
	b := put(t, repo, &message.Message{AccountID: 1, FolderID: 1, UID: 2, MessageID: "b@x", Subject: "Re: hi", Date: at(1)})
	c := put(t, repo, &message.Message{AccountID: 1, FolderID: 1, UID: 3, MessageID: "c@x", InReplyTo: "b@x", Subject: "Re: hi", Date: at(2)})
	if b.ThreadID == root.ThreadID || c.ThreadID != b.ThreadID {
		t.Fatalf("precondition: root=%q b=%q c=%q", root.ThreadID, b.ThreadID, c.ThreadID)
	}
	// 打开 b 的正文：整封解析出 In-Reply-To → 回填 + 归并，c 跟着一起并进 root
	err := svc.StoreParsedBody(b.ID, &types.ParsedEmail{MessageID: "b@x", InReplyTo: "a@x", References: "a@x", TextBody: "reply"})
	if err != nil {
		t.Fatal(err)
	}
	if threadOf(t, repo, 1, 2) != root.ThreadID || threadOf(t, repo, 1, 3) != root.ThreadID {
		t.Errorf("after body: b=%q c=%q want %q", threadOf(t, repo, 1, 2), threadOf(t, repo, 1, 3), root.ThreadID)
	}
	m, _ := repo.GetByID(b.ID)
	if m.InReplyTo != "a@x" || m.References != "a@x" || !m.BodySynced {
		t.Errorf("headers/body not stored: %+v", m)
	}
	// 已有头的行不被正文覆盖（正文里的头与元数据一致，重复落正文不该再触发归属）
	if err := svc.StoreParsedBody(c.ID, &types.ParsedEmail{InReplyTo: "zzz@x", TextBody: "x"}); err != nil {
		t.Fatal(err)
	}
	if m, _ := repo.GetByID(c.ID); m.InReplyTo != "b@x" {
		t.Errorf("existing header overwritten: %q", m.InReplyTo)
	}
}

func TestRebuildThreadsLegacyAndSubjectFallback(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedThreadFixture(t, db)
	// 老库：thread_id 全空，且没有头字段；只有主题可用
	legacy := []*message.Message{
		{AccountID: 1, FolderID: 1, UID: 1, MessageID: "a@x", Subject: "发票报销", Date: at(0)},
		{AccountID: 1, FolderID: 2, UID: 1, MessageID: "b@x", Subject: "Re: 发票报销", Date: at(1)},
		{AccountID: 1, FolderID: 1, UID: 2, MessageID: "c@x", Subject: "回复：发票报销", Date: at(2)},
		// 无前缀的同主题：独立
		{AccountID: 1, FolderID: 1, UID: 3, MessageID: "d@x", Subject: "发票报销", Date: at(3)},
		// 有前缀但相隔超过 90 天：独立
		{AccountID: 1, FolderID: 1, UID: 4, MessageID: "e@x", Subject: "Re: 发票报销", Date: t0.AddDate(0, 0, 120)},
		// 两封回复指向同一封缺失的原信：并查集把它们并起来（写入路径做不到）
		{AccountID: 1, FolderID: 1, UID: 5, MessageID: "f@x", InReplyTo: "zz@x", Subject: "Re: zz", Date: at(5)},
		{AccountID: 1, FolderID: 1, UID: 6, MessageID: "g@x", References: "zz@x", Subject: "Re: zz", Date: at(6)},
	}
	for _, m := range legacy {
		if err := repo.Upsert(m); err != nil {
			t.Fatal(err)
		}
	}
	// 启动检查：有未归属的行 → 自动重建
	if err := message.EnsureThreads(db); err != nil {
		t.Fatalf("EnsureThreads: %v", err)
	}
	ta := threadOf(t, repo, 1, 1)
	if ta != "1:a@x" {
		t.Errorf("root thread id: %q", ta)
	}
	if threadOf(t, repo, 2, 1) != ta || threadOf(t, repo, 1, 2) != ta {
		t.Errorf("Re:/回复： should join by subject: %q %q", threadOf(t, repo, 2, 1), threadOf(t, repo, 1, 2))
	}
	// 注意：UID 3 没前缀不并入，但它更新了「该主题最近一封」，UID 4 又超窗，所以两者都独立
	if threadOf(t, repo, 1, 3) == ta || threadOf(t, repo, 1, 4) == ta || threadOf(t, repo, 1, 3) == threadOf(t, repo, 1, 4) {
		t.Errorf("plain / out-of-window subjects must stay separate: %q %q", threadOf(t, repo, 1, 3), threadOf(t, repo, 1, 4))
	}
	if threadOf(t, repo, 1, 5) != threadOf(t, repo, 1, 6) {
		t.Errorf("replies to a missing root should merge: %q %q", threadOf(t, repo, 1, 5), threadOf(t, repo, 1, 6))
	}
	// 再跑一次不变（幂等）
	n, err := message.RebuildThreads(db)
	if err != nil || n != 4 {
		t.Errorf("rebuild: n=%d err=%v (want 4 threads)", n, err)
	}
	if threadOf(t, repo, 1, 1) != ta {
		t.Errorf("rebuild not idempotent")
	}
	// 没有未归属的行时 EnsureThreads 不动（线程 id 保持）
	if err := message.EnsureThreads(db); err != nil || threadOf(t, repo, 1, 5) != threadOf(t, repo, 1, 6) {
		t.Errorf("EnsureThreads noop failed: %v", err)
	}
}

// seedConversation 造一条跨文件夹会话 + 两封独立邮件，供列表测试用。
//
//	线程 T1（hi）：INBOX#1 a(未读, 附件) ← Sent#1 b(我回) ← INBOX#2 c(已读, 星标)   最新 at(2)
//	线程 T2（solo）：INBOX#3 s(未读)                                                   at(5)
//	线程 T3（old）：INBOX#4 o(已读)                                                    at(-10)
//	账户 2：INBOX#4/1 p(未读)                                                          at(3)
func seedConversation(t *testing.T, repo *message.Repository, db *gorm.DB) {
	t.Helper()
	seedThreadFixture(t, db)
	put(t, repo, &message.Message{AccountID: 1, FolderID: 1, UID: 1, MessageID: "a@x", Subject: "hi", FromName: "Alice", FromAddr: "alice@x", Date: at(0), HasAttachment: true, Snippet: "first"})
	put(t, repo, &message.Message{AccountID: 1, FolderID: 2, UID: 1, MessageID: "b@x", InReplyTo: "a@x", Subject: "Re: hi", FromName: "我", FromAddr: "me@work.com", Date: at(1), Seen: true, Snippet: "reply"})
	put(t, repo, &message.Message{AccountID: 1, FolderID: 1, UID: 2, MessageID: "c@x", InReplyTo: "b@x", Subject: "Re: hi", FromName: "alice", FromAddr: "ALICE@x", Date: at(2), Seen: true, Flagged: true, Snippet: "latest"})
	put(t, repo, &message.Message{AccountID: 1, FolderID: 1, UID: 3, MessageID: "s@x", Subject: "solo", FromName: "Bob", FromAddr: "bob@x", Date: at(5)})
	put(t, repo, &message.Message{AccountID: 1, FolderID: 1, UID: 4, MessageID: "o@x", Subject: "old", FromName: "Carol", FromAddr: "carol@x", Date: at(-10), Seen: true})
	put(t, repo, &message.Message{AccountID: 2, FolderID: 4, UID: 1, MessageID: "p@y", Subject: "personal", FromName: "Dan", FromAddr: "dan@y", Date: at(3)})
}

func TestFolderThreadsPage(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedConversation(t, repo, db)

	page, err := repo.FolderThreads(1, message.Filter{}, nil, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(page.Threads) != 3 || page.NextCursor != nil {
		t.Fatalf("page: total=%d n=%d cursor=%v", page.Total, len(page.Threads), page.NextCursor)
	}
	// 排序：solo(at5) > hi(at2) > old(at-10)
	if page.Threads[0].Subject != "solo" || page.Threads[1].Subject != "Re: hi" || page.Threads[2].Subject != "old" {
		t.Errorf("order: %s / %s / %s", page.Threads[0].Subject, page.Threads[1].Subject, page.Threads[2].Subject)
	}
	hi := page.Threads[1]
	// 汇总按整条会话（含 Sent 那封）：3 封、1 未读、有星标、有附件；参与者去重（大小写不敏感）
	if hi.Count != 3 || hi.Unread != 1 || !hi.Flagged || !hi.HasAttachment {
		t.Errorf("summary: %+v", hi)
	}
	if len(hi.Participants) != 2 || hi.Participants[0].Email != "alice@x" || hi.Participants[1].Email != "me@work.com" {
		t.Errorf("participants: %+v", hi.Participants)
	}
	// 主题/摘要/latest 取范围内（INBOX）最新一封 c
	c, _ := repo.GetByFolderUID(1, 2)
	if hi.LatestID != c.ID || hi.LatestFolderID != 1 || hi.Snippet != "latest" || hi.AccountID != 1 {
		t.Errorf("latest: %+v (want id %d)", hi, c.ID)
	}

	// Sent 视角：同一线程出现，但 latest 是 b
	sent, _ := repo.FolderThreads(2, message.Filter{}, nil, "", 50)
	b, _ := repo.GetByFolderUID(2, 1)
	if len(sent.Threads) != 1 || sent.Threads[0].ThreadID != hi.ThreadID || sent.Threads[0].LatestID != b.ID || sent.Threads[0].Count != 3 {
		t.Errorf("sent view: %+v", sent.Threads)
	}

	// 筛选：未读 → 只剩 hi（a 未读）与 solo
	f := false
	unread, _ := repo.FolderThreads(1, message.Filter{Seen: &f}, nil, "", 50)
	if unread.Total != 2 || len(unread.Threads) != 2 {
		t.Errorf("unread filter: total=%d n=%d", unread.Total, len(unread.Threads))
	}

	// 分页：每页 1 条，游标接力，最后一页 cursor 为空，翻页不重不漏
	var got []string
	var cur *message.ThreadCursor
	for i := 0; i < 5; i++ {
		var before *time.Time
		var bt string
		if cur != nil {
			tm, err := time.Parse(time.RFC3339Nano, cur.BeforeDate)
			if err != nil {
				t.Fatal(err)
			}
			before, bt = &tm, cur.BeforeThread
		}
		p, err := repo.FolderThreads(1, message.Filter{}, before, bt, 1)
		if err != nil {
			t.Fatal(err)
		}
		for _, th := range p.Threads {
			got = append(got, th.Subject)
		}
		if p.NextCursor == nil {
			break
		}
		cur = p.NextCursor
	}
	if strings.Join(got, ",") != "solo,Re: hi,old" {
		t.Errorf("paged: %v", got)
	}
}

// pageAll 逐页翻完一个文件夹的会话，返回主题序列（用来断言不重不漏）。
func pageAll(t *testing.T, repo *message.Repository, folderID uint, limit int) []string {
	t.Helper()
	var got []string
	var cur *message.ThreadCursor
	for i := 0; i < 20; i++ {
		var before *time.Time
		var bt string
		if cur != nil {
			tm, err := time.Parse(time.RFC3339Nano, cur.BeforeDate)
			if err != nil {
				t.Fatal(err)
			}
			before, bt = &tm, cur.BeforeThread
		}
		p, err := repo.FolderThreads(folderID, message.Filter{}, before, bt, limit)
		if err != nil {
			t.Fatal(err)
		}
		for _, th := range p.Threads {
			got = append(got, th.Subject)
		}
		if p.NextCursor == nil {
			break
		}
		cur = p.NextCursor
	}
	return got
}

// TestFolderThreadsCursorNoDupNoLoss 针对两个真实翻页缺陷：
//  1. 首页里的多封会话有成员早于游标：若分组前按 date <= 游标预筛，重算的 MAX 会落到游标下方，
//     该会话在第 2 页重复出现（且展示成旧邮件）；
//  2. 两条会话最新一封日期文本相同、thread_id 序与 id 序相反：排序键与游标比较键不同源时会漏掉一条。
func TestFolderThreadsCursorNoDupNoLoss(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedThreadFixture(t, db)
	// 会话 A：09:00 + 15:00；B：12:00；C：10:00 → 页大小 2 时首页 [A, B]，第 2 页必须只有 C
	put(t, repo, &message.Message{AccountID: 1, FolderID: 1, UID: 1, MessageID: "a@x", Subject: "A", Date: at(0)})
	put(t, repo, &message.Message{AccountID: 1, FolderID: 1, UID: 2, MessageID: "a2@x", InReplyTo: "a@x", Subject: "A", Date: at(6)})
	put(t, repo, &message.Message{AccountID: 1, FolderID: 1, UID: 3, MessageID: "b@x", Subject: "B", Date: at(3)})
	put(t, repo, &message.Message{AccountID: 1, FolderID: 1, UID: 4, MessageID: "c@x", Subject: "C", Date: at(1)})
	if got := strings.Join(pageAll(t, repo, 1, 2), ","); got != "A,B,C" {
		t.Errorf("limit=2: %s (want A,B,C)", got)
	}
	if got := strings.Join(pageAll(t, repo, 1, 1), ","); got != "A,B,C" {
		t.Errorf("limit=1: %s (want A,B,C)", got)
	}

	// 同秒平局：文件夹 2 里 "zzz"(id 小) 与 "aaa"(id 大) 日期相同，再加一条更早的
	put(t, repo, &message.Message{AccountID: 1, FolderID: 2, UID: 1, MessageID: "zzz@x", Subject: "Z", Date: at(10)})
	put(t, repo, &message.Message{AccountID: 1, FolderID: 2, UID: 2, MessageID: "aaa@x", Subject: "S", Date: at(10)})
	put(t, repo, &message.Message{AccountID: 1, FolderID: 2, UID: 3, MessageID: "old@x", Subject: "O", Date: at(2)})
	for _, limit := range []int{1, 2} {
		got := pageAll(t, repo, 2, limit)
		if len(got) != 3 || got[2] != "O" {
			t.Errorf("limit=%d tie paging: %v (want 3 rows ending with O)", limit, got)
		}
		seen := map[string]bool{}
		for _, s := range got {
			if seen[s] {
				t.Errorf("limit=%d duplicate %q", limit, s)
			}
			seen[s] = true
		}
	}
}

func TestAggregateAndSearchThreads(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedConversation(t, repo, db)

	// inbox 聚合：账户 1 三条 + 账户 2 一条
	page, err := repo.AggregateThreads("inbox", message.Filter{}, nil, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 4 || len(page.Threads) != 4 || page.Threads[0].Subject != "solo" || page.Threads[1].Subject != "personal" {
		t.Errorf("inbox aggregate: total=%d %+v", page.Total, page.Threads)
	}
	// unread 聚合：hi(a 未读)、solo、personal
	unread, _ := repo.AggregateThreads("unread", message.Filter{}, nil, "", 50)
	if unread.Total != 3 {
		t.Errorf("unread aggregate total=%d", unread.Total)
	}
	// starred：只有 hi（c 星标）
	starred, _ := repo.AggregateThreads("starred", message.Filter{}, nil, "", 50)
	if starred.Total != 1 || starred.Threads[0].Subject != "Re: hi" {
		t.Errorf("starred aggregate: %+v", starred.Threads)
	}

	svc := message.NewService(repo, message.NewBodyRepository(db))
	// 搜索：命中 3 封 hi（主题 "hi"）折叠成 1 条会话
	sp, err := svc.ListSearchThreads("hi", message.Filter{}, nil, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if sp.Total != 1 || len(sp.Threads) != 1 || sp.Threads[0].Count != 3 {
		t.Errorf("search threads: total=%d %+v", sp.Total, sp.Threads)
	}
	// 空查询 → 空页
	if ep, _ := svc.ListSearchThreads("...", message.Filter{}, nil, "", 50); len(ep.Threads) != 0 || ep.Total != 0 {
		t.Errorf("empty query should be empty page: %+v", ep)
	}
}

func TestThreadMessagesOrderAndDedupe(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedConversation(t, repo, db)
	// Gmail 副本：a 在「所有邮件」里再来一份，展开时不该出现两次
	put(t, repo, &message.Message{AccountID: 1, FolderID: 3, UID: 100, MessageID: "a@x", Subject: "hi", FromName: "Alice", FromAddr: "alice@x", Date: at(0)})
	tid := threadOf(t, repo, 1, 1)
	svc := message.NewService(repo, message.NewBodyRepository(db))
	list, err := svc.ThreadMessages(tid, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 || list[0].FolderID != 1 || list[1].FolderID != 2 || list[2].UID != 2 {
		t.Errorf("thread messages: %+v", list)
	}
	if capped, _ := svc.ThreadMessages(tid, 2); len(capped) != 2 {
		t.Errorf("limit not applied: %d", len(capped))
	}
	// 副本进了同一线程，汇总 count 仍是 3
	page, _ := repo.FolderThreads(1, message.Filter{}, nil, "", 50)
	for _, th := range page.Threads {
		if th.ThreadID == tid && th.Count != 3 {
			t.Errorf("count with gmail copy = %d, want 3", th.Count)
		}
	}
	members, _ := repo.ThreadMembers([]string{tid})
	if len(members) != 4 {
		t.Errorf("ThreadMembers should include copies: %d", len(members))
	}
}

func TestThreadHandlers(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedConversation(t, repo, db)
	svc := message.NewService(repo, message.NewBodyRepository(db))
	gin.SetMode(gin.TestMode)
	r := gin.New()
	message.RegisterRoutes(r.Group(""), svc)

	get := func(path string) (int, map[string]json.RawMessage) {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var out map[string]json.RawMessage
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	code, out := get("/folders/1/threads?limit=2")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	var threads []message.ThreadListItem
	_ = json.Unmarshal(out["threads"], &threads)
	var cur message.ThreadCursor
	_ = json.Unmarshal(out["next_cursor"], &cur)
	if len(threads) != 2 || string(out["total"]) != "3" || cur.BeforeThread == "" {
		t.Errorf("first page: n=%d total=%s cursor=%+v", len(threads), out["total"], cur)
	}
	// 第二页：不带 total
	code, out = get("/folders/1/threads?limit=2&before_date=" + url.QueryEscape(cur.BeforeDate) + "&before_thread=" + url.QueryEscape(cur.BeforeThread))
	_ = json.Unmarshal(out["threads"], &threads)
	if code != 200 || len(threads) != 1 || threads[0].Subject != "old" || out["total"] != nil {
		t.Errorf("second page: code=%d n=%d total=%s", code, len(threads), out["total"])
	}
	// 成员：thread_id 含 ':' '@'，经 query 编码
	code, out = get("/threads/messages?thread_id=" + url.QueryEscape(threads[0].ThreadID))
	var msgs []message.MessageListItem
	_ = json.Unmarshal(out["messages"], &msgs)
	if code != 200 || len(msgs) != 1 || msgs[0].Subject != "old" {
		t.Errorf("thread messages: code=%d %+v", code, msgs)
	}
	if code, _ := get("/threads/messages"); code != 400 {
		t.Errorf("missing thread_id should be 400, got %d", code)
	}
	code, out = get("/aggregate/threads?view=inbox&seen=false")
	_ = json.Unmarshal(out["threads"], &threads)
	if code != 200 || len(threads) != 3 || string(out["total"]) != "3" {
		t.Errorf("aggregate unread: code=%d n=%d total=%s", code, len(threads), out["total"])
	}
	if code, _ := get("/aggregate/threads?view=bogus"); code != 400 {
		t.Errorf("invalid view should be 400, got %d", code)
	}
	code, out = get("/search/threads?q=hi")
	_ = json.Unmarshal(out["threads"], &threads)
	if code != 200 || len(threads) != 1 || threads[0].Count != 3 {
		t.Errorf("search threads: code=%d %+v", code, threads)
	}
	// 重建
	req := httptest.NewRequest(http.MethodPost, "/threads/rebuild", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"threads":4`) {
		t.Errorf("rebuild: %d %s", w.Code, w.Body.String())
	}
}
