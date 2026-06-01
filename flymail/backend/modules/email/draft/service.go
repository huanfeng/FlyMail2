package draft

import "flymail/modules/email/send"

// Sender 发送邮件的最小接口（send.Service 满足此接口）。
type Sender interface {
	Send(req send.SendRequest) error
}

// Service 草稿业务层。
type Service struct{ repo *Repository }

// NewService 构建 Service。
func NewService(repo *Repository) *Service { return &Service{repo: repo} }

// Create 创建草稿并返回响应。
func (s *Service) Create(req DraftRequest) (*DraftResponse, error) {
	d := &Draft{
		AccountID:  req.AccountID,
		ToStr:      joinAddrs(req.To),
		CcStr:      joinAddrs(req.Cc),
		BccStr:     joinAddrs(req.Bcc),
		Subject:    req.Subject,
		BodyHTML:   req.BodyHTML,
		InReplyTo:  req.InReplyTo,
		References: req.References,
	}
	if err := s.repo.Create(d); err != nil {
		return nil, err
	}
	return toResponse(d), nil
}

// Update 更新草稿并返回响应；不存在时透传 ErrDraftNotFound。
func (s *Service) Update(id uint, req DraftRequest) (*DraftResponse, error) {
	d, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	d.AccountID = req.AccountID
	d.ToStr = joinAddrs(req.To)
	d.CcStr = joinAddrs(req.Cc)
	d.BccStr = joinAddrs(req.Bcc)
	d.Subject = req.Subject
	d.BodyHTML = req.BodyHTML
	d.InReplyTo = req.InReplyTo
	d.References = req.References
	if err := s.repo.Update(d); err != nil {
		return nil, err
	}
	return toResponse(d), nil
}

// Get 按 ID 查询草稿。
func (s *Service) Get(id uint) (*DraftResponse, error) {
	d, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	return toResponse(d), nil
}

// List 列出指定账户的所有草稿。
func (s *Service) List(accountID uint) ([]DraftResponse, error) {
	list, err := s.repo.ListByAccount(accountID)
	if err != nil {
		return nil, err
	}
	result := make([]DraftResponse, len(list))
	for i := range list {
		result[i] = *toResponse(&list[i])
	}
	return result, nil
}

// Delete 删除草稿。
func (s *Service) Delete(id uint) error {
	return s.repo.Delete(id)
}

// SendDraft 取草稿 → 构造 SendRequest → 发送 → 删除草稿。
func (s *Service) SendDraft(id uint, sender Sender) error {
	d, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	req := send.SendRequest{
		AccountID:  d.AccountID,
		To:         splitAddrs(d.ToStr),
		Cc:         splitAddrs(d.CcStr),
		Bcc:        splitAddrs(d.BccStr),
		Subject:    d.Subject,
		BodyHTML:   d.BodyHTML,
		InReplyTo:  d.InReplyTo,
		References: d.References,
	}
	if err := sender.Send(req); err != nil {
		return err
	}
	return s.repo.Delete(id)
}
