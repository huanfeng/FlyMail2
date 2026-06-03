package folder

import (
	"strings"
	"time"

	"flymail-core/types"
)

// IMAPLister 是文件夹同步所需的最小 IMAP 能力（便于测试 mock）。*coreimap.Session 满足此接口。
type IMAPLister interface {
	ListFolders() ([]types.FolderInfo, error)
}

type Service struct{ repo *Repository }

func NewService(repo *Repository) *Service { return &Service{repo: repo} }

func (s *Service) SyncFolders(accountID uint, lister IMAPLister) error {
	infos, err := lister.ListFolders()
	if err != nil {
		return err
	}
	for _, info := range infos {
		ft := types.ClassifyFolder(info.Name, info.Path, info.Attributes).String()
		selectable := true
		for _, a := range info.Attributes {
			if strings.EqualFold(a, "\\Noselect") {
				selectable = false
				break
			}
		}
		f := &Folder{
			AccountID:   accountID,
			Path:        info.Path,
			DisplayName: info.Name,
			Delimiter:   info.Delimiter,
			Type:        ft,
			Attributes:  strings.Join(info.Attributes, ","),
			Selectable:  selectable,
			SortOrder:   SortOrderForType(ft),
		}
		if err := s.repo.UpsertByPath(f); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) List(accountID uint) ([]Folder, error) { return s.repo.ListByAccount(accountID) }

// CountByAccount 返回账户下全部文件夹数量。
func (s *Service) CountByAccount(accountID uint) (int64, error) {
	return s.repo.CountByAccount(accountID)
}

func (s *Service) FindInbox(accountID uint) (*Folder, error) { return s.repo.FindInbox(accountID) }

func (s *Service) FindByType(accountID uint, folderType string) (*Folder, error) {
	return s.repo.FindByType(accountID, folderType)
}

func (s *Service) GetByID(id uint) (*Folder, error) { return s.repo.GetByID(id) }

func (s *Service) UpdateSyncState(id uint, uidValidity, uidNext uint32, total, unread int, syncedAt time.Time) error {
	return s.repo.UpdateSyncState(id, uidValidity, uidNext, total, unread, syncedAt)
}

// SetUnreadCount 只更新文件夹未读数。
func (s *Service) SetUnreadCount(id uint, unread int) error {
	return s.repo.UpdateUnreadCount(id, unread)
}

// SetCounts 同时更新文件夹总数与未读数。
func (s *Service) SetCounts(id uint, total, unread int) error {
	return s.repo.UpdateCounts(id, total, unread)
}
