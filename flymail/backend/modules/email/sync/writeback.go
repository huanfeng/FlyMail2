package sync

import (
	"time"

	"flymail-core/logger"

	imapv2 "github.com/emersion/go-imap/v2"
	"go.uber.org/zap"
)

const maxRetry = 3

// wbOp 是一条写回任务，记录要对哪封邮件在 IMAP 服务器上更改哪个标志。
type wbOp struct {
	accountID uint
	folderID  uint
	uid       uint32
	seen      *bool // 非 nil 表示需要回写已读状态
	flagged   *bool // 非 nil 表示需要回写星标状态
	attempt   int
}

// enqueueWriteback 非阻塞地将任务投入写回队列；队列满时丢弃并打印警告。
func (s *Service) enqueueWriteback(op wbOp) {
	select {
	case s.wbCh <- op:
	default:
		logger.Warn("sync/writeback: 队列已满，丢弃回写操作",
			zap.Uint32("uid", uint32(op.uid)), zap.Uint("account_id", op.accountID))
	}
}

// writebackLoop 是后台 goroutine，持续消费写回任务。
//
// 已知限制（MVP 取舍，非可靠队列，勿误以为重试可靠）：
//   - 单 worker 串行处理；失败重试经 time.AfterFunc 异步延迟重新入队，
//     不在 worker 内 sleep，避免阻塞整个队列。
//   - enqueueWriteback 在 channel 满时非阻塞丢弃（含重试任务），属 MVP 取舍。
//   - 本地乐观写：回写彻底失败后，下次全量同步会用服务器的 Seen/Flagged
//     覆盖本地（服务器权威）。后续增量同步应跳过有 pending 回写的 UID 的
//     标记覆盖，避免覆盖掉尚未回写成功的本地改动。
func (s *Service) writebackLoop() {
	for op := range s.wbCh {
		s.processWriteback(op)
	}
}

// processWriteback 执行单条写回：建立 IMAP 连接 → 选文件夹 → 更新标志。
// 失败后按 attempt 做退避重试，超过 maxRetry 则放弃。
func (s *Service) processWriteback(op wbOp) {
	f, err := s.folders.GetByID(op.folderID)
	if err != nil {
		logger.Error("sync/writeback: 取文件夹失败",
			zap.Uint("folder_id", op.folderID), zap.Error(err))
		return
	}
	cfg, err := s.accounts.IMAPConfig(op.accountID)
	if err != nil {
		logger.Error("sync/writeback: 取 IMAP 配置失败",
			zap.Uint("account_id", op.accountID), zap.Error(err))
		return
	}
	sess, err := s.dial(cfg)
	if err != nil {
		s.retryOrDrop(op, err)
		return
	}
	defer sess.Close()

	if _, err := sess.SelectFolder(f.Path); err != nil {
		s.retryOrDrop(op, err)
		return
	}

	uid := imapv2.UID(op.uid)

	if op.seen != nil {
		if *op.seen {
			err = sess.MarkRead(uid)
		} else {
			err = sess.MarkUnread(uid)
		}
		if err != nil {
			s.retryOrDrop(op, err)
			return
		}
	}

	if op.flagged != nil {
		if *op.flagged {
			err = sess.MarkStarred(uid)
		} else {
			err = sess.MarkUnstarred(uid)
		}
		if err != nil {
			s.retryOrDrop(op, err)
			return
		}
	}
}

// retryOrDrop 在 attempt < maxRetry 时退避后重新入队，否则记录日志放弃。
func (s *Service) retryOrDrop(op wbOp, err error) {
	op.attempt++
	if op.attempt >= maxRetry {
		logger.Error("sync/writeback: 放弃回写",
			zap.Uint32("uid", uint32(op.uid)), zap.Uint("account_id", op.accountID),
			zap.Int("attempt", op.attempt), zap.Error(err))
		return
	}
	backoff := time.Duration(op.attempt) * time.Second
	logger.Warn("sync/writeback: 重试回写",
		zap.Int("attempt", op.attempt), zap.Int("max_retry", maxRetry),
		zap.Uint32("uid", uint32(op.uid)), zap.Duration("backoff", backoff), zap.Error(err))
	// 用 AfterFunc 异步延迟重新入队，避免在单 worker 内 sleep 阻塞整个队列。
	time.AfterFunc(backoff, func() { s.enqueueWriteback(op) })
}

// SetRead 本地先标记已读/未读，然后异步回写 IMAP。
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
	s.enqueueWriteback(wbOp{
		accountID: m.AccountID,
		folderID:  m.FolderID,
		uid:       m.UID,
		seen:      &read,
	})
	return nil
}

// SetFlagged 本地先标记星标/取消星标，然后异步回写 IMAP。
func (s *Service) SetFlagged(messageID uint, flagged bool) error {
	if err := s.messages.SetFlaggedLocal(messageID, flagged); err != nil {
		return err
	}
	m, err := s.messages.GetByID(messageID)
	if err != nil {
		return err
	}
	s.enqueueWriteback(wbOp{
		accountID: m.AccountID,
		folderID:  m.FolderID,
		uid:       m.UID,
		flagged:   &flagged,
	})
	return nil
}
