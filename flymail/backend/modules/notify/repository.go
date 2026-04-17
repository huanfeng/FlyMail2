package notify

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// Repository interface for notify data access
type Repository interface {
	// Channel operations
	CreateChannel(ctx context.Context, channel *NotifyChannel) error
	UpdateChannel(ctx context.Context, channel *NotifyChannel) error
	DeleteChannel(ctx context.Context, id uint) error
	GetChannel(ctx context.Context, id uint) (*NotifyChannel, error)
	GetChannelsByUser(ctx context.Context, userID uint) ([]*NotifyChannel, error)
	GetActiveChannelsByUser(ctx context.Context, userID uint) ([]*NotifyChannel, error)
	GetChannelsByType(ctx context.Context, userID uint, channelType ChannelType) ([]*NotifyChannel, error)

	// Time range operations
	CreateTimeRange(ctx context.Context, timeRange *NotifyChannelTimeRange) error
	UpdateTimeRange(ctx context.Context, timeRange *NotifyChannelTimeRange) error
	DeleteTimeRange(ctx context.Context, id uint) error
	GetTimeRangesByChannel(ctx context.Context, channelID uint) ([]*NotifyChannelTimeRange, error)

	// Event subscription operations
	CreateEventSubscription(ctx context.Context, event *NotifyChannelEvent) error
	DeleteEventSubscription(ctx context.Context, id uint) error
	GetEventsByChannel(ctx context.Context, channelID uint) ([]*NotifyChannelEvent, error)
	UpdateChannelEvents(ctx context.Context, channelID uint, events []NotifyChannelEvent) error

	// Log operations
	CreateLog(ctx context.Context, log *NotifyLog) error
	UpdateLog(ctx context.Context, log *NotifyLog) error
	GetLog(ctx context.Context, id uint) (*NotifyLog, error)
	GetLogsByUser(ctx context.Context, userID uint, limit, offset int) ([]*NotifyLog, error)
	GetLogsByChannel(ctx context.Context, channelID uint, limit, offset int) ([]*NotifyLog, error)
	GetFailedLogs(ctx context.Context, maxRetries int) ([]*NotifyLog, error)
	CountLogsByUser(ctx context.Context, userID uint) (int64, error)
}

// repository implements Repository interface
type repository struct {
	db *gorm.DB
}

// NewRepository creates a new notify repository
func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

// Channel operations

func (r *repository) CreateChannel(ctx context.Context, channel *NotifyChannel) error {
	return r.db.WithContext(ctx).Create(channel).Error
}

func (r *repository) UpdateChannel(ctx context.Context, channel *NotifyChannel) error {
	return r.db.WithContext(ctx).Save(channel).Error
}

func (r *repository) DeleteChannel(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&NotifyChannel{}, id).Error
}

func (r *repository) GetChannel(ctx context.Context, id uint) (*NotifyChannel, error) {
	var channel NotifyChannel
	err := r.db.WithContext(ctx).
		Preload("TimeRanges").
		Preload("Events").
		First(&channel, id).Error
	if err != nil {
		return nil, err
	}
	return &channel, nil
}

func (r *repository) GetChannelsByUser(ctx context.Context, userID uint) ([]*NotifyChannel, error) {
	var channels []*NotifyChannel
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Preload("TimeRanges").
		Preload("Events").
		Order("priority DESC, created_at DESC").
		Find(&channels).Error
	return channels, err
}

func (r *repository) GetActiveChannelsByUser(ctx context.Context, userID uint) ([]*NotifyChannel, error) {
	var channels []*NotifyChannel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND status = ?", userID, ChannelStatusActive).
		Preload("TimeRanges").
		Preload("Events").
		Order("priority DESC, created_at DESC").
		Find(&channels).Error
	return channels, err
}

func (r *repository) GetChannelsByType(ctx context.Context, userID uint, channelType ChannelType) ([]*NotifyChannel, error) {
	var channels []*NotifyChannel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND type = ? AND status = ?", userID, channelType, ChannelStatusActive).
		Preload("TimeRanges").
		Preload("Events").
		Find(&channels).Error
	return channels, err
}

// Time range operations

func (r *repository) CreateTimeRange(ctx context.Context, timeRange *NotifyChannelTimeRange) error {
	return r.db.WithContext(ctx).Create(timeRange).Error
}

func (r *repository) UpdateTimeRange(ctx context.Context, timeRange *NotifyChannelTimeRange) error {
	return r.db.WithContext(ctx).Save(timeRange).Error
}

func (r *repository) DeleteTimeRange(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&NotifyChannelTimeRange{}, id).Error
}

func (r *repository) GetTimeRangesByChannel(ctx context.Context, channelID uint) ([]*NotifyChannelTimeRange, error) {
	var timeRanges []*NotifyChannelTimeRange
	err := r.db.WithContext(ctx).
		Where("channel_id = ?", channelID).
		Find(&timeRanges).Error
	return timeRanges, err
}

// Event subscription operations

func (r *repository) CreateEventSubscription(ctx context.Context, event *NotifyChannelEvent) error {
	return r.db.WithContext(ctx).Create(event).Error
}

func (r *repository) DeleteEventSubscription(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&NotifyChannelEvent{}, id).Error
}

func (r *repository) GetEventsByChannel(ctx context.Context, channelID uint) ([]*NotifyChannelEvent, error) {
	var events []*NotifyChannelEvent
	err := r.db.WithContext(ctx).
		Where("channel_id = ?", channelID).
		Find(&events).Error
	return events, err
}

func (r *repository) UpdateChannelEvents(ctx context.Context, channelID uint, events []NotifyChannelEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete existing events
		if err := tx.Where("channel_id = ?", channelID).Delete(&NotifyChannelEvent{}).Error; err != nil {
			return err
		}

		// Create new events
		for i := range events {
			events[i].ChannelID = channelID
			if err := tx.Create(&events[i]).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

// Log operations

func (r *repository) CreateLog(ctx context.Context, log *NotifyLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

func (r *repository) UpdateLog(ctx context.Context, log *NotifyLog) error {
	return r.db.WithContext(ctx).Save(log).Error
}

func (r *repository) GetLog(ctx context.Context, id uint) (*NotifyLog, error) {
	var log NotifyLog
	err := r.db.WithContext(ctx).First(&log, id).Error
	if err != nil {
		return nil, err
	}
	return &log, nil
}

func (r *repository) GetLogsByUser(ctx context.Context, userID uint, limit, offset int) ([]*NotifyLog, error) {
	var logs []*NotifyLog
	query := r.db.WithContext(ctx).Where("user_id = ?", userID)

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	err := query.Order("created_at DESC").Find(&logs).Error
	return logs, err
}

func (r *repository) GetLogsByChannel(ctx context.Context, channelID uint, limit, offset int) ([]*NotifyLog, error) {
	var logs []*NotifyLog
	query := r.db.WithContext(ctx).Where("channel_id = ?", channelID)

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	err := query.Order("created_at DESC").Find(&logs).Error
	return logs, err
}

func (r *repository) GetFailedLogs(ctx context.Context, maxRetries int) ([]*NotifyLog, error) {
	var logs []*NotifyLog
	err := r.db.WithContext(ctx).
		Where("status = ? AND retry_count < ?", LogStatusFailed, maxRetries).
		Where("created_at > ?", time.Now().Add(-24*time.Hour)). // Only retry logs from last 24 hours
		Order("created_at ASC").
		Find(&logs).Error
	return logs, err
}

func (r *repository) CountLogsByUser(ctx context.Context, userID uint) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&NotifyLog{}).
		Where("user_id = ?", userID).
		Count(&count).Error
	return count, err
}
