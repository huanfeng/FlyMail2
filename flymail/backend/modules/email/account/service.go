package account

import "flymail/internal/crypto"

type Service struct {
	repo *Repository
	enc  *crypto.Encryptor
	// emit 通知回调（解耦：account 包不依赖 notify 包，由 app 装配）。
	emit func(eventType string, accountID uint, title, body string)
}

func NewService(repo *Repository, enc *crypto.Encryptor) *Service {
	return &Service{repo: repo, enc: enc}
}

// SetEmitter 注入通知回调（账户状态变化等事件）。
func (s *Service) SetEmitter(fn func(eventType string, accountID uint, title, body string)) {
	s.emit = fn
}

func (s *Service) Create(req CreateAccountRequest) (*AccountResponse, error) {
	encPw, err := s.enc.Encrypt(req.Password)
	if err != nil {
		return nil, err
	}
	a := &Account{
		Name: req.Name, Email: req.Email, Username: req.Username,
		AuthType: "password", PasswordEnc: encPw,
		IMAPHost: req.IMAPHost, IMAPPort: req.IMAPPort, IMAPSecurity: req.IMAPSecurity,
		SMTPHost: req.SMTPHost, SMTPPort: req.SMTPPort, SMTPSecurity: req.SMTPSecurity,
		Status:  "new",
		Enabled: true,
	}
	if err := s.applyProxy(a, req.Proxy); err != nil {
		return nil, err
	}
	if err := s.repo.Create(a); err != nil {
		return nil, err
	}
	resp := toResponse(a)
	return &resp, nil
}

func (s *Service) Get(id uint) (*AccountResponse, error) {
	a, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	resp := toResponse(a)
	return &resp, nil
}

func (s *Service) List() ([]AccountResponse, error) {
	list, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	out := make([]AccountResponse, 0, len(list))
	for i := range list {
		out = append(out, toResponse(&list[i]))
	}
	return out, nil
}

func (s *Service) Update(id uint, req UpdateAccountRequest) (*AccountResponse, error) {
	a, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	a.Name = req.Name
	a.Email = req.Email
	a.Username = req.Username
	a.IMAPHost = req.IMAPHost
	a.IMAPPort = req.IMAPPort
	a.IMAPSecurity = req.IMAPSecurity
	a.SMTPHost = req.SMTPHost
	a.SMTPPort = req.SMTPPort
	a.SMTPSecurity = req.SMTPSecurity
	if req.Password != "" {
		encPw, err := s.enc.Encrypt(req.Password)
		if err != nil {
			return nil, err
		}
		a.PasswordEnc = encPw
	}
	if err := s.applyProxy(a, req.Proxy); err != nil {
		return nil, err
	}
	if err := s.repo.Update(a); err != nil {
		return nil, err
	}
	resp := toResponse(a)
	return &resp, nil
}

func (s *Service) Delete(id uint) error { return s.repo.Delete(id) }

func (s *Service) SetEnabled(id uint, enabled bool) error {
	if err := s.repo.SetEnabled(id, enabled); err != nil {
		return err
	}
	// 站内/外发通知：账户状态变化
	if s.emit != nil {
		name := ""
		if a, err := s.repo.GetByID(id); err == nil {
			name = a.Name
			if name == "" {
				name = a.Email
			}
		}
		state := "已停用"
		if enabled {
			state = "已启用"
		}
		s.emit("account_status", id, "账户状态变化", "账户「"+name+"」"+state)
	}
	return nil
}

func (s *Service) IsEnabled(id uint) (bool, error) {
	return s.repo.IsEnabled(id)
}

// ListEnabledIDs 透传启用账户 ID 列表（供同步管理器调度）。
func (s *Service) ListEnabledIDs() ([]uint, error) { return s.repo.ListEnabledIDs() }

func (s *Service) applyProxy(a *Account, p *ProxyDTO) error {
	if p == nil || p.Host == "" {
		a.ProxyType, a.ProxyHost, a.ProxyPort, a.ProxyUsername, a.ProxyPasswordEnc = "", "", 0, "", ""
		return nil
	}
	a.ProxyType = p.Type
	a.ProxyHost = p.Host
	a.ProxyPort = p.Port
	a.ProxyUsername = p.Username
	if p.Password != "" {
		enc, err := s.enc.Encrypt(p.Password)
		if err != nil {
			return err
		}
		a.ProxyPasswordEnc = enc
	}
	return nil
}
