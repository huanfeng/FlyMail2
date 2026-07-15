package sync

import (
	"time"

	"flymail-core/logger"

	imapv2 "github.com/emersion/go-imap/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// SetRead 本地先标记已读/未读，然后持久化入队、异步回写 IMAP。
func (s *Service) SetRead(messageID uint, read bool) error {
	if err := s.messages.SetSeenLocal(messageID, read); err != nil {
		return err
	}
	m, err := s.messages.GetByID(messageID)
	if err != nil {
		return err
	}
	// 重算该文件夹未读数并持久化，使文件夹列表未读角标即时刷新（不必等下次同步）。
	if unread, uerr := s.messages.UnreadCountByFolder(m.FolderID); uerr == nil {
		_ = s.folders.SetUnreadCount(m.FolderID, int(unread))
	}
	op := wbOpUnread
	if read {
		op = wbOpRead
	}
	s.enqueueWriteback(m.AccountID, m.FolderID, m.UID, op)
	return nil
}

// SetFlagged 本地先标记星标/取消星标，然后持久化入队、异步回写 IMAP。
func (s *Service) SetFlagged(messageID uint, flagged bool) error {
	if err := s.messages.SetFlaggedLocal(messageID, flagged); err != nil {
		return err
	}
	m, err := s.messages.GetByID(messageID)
	if err != nil {
		return err
	}
	op := wbOpUnstar
	if flagged {
		op = wbOpStar
	}
	s.enqueueWriteback(m.AccountID, m.FolderID, m.UID, op)
	return nil
}

// enqueueWriteback 构造回写操作并投递：有 Manager 走持久队列 + runner 连接；
// 无 Manager（单测）退回即时直连尽力而为。
func (s *Service) enqueueWriteback(accountID, folderID uint, uid uint32, op string) {
	f, err := s.folders.GetByID(folderID)
	if err != nil {
		logger.Error("sync/writeback: 取文件夹失败", zap.Uint("folder_id", folderID), zap.Error(err))
		return
	}
	wo := &WritebackOp{AccountID: accountID, FolderPath: f.Path, UID: uid, Op: op}
	if s.orch != nil {
		s.orch.EnqueueWriteback(wo)
		return
	}
	// 回退：即时直连回写（尽力而为，失败仅记日志，本地状态已乐观写入）。
	if err := s.withDialedSession(accountID, func(sess Session) error {
		return applyWriteback(sess, *wo)
	}); err != nil {
		logger.Warn("sync/writeback: 直连回写失败(回退路径)",
			zap.Uint("account_id", accountID), zap.Uint32("uid", uid), zap.Error(err))
	}
}

// applyWriteback 在给定连接上执行一条回写操作（选文件夹→更新标志）。
func applyWriteback(sess Session, op WritebackOp) error {
	if _, err := sess.SelectFolder(op.FolderPath); err != nil {
		return err
	}
	uid := imapv2.UID(op.UID)
	switch op.Op {
	case wbOpRead:
		return sess.MarkRead(uid)
	case wbOpUnread:
		return sess.MarkUnread(uid)
	case wbOpStar:
		return sess.MarkStarred(uid)
	case wbOpUnstar:
		return sess.MarkUnstarred(uid)
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
				m.emit(notifySyncFailed, op.AccountID, "回写失败",
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
