package folder

import (
	"flymail/modules/email/account"

	"gorm.io/gorm"
)

// 清理服务端已经不存在的文件夹。
//
// ── 缘起 ─────────────────────────────────────────────────────────────────────
//
// SyncFolders 原先只 Upsert、从不删除。用户在网页端删掉一个文件夹之后，本地那行
// 一直留着，每一轮同步都会去 SELECT 它、每一轮都失败：测试账户上两天刷了 8876 次
// `NO SELECT failed. No such mailbox`。除了刷日志，侧栏里还挂着一堆点进去就报错
// 的文件夹。
//
// ── ⚠ 为什么这段代码要格外小心 ──────────────────────────────────────────────
//
// 它是这个项目里**少数会不可逆地删用户数据**的路径：判断依据是一次 IMAP LIST 的
// 返回值，而 LIST 会因为连接抖动、权限变化、服务端故障返回空或残缺的结果。
// 照单全收的话，一次抖动就能把某个账户的本地邮件全部删掉，而用户对此毫无察觉——
// 直到他发现邮件没了，且本地没有任何地方能恢复。
//
// 所以护栏是「宁可留着陈旧文件夹」：空列表一律不删（见 SyncFolders）。
// 留着的代价只是日志里多几行错误，删错的代价是数据没了，两者不对称。

// pruneMissing 删掉本地有、而 keep 里没有的文件夹，连带它们的邮件。
//
// keep 的键是 IMAP 路径。返回被删掉的文件夹数量。
//
// ⚠ 调用方必须保证 keep 不为空：空的 keep 在这里会被理解成「服务端一个文件夹都
// 没有」，从而删光该账户的全部本地邮件。这个判断留在 SyncFolders 里做，那里才
// 知道 LIST 是真的返回了空，还是压根没查成功。
func (r *Repository) pruneMissing(accountID uint, keep map[string]bool) (int, error) {
	var stale []Folder
	if err := r.db.Where("account_id = ?", accountID).Find(&stale).Error; err != nil {
		return 0, err
	}
	ids := make([]uint, 0)
	for i := range stale {
		if !keep[stale[i].Path] {
			ids = append(ids, stale[i].ID)
		}
	}
	if len(ids) == 0 {
		return 0, nil
	}

	err := r.db.Transaction(func(tx *gorm.DB) error {
		// ⚠ 顺序有讲究：attachments / message_bodies / message_translations 靠
		// message_id 挂在邮件上，必须**在 messages 之前删**——messages 先没了，
		// 那几张表的子查询就选不出任何行，它们当场变成永远没人认领的孤儿。
		// 表名从 account 包取同一份名单，免得新增子表时这里漏掉一处。
		// 全文索引由触发器维护，走 SQL 删除会正常触发，不用另管。
		const sub = "SELECT id FROM messages WHERE folder_id IN (?)"
		for _, t := range account.MessageOwnedTables() {
			if !tx.Migrator().HasTable(t) {
				continue
			}
			if err := tx.Exec("DELETE FROM "+t+" WHERE message_id IN ("+sub+")", ids).Error; err != nil {
				return err
			}
		}
		if tx.Migrator().HasTable("messages") {
			if err := tx.Exec("DELETE FROM messages WHERE folder_id IN (?)", ids).Error; err != nil {
				return err
			}
		}
		return tx.Where("id IN ?", ids).Delete(&Folder{}).Error
	})
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}
