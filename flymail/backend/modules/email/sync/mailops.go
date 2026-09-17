package sync

import (
	"errors"

	"flymail/modules/email/folder"
	"flymail/modules/email/message"
)

// ErrCrossAccountMove 表示尝试把邮件移动到不属于同一账户的文件夹。
var ErrCrossAccountMove = errors.New("cannot move message across accounts")

// deleteTargetFor 判定一封/一组邮件在源文件夹上的删除语义：
//   - 源文件夹本身是回收站，或该账户没有回收站 → 永久删除（\Deleted + EXPUNGE）。
//   - 否则 → 移动到该账户的回收站。
func (s *Service) deleteTargetFor(accountID uint, src *folder.Folder) (op, targetPath string) {
	trash, _ := s.folders.FindByType(accountID, "trash")
	if src.Type == "trash" || trash == nil || trash.ID == src.ID {
		return wbOpExpunge, ""
	}
	return wbOpMove, trash.Path
}

// DeleteMessage 删除一封邮件：本地立即删行并刷新计数，服务器侧的删除/移到回收站
// 交给持久化回写队列异步完成。
//
// 之所以不等服务器确认：每次删除都要新建一条 IMAP 连接并 SELECT，慢的服务商上要数秒，
// 界面会整个卡住。队列有退避重试与放弃兜底，最坏情况下次全量同步会以服务器状态为准
// 把邮件拉回来，代价远小于每次操作都阻塞。
func (s *Service) DeleteMessage(messageID uint) error {
	m, err := s.messages.GetByID(messageID)
	if err != nil {
		return err
	}
	src, err := s.folders.GetByID(m.FolderID)
	if err != nil {
		return err
	}
	op, target := s.deleteTargetFor(m.AccountID, src)

	if err := s.messages.DeleteByID(messageID); err != nil {
		return err
	}
	s.refreshFolderCounts(src.ID)
	s.enqueueWritebackUIDs(m.AccountID, src.ID, []uint32{m.UID}, op, target)
	return nil
}

// MoveMessage 把邮件移动到同账户下的另一个文件夹：本地立即从源文件夹移除并刷新计数，
// IMAP MOVE 交给回写队列。目标文件夹必须与邮件同属一个账户。
// 目标文件夹里的那一份由下次同步补齐（与此前行为一致）。
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

	if err := s.messages.DeleteByID(messageID); err != nil {
		return err
	}
	s.refreshFolderCounts(src.ID)
	s.enqueueWritebackUIDs(m.AccountID, src.ID, []uint32{m.UID}, wbOpMove, dst.Path)
	return nil
}

// refreshFolderCounts 重算并持久化文件夹总数/未读数，使角标即时刷新（不必等下次同步）。
// 目标文件夹的计数由下次同步补正，这里只刷新源文件夹（本地已发生行删除）。
func (s *Service) refreshFolderCounts(folderID uint) {
	total, terr := s.messages.CountByFolder(folderID, message.Filter{})
	unread, uerr := s.messages.UnreadCountByFolder(folderID)
	if terr == nil && uerr == nil {
		_ = s.folders.SetCounts(folderID, int(total), int(unread))
	}
}

// ── 批量操作 ────────────────────────────────────────────────────────────────
// 思路：按 账户→文件夹 分组，本地状态立即改完，每组 UID 合并成一条回写队列记录
// （服务器侧一次 SELECT + 一次 MOVE/STORE）。全程不建连接，接口即时返回。
// 不存在的邮件 id 静默跳过。

// loadGrouped 把邮件 id 按 账户ID→文件夹ID 分组（已不存在的自然缺席）。
// 一次 IN 查询而不是逐 id 取：规则引擎的批量远大于界面手动多选。
func (s *Service) loadGrouped(ids []uint) (map[uint]map[uint][]*message.Message, error) {
	groups := map[uint]map[uint][]*message.Message{}
	rows, err := s.messages.GetByIDs(ids)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		m := &rows[i]
		if groups[m.AccountID] == nil {
			groups[m.AccountID] = map[uint][]*message.Message{}
		}
		groups[m.AccountID][m.FolderID] = append(groups[m.AccountID][m.FolderID], m)
	}
	return groups, nil
}

// uint32sOf 取一组邮件的 UID（用于合并成一条回写记录）。
func uint32sOf(msgs []*message.Message) []uint32 {
	uids := make([]uint32, 0, len(msgs))
	for _, m := range msgs {
		uids = append(uids, m.UID)
	}
	return uids
}

// idsOf 取一组邮件的本地主键（用于批量 SQL）。
func idsOf(msgs []*message.Message) []uint {
	ids := make([]uint, 0, len(msgs))
	for _, m := range msgs {
		ids = append(ids, m.ID)
	}
	return ids
}

// BatchDelete 批量删除：本地按源文件夹整组删除并刷新计数，服务器侧
// （回收站/无回收站则 EXPUNGE，否则 MOVE 到回收站）入回写队列。
func (s *Service) BatchDelete(ids []uint) error {
	groups, err := s.loadGrouped(ids)
	if err != nil {
		return err
	}
	for accountID, byFolder := range groups {
		for folderID, msgs := range byFolder {
			src, err := s.folders.GetByID(folderID)
			if err != nil {
				return err
			}
			op, target := s.deleteTargetFor(accountID, src)
			uids := uint32sOf(msgs)
			if err := s.messages.DeleteByIDs(idsOf(msgs)); err != nil {
				return err
			}
			s.refreshFolderCounts(folderID)
			s.enqueueWritebackUIDs(accountID, folderID, uids, op, target)
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
	for folderID, msgs := range byFolder {
		if folderID == dst.ID {
			continue
		}
		uids := uint32sOf(msgs)
		if err := s.messages.DeleteByIDs(idsOf(msgs)); err != nil {
			return err
		}
		s.refreshFolderCounts(folderID)
		s.enqueueWritebackUIDs(dst.AccountID, folderID, uids, wbOpMove, dst.Path)
	}
	return nil
}

// BatchSetRead 批量标记已读/未读：本地立即改并刷新未读角标，STORE 入回写队列。
//
// ⚠ 要连同标签副本一起标。Gmail 的同一封邮件在 INBOX / 所有邮件 / 各标签下各有
// 一行，只标被点的那份会让本地状态分裂——用户在收件箱读过之后，[Gmail]/重要 里
// 那份仍是未读，而会话统计取的是收件箱那份（已读），于是「筛得出未读却标不掉」。
// 详见 message/copies.go。
func (s *Service) BatchSetRead(ids []uint, read bool) error {
	op := wbOpUnread
	if read {
		op = wbOpRead
	}
	ids, err := s.messages.WithCopies(ids)
	if err != nil {
		return err
	}
	return s.batchSetFlag(ids, op, func(msgIDs []uint) error {
		return s.messages.SetSeenByIDs(msgIDs, read)
	}, true)
}

// markFolderReadChunk 是文件夹级"全部标为已读"每批处理的邮件数。
//
// 为什么必须分批：`enqueueWritebackUIDs` 把一组 UID **拼成一条**回写记录
// （joinUIDs 是逗号连接，没有任何分块），而 applyWriteback 又把它作为**一条**
// IMAP STORE 命令发出去。界面上的批量操作受列表分页约束、最多几十条，
// 所以至今没人撞到上限；而"全部标为已读"一次就可能是几万封——
// 拼出来的命令行有几百 KB，绝大多数 IMAP 服务端会直接拒（命令行长度上限通常
// 在 8~64KB），表现为"点了没反应，服务器那边没变"。
//
// 500 × 最长 10 位数字 + 分隔符 ≈ 5.5KB，留足了余量。
const markFolderReadChunk = 500

// MarkFolderRead 把一个文件夹里的未读邮件全部标为已读，返回实际标记的封数。
//
// 只捞未读的主键（不是整行、也不是全部邮件）：这个集合可能有几万条，
// 而已读的那些既不用改本地也不用回写，带上只会让 STORE 的 UID 集合白白膨胀。
//
// 分批调用 BatchSetRead 而不是自己写一遍：本地更新、未读角标刷新、回写入队
// 三件事已经在那条路径上验过了，重写一遍只会多一处要同步维护的逻辑。
func (s *Service) MarkFolderRead(folderID uint) (int, error) {
	ids, err := s.messages.UnreadIDsByFolder(folderID)
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	for start := 0; start < len(ids); start += markFolderReadChunk {
		end := start + markFolderReadChunk
		if end > len(ids) {
			end = len(ids)
		}
		if err := s.BatchSetRead(ids[start:end], true); err != nil {
			// 已经处理完的批次不回滚：本地已改、回写也已入队，而且它们本来就是
			// 独立的幂等操作。返回已完成数让调用方能如实告诉用户改了多少。
			return start, err
		}
	}
	return len(ids), nil
}

// BatchSetFlagged 批量加/取消星标：本地立即改，STORE 入回写队列。
func (s *Service) BatchSetFlagged(ids []uint, flagged bool) error {
	op := wbOpUnstar
	if flagged {
		op = wbOpStar
	}
	// 星标同样是「按邮件」的属性，理由见 BatchSetRead
	ids, err := s.messages.WithCopies(ids)
	if err != nil {
		return err
	}
	return s.batchSetFlag(ids, op, func(msgIDs []uint) error {
		return s.messages.SetFlaggedByIDs(msgIDs, flagged)
	}, false)
}

// ── 会话级操作 ──────────────────────────────────────────────────────────────
// 把 thread_id 解析成成员 message id 后全部复用上面的 Batch*：本地即时生效、回写队列合并。
//
// 已读 / 星标作用于会话全部成员（含 Gmail 副本行，各文件夹的本地未读数才对得上）。
// 删除 / 移动的范围要收窄：文件夹视图（inFolderID > 0）只动该文件夹里的成员；
// 聚合 / 搜索视图排除 sent / drafts——IMAP MOVE 会把自己的回复从「已发送」挪走，
// Gmail 的「移动会话」也不会动 Sent 副本。

// threadMemberIDs 解析会话成员 id。inFolderID > 0 时只取该文件夹内的；否则排除 sent / drafts。
func (s *Service) threadMemberIDs(threadIDs []string, inFolderID uint, narrow bool) ([]uint, error) {
	members, err := s.messages.ThreadMembers(threadIDs)
	if err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(members))
	folderType := map[uint]string{}
	for i := range members {
		m := &members[i]
		if !narrow {
			ids = append(ids, m.ID)
			continue
		}
		if inFolderID > 0 {
			if m.FolderID == inFolderID {
				ids = append(ids, m.ID)
			}
			continue
		}
		ft, ok := folderType[m.FolderID]
		if !ok {
			f, ferr := s.folders.GetByID(m.FolderID)
			if ferr != nil {
				return nil, ferr
			}
			ft = f.Type
			folderType[m.FolderID] = ft
		}
		if ft != "sent" && ft != "drafts" {
			ids = append(ids, m.ID)
		}
	}
	return ids, nil
}

// ThreadDelete 删除会话：文件夹视图只删该文件夹内成员，否则删除 sent/drafts 之外的全部成员。
func (s *Service) ThreadDelete(threadIDs []string, inFolderID uint) error {
	ids, err := s.threadMemberIDs(threadIDs, inFolderID, true)
	if err != nil {
		return err
	}
	return s.BatchDelete(ids)
}

// ThreadMove 移动会话到目标文件夹，成员范围同 ThreadDelete。
func (s *Service) ThreadMove(threadIDs []string, targetFolderID, inFolderID uint) error {
	ids, err := s.threadMemberIDs(threadIDs, inFolderID, true)
	if err != nil {
		return err
	}
	return s.BatchMove(ids, targetFolderID)
}

// ThreadSetRead 整条会话标已读 / 未读。
func (s *Service) ThreadSetRead(threadIDs []string, read bool) error {
	ids, err := s.threadMemberIDs(threadIDs, 0, false)
	if err != nil {
		return err
	}
	return s.BatchSetRead(ids, read)
}

// ThreadSetFlagged 整条会话加 / 去星标。
func (s *Service) ThreadSetFlagged(threadIDs []string, flagged bool) error {
	ids, err := s.threadMemberIDs(threadIDs, 0, false)
	if err != nil {
		return err
	}
	return s.BatchSetFlagged(ids, flagged)
}

// batchSetFlag 是「批量改标志位」的共同骨架：分组 → 每组一条 UPDATE 改本地 → 每组合并入队。
// refreshUnread 为 true 时同时重算文件夹未读数（已读类操作需要，星标不需要）。
func (s *Service) batchSetFlag(ids []uint, op string, applyLocal func([]uint) error, refreshUnread bool) error {
	groups, err := s.loadGrouped(ids)
	if err != nil {
		return err
	}
	for accountID, byFolder := range groups {
		for folderID, msgs := range byFolder {
			if err := applyLocal(idsOf(msgs)); err != nil {
				return err
			}
			if refreshUnread {
				if unread, uerr := s.messages.UnreadCountByFolder(folderID); uerr == nil {
					_ = s.folders.SetUnreadCount(folderID, int(unread))
				}
			}
			s.enqueueWritebackUIDs(accountID, folderID, uint32sOf(msgs), op, "")
		}
	}
	return nil
}
