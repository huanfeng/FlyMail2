package message

import (
	"encoding/json"
	"regexp"
	"strings"

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
	FetchBySeqRange(from, to uint32, opts coreimap.FetchOptions) ([]*types.ParsedEmail, error)
}

// FolderState 是单文件夹同步后回写文件夹表所需的状态。
type FolderState struct {
	UIDValidity uint32
	UIDNext     uint32
	Total       int
	Unread      int
}

type Service struct {
	repo     *Repository
	bodyRepo *BodyRepository
}

func NewService(repo *Repository, bodyRepo *BodyRepository) *Service {
	return &Service{repo: repo, bodyRepo: bodyRepo}
}

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

	// UIDNEXT 兜底：部分服务商 SELECT 不返回 UIDNEXT，尝试 STATUS。
	uidNext := sel.UIDNext
	if uidNext == 0 {
		if st, serr := c.FolderStatus(folderPath, coreimap.StatusUIDNext); serr == nil && st != nil && st.UIDNext != nil {
			uidNext = *st.UIDNext
		}
	}

	if sel.NumMessages > 0 {
		if uidNext > 0 {
			// 已知 UIDNEXT：按 UID 区间抓最近 ~depth 封。
			from := imapv2.UID(1)
			if uidNext > uint32(defaultSyncDepth) {
				from = imapv2.UID(uidNext - uint32(defaultSyncDepth))
			}
			end := imapv2.UID(uidNext - 1)
			if err := s.fetchRangeBatched(accountID, folderID, from, end, c); err != nil {
				return nil, rebuilt, err
			}
		} else {
			// 服务商不报 UIDNEXT（如网易 163）：按序号抓最近 ~depth 封。
			// 序号区间 [total-depth+1, total]，FETCH 响应仍带真实 UID。
			total := sel.NumMessages
			seqFrom := uint32(1)
			if total > uint32(defaultSyncDepth) {
				seqFrom = total - uint32(defaultSyncDepth) + 1
			}
			if err := s.fetchSeqRangeBatched(accountID, folderID, seqFrom, total, c); err != nil {
				return nil, rebuilt, err
			}
		}
	}

	total, _ := s.repo.CountByFolder(folderID)
	unread, _ := s.repo.UnreadCountByFolder(folderID)
	// UIDNEXT 未知时，用本地已存的最大 UID + 1 作为锚点（供后续增量同步）。
	if uidNext == 0 {
		if maxUID, _ := s.repo.MaxUID(folderID); maxUID > 0 {
			uidNext = maxUID + 1
		}
	}
	return &FolderState{
		UIDValidity: uidValidity,
		UIDNext:     uidNext,
		Total:       int(total),
		Unread:      int(unread),
	}, rebuilt, nil
}

// fetchSeqRangeBatched 把序号区间 [from,end] 切成 fetchBatchSize 的子区间逐批抓取并 upsert。
func (s *Service) fetchSeqRangeBatched(accountID, folderID uint, from, end uint32, c IMAPFetcher) error {
	for start := from; start <= end; {
		batchEnd := start + fetchBatchSize - 1
		if batchEnd > end || batchEnd < start {
			batchEnd = end
		}
		emails, err := c.FetchBySeqRange(start, batchEnd, coreimap.FetchOptions{FetchBody: false, FallbackHeaders: true})
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

// StoreParsedBody 落正文+附件，回填 snippet/has_attachment/body_synced。
func (s *Service) StoreParsedBody(messageID uint, e *types.ParsedEmail) error {
	if err := s.bodyRepo.Upsert(&MessageBody{MessageID: messageID, TextBody: e.TextBody, HTMLBody: e.HTMLBody}); err != nil {
		return err
	}
	atts := make([]Attachment, 0, len(e.Attachments))
	for _, a := range e.Attachments {
		atts = append(atts, Attachment{
			MessageID:   messageID,
			Filename:    a.Filename,
			ContentType: a.ContentType,
			Size:        a.Size,
			ContentID:   a.ContentID,
			IsInline:    a.IsInline,
		})
	}
	if err := s.bodyRepo.ReplaceAttachments(messageID, atts); err != nil {
		return err
	}
	return s.repo.MarkBodySynced(messageID, makeSnippet(e.TextBody, e.HTMLBody), len(atts) > 0)
}

// Detail 从本地组装邮件详情。
func (s *Service) Detail(messageID uint) (*MessageDetail, error) {
	m, err := s.repo.GetByID(messageID)
	if err != nil {
		return nil, err
	}
	item := toListItem(m)
	d := &MessageDetail{
		MessageListItem: item,
		BodySynced:      m.BodySynced,
		Attachments:     []Attachment{},
	}
	if b, _ := s.bodyRepo.GetByMessageID(messageID); b != nil {
		d.TextBody = b.TextBody
		d.HTMLBody = b.HTMLBody
	}
	if atts, _ := s.bodyRepo.ListAttachments(messageID); len(atts) > 0 {
		d.Attachments = atts
	}
	if m.CcJSON != "" {
		_ = json.Unmarshal([]byte(m.CcJSON), &d.Cc)
	}
	return d, nil
}

// GetByID 透传单封邮件原始记录。
func (s *Service) GetByID(id uint) (*Message, error) { return s.repo.GetByID(id) }

// SetSeenLocal 本地标记已读/未读。
func (s *Service) SetSeenLocal(id uint, seen bool) error { return s.repo.SetSeen(id, seen) }

// SetFlaggedLocal 本地标记星标/取消星标。
func (s *Service) SetFlaggedLocal(id uint, flagged bool) error {
	return s.repo.SetFlagged(id, flagged)
}

var reHTML = regexp.MustCompile(`<[^>]*>`)

// stripHTML 简单去除 HTML 标签。
func stripHTML(html string) string {
	return reHTML.ReplaceAllString(html, " ")
}

// makeSnippet 生成不超过 150 字的摘要。
func makeSnippet(text, html string) string {
	s := text
	if s == "" {
		s = stripHTML(html)
	}
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > 150 {
		return string(r[:150]) + "…"
	}
	return s
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
