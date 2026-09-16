package sync

// 正文预取：同步时顺带把邮件正文也下载到本地，这样点开邮件不必现抓（现抓要走前台
// 队列等一次 IMAP 往返，慢的服务商上就是肉眼可见的卡顿）。
//
// 三档模式（设置项 body_sync_mode）：
//   new    仅新邮件——只补这一轮增量收到的（默认）
//   recent 额外回补最近 N 天的历史邮件
//   all    额外回补全部历史邮件
//
// 回补是渐进的：每轮同步只补一批（bodyPrefetchPerRound），补完为止。这样几千封的
// 邮箱不会把连接长时间霸着，也不会让首次开启该选项的用户干等。

import (
	coreimap "flymail-core/imap"
	"flymail-core/logger"

	imapv2 "github.com/emersion/go-imap/v2"
	"go.uber.org/zap"

	"flymail/modules/email/folder"
	"flymail/modules/email/message"
)

const (
	// bodyFetchBatch 是单次 FETCH 拉取的封数：正文体积远大于元数据，
	// 一次拉太多既占内存也让单条命令过久无响应。
	bodyFetchBatch = 20
	// bodyPrefetchPerRound 是每轮同步回补历史正文的上限，分多轮渐进补齐。
	bodyPrefetchPerRound = 200
	// bodyPrefetchNewCap 是 new 模式下单文件夹每轮的上限（防御性封顶，正常远小于此）。
	bodyPrefetchNewCap = 50
)

// 正文预取模式取值（与 setting 包常量对应，此处镜像以避免 sync 反向依赖 setting）。
const (
	bodyModeNew    = "new"
	bodyModeRecent = "recent"
	bodyModeAll    = "all"
)

// SetBodySyncProviders 注入正文预取配置读取器（app 装配时指向 settings）。
// mode 返回 new/recent/all；recentDays 是 recent 模式的天数窗口。
func (m *Manager) SetBodySyncProviders(mode func() string, recentDays func() int) {
	if mode != nil {
		m.bodyMode = mode
	}
	if recentDays != nil {
		m.bodyRecentDays = recentDays
	}
}

// bodyPrefetchEnabled 报告某文件夹类型是否参与预取。
// 与未读口径一致：archive 是 Gmail「所有邮件」全库镜像，正文与收件箱那份重复；
// junk/trash/sent/drafts 不值得占磁盘。
func bodyPrefetchFolder(folderType string) bool {
	return folderType == "inbox" || folderType == "custom"
}

// prefetchNewBodies 在单文件夹增量同步后，把这一轮新收到邮件的正文顺手抓下来。
// 三档模式都执行——「仅新邮件」是最低档，recent/all 自然也包含新邮件。
// 基线导入（首次把历史邮件拉进来）不在此处理：那是「历史」，交给 recent/all 分轮回补，
// 否则新加一个账户会立刻触发几千封正文下载。
func (m *Manager) prefetchNewBodies(accountID uint, f *folder.Folder, nm *message.NewMail, sess Session) {
	if nm == nil || nm.Baseline || nm.Count == 0 || !bodyPrefetchFolder(f.Type) {
		return
	}
	msgs, err := m.messages.PendingBodiesAfterID(f.ID, nm.AfterID, bodyPrefetchNewCap)
	if err != nil || len(msgs) == 0 {
		return
	}
	n := m.fetchBodies(f.Path, msgs, sess)
	logger.Info("sync-body: 新邮件正文已预取",
		zap.Uint("account_id", accountID), zap.String("folder", f.Path), zap.Int("fetched", n))
}

// prefetchHistoryBodies 在一轮全量同步收尾时回补一批历史邮件的正文（recent/all 模式）。
// 调用点在全局同步名额释放之后：回补是后台补齐，不该占着并发名额挡住其他账户。
func (m *Manager) prefetchHistoryBodies(accountID uint, sess Session, yield func()) {
	mode := m.bodySyncMode()
	if mode != bodyModeRecent && mode != bodyModeAll {
		return
	}
	sinceDays := 0
	if mode == bodyModeRecent {
		sinceDays = m.bodySyncRecentDays()
	}

	msgs, err := m.messages.PendingBodies(accountID, sinceDays, bodyPrefetchPerRound)
	if err != nil {
		logger.Warn("sync-body: 捞取待补正文失败", zap.Uint("account_id", accountID), zap.Error(err))
		return
	}
	if len(msgs) == 0 {
		return
	}
	// 这一轮的分母。比文件夹粒度精确得多——正文回补天然有封数级的进度可报。
	m.statusBodies(accountID, len(msgs))
	defer m.statusBodiesEnd(accountID)

	// 按文件夹分组：同一文件夹只 SELECT 一次。
	byFolder := map[uint][]message.Message{}
	order := make([]uint, 0, 4)
	for _, msg := range msgs {
		if _, seen := byFolder[msg.FolderID]; !seen {
			order = append(order, msg.FolderID)
		}
		byFolder[msg.FolderID] = append(byFolder[msg.FolderID], msg)
	}

	fetched := 0
	for _, folderID := range order {
		f, err := m.folders.GetByID(folderID)
		if err != nil {
			continue
		}
		fetched += m.fetchBodies(f.Path, byFolder[folderID], sess)
		m.statusBodiesDone(accountID, fetched)
		if yield != nil {
			yield() // 文件夹边界让位前台任务（用户正在打开的邮件/附件优先）
		}
	}

	remaining, _ := m.messages.CountPendingBodies(accountID, sinceDays)
	logger.Info("sync-body: 历史正文回补一批",
		zap.Uint("account_id", accountID), zap.String("mode", mode),
		zap.Int("fetched", fetched), zap.Int64("remaining", remaining))
}

// fetchBodies 抓取一组同文件夹邮件的正文并落库，返回成功落库的封数。
// 失败只记日志不返回错误：预取是尽力而为，不该因此判定连接故障触发重连/熔断。
func (m *Manager) fetchBodies(folderPath string, msgs []message.Message, sess Session) int {
	if len(msgs) == 0 {
		return 0
	}
	if _, err := sess.SelectFolder(folderPath); err != nil {
		logger.Warn("sync-body: 选文件夹失败", zap.String("folder", folderPath), zap.Error(err))
		return 0
	}

	// UID → 本地主键：FETCH 回来的顺序不保证，按 UID 对回去。
	idByUID := make(map[uint32]uint, len(msgs))
	uids := make([]imapv2.UID, 0, len(msgs))
	for i := range msgs {
		idByUID[msgs[i].UID] = msgs[i].ID
		uids = append(uids, imapv2.UID(msgs[i].UID))
	}

	done := 0
	for start := 0; start < len(uids); start += bodyFetchBatch {
		end := min(start+bodyFetchBatch, len(uids))
		emails, err := sess.FetchByUIDs(uids[start:end], coreimap.FetchOptions{
			FetchBody: true,
		})
		if err != nil {
			logger.Warn("sync-body: 抓正文失败",
				zap.String("folder", folderPath), zap.Int("batch", end-start), zap.Error(err))
			return done
		}
		for _, e := range emails {
			id, ok := idByUID[e.UID]
			if !ok {
				continue
			}
			if err := m.messages.StoreParsedBody(id, e); err != nil {
				logger.Warn("sync-body: 落正文失败", zap.Uint("message_id", id), zap.Error(err))
				continue
			}
			done++
		}
	}
	return done
}

// bodySyncMode 读取当前模式，未注入或取值非法时退回 new。
func (m *Manager) bodySyncMode() string {
	if m.bodyMode == nil {
		return bodyModeNew
	}
	switch v := m.bodyMode(); v {
	case bodyModeNew, bodyModeRecent, bodyModeAll:
		return v
	default:
		return bodyModeNew
	}
}

// bodySyncRecentDays 读取 recent 窗口天数，未注入或非正值时退回 30 天。
func (m *Manager) bodySyncRecentDays() int {
	if m.bodyRecentDays == nil {
		return 30
	}
	if d := m.bodyRecentDays(); d > 0 {
		return d
	}
	return 30
}
