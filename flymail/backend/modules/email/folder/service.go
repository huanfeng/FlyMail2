package folder

import (
	"strings"
	"time"

	"flymail-core/logger"
	"flymail-core/types"

	"go.uber.org/zap"
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
	// ⚠ 空列表一律不删，直接返回。
	//
	// 下面的清理以「服务端列表」为准删本地文件夹连同邮件，是不可逆的。而 IMAP 的
	// LIST 会因为连接抖动、权限变化、服务端故障返回空结果——照单全收的话，一次
	// 抖动就能把这个账户的本地邮件全部删光，用户毫无察觉也无处恢复。
	//
	// 正常账户至少有一个 INBOX，所以空列表基本可以断定是异常。留着陈旧文件夹的
	// 代价只是日志里多几行错误，删错的代价是数据没了——两者不对称，往安全一侧倒。
	if len(infos) == 0 {
		logger.Warn("folder: 服务端返回了空文件夹列表，跳过本轮同步与清理",
			zap.Uint("account_id", accountID))
		return nil
	}
	keep := make(map[string]bool, len(infos))
	for _, info := range infos {
		keep[info.Path] = true
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

	// 服务端已经没有的文件夹要跟着消失：留着的话侧栏里挂着点进去就报错的入口，
	// 每一轮同步还会去 SELECT 它们（测试账户上两天刷了 8876 次 No such mailbox）。
	removed, err := s.repo.pruneMissing(accountID, keep)
	if err != nil {
		return err
	}
	if removed > 0 {
		logger.Info("folder: 清理服务端已删除的文件夹",
			zap.Uint("account_id", accountID), zap.Int("removed", removed))
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
