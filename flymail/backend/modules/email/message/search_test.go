package message_test

import (
	"flymail/internal/fts"
	"testing"
	"time"

	"flymail/modules/email/message"
)

func TestSearchMessages(t *testing.T) {
	repo, db := newRepoWithDB(t)
	base := time.Date(2026, 2, 1, 9, 0, 0, 0, time.UTC)
	at := func(min int) time.Time { return base.Add(time.Duration(min) * time.Minute) }

	_ = repo.Upsert(&message.Message{AccountID: 1, FolderID: 1, UID: 1, Subject: "发票 Invoice 2026", FromName: "Alice", FromAddr: "alice@x.com", Snippet: "请查收附件", Date: at(10)})
	_ = repo.Upsert(&message.Message{AccountID: 1, FolderID: 1, UID: 2, Subject: "周会纪要", FromName: "Bob", FromAddr: "bob@y.com", Snippet: "本周进度", Date: at(20)})
	_ = repo.Upsert(&message.Message{AccountID: 2, FolderID: 2, UID: 1, Subject: "Newsletter", FromName: "News", FromAddr: "news@z.com", Date: at(30)})
	// 正文经触发器进索引（snippet 由正文派生，不单独索引）
	nl, _ := repo.GetByFolderUID(2, 1)
	_ = message.NewBodyRepository(db).Upsert(&message.MessageBody{MessageID: nl.ID, TextBody: "invoice link inside"})

	// 主题命中（中文）
	if got, _ := repo.SearchMessages(fts.Parse("纪要"), nil, 0, 50, message.Filter{}); len(got) != 1 || got[0].Subject != "周会纪要" {
		t.Errorf("主题搜索失败: %+v", got)
	}
	// 发件人命中
	if got, _ := repo.SearchMessages(fts.Parse("alice"), nil, 0, 50, message.Filter{}); len(got) != 1 || got[0].FromAddr != "alice@x.com" {
		t.Errorf("发件人搜索失败: %+v", got)
	}
	// 跨账户命中 + 大小写不敏感（subject "Invoice" 与正文 "invoice"）
	got, _ := repo.SearchMessages(fts.Parse("invoice"), nil, 0, 50, message.Filter{})
	if len(got) != 2 {
		t.Fatalf("invoice 应命中 2 封(跨账户)，实际 %d", len(got))
	}
	// date DESC：Newsletter(30min) 在 发票(10min) 之前
	if got[0].UID != 1 || got[0].AccountID != 2 {
		t.Errorf("排序错误，首条应为账户2的 Newsletter: %+v", got[0])
	}
}

// TestSearchLatinTokenIgnoresPunctuation：FTS 路径不走 LIKE，"100%" 里的 % 被当分隔符丢弃，
// 实际按 "100"* 前缀匹配；标点不能把搜索变成「匹配全部」。LIKE 转义的守护见 fts_test.go 的 in: 用例。
func TestSearchLatinTokenIgnoresPunctuation(t *testing.T) {
	repo := newRepo(t)
	_ = repo.Upsert(&message.Message{AccountID: 1, FolderID: 1, UID: 1, Subject: "100% done", Date: time.Now()})
	_ = repo.Upsert(&message.Message{AccountID: 1, FolderID: 1, UID: 2, Subject: "nothing here", Date: time.Now()})

	if got, _ := repo.SearchMessages(fts.Parse("100%"), nil, 0, 50, message.Filter{}); len(got) != 1 || got[0].Subject != "100% done" {
		t.Errorf("通配符转义失败，应只命中含 '100%%' 的一封: %+v", got)
	}
}
