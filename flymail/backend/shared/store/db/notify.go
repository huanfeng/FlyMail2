package db

import (
	"context"
	"flymail/shared/store"
	"flymail/shared/store/model"
	"time"

	"gorm.io/gorm"
)

// notifyRepository implements the NotifyRepository interface
type notifyRepository struct {
	db    *gorm.DB
	logDB *gorm.DB
}

// NewNotifyRepository creates a new instance of notifyRepository
func NewNotifyRepository(db *gorm.DB, logDB *gorm.DB) store.NotifyRepository {
	return &notifyRepository{db: db, logDB: logDB}
}

// Channel operations

func (r *notifyRepository) CreateChannel(ctx context.Context, channel *model.NotifyChannel) error {
	return r.db.WithContext(ctx).Create(channel).Error
}

func (r *notifyRepository) GetChannelByID(ctx context.Context, id string) (*model.NotifyChannel, error) {
	var channel model.NotifyChannel
	err := r.db.WithContext(ctx).
		Preload("TimeRanges").
		Preload("Events").
		First(&channel, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &channel, nil
}

func (r *notifyRepository) GetChannels(ctx context.Context, enabled *bool) ([]*model.NotifyChannel, error) {
	var channels []*model.NotifyChannel
	query := r.db.WithContext(ctx).Preload("TimeRanges").Preload("Events")
	if enabled != nil {
		query = query.Where("enabled = ?", *enabled)
	}
	err := query.Find(&channels).Error
	return channels, err
}

func (r *notifyRepository) UpdateChannel(ctx context.Context, channel *model.NotifyChannel) error {
	return r.db.WithContext(ctx).Save(channel).Error
}

func (r *notifyRepository) DeleteChannel(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.NotifyChannel{}, "id = ?", id).Error
}

// Log operations

func (r *notifyRepository) CreateLog(ctx context.Context, log *model.NotifyLog) error {
	return r.logDB.WithContext(ctx).Create(log).Error
}

func (r *notifyRepository) GetLogByID(ctx context.Context, id uint) (*model.NotifyLog, error) {
	var log model.NotifyLog
	err := r.logDB.WithContext(ctx).Preload("Channel").First(&log, id).Error
	if err != nil {
		return nil, err
	}
	return &log, nil
}

func (r *notifyRepository) GetLogs(ctx context.Context, filter map[string]interface{}, offset, limit int) ([]*model.NotifyLog, int64, error) {
	var logs []*model.NotifyLog
	var total int64

	query := r.logDB.WithContext(ctx).Model(&model.NotifyLog{})

	// Apply filters
	if channelID, ok := filter["channel_id"].(string); ok && channelID != "" {
		query = query.Where("channel_id = ?", channelID)
	}
	if eventType, ok := filter["event_type"].(string); ok && eventType != "" {
		query = query.Where("event_type = ?", eventType)
	}
	if status, ok := filter["status"].(string); ok && status != "" {
		query = query.Where("status = ?", status)
	}
	if startTime, ok := filter["start_time"].(time.Time); ok && !startTime.IsZero() {
		query = query.Where("created_at >= ?", startTime)
	}
	if endTime, ok := filter["end_time"].(time.Time); ok && !endTime.IsZero() {
		query = query.Where("created_at <= ?", endTime)
	}

	// Count total records
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Fetch records with pagination
	err := query.
		Preload("Channel").
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&logs).Error

	return logs, total, err
}

func (r *notifyRepository) UpdateLog(ctx context.Context, log *model.NotifyLog) error {
	return r.logDB.WithContext(ctx).Save(log).Error
}

func (r *notifyRepository) GetPendingLogs(ctx context.Context, maxRetries int) ([]*model.NotifyLog, error) {
	var logs []*model.NotifyLog
	err := r.logDB.WithContext(ctx).
		Where("status = ? AND retry_count < ?", model.NotifyStatusPending, maxRetries).
		Order("created_at ASC").
		Find(&logs).Error
	return logs, err
}

func (r *notifyRepository) CleanOldLogs(ctx context.Context, before time.Time) error {
	return r.logDB.WithContext(ctx).
		Where("created_at < ?", before).
		Delete(&model.NotifyLog{}).Error
}
