package task

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// ConfigRepository interface for task configuration data access
type ConfigRepository interface {
	Create(ctx context.Context, config *Config) error
	Update(ctx context.Context, config *Config) error
	Delete(ctx context.Context, id uint) error
	GetByID(ctx context.Context, id uint) (*Config, error)
	GetByTaskID(ctx context.Context, taskID string) (*Config, error)
	GetActiveByType(ctx context.Context, taskType Type) ([]*Config, error)
	GetActiveLoopTasks(ctx context.Context) ([]*Config, error)
	GetPendingOnceTasks(ctx context.Context) ([]*Config, error)
	GetAll(ctx context.Context) ([]*Config, error)
	GetByUserID(ctx context.Context, userID uint) ([]*Config, error)
	UpdateStatus(ctx context.Context, taskID string, isRunning bool, lastStatus Status, lastRunAt *time.Time) error
	UpdateNextRunAt(ctx context.Context, taskID string, nextRunAt time.Time) error
}

// LogRepository interface for task log data access
type LogRepository interface {
	Create(ctx context.Context, log *Log) error
	GetByTaskID(ctx context.Context, taskID string, limit int) ([]*Log, error)
	GetByStatus(ctx context.Context, status Status, limit int) ([]*Log, error)
	GetRecent(ctx context.Context, limit int) ([]*Log, error)
	GetByTimeRange(ctx context.Context, start, end time.Time) ([]*Log, error)
	CountByStatus(ctx context.Context, status Status) (int64, error)
	CleanOldLogs(ctx context.Context, before time.Time) error
}

// configRepository implements ConfigRepository
type configRepository struct {
	db *gorm.DB
}

// NewConfigRepository creates a new config repository
func NewConfigRepository(db *gorm.DB) ConfigRepository {
	return &configRepository{db: db}
}

func (r *configRepository) Create(ctx context.Context, config *Config) error {
	return r.db.WithContext(ctx).Create(config).Error
}

func (r *configRepository) Update(ctx context.Context, config *Config) error {
	return r.db.WithContext(ctx).Save(config).Error
}

func (r *configRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&Config{}, id).Error
}

func (r *configRepository) GetByID(ctx context.Context, id uint) (*Config, error) {
	var config Config
	err := r.db.WithContext(ctx).First(&config, id).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *configRepository) GetByTaskID(ctx context.Context, taskID string) (*Config, error) {
	var config Config
	err := r.db.WithContext(ctx).Where("task_id = ?", taskID).First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *configRepository) GetActiveByType(ctx context.Context, taskType Type) ([]*Config, error) {
	var configs []*Config
	err := r.db.WithContext(ctx).
		Where("task_type = ? AND is_active = ?", taskType, true).
		Find(&configs).Error
	return configs, err
}

func (r *configRepository) GetActiveLoopTasks(ctx context.Context) ([]*Config, error) {
	var configs []*Config
	err := r.db.WithContext(ctx).
		Where("task_type = ? AND is_active = ?", TypeLoop, true).
		Find(&configs).Error
	return configs, err
}

func (r *configRepository) GetPendingOnceTasks(ctx context.Context) ([]*Config, error) {
	var configs []*Config
	now := time.Now()
	err := r.db.WithContext(ctx).
		Where("task_type = ? AND is_active = ? AND execute_at <= ? AND (last_status IS NULL OR last_status != ?)",
			TypeOnce, true, now, StatusCompleted).
		Find(&configs).Error
	return configs, err
}

func (r *configRepository) GetAll(ctx context.Context) ([]*Config, error) {
	var configs []*Config
	err := r.db.WithContext(ctx).Find(&configs).Error
	return configs, err
}

func (r *configRepository) GetByUserID(ctx context.Context, userID uint) ([]*Config, error) {
	var configs []*Config
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&configs).Error
	return configs, err
}

func (r *configRepository) UpdateStatus(ctx context.Context, taskID string, isRunning bool, lastStatus Status, lastRunAt *time.Time) error {
	updates := map[string]interface{}{
		"is_running":  isRunning,
		"last_status": lastStatus,
	}
	if lastRunAt != nil {
		updates["last_run_at"] = lastRunAt
	}
	return r.db.WithContext(ctx).Model(&Config{}).
		Where("task_id = ?", taskID).
		Updates(updates).Error
}

func (r *configRepository) UpdateNextRunAt(ctx context.Context, taskID string, nextRunAt time.Time) error {
	return r.db.WithContext(ctx).Model(&Config{}).
		Where("task_id = ?", taskID).
		Update("next_run_at", nextRunAt).Error
}

// logRepository implements LogRepository
type logRepository struct {
	db *gorm.DB
}

// NewLogRepository creates a new log repository
func NewLogRepository(db *gorm.DB) LogRepository {
	return &logRepository{db: db}
}

func (r *logRepository) Create(ctx context.Context, log *Log) error {
	return r.db.WithContext(ctx).Create(log).Error
}

func (r *logRepository) GetByTaskID(ctx context.Context, taskID string, limit int) ([]*Log, error) {
	var logs []*Log
	query := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&logs).Error
	return logs, err
}

func (r *logRepository) GetByStatus(ctx context.Context, status Status, limit int) ([]*Log, error) {
	var logs []*Log
	query := r.db.WithContext(ctx).
		Where("status = ?", status).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&logs).Error
	return logs, err
}

func (r *logRepository) GetRecent(ctx context.Context, limit int) ([]*Log, error) {
	var logs []*Log
	query := r.db.WithContext(ctx).Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&logs).Error
	return logs, err
}

func (r *logRepository) GetByTimeRange(ctx context.Context, start, end time.Time) ([]*Log, error) {
	var logs []*Log
	err := r.db.WithContext(ctx).
		Where("created_at BETWEEN ? AND ?", start, end).
		Order("created_at DESC").
		Find(&logs).Error
	return logs, err
}

func (r *logRepository) CountByStatus(ctx context.Context, status Status) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Log{}).
		Where("status = ?", status).
		Count(&count).Error
	return count, err
}

func (r *logRepository) CleanOldLogs(ctx context.Context, before time.Time) error {
	return r.db.WithContext(ctx).
		Where("created_at < ?", before).
		Delete(&Log{}).Error
}
