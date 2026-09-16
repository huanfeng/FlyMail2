package account

import "gorm.io/gorm"

// 删账户时连带清理它名下的全部数据。
//
// ── 缘起 ─────────────────────────────────────────────────────────────────────
//
// 2026-09-16：删掉一个 QQ 账户之后，库里留下 1579 封邮件和 14 个文件夹。原先的
// Delete 只删 accounts 行加别名、签名，其余表一概不管。这些行在界面上完全不可见
// （没有账户就没有入口），只是白占空间——删两次就积了 3158 封。
//
// ── 为什么用裸 SQL 写表名 ───────────────────────────────────────────────────
//
// account 包不能 import message / folder / draft 那几个包（它们反过来依赖账户，
// 会成环）。跨模块的级联删除要么靠这里写表名，要么在上层编一套注册机制。
// 表就这么几张、且都在同一个 AutoMigrate 列表里，写表名是更实在的选择——
// 代价是加新表时要记得回来加一行，所以 accountOwnedTables 上有守卫测试。
//
// ── ⚠ 删除顺序有讲究 ────────────────────────────────────────────────────────
//
// attachments / message_bodies 是靠 message_id 挂在邮件上的，**必须在 messages
// 之前删**：messages 先没了，那两张表的子查询就选不出任何行，它们当场变成孤儿。
//
// 全文索引由触发器维护（messages_fts_ad 等），走 SQL 删除会正常触发，不用另管。

// accountOwnedTables 是按 account_id 归属账户的表。
//
// ⚠ 新增带 account_id 的表时必须加到这里，否则删账户又会留下孤儿行。
// 守卫见 purge_test.go：它扫 AutoMigrate 的模型，发现带 account_id 却不在表里就报错。
var accountOwnedTables = []string{
	"messages",
	"folders",
	"drafts",
	"inbox_rules",
	"rule_runs",
	"writeback_ops",
	"notifications",
	"account_aliases",
	"account_signatures",
}

// OwnedTables 返回按 account_id 归属账户的表名，供跨包的守卫测试核对。
func OwnedTables() []string { return append([]string(nil), accountOwnedTables...) }

// purgeAccountData 清理一个账户名下的全部数据行（不含 accounts 行本身）。
// 在调用方的事务里执行。
func purgeAccountData(tx *gorm.DB, accountID uint) error {
	if err := purgeMessageChildren(tx, "message_id IN (SELECT id FROM messages WHERE account_id = ?)", accountID); err != nil {
		return err
	}
	for _, t := range accountOwnedTables {
		if !tx.Migrator().HasTable(t) {
			continue // 只迁了一部分模型的场景（单元测试）不该因此失败
		}
		if err := tx.Exec("DELETE FROM "+t+" WHERE account_id = ?", accountID).Error; err != nil {
			return err
		}
	}
	return nil
}

// PurgeOrphans 清掉「账户已经不存在、数据还留着」的行。
//
// 给老库用：级联删除是 2026-09-16 才加的，在那之前删过账户的库里都有孤儿。
// 每次启动跑一次，正常库上是几条 DELETE 扫一遍索引，代价可以忽略。
func PurgeOrphans(db *gorm.DB) error {
	const orphanMsgs = "message_id IN (SELECT id FROM messages WHERE account_id NOT IN (SELECT id FROM accounts))"
	if err := purgeMessageChildren(db, orphanMsgs); err != nil {
		return err
	}
	for _, t := range accountOwnedTables {
		if !db.Migrator().HasTable(t) {
			continue
		}
		if err := db.Exec("DELETE FROM " + t + " WHERE account_id NOT IN (SELECT id FROM accounts)").Error; err != nil {
			return err
		}
	}
	// 邮件行没了但正文/附件还挂着的（历史遗留，或将来某条路径漏删）一并扫掉
	return purgeMessageChildren(db, "message_id NOT IN (SELECT id FROM messages)")
}

// purgeMessageChildren 删掉按 message_id 挂在邮件上的子表行。
func purgeMessageChildren(db *gorm.DB, where string, args ...any) error {
	for _, t := range []string{"attachments", "message_bodies"} {
		if !db.Migrator().HasTable(t) {
			continue
		}
		if err := db.Exec("DELETE FROM "+t+" WHERE "+where, args...).Error; err != nil {
			return err
		}
	}
	return nil
}
