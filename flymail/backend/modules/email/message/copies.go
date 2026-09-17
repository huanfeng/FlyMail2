package message

import "gorm.io/gorm"

// 同一封邮件的「标签副本」。
//
// ── 缘起（用户报的：[Gmail]/重要 里的邮件标不掉未读） ────────────────────────
//
// Gmail 把标签映射成 IMAP 文件夹，同一封邮件会同时出现在 INBOX、[Gmail]/所有邮件
// 和每个命中标签的文件夹里，本地是多行（folder_id/uid 各不相同）。
//
// 原先标已读只更新**被点的那一行**。用户在收件箱读了一封，INBOX 那份 seen=1，
// 另外两份还是 0：
//
//	INBOX            seen=1   ← 读过的那份
//	[Gmail]/所有邮件  seen=0
//	[Gmail]/重要      seen=0   ← 这里一直显示未读
//
// 而会话列表的统计走 dedupeSameMessage，代表行按「收件箱 > 自定义 > 其它」取，
// 拿到的正是已读的 INBOX 那份，于是会话 unread=0。结果就是：
// 文件夹未读角标说有未读、单封列表也能用未读筛出来，会话右键却只给「全部标为
// 未读」——想标已读根本没有入口。
//
// Gmail 上 \Seen 是**按邮件**的，服务器上三份本来就一致，分裂纯粹是本地造成的。
//
// 把副本一并纳入之后，本地三行同时更新，回写队列也会按各自的 folder+uid 分别
// STORE——对 Gmail 是幂等的重复标记，对「副本其实是独立邮件」的服务器则是各标各的，
// 两种语义下都正确。

// copyIDs 在给定邮件的基础上补齐它们的全部标签副本。
//
// 副本判据与 dedupeSameMessage 完全一致：account_id 相同，且
// message_id + date + size 三者全同。
//
// ⚠ 判据不能只看 Message-ID：GitHub 一类发信端会给同一会话的多封通知复用同一个
// Message-ID，只按它合并会把不同邮件误当成副本，一起改掉状态。
// message_id 为空的邮件不参与，各自独立。
func copyIDs(db *gorm.DB, ids []uint) ([]uint, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var out []uint
	err := db.Table("messages AS m").
		Joins("JOIN messages t ON t.account_id = m.account_id").
		Where("t.id IN ?", ids).
		Where(`m.id = t.id OR (t.message_id <> '' AND m.message_id = t.message_id
		       AND m.date = t.date AND m.size = t.size)`).
		Distinct().
		Pluck("m.id", &out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

// WithCopies 返回这些邮件加上它们全部标签副本的 id。
//
// 只给「按邮件」的状态用（已读、星标）——那些属性在 Gmail 上本来就是整封邮件的。
// **移动和删除不能用**：在 Gmail 里移动就是改标签，把副本一起搬走等于把用户
// 所有标签都抹掉。
func (s *Service) WithCopies(ids []uint) ([]uint, error) {
	return copyIDs(s.repo.db, ids)
}
