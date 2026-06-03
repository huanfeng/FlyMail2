package sync

import (
	"errors"

	imapv2 "github.com/emersion/go-imap/v2"
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
