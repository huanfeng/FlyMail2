package message_test

import (
	"testing"
	"time"

	"flymail/modules/email/message"
)

func TestSearchMessages(t *testing.T) {
	repo := newRepo(t)
	base := time.Date(2026, 2, 1, 9, 0, 0, 0, time.UTC)
	at := func(min int) time.Time { return base.Add(time.Duration(min) * time.Minute) }

	_ = repo.Upsert(&message.Message{AccountID: 1, FolderID: 1, UID: 1, Subject: "发票 Invoice 2026", FromName: "Alice", FromAddr: "alice@x.com", Snippet: "请查收附件", Date: at(10)})
	_ = repo.Upsert(&message.Message{AccountID: 1, FolderID: 1, UID: 2, Subject: "周会纪要", FromName: "Bob", FromAddr: "bob@y.com", Snippet: "本周进度", Date: at(20)})
	_ = repo.Upsert(&message.Message{AccountID: 2, FolderID: 2, UID: 1, Subject: "Newsletter", FromName: "News", FromAddr: "news@z.com", Snippet: "invoice link inside", Date: at(30)})

	// 主题命中（中文）
	if got, _ := repo.SearchMessages("纪要", nil, 0, 50); len(got) != 1 || got[0].Subject != "周会纪要" {
		t.Errorf("主题搜索失败: %+v", got)
	}
	// 发件人命中
	if got, _ := repo.SearchMessages("alice", nil, 0, 50); len(got) != 1 || got[0].FromAddr != "alice@x.com" {
		t.Errorf("发件人搜索失败: %+v", got)
	}
	// 跨账户命中 + 大小写不敏感（subject "Invoice" 与 snippet "invoice"）
	got, _ := repo.SearchMessages("invoice", nil, 0, 50)
	if len(got) != 2 {
		t.Fatalf("invoice 应命中 2 封(跨账户)，实际 %d", len(got))
	}
	// date DESC：Newsletter(30min) 在 发票(10min) 之前
	if got[0].UID != 1 || got[0].AccountID != 2 {
		t.Errorf("排序错误，首条应为账户2的 Newsletter: %+v", got[0])
	}
}

func TestSearchMessagesEscapesWildcards(t *testing.T) {
	repo := newRepo(t)
	_ = repo.Upsert(&message.Message{AccountID: 1, FolderID: 1, UID: 1, Subject: "100% done", Date: time.Now()})
	_ = repo.Upsert(&message.Message{AccountID: 1, FolderID: 1, UID: 2, Subject: "nothing here", Date: time.Now()})

	// "%" 应作为字面量匹配，而非通配符（否则会匹配全部）
	if got, _ := repo.SearchMessages("100%", nil, 0, 50); len(got) != 1 || got[0].Subject != "100% done" {
		t.Errorf("通配符转义失败，应只命中含 '100%%' 的一封: %+v", got)
	}
}
