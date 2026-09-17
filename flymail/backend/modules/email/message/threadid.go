package message

import (
	"fmt"
	"strings"

	"flymail-core/logger"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// EnsureOpaqueThreadIDs 把老库里形如 "{account}:{Message-ID}" 的 thread_id
// 改成 "{account}:{摘要}"。为什么要改见 threadKey 的说明。
//
// ⚠ 这是**按现有分组原地改名**，不是重算归属：同一个旧 thread_id 的行一律拿到同一个
// 新 id，绝不触碰谁和谁在一个会话里。重算是 RebuildThreads 的事，跟这里无关——
// 把两者混为一谈会在升级时悄悄改变用户看到的会话划分。
//
// ⚠ 幂等的判据是「后缀里有没有 @」。摘要是十六进制、不含 @；没有 Message-ID 时的兜底
// 形态 "{account}:u{folder}-{uid}" 也不含 @。两者都不会被二次处理。
// 不能用「长度 20 且全是十六进制」来判断：真有一个长得像摘要的 Message-ID 就会被漏掉，
// 那封邮件的域名就永远留在库里了。
func EnsureOpaqueThreadIDs(db *gorm.DB) error {
	// 只取还带 @ 的那些行的 (id, thread_id)。按 id 批量写回而不是
	// `WHERE thread_id = ?` 逐个会话更新：thread_id 自己也是索引列，
	// 边扫那个索引边改它的值不是个稳妥的写法，而按主键写是 RebuildThreads 一直用的路子。
	type row struct {
		ID       uint
		ThreadID string
	}
	var rows []row
	if err := db.Model(&Message{}).Select("id, thread_id").
		Where("thread_id LIKE ?", "%@%").Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	updates := map[string][]uint{} // 新 thread_id → 需要改的行
	renamed := map[string]string{} // 旧 → 新，同一个旧 id 只算一次
	taken := map[string]string{}   // 新 → 旧，用来发现碰撞
	for _, r := range rows {
		next, done := renamed[r.ThreadID]
		if !done {
			// Message-ID 里可以有冒号，账户 id 是纯数字不会有，所以按**第一个**冒号切。
			acct, mid, ok := strings.Cut(r.ThreadID, ":")
			if !ok {
				// 形态不认识：宁可留着一个带域名的 id，也不冒改错分组的险。
				continue
			}
			next = acct + ":" + threadDigest(mid)
			if prev, dup := taken[next]; dup {
				// 80 bit 下几乎不可能，但真撞上的后果是两段无关对话被静默并成一个，
				// 是数据损坏而不是报错——所以停下来，不要带着损坏继续启动。
				return fmt.Errorf("thread_id 摘要碰撞：%q 与 %q 都映射到 %q", prev, r.ThreadID, next)
			}
			taken[next] = r.ThreadID
			renamed[r.ThreadID] = next
		}
		updates[next] = append(updates[next], r.ID)
	}
	if len(updates) == 0 {
		return nil
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		for tid, ids := range updates {
			for start := 0; start < len(ids); start += uidChunk {
				end := start + uidChunk
				if end > len(ids) {
					end = len(ids)
				}
				if err := tx.Model(&Message{}).Where("id IN ?", ids[start:end]).
					Update("thread_id", tid).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	logger.Info("threads: thread_id 已换成摘要形态",
		zap.Int("threads", len(updates)), zap.Int("messages", len(rows)))
	return nil
}
