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
// folders: 1=acct1/inbox 2=acct1/trash 3=acct2/inbox 4=acct2/junk
func seedAggregate(t *testing.T, repo *message.Repository, db *gorm.DB) {
	t.Helper()
	folders := []folder.Folder{
		{ID: 1, AccountID: 1, Path: "INBOX", DisplayName: "Inbox", Type: "inbox", Selectable: true},
		{ID: 2, AccountID: 1, Path: "Trash", DisplayName: "Trash", Type: "trash", Selectable: true},
		{ID: 3, AccountID: 2, Path: "INBOX", DisplayName: "Inbox", Type: "inbox", Selectable: true},
		{ID: 4, AccountID: 2, Path: "Junk", DisplayName: "Junk", Type: "junk", Selectable: true},
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
	// unread：全部未读但排除 trash/junk = i1-unread + i2-unread = 2
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

	list, err := repo.ListAggregate("unread", nil, 0, 50)
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

func TestListAggregateKeysetPaging(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedAggregate(t, repo, db)

	// inbox 聚合（收件箱全部邮件，无未读过滤）= i1-unread, i1-read-star, i2-unread 共 3 封
	page1, err := repo.ListAggregate("inbox", nil, 0, 2)
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
	page2, err := repo.ListAggregate("inbox", &last.Date, last.ID, 2)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2) != 1 || page2[0].Subject != "i1-unread" {
		t.Fatalf("page2 wrong: %+v", page2)
	}
}
