package sync

import (
	"errors"

	imapv2 "github.com/emersion/go-imap/v2"

	"flymail/modules/email/message"
)

// ErrCrossAccountMove 表示尝试把邮件移动到不属于同一账户的文件夹。
var ErrCrossAccountMove = errors.New("cannot move message across accounts")

// DeleteMessage 删除一封邮件：
//   - 源文件夹本身是回收站，或该账户没有回收站：永久删除（IMAP \Deleted + EXPUNGE）。
//   - 否则：移动到该账户的回收站。
//
// 操作成功后清理本地行并刷新源文件夹计数。删除是同步操作（需确认服务器成功再清本地）。
func (s *Service) DeleteMessage(messageID uint) error {
	m, err := s.messages.GetByID(messageID)
	if err != nil {
		return err
	}
	src, err := s.folders.GetByID(m.FolderID)
	if err != nil {
		return err
	}
	cfg, err := s.accounts.IMAPConfig(m.AccountID)
	if err != nil {
		return err
	}
	sess, err := s.dial(cfg)
	if err != nil {
		return err
	}
	defer sess.Close()
	if _, err := sess.SelectFolder(src.Path); err != nil {
		return err
	}

	uid := imapv2.UID(m.UID)
	trash, _ := s.folders.FindByType(m.AccountID, "trash")
	if src.Type == "trash" || trash == nil || trash.ID == src.ID {
		if err := sess.Delete(uid); err != nil {
			return err
		}
	} else {
		if err := sess.Move(trash.Path, uid); err != nil {
			return err
		}
	}

	if err := s.messages.DeleteByID(messageID); err != nil {
		return err
	}
	s.refreshFolderCounts(src.ID)
	return nil
}

// MoveMessage 把邮件移动到同账户下的另一个文件夹（IMAP MOVE），成功后清本地行并刷新源计数。
// 目标文件夹必须与邮件同属一个账户。
func (s *Service) MoveMessage(messageID, targetFolderID uint) error {
	m, err := s.messages.GetByID(messageID)
	if err != nil {
		return err
	}
	src, err := s.folders.GetByID(m.FolderID)
	if err != nil {
		return err
	}
	dst, err := s.folders.GetByID(targetFolderID)
	if err != nil {
		return err
	}
	if dst.AccountID != m.AccountID {
		return ErrCrossAccountMove
	}
	if dst.ID == src.ID {
		return nil
	}

	cfg, err := s.accounts.IMAPConfig(m.AccountID)
	if err != nil {
		return err
	}
	sess, err := s.dial(cfg)
	if err != nil {
		return err
	}
	defer sess.Close()
	if _, err := sess.SelectFolder(src.Path); err != nil {
		return err
	}
	if err := sess.Move(dst.Path, imapv2.UID(m.UID)); err != nil {
		return err
	}

	if err := s.messages.DeleteByID(messageID); err != nil {
		return err
	}
	s.refreshFolderCounts(src.ID)
	return nil
}

// refreshFolderCounts 重算并持久化文件夹总数/未读数，使角标即时刷新（不必等下次同步）。
// 目标文件夹的计数由下次同步补正，这里只刷新源文件夹（本地已发生行删除）。
func (s *Service) refreshFolderCounts(folderID uint) {
	total, terr := s.messages.CountByFolder(folderID)
	unread, uerr := s.messages.UnreadCountByFolder(folderID)
	if terr == nil && uerr == nil {
		_ = s.folders.SetCounts(folderID, int(total), int(unread))
	}
}

// ── 批量操作 ────────────────────────────────────────────────────────────────
// 思路：按 账户→文件夹 分组，每个账户只建一个 IMAP 会话，对一组 UID 一次性
// MOVE/STORE，避免逐封 dial。不存在的邮件 id 静默跳过。

// loadGrouped 把邮件 id 按 账户ID→文件夹ID 分组（跳过已不存在的）。
func (s *Service) loadGrouped(ids []uint) (map[uint]map[uint][]*message.Message, error) {
	groups := map[uint]map[uint][]*message.Message{}
	for _, id := range ids {
		m, err := s.messages.GetByID(id)
		if err != nil {
			if errors.Is(err, message.ErrMessageNotFound) {
				continue
			}
			return nil, err
		}
		if groups[m.AccountID] == nil {
			groups[m.AccountID] = map[uint][]*message.Message{}
		}
		groups[m.AccountID][m.FolderID] = append(groups[m.AccountID][m.FolderID], m)
	}
	return groups, nil
}

// withSession 为某账户建立一个 IMAP 会话并在回调内复用，结束自动关闭。
func (s *Service) withSession(accountID uint, fn func(sess Session) error) error {
	cfg, err := s.accounts.IMAPConfig(accountID)
	if err != nil {
		return err
	}
	sess, err := s.dial(cfg)
	if err != nil {
		return err
	}
	defer sess.Close()
	return fn(sess)
}

func uidsOf(msgs []*message.Message) []imapv2.UID {
	uids := make([]imapv2.UID, 0, len(msgs))
	for _, m := range msgs {
		uids = append(uids, imapv2.UID(m.UID))
	}
	return uids
}

// BatchDelete 批量删除：每个源文件夹一次性处理（回收站/无回收站则 EXPUNGE，否则 MOVE 到回收站）。
func (s *Service) BatchDelete(ids []uint) error {
	groups, err := s.loadGrouped(ids)
	if err != nil {
		return err
	}
	for accountID, byFolder := range groups {
		trash, _ := s.folders.FindByType(accountID, "trash")
		if err := s.withSession(accountID, func(sess Session) error {
			for folderID, msgs := range byFolder {
				src, err := s.folders.GetByID(folderID)
				if err != nil {
					return err
				}
				if _, err := sess.SelectFolder(src.Path); err != nil {
					return err
				}
				uids := uidsOf(msgs)
				if src.Type == "trash" || trash == nil || trash.ID == src.ID {
					if err := sess.Delete(uids...); err != nil {
						return err
					}
				} else {
					if err := sess.Move(trash.Path, uids...); err != nil {
						return err
					}
				}
				for _, m := range msgs {
					_ = s.messages.DeleteByID(m.ID)
				}
				s.refreshFolderCounts(folderID)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

// BatchMove 批量移动到 targetFolderID（要求全部邮件与目标同账户）。
func (s *Service) BatchMove(ids []uint, targetFolderID uint) error {
	dst, err := s.folders.GetByID(targetFolderID)
	if err != nil {
		return err
	}
	groups, err := s.loadGrouped(ids)
	if err != nil {
		return err
	}
	for accountID := range groups {
		if accountID != dst.AccountID {
			return ErrCrossAccountMove
		}
	}
	byFolder := groups[dst.AccountID]
	if byFolder == nil {
		return nil
	}
	return s.withSession(dst.AccountID, func(sess Session) error {
		for folderID, msgs := range byFolder {
			if folderID == dst.ID {
				continue
			}
			src, err := s.folders.GetByID(folderID)
			if err != nil {
				return err
			}
			if _, err := sess.SelectFolder(src.Path); err != nil {
				return err
			}
			if err := sess.Move(dst.Path, uidsOf(msgs)...); err != nil {
				return err
			}
			for _, m := range msgs {
				_ = s.messages.DeleteByID(m.ID)
			}
			s.refreshFolderCounts(folderID)
		}
		return nil
	})
}

// BatchSetRead 批量标记已读/未读（每文件夹一次 STORE，并刷新未读角标）。
func (s *Service) BatchSetRead(ids []uint, read bool) error {
	groups, err := s.loadGrouped(ids)
	if err != nil {
		return err
	}
	for accountID, byFolder := range groups {
		if err := s.withSession(accountID, func(sess Session) error {
			for folderID, msgs := range byFolder {
				src, err := s.folders.GetByID(folderID)
				if err != nil {
					return err
				}
				if _, err := sess.SelectFolder(src.Path); err != nil {
					return err
				}
				uids := uidsOf(msgs)
				if read {
					err = sess.MarkRead(uids...)
				} else {
					err = sess.MarkUnread(uids...)
				}
				if err != nil {
					return err
				}
				for _, m := range msgs {
					_ = s.messages.SetSeenLocal(m.ID, read)
				}
				if unread, uerr := s.messages.UnreadCountByFolder(folderID); uerr == nil {
					_ = s.folders.SetUnreadCount(folderID, int(unread))
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

// BatchSetFlagged 批量加/取消星标（每文件夹一次 STORE）。
func (s *Service) BatchSetFlagged(ids []uint, flagged bool) error {
	groups, err := s.loadGrouped(ids)
	if err != nil {
		return err
	}
	for accountID, byFolder := range groups {
		if err := s.withSession(accountID, func(sess Session) error {
			for folderID, msgs := range byFolder {
				src, err := s.folders.GetByID(folderID)
				if err != nil {
					return err
				}
				if _, err := sess.SelectFolder(src.Path); err != nil {
					return err
				}
				uids := uidsOf(msgs)
				if flagged {
					err = sess.MarkStarred(uids...)
				} else {
					err = sess.MarkUnstarred(uids...)
				}
				if err != nil {
					return err
				}
				for _, m := range msgs {
					_ = s.messages.SetFlaggedLocal(m.ID, flagged)
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}
