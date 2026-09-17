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

	// ⚠ 必须绕开驱动读真实存储，见 rawDate 的说明。
	raw := rawDate(t, db, 1)
	// ⚠ 必须与 EnsureUTCDates 迁移出来的形态**完全一致**，否则新旧行混在一列里，
	// 空格与 T（0x20 / 0x54）不同序，排序照样乱。
	if raw != "2026-09-16 11:09:44+00:00" {
		t.Errorf("落库形态不对：%q（想要 2026-09-16 11:09:44+00:00）", raw)
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

// rawDate 读出 date 列的**真实存储文本**。
//
// ⚠ 不能用 Pluck / Scan 到 string——驱动在读取时会把带偏移的文本归一化成
// `...T...Z`，两种截然不同的存储形态看起来完全一样。上一版的测试正是这么写的，
// 所以库里混着两种表示、新邮件排到旧邮件后面，它却一直是绿的。
// `|| ”` 让 SQLite 按字符串求值，绕过驱动的日期类型转换。
func rawDate(t *testing.T, db *gorm.DB, uid int) string {
	t.Helper()
	var raw string
	if err := db.Raw("SELECT date || '' FROM messages WHERE uid = ?", uid).Scan(&raw).Error; err != nil {
		t.Fatalf("读真实存储失败：%v", err)
	}
	return raw
}

// ⚠⚠ 规范形态必须等于**驱动写 UTC 时间时产出的**文本。
//
// 这是排序错乱的根：EnsureUTCDates 只在**启动时**跑一次，之后每封新邮件都按驱动的
// 形态落库。两者只要不一致，库里就会长期混着两种表示——按字节序排序时新邮件反而
// 排在旧邮件后面（空格 0x20 < T 0x54），而按 date 的范围比较（分页游标、时间筛选）
// 也会失准，因为绑定参数走的是驱动。
//
// ⚠ 测试必须复现这个**时序**：先迁移、再写新行。
// 在同一次运行里「写一行、改成旧形态、跑迁移」是测不出来的——迁移会把驱动写的那行
// 也一并规整掉，两行自然一致。真实场景里新邮件是在迁移之后才落库的，没人再规整它。
func TestNewRowsKeepMigrationFormat(t *testing.T) {
	db := freshDB(t)
	folderID := seedFolder(t, db, "fmt@example.com")
	at := time.Date(2026, 9, 16, 11, 9, 44, 0, time.UTC)
	repo := message.NewRepository(db)

	// ① 老库里的一行，形态五花八门
	if err := repo.Upsert(&message.Message{
		AccountID: 1, FolderID: folderID, UID: 101, Subject: "old", FromAddr: "a@x.com", Date: at,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := db.Exec(`UPDATE messages SET date = '2026-09-16T19:09:44+08:00' WHERE uid = 101`).Error; err != nil {
		t.Fatalf("改成旧形态失败：%v", err)
	}

	// ② 启动时跑一次迁移
	if err := message.EnsureUTCDates(db); err != nil {
		t.Fatalf("EnsureUTCDates: %v", err)
	}
	migrated := rawDate(t, db, 101)

	// ③ 之后新到的邮件——这一封不会再被任何迁移碰到
	if err := repo.Upsert(&message.Message{
		AccountID: 1, FolderID: folderID, UID: 102, Subject: "new", FromAddr: "a@x.com", Date: at,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	fresh := rawDate(t, db, 102)

	if migrated != fresh {
		t.Errorf("迁移后的老行与新到的邮件形态不一致：\n  迁移写的   %q\n  新邮件     %q\n"+
			"两种表示混在同一列里，字节序排序会把新邮件排到旧邮件后面", migrated, fresh)
	}
}

// 老库里混着两种表示时，迁移要能把排序收拾正确。
//
// 上面那条防的是「将来再次分裂」，这条管的是「已经分裂的库升级上来」——
// 用户当前的库就是这个状态：18851 行 T...Z 加 6 行带偏移，最新的邮件排在了最后。
func TestMigrationFixesMixedFormatOrdering(t *testing.T) {
	db := freshDB(t)
	folderID := seedFolder(t, db, "mix@example.com")
	repo := message.NewRepository(db)

	// 早到的一封，用 T...Z 表示
	if err := repo.Upsert(&message.Message{
		AccountID: 1, FolderID: folderID, UID: 1, Subject: "早到的",
		FromAddr: "a@x.com", Date: time.Date(2026, 9, 17, 6, 20, 59, 0, time.UTC),
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := db.Exec(`UPDATE messages SET date = '2026-09-17T06:20:59Z' WHERE uid = 1`).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	// 晚到的一封，走驱动的正常写入
	if err := repo.Upsert(&message.Message{
		AccountID: 1, FolderID: folderID, UID: 2, Subject: "晚到的",
		FromAddr: "a@x.com", Date: time.Date(2026, 9, 17, 8, 51, 7, 0, time.UTC),
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// 此刻库里是混合状态，排序本来就是错的——那正是这个 bug 的样子，不必断言。
	// 要钉的是「迁移能把它收拾干净」。
	if err := message.EnsureUTCDates(db); err != nil {
		t.Fatalf("EnsureUTCDates: %v", err)
	}
	var got []string
	if err := db.Model(&message.Message{}).Where("folder_id = ?", folderID).
		Order("date DESC").Pluck("subject", &got).Error; err != nil {
		t.Fatalf("查询失败：%v", err)
	}
	if len(got) != 2 || got[0] != "晚到的" {
		t.Fatalf("迁移之后排序仍然错：%v\n晚到的邮件排在后面，说明两种表示没被收敛成一种", got)
	}
}
