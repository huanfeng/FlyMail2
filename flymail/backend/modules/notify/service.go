package notify

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Service interface for notification operations
type Service interface {
	Manager // Embed manager interface

	// Channel management
	CreateChannel(ctx context.Context, userID uint, channel *NotifyChannel) error
	UpdateChannel(ctx context.Context, userID uint, channelID uint, channel *NotifyChannel) error
	DeleteChannel(ctx context.Context, userID uint, channelID uint) error
	GetChannel(ctx context.Context, userID uint, channelID uint) (*NotifyChannel, error)
	GetUserChannels(ctx context.Context, userID uint) ([]*NotifyChannel, error)
	TestChannel(ctx context.Context, userID uint, channelID uint) error

	// Event subscription management
	UpdateChannelEvents(ctx context.Context, userID uint, channelID uint, events []NotifyChannelEvent) error
	GetChannelEvents(ctx context.Context, userID uint, channelID uint) ([]*NotifyChannelEvent, error)

	// Log operations
	GetLogs(ctx context.Context, userID uint, limit, offset int) ([]*NotifyLog, error)
	GetChannelLogs(ctx context.Context, userID uint, channelID uint, limit, offset int) ([]*NotifyLog, error)
	CountLogs(ctx context.Context, userID uint) (int64, error)
}

// service implements Service interface
type service struct {
	repo             Repository
	logger           *zap.Logger
	channels         map[string]Channel // key: "{user_id}:{channel_id}"
	channelsMu       sync.RWMutex
	queue            chan *Event
	workers          int
	maxRetries       int
	retryInterval    time.Duration
	wg               sync.WaitGroup
	stopCh           chan struct{}
	running          bool
	runningMu        sync.RWMutex
	channelFactory   ChannelFactory
	internalChannels []Channel // Internal channels like SSE
}

// ChannelFactory creates channel instances from database models
type ChannelFactory func(channel *NotifyChannel) (Channel, error)

// Config for notification service
type Config struct {
	Workers       int
	QueueSize     int
	MaxRetries    int
	RetryInterval time.Duration
}

// NewService creates a new notification service
func NewService(repo Repository, logger *zap.Logger, config *Config, channelFactory ChannelFactory) Service {
	if config.Workers <= 0 {
		config.Workers = 4
	}
	if config.QueueSize <= 0 {
		config.QueueSize = 1000
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.RetryInterval <= 0 {
		config.RetryInterval = 5 * time.Minute
	}

	return &service{
		repo:             repo,
		logger:           logger,
		channels:         make(map[string]Channel),
		queue:            make(chan *Event, config.QueueSize),
		workers:          config.Workers,
		maxRetries:       config.MaxRetries,
		retryInterval:    config.RetryInterval,
		stopCh:           make(chan struct{}),
		channelFactory:   channelFactory,
		internalChannels: make([]Channel, 0),
	}
}

// Start starts the notification service
func (s *service) Start() error {
	s.runningMu.Lock()
	if s.running {
		s.runningMu.Unlock()
		return fmt.Errorf("service already running")
	}
	s.running = true
	s.runningMu.Unlock()

	// Start workers
	for i := 0; i < s.workers; i++ {
		s.wg.Add(1)
		go s.worker(i)
	}

	// Start retry worker
	s.wg.Add(1)
	go s.retryWorker()

	s.logger.Info("Notification service started",
		zap.Int("workers", s.workers),
		zap.Int("max_retries", s.maxRetries))

	return nil
}

// Stop stops the notification service
func (s *service) Stop() error {
	s.runningMu.Lock()
	if !s.running {
		s.runningMu.Unlock()
		return fmt.Errorf("service not running")
	}
	s.running = false
	s.runningMu.Unlock()

	// Close stop channel
	close(s.stopCh)

	// Wait for workers to finish
	s.wg.Wait()

	// Close queue
	close(s.queue)

	s.logger.Info("Notification service stopped")
	return nil
}

// RegisterChannel registers a notification channel
func (s *service) RegisterChannel(channel Channel) error {
	if channel == nil {
		return fmt.Errorf("channel cannot be nil")
	}

	// Internal channels are stored separately
	s.channelsMu.Lock()
	s.internalChannels = append(s.internalChannels, channel)
	s.channelsMu.Unlock()

	return nil
}

// UnregisterChannel unregisters a notification channel
func (s *service) UnregisterChannel(channelType ChannelType, channelID uint) error {
	key := fmt.Sprintf(":%d", channelID)

	s.channelsMu.Lock()
	delete(s.channels, key)
	s.channelsMu.Unlock()

	return nil
}

// Send sends a notification event synchronously
func (s *service) Send(event *Event) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	// Set timestamp if not set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Send to all applicable channels
	errors := make([]error, 0)

	// Send to database channels
	if event.UserID > 0 {
		channels, err := s.repo.GetActiveChannelsByUser(context.Background(), event.UserID)
		if err != nil {
			s.logger.Error("Failed to get channels", zap.Error(err))
		} else {
			for _, ch := range channels {
				channel, err := s.getOrCreateChannel(ch)
				if err != nil {
					errors = append(errors, err)
					continue
				}

				if err := s.sendToChannel(channel, event, ch.ID); err != nil {
					errors = append(errors, err)
				}
			}
		}
	}

	// Send to internal channels
	s.channelsMu.RLock()
	internalChannels := make([]Channel, len(s.internalChannels))
	copy(internalChannels, s.internalChannels)
	s.channelsMu.RUnlock()

	for _, channel := range internalChannels {
		if err := s.sendToChannel(channel, event, 0); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to send to some channels: %v", errors)
	}

	return nil
}

// SendAsync sends a notification event asynchronously
func (s *service) SendAsync(event *Event) {
	if event == nil {
		return
	}

	s.runningMu.RLock()
	if !s.running {
		s.runningMu.RUnlock()
		s.logger.Warn("Cannot send event, service not running")
		return
	}
	s.runningMu.RUnlock()

	// Try to queue the event
	select {
	case s.queue <- event:
		// Successfully queued
	default:
		// Queue is full
		s.logger.Warn("Notification queue is full, dropping event",
			zap.String("event_type", string(event.Type)))
	}
}

// GetChannels returns all registered channels (internal channels only)
func (s *service) GetChannels() []Channel {
	s.channelsMu.RLock()
	defer s.channelsMu.RUnlock()

	channels := make([]Channel, len(s.internalChannels))
	copy(channels, s.internalChannels)
	return channels
}

// Channel management

func (s *service) CreateChannel(ctx context.Context, userID uint, channel *NotifyChannel) error {
	channel.UserID = userID
	channel.Status = ChannelStatusActive

	// Validate channel configuration
	testChannel, err := s.channelFactory(channel)
	if err != nil {
		return fmt.Errorf("invalid channel configuration: %w", err)
	}
	if err := testChannel.ValidateConfig(); err != nil {
		return fmt.Errorf("invalid channel configuration: %w", err)
	}

	return s.repo.CreateChannel(ctx, channel)
}

func (s *service) UpdateChannel(ctx context.Context, userID uint, channelID uint, channel *NotifyChannel) error {
	// Verify ownership
	existing, err := s.repo.GetChannel(ctx, channelID)
	if err != nil {
		return err
	}
	if existing.UserID != userID {
		return fmt.Errorf("channel not found")
	}

	// Update fields
	existing.Name = channel.Name
	existing.Config = channel.Config
	existing.Status = channel.Status
	existing.Priority = channel.Priority
	existing.Description = channel.Description

	// Validate new configuration
	testChannel, err := s.channelFactory(existing)
	if err != nil {
		return fmt.Errorf("invalid channel configuration: %w", err)
	}
	if err := testChannel.ValidateConfig(); err != nil {
		return fmt.Errorf("invalid channel configuration: %w", err)
	}

	// Clear cached channel
	key := fmt.Sprintf("%d:%d", userID, channelID)
	s.channelsMu.Lock()
	delete(s.channels, key)
	s.channelsMu.Unlock()

	return s.repo.UpdateChannel(ctx, existing)
}

func (s *service) DeleteChannel(ctx context.Context, userID uint, channelID uint) error {
	// Verify ownership
	channel, err := s.repo.GetChannel(ctx, channelID)
	if err != nil {
		return err
	}
	if channel.UserID != userID {
		return fmt.Errorf("channel not found")
	}

	// Clear cached channel
	key := fmt.Sprintf("%d:%d", userID, channelID)
	s.channelsMu.Lock()
	delete(s.channels, key)
	s.channelsMu.Unlock()

	return s.repo.DeleteChannel(ctx, channelID)
}

func (s *service) GetChannel(ctx context.Context, userID uint, channelID uint) (*NotifyChannel, error) {
	channel, err := s.repo.GetChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if channel.UserID != userID {
		return nil, fmt.Errorf("channel not found")
	}
	return channel, nil
}

func (s *service) GetUserChannels(ctx context.Context, userID uint) ([]*NotifyChannel, error) {
	return s.repo.GetChannelsByUser(ctx, userID)
}

func (s *service) TestChannel(ctx context.Context, userID uint, channelID uint) error {
	// Get channel
	channel, err := s.GetChannel(ctx, userID, channelID)
	if err != nil {
		return err
	}

	// Create test event
	event := &Event{
		Type:      EventSystemWarning,
		Severity:  SeverityInfo,
		Title:     "测试通知",
		Message:   fmt.Sprintf("这是来自FlyMail的测试通知，用于验证 %s 渠道配置是否正确。", channel.Name),
		Timestamp: time.Now(),
		UserID:    userID,
	}

	// Create channel instance
	ch, err := s.channelFactory(channel)
	if err != nil {
		return fmt.Errorf("failed to create channel: %w", err)
	}

	// Send test notification
	if err := ch.Send(event); err != nil {
		return fmt.Errorf("failed to send test notification: %w", err)
	}

	return nil
}

// Event subscription management

func (s *service) UpdateChannelEvents(ctx context.Context, userID uint, channelID uint, events []NotifyChannelEvent) error {
	// Verify ownership
	channel, err := s.GetChannel(ctx, userID, channelID)
	if err != nil {
		return err
	}

	// Clear cached channel
	key := fmt.Sprintf("%d:%d", userID, channelID)
	s.channelsMu.Lock()
	delete(s.channels, key)
	s.channelsMu.Unlock()

	return s.repo.UpdateChannelEvents(ctx, channel.ID, events)
}

func (s *service) GetChannelEvents(ctx context.Context, userID uint, channelID uint) ([]*NotifyChannelEvent, error) {
	// Verify ownership
	if _, err := s.GetChannel(ctx, userID, channelID); err != nil {
		return nil, err
	}

	return s.repo.GetEventsByChannel(ctx, channelID)
}

// Log operations

func (s *service) GetLogs(ctx context.Context, userID uint, limit, offset int) ([]*NotifyLog, error) {
	return s.repo.GetLogsByUser(ctx, userID, limit, offset)
}

func (s *service) GetChannelLogs(ctx context.Context, userID uint, channelID uint, limit, offset int) ([]*NotifyLog, error) {
	// Verify ownership
	if _, err := s.GetChannel(ctx, userID, channelID); err != nil {
		return nil, err
	}

	return s.repo.GetLogsByChannel(ctx, channelID, limit, offset)
}

func (s *service) CountLogs(ctx context.Context, userID uint) (int64, error) {
	return s.repo.CountLogsByUser(ctx, userID)
}

// Internal methods

func (s *service) worker(id int) {
	defer s.wg.Done()

	for {
		select {
		case <-s.stopCh:
			return
		case event := <-s.queue:
			if event != nil {
				if err := s.Send(event); err != nil {
					s.logger.Error("Failed to send notification",
						zap.Int("worker", id),
						zap.String("event_type", string(event.Type)),
						zap.Error(err))
				}
			}
		}
	}
}

func (s *service) retryWorker() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.retryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.retryFailedNotifications()
		}
	}
}

func (s *service) retryFailedNotifications() {
	ctx := context.Background()
	logs, err := s.repo.GetFailedLogs(ctx, s.maxRetries)
	if err != nil {
		s.logger.Error("Failed to get failed logs", zap.Error(err))
		return
	}

	for _, log := range logs {
		// Recreate event from log
		event := &Event{
			Type:      log.EventType,
			Severity:  log.Severity,
			Title:     log.Title,
			Message:   log.Message,
			Timestamp: log.CreatedAt,
			UserID:    log.UserID,
			Data:      log.Data,
		}

		// Get channel
		channel, err := s.repo.GetChannel(ctx, log.ChannelID)
		if err != nil {
			continue
		}

		// Create channel instance
		ch, err := s.channelFactory(channel)
		if err != nil {
			continue
		}

		// Retry sending
		log.RetryCount++
		now := time.Now()

		if err := ch.Send(event); err != nil {
			log.Status = LogStatusFailed
			log.Error = err.Error()
		} else {
			log.Status = LogStatusSent
			log.SentAt = &now
			log.Error = ""
		}

		// Update log
		if err := s.repo.UpdateLog(ctx, log); err != nil {
			s.logger.Error("Failed to update log", zap.Error(err))
		}
	}
}

func (s *service) getOrCreateChannel(dbChannel *NotifyChannel) (Channel, error) {
	key := fmt.Sprintf("%d:%d", dbChannel.UserID, dbChannel.ID)

	// Check cache
	s.channelsMu.RLock()
	if ch, exists := s.channels[key]; exists {
		s.channelsMu.RUnlock()
		return ch, nil
	}
	s.channelsMu.RUnlock()

	// Create new channel
	channel, err := s.channelFactory(dbChannel)
	if err != nil {
		return nil, err
	}

	// Cache it
	s.channelsMu.Lock()
	s.channels[key] = channel
	s.channelsMu.Unlock()

	return channel, nil
}

func (s *service) sendToChannel(channel Channel, event *Event, channelID uint) error {
	// Create log entry
	log := &NotifyLog{
		UserID:    event.UserID,
		ChannelID: channelID,
		EventType: event.Type,
		Severity:  event.Severity,
		Title:     event.Title,
		Message:   event.Message,
		Status:    LogStatusPending,
		Data:      event.Data,
		CreatedAt: time.Now(),
	}

	// Send notification
	now := time.Now()
	if err := channel.Send(event); err != nil {
		log.Status = LogStatusFailed
		log.Error = err.Error()

		// Only log to database for non-internal channels
		if channelID > 0 {
			if logErr := s.repo.CreateLog(context.Background(), log); logErr != nil {
				s.logger.Error("Failed to create log", zap.Error(logErr))
			}
		}

		return err
	}

	// Success
	log.Status = LogStatusSent
	log.SentAt = &now

	// Only log to database for non-internal channels
	if channelID > 0 {
		if err := s.repo.CreateLog(context.Background(), log); err != nil {
			s.logger.Error("Failed to create log", zap.Error(err))
		}
	}

	return nil
}
