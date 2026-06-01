package message

import (
	"encoding/json"

	coreimap "flymail-core/imap"
	"flymail-core/types"

	imapv2 "github.com/emersion/go-imap/v2"
)

const (
	defaultSyncDepth = 1000
	fetchBatchSize   = 200
)

// IMAPFetcher 是邮件元数据同步所需的最小 IMAP 能力（便于测试 mock）。*coreimap.Session 满足此接口。
type IMAPFetcher interface {
	SelectFolder(path string) (*coreimap.SelectedFolder, error)
	FolderStatus(path string, items ...coreimap.StatusItem) (*coreimap.FolderStatusResult, error)
	FetchByUIDRange(from, to imapv2.UID, opts coreimap.FetchOptions) ([]*types.ParsedEmail, error)
}

// FolderState 是单文件夹同步后回写文件夹表所需的状态。
type FolderState struct {
	UIDValidity uint32
	UIDNext     uint32
	Total       int
	Unread      int
}

type Service struct{ repo *Repository }

func NewService(repo *Repository) *Service { return &Service{repo: repo} }

// SyncFolderMessages 同步单个文件夹最近 ~defaultSyncDepth 封邮件的元数据。
// prevUIDValidity 为本地已存的该文件夹 UIDVALIDITY（0=从未同步）。
// 返回：同步后状态、是否因 UIDVALIDITY 变化而重建、错误。
func (s *Service) SyncFolderMessages(accountID, folderID uint, folderPath string, prevUIDValidity uint32, c IMAPFetcher) (*FolderState, bool, error) {
	sel, err := c.SelectFolder(folderPath)
	if err != nil {
		return nil, false, err
	}

	uidValidity := sel.UIDValidity
	if uidValidity == 0 {
		if st, serr := c.FolderStatus(folderPath, coreimap.StatusUIDValidity); serr == nil && st != nil && st.UIDValidity != nil {
			uidValidity = *st.UIDValidity
		}
	}

	rebuilt := false
	if prevUIDValidity != 0 && uidValidity != 0 && uidValidity != prevUIDValidity {
		if err := s.repo.DeleteByFolder(folderID); err != nil {
			return nil, false, err
		}
		rebuilt = true
	}

	if sel.NumMessages > 0 && sel.UIDNext > 0 {
		from := imapv2.UID(1)
		if sel.UIDNext > uint32(defaultSyncDepth) {
			from = imapv2.UID(sel.UIDNext - uint32(defaultSyncDepth))
		}
		end := imapv2.UID(sel.UIDNext - 1)
		if err := s.fetchRangeBatched(accountID, folderID, from, end, c); err != nil {
			return nil, rebuilt, err
		}
	}

	total, _ := s.repo.CountByFolder(folderID)
	unread, _ := s.repo.UnreadCountByFolder(folderID)
	return &FolderState{
		UIDValidity: uidValidity,
		UIDNext:     sel.UIDNext,
		Total:       int(total),
		Unread:      int(unread),
	}, rebuilt, nil
}

// fetchRangeBatched 把 [from,end] 切成 fetchBatchSize 的子区间逐批抓取并 upsert。
func (s *Service) fetchRangeBatched(accountID, folderID uint, from, end imapv2.UID, c IMAPFetcher) error {
	for start := from; start <= end; {
		batchEnd := start + fetchBatchSize - 1
		if batchEnd > end || batchEnd < start { // 上限裁剪 + uint32 溢出保护
			batchEnd = end
		}
		emails, err := c.FetchByUIDRange(start, batchEnd, coreimap.FetchOptions{FetchBody: false, FallbackHeaders: true})
		if err != nil {
			return err
		}
		for _, e := range emails {
			if err := s.repo.Upsert(toMessage(accountID, folderID, e)); err != nil {
				return err
			}
		}
		if batchEnd == end {
			break
		}
		start = batchEnd + 1
	}
	return nil
}

// List 返回文件夹内的邮件列表项（UID 游标分页）。
func (s *Service) List(folderID uint, beforeUID uint32, limit int) ([]MessageListItem, error) {
	rows, err := s.repo.ListByFolder(folderID, beforeUID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]MessageListItem, 0, len(rows))
	for i := range rows {
		out = append(out, toListItem(&rows[i]))
	}
	return out, nil
}

func toMessage(accountID, folderID uint, e *types.ParsedEmail) *Message {
	m := &Message{
		AccountID: accountID,
		FolderID:  folderID,
		UID:       e.UID,
		MessageID: e.MessageID,
		Subject:   e.Subject,
		Date:      e.Date,
		Size:      e.Size,
		Seen:      e.IsRead,
		Flagged:   e.IsStarred,
	}
	if len(e.From) > 0 {
		m.FromName = e.From[0].Name
		m.FromAddr = e.From[0].Email
	}
	if b, err := json.Marshal(e.To); err == nil {
		m.ToJSON = string(b)
	}
	if b, err := json.Marshal(e.CC); err == nil {
		m.CcJSON = string(b)
	}
	for _, f := range e.Flags {
		switch f {
		case "\\Answered":
			m.Answered = true
		case "\\Deleted":
			m.Deleted = true
		}
	}
	return m
}
