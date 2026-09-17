package sync

import (
	"strconv"
	"strings"
	"time"

	"flymail-core/logger"

	imapv2 "github.com/emersion/go-imap/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// SetRead 本地先标记已读/未读，然后持久化入队、异步回写 IMAP。
// ⚠ 委托给 BatchSetRead，不要在这里自己写一遍。
//
// 原先这里是一份独立实现：只改被点的那一行、只给那一个文件夹入队。
// 于是 Gmail 的标签副本改不到——用户在收件箱读完，[Gmail]/重要 里那份仍是未读
// （详见 message/copies.go）。批量那条路径已经处理了副本扩展、逐文件夹的未读
// 重算和分组入队，这里再维护一份只会继续分叉：这个 bug 就是两份实现改了一份
// 造成的。
//
// 先 GetByID 是为了保住「邮件不存在返回 404」的语义——批量路径对不存在的 id
// 是静默跳过的。
func (s *Service) SetRead(messageID uint, read bool) error {
	if _, err := s.messages.GetByID(messageID); err != nil {
		return err
	}
	return s.BatchSetRead([]uint{messageID}, read)
}

// SetFlagged 本地先标记星标/取消星标，然后持久化入队、异步回写 IMAP。
// 同 SetRead：委托给批量路径，星标也是「按邮件」的属性，各标签副本要一起改。
func (s *Service) SetFlagged(messageID uint, flagged bool) error {
	if _, err := s.messages.GetByID(messageID); err != nil {
		return err
	}
	return s.BatchSetFlagged([]uint{messageID}, flagged)
}

// enqueueWriteback 构造单封邮件的回写操作并投递。
func (s *Service) enqueueWriteback(accountID, folderID uint, uid uint32, op string) {
	s.enqueueWritebackUIDs(accountID, folderID, []uint32{uid}, op, "")
}

// enqueueWritebackUIDs 构造一条覆盖多个 UID 的回写操作并投递：有 Manager 走持久队列
// + runner 连接；无 Manager（单测）退回即时直连尽力而为。
// targetPath 仅 move 用；同组 UID 合并成一条，服务器侧一次 SELECT + 一次动作即可完成。
func (s *Service) enqueueWritebackUIDs(accountID, folderID uint, uids []uint32, op, targetPath string) {
	if len(uids) == 0 {
		return
	}
	f, err := s.folders.GetByID(folderID)
	if err != nil {
		logger.Error("sync/writeback: 取文件夹失败", zap.Uint("folder_id", folderID), zap.Error(err))
		return
	}
	wo := &WritebackOp{
		AccountID:  accountID,
		FolderPath: f.Path,
		UID:        uids[0],
		UIDs:       joinUIDs(uids),
		Op:         op,
		TargetPath: targetPath,
	}
	if s.orch != nil {
		s.orch.EnqueueWriteback(wo)
		return
	}
	// 回退：即时直连回写（尽力而为，失败仅记日志，本地状态已乐观写入）。
	if err := s.withDialedSession(accountID, func(sess Session) error {
		return applyWriteback(sess, *wo)
	}); err != nil {
		logger.Warn("sync/writeback: 直连回写失败(回退路径)",
			zap.Uint("account_id", accountID), zap.String("op", op),
			zap.Int("uids", len(uids)), zap.Error(err))
	}
}

// opUIDs 解析一条回写操作覆盖的 UID 列表：优先 UIDs（批量合并），为空回退单个 UID
// （兼容升级前入队的旧数据）。无法解析的片段跳过。
func opUIDs(op WritebackOp) []imapv2.UID {
	if op.UIDs == "" {
		if op.UID == 0 {
			return nil
		}
		return []imapv2.UID{imapv2.UID(op.UID)}
	}
	parts := strings.Split(op.UIDs, ",")
	uids := make([]imapv2.UID, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.ParseUint(strings.TrimSpace(p), 10, 32)
		if err != nil || n == 0 {
			continue
		}
		uids = append(uids, imapv2.UID(uint32(n)))
	}
	return uids
}

// joinUIDs 把 UID 列表序列化成 UIDs 字段的逗号分隔形式。
func joinUIDs(uids []uint32) string {
	parts := make([]string, 0, len(uids))
	for _, u := range uids {
		parts = append(parts, strconv.FormatUint(uint64(u), 10))
	}
	return strings.Join(parts, ",")
}

// applyWriteback 在给定连接上执行一条回写操作（选文件夹 → 对该组 UID 一次性动作）。
func applyWriteback(sess Session, op WritebackOp) error {
	uids := opUIDs(op)
	if len(uids) == 0 {
		return nil // 空操作：直接判成功，避免死留在队列里
	}
	if _, err := sess.SelectFolder(op.FolderPath); err != nil {
		return err
	}
	switch op.Op {
	case wbOpRead:
		return sess.MarkRead(uids...)
	case wbOpUnread:
		return sess.MarkUnread(uids...)
	case wbOpStar:
		return sess.MarkStarred(uids...)
	case wbOpUnstar:
		return sess.MarkUnstarred(uids...)
	case wbOpMove:
		return sess.Move(op.TargetPath, uids...)
	case wbOpExpunge:
		return sess.Delete(uids...)
	}
	return nil
}

// ── Manager 侧：持久队列的执行/重试/恢复 ─────────────────────────────────────

// EnableWriteback 装配持久化回写存储（app 装配时以 db 调用）。
func (m *Manager) EnableWriteback(db *gorm.DB) { m.wb = newWBStore(db) }

// EnqueueWriteback 持久化一条回写操作并立即投递一次执行任务到账户 runner。
func (m *Manager) EnqueueWriteback(op *WritebackOp) {
	if m.wb == nil {
		return
	}
	if err := m.wb.Enqueue(op); err != nil {
		logger.Error("sync/writeback: 入队失败",
			zap.Uint("account_id", op.AccountID), zap.Uint32("uid", op.UID), zap.Error(err))
		return
	}
	id := op.ID
	r := m.ensureRunner(op.AccountID)
	r.submitBackground(func(sess Session) error {
		m.processWriteback(sess, id)
		return nil // 回写失败经 DB 退避重试处理，不因单条回写失败牵连连接/熔断
	})
}

// DrainWriteback 捎带清理某账户此刻到期的回写操作（runner 轮询 tick 及启动恢复调用）。
func (m *Manager) DrainWriteback(accountID uint, sess Session) {
	if m.wb == nil {
		return
	}
	ops, err := m.wb.DuePending(accountID, time.Now())
	if err != nil {
		logger.Error("sync/writeback: 捞取到期项失败", zap.Uint("account_id", accountID), zap.Error(err))
		return
	}
	for i := range ops {
		m.applyAndSettle(sess, ops[i])
	}
}

// processWriteback 执行单条（据 id 取最新状态，可能已被删除）。
func (m *Manager) processWriteback(sess Session, id uint) {
	op, err := m.wb.GetByID(id)
	if err != nil {
		return // 已完成/放弃
	}
	m.applyAndSettle(sess, op)
}

// applyAndSettle 执行一条回写并结算：成功删行；失败退避重试；达上限放弃并通知。
func (m *Manager) applyAndSettle(sess Session, op WritebackOp) {
	if err := applyWriteback(sess, op); err != nil {
		attempts, ferr := m.wb.Fail(op.ID, err.Error(), time.Now())
		if ferr != nil {
			logger.Error("sync/writeback: 记失败出错", zap.Uint("op_id", op.ID), zap.Error(ferr))
			return
		}
		if attempts >= maxWritebackAttempts {
			_ = m.wb.Delete(op.ID)
			logger.Warn("sync/writeback: 放弃回写",
				zap.Uint("account_id", op.AccountID), zap.Uint32("uid", op.UID),
				zap.String("op", op.Op), zap.Int("attempts", attempts), zap.Error(err))
			if m.emit != nil {
				m.emit(notifySyncFailed, op.AccountID, 0, "回写失败",
					"邮件标记回写多次失败已放弃，下次同步将以服务器状态为准")
			}
		}
		return
	}
	_ = m.wb.Delete(op.ID)
}

// recoverWriteback 在 runner 刚创建时投递一次性任务，恢复该账户遗留的到期回写。
func (m *Manager) recoverWriteback(accountID uint, r *runner) {
	if m.wb == nil {
		return
	}
	pending, err := m.wb.PendingByAccount(accountID)
	if err != nil || len(pending) == 0 {
		return
	}
	logger.Info("sync/writeback: 启动恢复待回写",
		zap.Uint("account_id", accountID), zap.Int("pending", len(pending)))
	r.submitBackground(func(sess Session) error {
		m.DrainWriteback(accountID, sess)
		return nil
	})
}
