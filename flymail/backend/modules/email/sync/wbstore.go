package sync

import (
	"time"

	"gorm.io/gorm"
)

// 回写操作动词（与 SetRead/SetFlagged 一一对应）。
// delete/move 为即时同步执行、不入此队列（见设计 §6）。
const (
	wbOpRead   = "read"
	wbOpUnread = "unread"
	wbOpStar   = "star"
	wbOpUnstar = "unstar"
)

const (
	// maxWritebackAttempts 到达即放弃：删行 + Warn + 站内通知，下次全量同步以服务器状态兜底。
	maxWritebackAttempts = 8
	// 退避：next_attempt_at = now + 2^attempts × 30s，封顶 30min（设计 §5）。
	writebackBaseBackoff = 30 * time.Second
	writebackMaxBackoff  = 30 * time.Minute
)

// WritebackOp 是一条持久化的回写操作：把某封邮件的已读/星标状态改到 IMAP 服务器。
// 表名 writeback_ops，随 gorm AutoMigrate 建立（见 MigrateWriteback）。
type WritebackOp struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// account_id + next_attempt_at 组合索引：DuePending 按账户捞取到期项的主查询路径。
	AccountID     uint      `gorm:"not null;index:idx_wb_due,priority:1" json:"account_id"`
	FolderPath    string    `gorm:"not null" json:"folder_path"`
	UID           uint32    `gorm:"not null" json:"uid"`
	Op            string    `gorm:"not null" json:"op"` // read/unread/star/unstar
	Attempts      int       `gorm:"not null;default:0" json:"attempts"`
	NextAttemptAt time.Time `gorm:"index:idx_wb_due,priority:2" json:"next_attempt_at"`
	LastError     string    `json:"last_error,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// TableName 固定表名，避免 gorm 复数化推断。
func (WritebackOp) TableName() string { return "writeback_ops" }

// MigrateWriteback 迁移回写队列表。由 app 在 database.Migrate 之后调用
// （不把此模型放入 database.Migrate，以免 database 反向依赖 sync 触发测试期 import cycle）。
func MigrateWriteback(db *gorm.DB) error {
	return db.AutoMigrate(&WritebackOp{})
}

// writebackBackoff 计算第 attempts 次失败后的退避时长：30s × 2^attempts，封顶 30min。
// attempts<1 视为 1；过大时直接返回封顶值，避免移位溢出。
func writebackBackoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	if attempts > 20 {
		return writebackMaxBackoff
	}
	d := writebackBaseBackoff << uint(attempts)
	if d <= 0 || d > writebackMaxBackoff {
		return writebackMaxBackoff
	}
	return d
}

// wbStore 是回写操作的持久化仓库（SQLite via gorm）。
type wbStore struct{ db *gorm.DB }

// newWBStore 构建仓库。
func newWBStore(db *gorm.DB) *wbStore { return &wbStore{db: db} }

// Enqueue 入队一条回写操作；next_attempt_at 未设时置为当前时刻（立即到期）。
func (s *wbStore) Enqueue(op *WritebackOp) error {
	if op.NextAttemptAt.IsZero() {
		op.NextAttemptAt = time.Now()
	}
	return s.db.Create(op).Error
}

// Delete 删除一条（成功或放弃）。
func (s *wbStore) Delete(id uint) error {
	return s.db.Delete(&WritebackOp{}, id).Error
}

// DuePending 返回某账户 next_attempt_at ≤ now 的到期操作，按 id 升序（FIFO）。
func (s *wbStore) DuePending(accountID uint, now time.Time) ([]WritebackOp, error) {
	var ops []WritebackOp
	err := s.db.Where("account_id = ? AND next_attempt_at <= ?", accountID, now).
		Order("id asc").Find(&ops).Error
	return ops, err
}

// PendingByAccount 返回某账户全部未完成操作（启动恢复用），按 id 升序。
func (s *wbStore) PendingByAccount(accountID uint) ([]WritebackOp, error) {
	var ops []WritebackOp
	err := s.db.Where("account_id = ?", accountID).Order("id asc").Find(&ops).Error
	return ops, err
}

// PendingAccountIDs 返回有未完成回写的账户 id 去重列表（启动时据此拉起对应 runner 恢复）。
func (s *wbStore) PendingAccountIDs() ([]uint, error) {
	var ids []uint
	err := s.db.Model(&WritebackOp{}).Distinct().Pluck("account_id", &ids).Error
	return ids, err
}

// CountPending 返回待回写总数（监控用）。
func (s *wbStore) CountPending() (int64, error) {
	var n int64
	err := s.db.Model(&WritebackOp{}).Count(&n).Error
	return n, err
}

// Fail 记一次失败：attempts++，按退避设置 next_attempt_at，记录 last_error。
// 返回更新后的 attempts；调用方据 attempts ≥ maxWritebackAttempts 决定放弃（Delete + 通知）。
func (s *wbStore) Fail(id uint, cause string, now time.Time) (int, error) {
	var op WritebackOp
	if err := s.db.First(&op, id).Error; err != nil {
		return 0, err
	}
	op.Attempts++
	next := now.Add(writebackBackoff(op.Attempts))
	if err := s.db.Model(&WritebackOp{}).Where("id = ?", id).
		Updates(map[string]any{
			"attempts":        op.Attempts,
			"next_attempt_at": next,
			"last_error":      cause,
		}).Error; err != nil {
		return 0, err
	}
	return op.Attempts, nil
}
