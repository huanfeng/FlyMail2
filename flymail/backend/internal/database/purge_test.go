package database

import (
	"testing"
	"time"

	"flymail/modules/email/account"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"

	"gorm.io/gorm"
)

// 删账户必须连带清掉它名下的全部数据。
//
// ── 缘起（2026-09-16） ───────────────────────────────────────────────────────
//
// 删掉一个 QQ 账户之后，库里留下 1579 封邮件和 14 个文件夹：原先的 Delete 只删
// accounts 行加别名、签名。这些行在界面上完全不可见（没有账户就没有入口），
// 属于「不报错、不影响使用、只是一直占着空间」的那类缺陷——删两次就积了 3158 封。
//
// 这条用例在**完整 schema** 上跑，因为级联删除是按表名写的裸 SQL
// （account 包不能 import message/folder，会成环），只有真库能验证表名没写错。
func TestDeleteAccountPurgesItsData(t *testing.T) {
	db := freshDB(t)
	repo := account.NewRepository(db)

	keep := seedAccount(t, db, "keep@example.com")
	drop := seedAccount(t, db, "drop@example.com")

	countFor := func(id uint) (msgs, folders, bodies, atts int64) {
		db.Model(&message.Message{}).Where("account_id = ?", id).Count(&msgs)
		db.Model(&folder.Folder{}).Where("account_id = ?", id).Count(&folders)
		db.Table("message_bodies").
			Where("message_id IN (SELECT id FROM messages WHERE account_id = ?)", id).Count(&bodies)
		db.Table("attachments").
			Where("message_id IN (SELECT id FROM messages WHERE account_id = ?)", id).Count(&atts)
		return
	}

	if m, f, b, a := countFor(drop); m == 0 || f == 0 || b == 0 || a == 0 {
		t.Fatalf("前提不成立，种子数据没建全：邮件=%d 文件夹=%d 正文=%d 附件=%d", m, f, b, a)
	}

	if err := repo.Delete(drop); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if m, f, b, a := countFor(drop); m+f+b+a != 0 {
		t.Errorf("删号之后还留着孤儿行：邮件=%d 文件夹=%d 正文=%d 附件=%d", m, f, b, a)
	}
	// ⚠ 另一个方向：别把别的账户一起删了
	if m, f, b, a := countFor(keep); m == 0 || f == 0 || b == 0 || a == 0 {
		t.Errorf("误删了其它账户的数据：邮件=%d 文件夹=%d 正文=%d 附件=%d", m, f, b, a)
	}
}

// 老库里已经存在的孤儿行要被扫掉。
//
// 级联删除是后加的，在那之前删过账户的库里都留着数据。Migrate 每次启动跑一遍
// PurgeOrphans，这条钉的就是它。
func TestPurgeOrphansCleansPreexistingRows(t *testing.T) {
	db := freshDB(t)

	keep := seedAccount(t, db, "keep@example.com")
	ghost := seedAccount(t, db, "ghost@example.com")
	// 模拟老库：只删 accounts 行，数据全留着（这正是修复前 Delete 的行为）
	if err := db.Exec("DELETE FROM accounts WHERE id = ?", ghost).Error; err != nil {
		t.Fatalf("模拟老库删除失败：%v", err)
	}

	var before int64
	db.Model(&message.Message{}).Where("account_id = ?", ghost).Count(&before)
	if before == 0 {
		t.Fatal("前提不成立：模拟出来的孤儿行是 0")
	}

	if err := account.PurgeOrphans(db); err != nil {
		t.Fatalf("PurgeOrphans: %v", err)
	}

	var after, keepMsgs, orphanBodies int64
	db.Model(&message.Message{}).Where("account_id = ?", ghost).Count(&after)
	db.Model(&message.Message{}).Where("account_id = ?", keep).Count(&keepMsgs)
	db.Table("message_bodies").
		Where("message_id NOT IN (SELECT id FROM messages)").Count(&orphanBodies)

	if after != 0 {
		t.Errorf("孤儿邮件没清干净，还剩 %d 封", after)
	}
	if orphanBodies != 0 {
		t.Errorf("正文成了孤儿，还剩 %d 条", orphanBodies)
	}
	if keepMsgs == 0 {
		t.Error("把还在的账户的邮件也清掉了")
	}
}

// ⚠ 新增带 account_id 的表时，必须同时加进 account.OwnedTables()。
//
// 漏了不会报错，只会在下一次删账户时悄悄留下孤儿——正是上面那个缺陷的复发方式。
// 这条扫 AutoMigrate 建出来的所有表，凡带 account_id 的都要在清单里。
func TestOwnedTablesCoversEveryAccountScopedTable(t *testing.T) {
	db := freshDB(t)

	owned := map[string]bool{}
	for _, name := range account.OwnedTables() {
		owned[name] = true
		if !db.Migrator().HasTable(name) {
			t.Errorf("清单里的表 %q 在库里不存在（表名写错了？写错会被静默跳过）", name)
		}
	}

	var tables []string
	if err := db.Raw(
		`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE '%_fts%'`,
	).Scan(&tables).Error; err != nil {
		t.Fatalf("列表名失败：%v", err)
	}
	if len(tables) < 10 {
		t.Fatalf("只扫到 %d 张表，迁移没跑全，下面的断言没有意义", len(tables))
	}

	for _, name := range tables {
		if name == "accounts" || owned[name] {
			continue
		}
		var cols []struct {
			Name string `gorm:"column:name"`
		}
		if err := db.Raw("SELECT name FROM pragma_table_info(?)", name).Scan(&cols).Error; err != nil {
			t.Fatalf("读 %s 的列失败：%v", name, err)
		}
		for _, c := range cols {
			if c.Name == "account_id" {
				t.Errorf("表 %q 带 account_id 却不在 account.OwnedTables() 里——删账户会给它留孤儿行", name)
			}
		}
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────

// seedAccount 建一个账户，并在它名下放文件夹、邮件、正文、附件各一份。
func seedAccount(t *testing.T, db *gorm.DB, email string) uint {
	t.Helper()
	acc := account.Account{Email: email, Name: email, IMAPHost: "imap.example.com", IMAPPort: 993}
	if err := db.Create(&acc).Error; err != nil {
		t.Fatalf("建账户：%v", err)
	}
	f := folder.Folder{AccountID: acc.ID, Path: "INBOX", DisplayName: "INBOX", Type: "inbox", Selectable: true}
	if err := db.Create(&f).Error; err != nil {
		t.Fatalf("建文件夹：%v", err)
	}
	m := message.Message{
		AccountID: acc.ID, FolderID: f.ID, UID: 1,
		Subject: "seed", FromAddr: email, Date: time.Now().UTC(),
	}
	if err := db.Create(&m).Error; err != nil {
		t.Fatalf("建邮件：%v", err)
	}
	if err := db.Create(&message.MessageBody{MessageID: m.ID, TextBody: "hello"}).Error; err != nil {
		t.Fatalf("建正文：%v", err)
	}
	if err := db.Create(&message.Attachment{MessageID: m.ID, Filename: "a.pdf", Size: 1}).Error; err != nil {
		t.Fatalf("建附件：%v", err)
	}
	return acc.ID
}
