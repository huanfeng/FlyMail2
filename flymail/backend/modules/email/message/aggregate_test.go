package message_test

import (
	"path/filepath"
	"testing"
	"time"

	"flymail/internal/database"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"

	"gorm.io/gorm"
)

// newRepoWithDB 返回聚合测试所需的 repo + 原始 db（用于插入 folders 行）。
func newRepoWithDB(t *testing.T) (*message.Repository, *gorm.DB) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return message.NewRepository(db), db
}

// seedAggregate 构造两账户、各 inbox + trash 的数据集，覆盖聚合的过滤分支。
//
// folders: 1=acct1/inbox 2=acct1/trash 3=acct2/inbox 4=acct2/junk 5=acct1/archive
func seedAggregate(t *testing.T, repo *message.Repository, db *gorm.DB) {
	t.Helper()
	folders := []folder.Folder{
		{ID: 1, AccountID: 1, Path: "INBOX", DisplayName: "Inbox", Type: "inbox", Selectable: true},
		{ID: 2, AccountID: 1, Path: "Trash", DisplayName: "Trash", Type: "trash", Selectable: true},
		{ID: 3, AccountID: 2, Path: "INBOX", DisplayName: "Inbox", Type: "inbox", Selectable: true},
		{ID: 4, AccountID: 2, Path: "Junk", DisplayName: "Junk", Type: "junk", Selectable: true},
		{ID: 5, AccountID: 1, Path: "[Gmail]/All Mail", DisplayName: "所有邮件", Type: "archive", Selectable: true},
	}
	if err := db.Create(&folders).Error; err != nil {
		t.Fatalf("seed folders: %v", err)
	}

	base := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	at := func(min int) time.Time { return base.Add(time.Duration(min) * time.Minute) }

	msgs := []*message.Message{
		// acct1 inbox：1 未读、1 已读星标
		{AccountID: 1, FolderID: 1, UID: 1, Subject: "i1-unread", Seen: false, Flagged: false, Date: at(10)},
		{AccountID: 1, FolderID: 1, UID: 2, Subject: "i1-read-star", Seen: true, Flagged: true, Date: at(20)},
		// acct1 trash：未读 + 星标（应被 unread/starred 聚合排除）
		{AccountID: 1, FolderID: 2, UID: 1, Subject: "trash-unread-star", Seen: false, Flagged: true, Date: at(30)},
		// acct2 inbox：未读
		{AccountID: 2, FolderID: 3, UID: 1, Subject: "i2-unread", Seen: false, Flagged: false, Date: at(40)},
		// acct2 junk：未读（unread 聚合排除 junk；inbox 聚合也不含 junk）
		{AccountID: 2, FolderID: 4, UID: 1, Subject: "junk-unread", Seen: false, Flagged: false, Date: at(50)},
		// acct1 archive（Gmail 所有邮件镜像）：未读——unread 聚合应排除，避免与收件箱重复计
		{AccountID: 1, FolderID: 5, UID: 1, Subject: "archive-unread", Seen: false, Flagged: false, Date: at(60)},
	}
	for _, m := range msgs {
		if err := repo.Upsert(m); err != nil {
			t.Fatalf("seed msg: %v", err)
		}
	}
}

func TestCountAggregate(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedAggregate(t, repo, db)

	// inbox：各账户收件箱未读 = i1-unread + i2-unread = 2（i1-read-star 已读不计）
	if n, _ := repo.CountAggregate("inbox"); n != 2 {
		t.Errorf("inbox count = %d, want 2", n)
	}
	// unread：只计收件箱+自定义（排除 trash/junk/archive/sent/drafts）= i1-unread + i2-unread = 2
	if n, _ := repo.CountAggregate("unread"); n != 2 {
		t.Errorf("unread count = %d, want 2", n)
	}
	// starred：星标但排除 trash = i1-read-star = 1（trash-unread-star 被排除）
	if n, _ := repo.CountAggregate("starred"); n != 1 {
		t.Errorf("starred count = %d, want 1", n)
	}
}

func TestListAggregateUnreadExcludesTrashJunk(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedAggregate(t, repo, db)

	list, err := repo.ListAggregate("unread", nil, 0, 50, message.Filter{})
	if err != nil {
		t.Fatalf("ListAggregate: %v", err)
	}
	subjects := make([]string, 0, len(list))
	for _, m := range list {
		subjects = append(subjects, m.Subject)
	}
	if len(list) != 2 {
		t.Fatalf("unread list = %v, want 2 items", subjects)
	}
	// date DESC：i2-unread(40min) 在 i1-unread(10min) 之前
	if list[0].Subject != "i2-unread" || list[1].Subject != "i1-unread" {
		t.Errorf("order wrong: %v", subjects)
	}
}

// TestAggregateDedupesGmailLabelCopies 覆盖 Gmail 标签副本去重：
// 同一封邮件在 INBOX / 所有邮件 / 标签文件夹各存一行，聚合里只应出现一次（取收件箱那份）；
// 而复用同一个 Message-ID 的同线程不同邮件（GitHub 通知就是这样）必须各自保留。
func TestAggregateDedupesGmailLabelCopies(t *testing.T) {
	repo, db := newRepoWithDB(t)
	folders := []folder.Folder{
		{ID: 1, AccountID: 1, Path: "INBOX", DisplayName: "Inbox", Type: "inbox", Selectable: true},
		{ID: 2, AccountID: 1, Path: "[Gmail]/All Mail", DisplayName: "所有邮件", Type: "archive", Selectable: true},
		{ID: 3, AccountID: 1, Path: "重要", DisplayName: "重要", Type: "custom", Selectable: true},
	}
	if err := db.Create(&folders).Error; err != nil {
		t.Fatalf("seed folders: %v", err)
	}

	base := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	at := func(min int) time.Time { return base.Add(time.Duration(min) * time.Minute) }

	// copies 造出「一封邮件三份标签副本」：message_id/date/size 相同，folder/uid 不同。
	copies := func(msgID, subject string, date time.Time, size int64, uidBase uint32) []*message.Message {
		out := make([]*message.Message, 0, 3)
		for i, fid := range []uint{1, 2, 3} {
			out = append(out, &message.Message{
				AccountID: 1, FolderID: fid, UID: uidBase + uint32(i),
				MessageID: msgID, Subject: subject, Size: size, Seen: false, Date: date,
			})
		}
		return out
	}

	var msgs []*message.Message
	// 同一个 Message-ID 下的两封不同邮件（日期/大小不同），各有三份副本。
	msgs = append(msgs, copies("thread@github.com", "pr-comment-1", at(10), 1000, 10)...)
	msgs = append(msgs, copies("thread@github.com", "pr-comment-2", at(20), 2000, 20)...)
	// 无 Message-ID 的两封：不参与去重，各自独立展示。
	msgs = append(msgs,
		&message.Message{AccountID: 1, FolderID: 1, UID: 30, Subject: "no-msgid-1", Seen: false, Date: at(30)},
		&message.Message{AccountID: 1, FolderID: 1, UID: 31, Subject: "no-msgid-2", Seen: false, Date: at(40)},
	)
	for _, m := range msgs {
		if err := repo.Upsert(m); err != nil {
			t.Fatalf("seed msg: %v", err)
		}
	}

	list, err := repo.ListAggregate("unread", nil, 0, 50, message.Filter{})
	if err != nil {
		t.Fatalf("ListAggregate: %v", err)
	}
	subjects := make([]string, 0, len(list))
	for _, m := range list {
		subjects = append(subjects, m.Subject)
	}
	// 6 行副本折叠成 2 封 + 2 封无 Message-ID = 4
	if len(list) != 4 {
		t.Fatalf("unread list = %v, want 4 items", subjects)
	}
	for _, m := range list {
		if m.MessageID != "" && m.FolderID != 1 {
			t.Errorf("%s 的代表行在 folder %d，应取收件箱那份", m.Subject, m.FolderID)
		}
	}
	if n, _ := repo.CountAggregate("unread"); n != 4 {
		t.Errorf("unread count = %d, want 4（须与列表条数一致）", n)
	}

	// 账户未读走同一口径，必须与聚合计数一致，否则侧栏角标与聚合入口互相矛盾。
	counts, err := repo.AccountUnreadCounts()
	if err != nil {
		t.Fatalf("AccountUnreadCounts: %v", err)
	}
	if counts[1] != 4 {
		t.Errorf("account 1 unread = %d, want 4", counts[1])
	}
}

func TestListAggregateKeysetPaging(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedAggregate(t, repo, db)

	// inbox 聚合（收件箱全部邮件，无未读过滤）= i1-unread, i1-read-star, i2-unread 共 3 封
	page1, err := repo.ListAggregate("inbox", nil, 0, 2, message.Filter{})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 len = %d, want 2", len(page1))
	}
	// date DESC：i2-unread(40) > i1-read-star(20) > i1-unread(10)
	if page1[0].Subject != "i2-unread" || page1[1].Subject != "i1-read-star" {
		t.Fatalf("page1 order wrong: %s,%s", page1[0].Subject, page1[1].Subject)
	}
	last := page1[len(page1)-1]
	page2, err := repo.ListAggregate("inbox", &last.Date, last.ID, 2, message.Filter{})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2) != 1 || page2[0].Subject != "i1-unread" {
		t.Fatalf("page2 wrong: %+v", page2)
	}
}
