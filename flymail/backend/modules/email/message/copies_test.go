package message_test

import (
	"path/filepath"
	"testing"
	"time"

	"flymail/internal/database"
	"flymail/modules/email/message"

	"gorm.io/gorm"
)

// 同一封邮件的标签副本要一起改状态。
//
// ── 缘起（用户报的：[Gmail]/重要 里的邮件标不掉未读） ────────────────────────
//
// Gmail 把标签映射成 IMAP 文件夹，一封邮件在 INBOX、[Gmail]/所有邮件、各标签下
// 各有一行。原先标已读只更新被点的那一行，于是本地分裂成：
//
//	INBOX            seen=1   ← 读过的那份
//	[Gmail]/重要      seen=0   ← 这里一直显示未读
//
// 而会话统计走 dedupeSameMessage，代表行按「收件箱优先」取到已读那份，算出
// unread=0。结果是文件夹角标说有未读、单封列表筛得出未读，会话右键却只给
// 「全部标为未读」——想标已读根本没有入口。
func TestWithCopiesFindsLabelCopies(t *testing.T) {
	db, svc := newCopiesDB(t)
	when := time.Date(2026, 9, 17, 5, 42, 30, 0, time.UTC)

	inbox := seedCopy(t, db, 3, 1, "gh-134@github.com", when, 12906)
	all := seedCopy(t, db, 3, 2, "gh-134@github.com", when, 12906)
	important := seedCopy(t, db, 3, 3, "gh-134@github.com", when, 12906)

	got, err := svc.WithCopies([]uint{inbox})
	if err != nil {
		t.Fatalf("WithCopies: %v", err)
	}
	assertSameSet(t, got, []uint{inbox, all, important},
		"只点了收件箱那份，另外两个标签下的副本没被带上")
}

// ⚠ 判据不能只看 Message-ID。
//
// GitHub 一类发信端会给同一会话的多封通知**复用同一个 Message-ID**（好让客户端
// 归拢成一个线程），实测一个 id 下挂着 3 封日期与内容都不同的邮件。
// 只按 Message-ID 合并，用户标一封已读会把整串通知都标掉。
func TestWithCopiesRefusesToMergeDifferentMails(t *testing.T) {
	db, svc := newCopiesDB(t)
	shared := "huanfeng/WindInput/issues/134@github.com"

	first := seedCopy(t, db, 3, 1, shared, time.Date(2026, 9, 17, 5, 0, 0, 0, time.UTC), 12906)
	// 同一个 Message-ID，但日期和大小都不同——这是另一封邮件，不是副本
	other := seedCopy(t, db, 3, 1, shared, time.Date(2026, 9, 17, 9, 30, 0, 0, time.UTC), 20481)

	got, err := svc.WithCopies([]uint{first})
	if err != nil {
		t.Fatalf("WithCopies: %v", err)
	}
	assertSameSet(t, got, []uint{first}, "把同 Message-ID 的另一封邮件当成了副本")
	if contains(got, other) {
		t.Error("标一封已读会把整串 GitHub 通知一起标掉")
	}
}

// 跨账户不能混。两个账户各自收到同一封群发邮件时，message_id/date/size 会完全一样。
func TestWithCopiesStaysInsideAccount(t *testing.T) {
	db, svc := newCopiesDB(t)
	when := time.Date(2026, 9, 17, 5, 42, 30, 0, time.UTC)

	mine := seedCopy(t, db, 3, 1, "newsletter@example.com", when, 4096)
	otherAccount := seedCopy(t, db, 4, 9, "newsletter@example.com", when, 4096)

	got, err := svc.WithCopies([]uint{mine})
	if err != nil {
		t.Fatalf("WithCopies: %v", err)
	}
	assertSameSet(t, got, []uint{mine}, "把另一个账户的同名邮件当成了副本")
	if contains(got, otherAccount) {
		t.Error("在一个账户里标已读，改掉了另一个账户的邮件")
	}
}

// message_id 为空的邮件（少数不合规发信端）不参与去重，各自独立。
func TestWithCopiesIgnoresEmptyMessageID(t *testing.T) {
	db, svc := newCopiesDB(t)
	when := time.Date(2026, 9, 17, 5, 42, 30, 0, time.UTC)

	a := seedCopy(t, db, 3, 1, "", when, 512)
	b := seedCopy(t, db, 3, 2, "", when, 512)

	got, err := svc.WithCopies([]uint{a})
	if err != nil {
		t.Fatalf("WithCopies: %v", err)
	}
	assertSameSet(t, got, []uint{a}, "没有 Message-ID 的邮件被错误地合并了")
	if contains(got, b) {
		t.Error("两封无 Message-ID 的邮件被当成了同一封")
	}
}

func TestWithCopiesEmptyInput(t *testing.T) {
	_, svc := newCopiesDB(t)
	got, err := svc.WithCopies(nil)
	if err != nil {
		t.Fatalf("WithCopies(nil): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("空输入应当返回空，拿到 %v", got)
	}
}

// ── 夹具 ────────────────────────────────────────────────────────────────────

func newCopiesDB(t *testing.T) (*gorm.DB, *message.Service) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	return db, message.NewService(message.NewRepository(db), message.NewBodyRepository(db))
}

// seedCopy 插一封邮件，返回它的 id。folderID 不同即模拟不同标签下的副本。
func seedCopy(t *testing.T, db *gorm.DB, accountID, folderID uint, msgID string, date time.Time, size int64) uint {
	t.Helper()
	m := &message.Message{
		AccountID: accountID, FolderID: folderID, UID: uidSeq(),
		MessageID: msgID, Date: date, Size: size,
		Subject: "t", FromAddr: "a@x.com",
	}
	if err := db.Create(m).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	return m.ID
}

var uidCounter uint32

func uidSeq() uint32 { uidCounter++; return uidCounter }

func contains(ids []uint, id uint) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func assertSameSet(t *testing.T, got, want []uint, msg string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s：拿到 %v，想要 %v", msg, got, want)
		return
	}
	for _, w := range want {
		if !contains(got, w) {
			t.Errorf("%s：%v 里缺了 %d（想要 %v）", msg, got, w, want)
		}
	}
}
