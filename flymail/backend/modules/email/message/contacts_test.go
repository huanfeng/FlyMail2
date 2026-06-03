package message_test

import (
	"testing"
	"time"

	"flymail/modules/email/message"
)

func TestSearchContacts(t *testing.T) {
	repo := newRepo(t)
	now := time.Now()
	// alice 出现 3 次，bob 1 次，carol 2 次 → 频率序 alice, carol, bob
	for i := 0; i < 3; i++ {
		_ = repo.Upsert(&message.Message{AccountID: 1, FolderID: 1, UID: uint32(100 + i), FromName: "Alice", FromAddr: "alice@x.com", Date: now})
	}
	for i := 0; i < 2; i++ {
		_ = repo.Upsert(&message.Message{AccountID: 1, FolderID: 1, UID: uint32(200 + i), FromName: "Carol", FromAddr: "carol@y.com", Date: now})
	}
	_ = repo.Upsert(&message.Message{AccountID: 1, FolderID: 1, UID: 300, FromName: "Bob", FromAddr: "bob@z.com", Date: now})

	// 无 q：按频率降序去重
	all, err := repo.SearchContacts("", 10)
	if err != nil {
		t.Fatalf("SearchContacts: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("应去重为 3 个联系人，实际 %d: %+v", len(all), all)
	}
	if all[0].Email != "alice@x.com" || all[1].Email != "carol@y.com" || all[2].Email != "bob@z.com" {
		t.Errorf("频率排序错误: %+v", all)
	}
	if all[0].Name != "Alice" {
		t.Errorf("姓名回填错误: %+v", all[0])
	}

	// 带 q：按地址/姓名过滤
	filtered, _ := repo.SearchContacts("carol", 10)
	if len(filtered) != 1 || filtered[0].Email != "carol@y.com" {
		t.Errorf("过滤错误: %+v", filtered)
	}
}
