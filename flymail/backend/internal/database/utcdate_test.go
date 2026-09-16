package database

import (
	"path/filepath"
	"testing"
	"time"

	"flymail/modules/email/account"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"
	syncmod "flymail/modules/email/sync"

	"gorm.io/gorm"
)

// 跨服务商的邮件必须按真实时刻排序。
//
// ── 缘起（2026-09-16 用户报「新收到的 GitHub 邮件排在 QQ 的旧通知后面」） ─────
//
// date 是 TEXT 列，写进去的文本形态取决于 time.Time 自带的时区，于是同一个库里
// 并存好几种串：
//
//	2026-09-16 19:09:44+08:00   QQ / 163 返回 +0800，实际是 19:09
//	2026-09-16 12:37:35+00:00   Gmail 返回 UTC，实际是北京时间 20:37
//	2026-09-16T11:09:44.5Z      驱动对 UTC 时间写的是 RFC3339，还带可变小数秒
//
// 排序按**字节序**：`19:` > `12:`，于是晚一个半小时到的 GitHub 邮件被排在后面；
// 空格与 `T` 也不同序。只用一个服务商时看不出来，多账户一混就乱
// （实测库里 10321 行 +00:00、8568 行 +08:00）。
//
// 修法是落库与迁移都规范成 `2026-09-16T11:09:44Z` 这一种形态。
func TestMessagesSortByRealInstantAcrossTimezones(t *testing.T) {
	db := freshDB(t)
	folderID := seedFolder(t, db, "mix@example.com")

	// 用户现场的两封：GitHub 那封晚 88 分钟到，必须排在前面
	beijing := time.FixedZone("CST", 8*3600)
	qq := time.Date(2026, 9, 16, 19, 9, 44, 0, beijing)
	github := time.Date(2026, 9, 16, 12, 37, 35, 0, time.UTC) // = 北京时间 20:37:35

	repo := message.NewRepository(db)
	for i, tc := range []struct {
		subject string
		at      time.Time
	}{
		{"QQ 的登录提醒", qq},
		{"GitHub 的新回复", github},
	} {
		m := &message.Message{
			AccountID: 1, FolderID: folderID, UID: uint32(i + 1),
			Subject: tc.subject, FromAddr: "a@x.com", Date: tc.at,
		}
		if err := repo.Upsert(m); err != nil {
			t.Fatalf("Upsert %s: %v", tc.subject, err)
		}
	}

	var got []string
	if err := db.Model(&message.Message{}).
		Order("date DESC").Pluck("subject", &got).Error; err != nil {
		t.Fatalf("查询失败：%v", err)
	}
	if len(got) != 2 || got[0] != "GitHub 的新回复" {
		t.Fatalf("排序错了：%v\n晚到的邮件被排在后面，说明 date 还在按带偏移的文本比字节序", got)
	}
}

// 落库的表示统一成 UTC，且时刻本身不能被改动。
//
// 「统一时区」写成「把偏移量直接抹掉」也能让上面那条通过，代价是时间整体偏移 8 小时。
func TestUpsertStoresUTCWithoutShiftingTheInstant(t *testing.T) {
	db := freshDB(t)
	folderID := seedFolder(t, db, "tz@example.com")

	beijing := time.FixedZone("CST", 8*3600)
	at := time.Date(2026, 9, 16, 19, 9, 44, 0, beijing)

	repo := message.NewRepository(db)
	if err := repo.Upsert(&message.Message{
		AccountID: 1, FolderID: folderID, UID: 1,
		Subject: "tz", FromAddr: "a@x.com", Date: at,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	var raw string
	if err := db.Model(&message.Message{}).Where("uid = 1").Pluck("date", &raw).Error; err != nil {
		t.Fatalf("取原始文本失败：%v", err)
	}
	// ⚠ 必须与 EnsureUTCDates 迁移出来的形态**完全一致**，否则新旧行混在一列里，
	// 空格与 T（0x20 / 0x54）不同序，排序照样乱。
	if raw != "2026-09-16T11:09:44Z" {
		t.Errorf("落库形态不对：%q（想要 2026-09-16T11:09:44Z）", raw)
	}

	var back message.Message
	if err := db.Where("uid = 1").First(&back).Error; err != nil {
		t.Fatalf("读回失败：%v", err)
	}
	if !back.Date.Equal(at) {
		t.Errorf("时刻被改动了：存进去 %v，读回来 %v", at, back.Date)
	}
}

// 老库里已有的带偏移行要被迁移成 UTC。
//
// Migrate 每次启动跑 EnsureUTCDates；没有它的话，升级之前同步下来的邮件
// 会一直用旧表示，排序照样乱。
func TestEnsureUTCDatesMigratesExistingRows(t *testing.T) {
	db := freshDB(t)
	folderID := seedFolder(t, db, "old@example.com")

	// 直接写入旧格式，模拟升级前落库的行
	if err := db.Exec(`INSERT INTO messages
	        (account_id, folder_id, uid, subject, from_addr, date,
	         seen, flagged, answered, deleted, has_attachment, body_synced, size,
	         created_at, updated_at)
	        VALUES (1, ?, 1, '旧行', 'a@x.com', '2026-09-16 19:09:44+08:00',
	                0, 0, 0, 0, 0, 0, 0,
	                '2026-09-16 19:09:44+08:00', '2026-09-16 19:09:44+08:00')`,
		folderID).Error; err != nil {
		t.Fatalf("插入旧行失败：%v", err)
	}

	if err := message.EnsureUTCDates(db); err != nil {
		t.Fatalf("EnsureUTCDates: %v", err)
	}

	var raw string
	if err := db.Model(&message.Message{}).Where("uid = 1").Pluck("date", &raw).Error; err != nil {
		t.Fatalf("取原始文本失败：%v", err)
	}
	// 19:09:44+08:00 == 11:09:44 UTC
	if raw != "2026-09-16T11:09:44Z" {
		t.Errorf("迁移结果不对：%q（想要 2026-09-16T11:09:44Z）", raw)
	}

	// ⚠ 反复跑要幂等：Migrate 每次启动都会调
	if err := message.EnsureUTCDates(db); err != nil {
		t.Fatalf("第二次 EnsureUTCDates: %v", err)
	}
	var again string
	db.Model(&message.Message{}).Where("uid = 1").Pluck("date", &again)
	if again != raw {
		t.Errorf("不幂等：再跑一次变成了 %q", again)
	}
}

func freshDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if sqlDB, e := db.DB(); e == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// writeback_ops 不在 database.Migrate 里，由 sync 模块自己迁移
	// （生产上在 app 启动时调）。这里补上，否则守卫测试看不到这张表。
	if err := syncmod.MigrateWriteback(db); err != nil {
		t.Fatalf("MigrateWriteback: %v", err)
	}
	return db
}

func seedFolder(t *testing.T, db *gorm.DB, email string) uint {
	t.Helper()
	acc := account.Account{Email: email, Name: email, IMAPHost: "imap.example.com", IMAPPort: 993}
	if err := db.Create(&acc).Error; err != nil {
		t.Fatalf("建账户：%v", err)
	}
	f := folder.Folder{AccountID: acc.ID, Path: "INBOX", DisplayName: "INBOX", Type: "inbox", Selectable: true}
	if err := db.Create(&f).Error; err != nil {
		t.Fatalf("建文件夹：%v", err)
	}
	return f.ID
}
