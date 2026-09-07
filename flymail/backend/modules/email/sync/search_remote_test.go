package sync_test

import (
	"context"
	"testing"

	imapv2 "github.com/emersion/go-imap/v2"
)

// TestRemoteSearchFetchesMissingHits 验证服务端搜索：命中而本地没有的邮件被补抓入库（含正文），
// 第二次搜索时这些邮件已可本地检索，不再重复抓取。
func TestRemoteSearchFetchesMissingHits(t *testing.T) {
	svc, _, fsvc, sess := newSyncService(t)
	if err := fsvc.SyncFolders(1, sess); err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	sess.searchHits = []imapv2.UID{5, 6}

	res, err := svc.RemoteSearch(context.Background(), "hello is:unread")
	if err != nil {
		t.Fatalf("RemoteSearch: %v", err)
	}
	// fakeSession 有 INBOX + Sent 两个文件夹，每个都返回 2 个命中
	if res.Accounts != 1 || res.Folders != 2 || res.Matched != 4 || res.Fetched != 4 || len(res.Errors) != 0 {
		t.Fatalf("unexpected result: %+v", res)
	}
	// 条件翻译：自由词进 TEXT，is:unread 进 NotFlag
	if len(sess.searchCriteria) != 2 {
		t.Fatalf("expected 2 SEARCH calls, got %d", len(sess.searchCriteria))
	}
	c := sess.searchCriteria[0]
	if len(c.Text) != 1 || c.Text[0] != "hello" || len(c.NotFlag) != 1 || c.NotFlag[0] != imapv2.FlagSeen {
		t.Errorf("criteria = %+v", c)
	}

	// 抓回来的邮件正文已落库：按正文词本地能搜到（fakeSession 的正文是 "hello text"）
	detail, err := svc.MessageDetail(1)
	if err != nil {
		t.Fatalf("MessageDetail: %v", err)
	}
	if !detail.BodySynced || detail.TextBody != "hello text" {
		t.Errorf("fetched hit should have body synced: %+v", detail)
	}

	// 再搜一次：命中仍是 4，但已全部可本地检索，不再补抓
	res2, err := svc.RemoteSearch(context.Background(), "hello")
	if err != nil {
		t.Fatalf("RemoteSearch#2: %v", err)
	}
	if res2.Matched != 4 || res2.Fetched != 0 {
		t.Errorf("second search should fetch nothing: %+v", res2)
	}
}

// TestRemoteSearchFolderQualifier 验证 in: 限定符只搜匹配的文件夹。
func TestRemoteSearchFolderQualifier(t *testing.T) {
	svc, _, fsvc, sess := newSyncService(t)
	if err := fsvc.SyncFolders(1, sess); err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	sess.searchHits = []imapv2.UID{9}

	res, err := svc.RemoteSearch(context.Background(), "in:sent report")
	if err != nil {
		t.Fatalf("RemoteSearch: %v", err)
	}
	if res.Folders != 1 || res.Matched != 1 || res.Fetched != 1 {
		t.Errorf("in:sent should search one folder: %+v", res)
	}
}

// TestRemoteSearchNoIMAPCriteria 验证只有本地限定符（has:/account:/in:）时不向服务器发 SEARCH ALL。
func TestRemoteSearchNoIMAPCriteria(t *testing.T) {
	svc, _, fsvc, sess := newSyncService(t)
	if err := fsvc.SyncFolders(1, sess); err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	sess.searchHits = []imapv2.UID{1}
	for _, q := range []string{"has:attachment", "in:inbox", "account:work", "has:attachment in:inbox"} {
		res, err := svc.RemoteSearch(context.Background(), q)
		if err != nil || res.Folders != 0 || len(sess.searchCriteria) != 0 {
			t.Errorf("%q should not hit server: res=%+v err=%v calls=%d", q, res, err, len(sess.searchCriteria))
		}
	}
}

// TestRemoteSearchAccountQualifier 验证 account: 只搜匹配的账户。
func TestRemoteSearchAccountQualifier(t *testing.T) {
	svc, _, fsvc, sess := newSyncService(t)
	if err := fsvc.SyncFolders(1, sess); err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	sess.searchHits = []imapv2.UID{1}
	// 账户 1 是「工作邮箱 / me@work.com」：account:home 不匹配，不应搜；account:work 匹配
	res, err := svc.RemoteSearch(context.Background(), "account:home hello")
	if err != nil || res.Accounts != 0 || len(sess.searchCriteria) != 0 {
		t.Errorf("account:home should match nothing: res=%+v err=%v", res, err)
	}
	res, err = svc.RemoteSearch(context.Background(), "account:work hello")
	if err != nil || res.Accounts != 1 || res.Folders != 2 {
		t.Errorf("account:work should search account 1: res=%+v err=%v", res, err)
	}
}

// TestRemoteSearchEmptyQuery 验证空查询不碰服务器。
func TestRemoteSearchEmptyQuery(t *testing.T) {
	svc, _, fsvc, sess := newSyncService(t)
	if err := fsvc.SyncFolders(1, sess); err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	res, err := svc.RemoteSearch(context.Background(), "...")
	if err != nil || res.Folders != 0 || len(sess.searchCriteria) != 0 {
		t.Errorf("empty query should be a no-op: res=%+v err=%v calls=%d", res, err, len(sess.searchCriteria))
	}
}
