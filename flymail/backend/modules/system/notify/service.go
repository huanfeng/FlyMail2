package notify

import (
	"errors"
	"strings"

	"flymail-core/logger"

	"go.uber.org/zap"
)

// Service 提供站内通知记录、外发渠道管理与事件分发。
type Service struct {
	repo *Repository
	ch   chan Event
	// dispatchFn 可在测试中替换真实 HTTP 投递。
	dispatchFn func(*Channel, Event) error
}

func NewService(repo *Repository) *Service {
	s := &Service{
		repo:       repo,
		ch:         make(chan Event, 256),
		dispatchFn: dispatch,
	}
	go s.worker()
	return s
}

// SetDispatcher 覆盖投递实现（测试注入）。
func (s *Service) SetDispatcher(fn func(*Channel, Event) error) { s.dispatchFn = fn }

// Emit 记录一条站内通知并异步投递到匹配的外发渠道。非阻塞，队列满则丢弃外发（站内已落库）。
func (s *Service) Emit(evt Event) {
	if evt.Type == "" {
		return
	}
	// 站内通知落库（best-effort）
	if err := s.repo.InsertNotification(&Notification{
		Type:      string(evt.Type),
		AccountID: evt.AccountID,
		Title:     evt.Title,
		Body:      evt.Body,
	}); err != nil {
		logger.Error("notify: 写入站内通知失败", zap.Error(err))
	}
	// 外发投递入队（非阻塞）
	select {
	case s.ch <- evt:
	default:
		logger.Warn("notify: 分发队列已满，丢弃外发", zap.String("type", string(evt.Type)))
	}
}

// EmitFunc 返回可注入到各事件源的轻量回调（解耦：事件源不依赖 notify 包）。
func (s *Service) EmitFunc() func(eventType string, accountID uint, title, body string) {
	return func(eventType string, accountID uint, title, body string) {
		s.Emit(Event{Type: EventType(eventType), AccountID: accountID, Title: title, Body: body})
	}
}

func (s *Service) worker() {
	for evt := range s.ch {
		s.deliver(evt)
	}
}

func (s *Service) deliver(evt Event) {
	channels, err := s.repo.EnabledChannelsFor(evt.Type)
	if err != nil {
		logger.Error("notify: 查询启用渠道失败", zap.Error(err))
		return
	}
	for i := range channels {
		c := &channels[i]
		entry := &Log{ChannelID: c.ID, ChannelName: c.Name, Type: string(evt.Type), Status: "ok"}
		if err := s.dispatchFn(c, evt); err != nil {
			entry.Status = "failed"
			entry.Error = err.Error()
		}
		if lerr := s.repo.InsertLog(entry); lerr != nil {
			logger.Error("notify: 写入投递日志失败", zap.Error(lerr))
		}
	}
}

// ── 站内通知 ────────────────────────────────────────────────

func (s *Service) List(beforeID uint, limit int) ([]Notification, error) {
	return s.repo.ListNotifications(beforeID, limit)
}
func (s *Service) UnreadCount() (int64, error) { return s.repo.CountUnread() }
func (s *Service) MarkRead(id uint) error      { return s.repo.MarkRead(id) }
func (s *Service) MarkAllRead() error          { return s.repo.MarkAllRead() }
func (s *Service) ClearAll() error             { return s.repo.DeleteAllNotifications() }

// ── 渠道 ────────────────────────────────────────────────────

var ErrInvalidChannel = errors.New("invalid channel")

func (s *Service) ListChannels() ([]ChannelDTO, error) {
	rows, err := s.repo.ListChannels()
	if err != nil {
		return nil, err
	}
	out := make([]ChannelDTO, 0, len(rows))
	for i := range rows {
		out = append(out, toChannelDTO(&rows[i]))
	}
	return out, nil
}

func (s *Service) CreateChannel(in ChannelInput) (*ChannelDTO, error) {
	if strings.TrimSpace(in.Name) == "" || !ValidKind(in.Kind) || strings.TrimSpace(in.URL) == "" {
		return nil, ErrInvalidChannel
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	c := &Channel{
		Name:    strings.TrimSpace(in.Name),
		Kind:    in.Kind,
		URL:     strings.TrimSpace(in.URL),
		Secret:  in.Secret,
		Events:  joinEvents(in.Events),
		Enabled: enabled,
	}
	if err := s.repo.CreateChannel(c); err != nil {
		return nil, err
	}
	dto := toChannelDTO(c)
	return &dto, nil
}

func (s *Service) UpdateChannel(id uint, in ChannelInput) (*ChannelDTO, error) {
	c, err := s.repo.GetChannel(id)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) != "" {
		c.Name = strings.TrimSpace(in.Name)
	}
	if ValidKind(in.Kind) {
		c.Kind = in.Kind
	}
	if strings.TrimSpace(in.URL) != "" {
		c.URL = strings.TrimSpace(in.URL)
	}
	// secret 留空表示不修改
	if in.Secret != "" {
		c.Secret = in.Secret
	}
	if in.Events != nil {
		c.Events = joinEvents(in.Events)
	}
	if in.Enabled != nil {
		c.Enabled = *in.Enabled
	}
	if err := s.repo.UpdateChannel(c); err != nil {
		return nil, err
	}
	dto := toChannelDTO(c)
	return &dto, nil
}

func (s *Service) DeleteChannel(id uint) error { return s.repo.DeleteChannel(id) }

// TestChannel 立即向指定渠道发送一条测试消息（同步，返回投递结果）。
func (s *Service) TestChannel(id uint) error {
	c, err := s.repo.GetChannel(id)
	if err != nil {
		return err
	}
	return s.dispatchFn(c, Event{
		Type:  EventMailNew,
		Title: "FlyMail 测试通知",
		Body:  "这是一条来自 FlyMail 通知中心的测试消息。",
	})
}

// ── 日志 ────────────────────────────────────────────────────

func (s *Service) ListLogs(limit int) ([]Log, error) { return s.repo.ListLogs(limit) }
