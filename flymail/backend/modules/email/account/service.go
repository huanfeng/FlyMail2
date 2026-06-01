package account

import "flymail/internal/crypto"

type Service struct {
	repo *Repository
	enc  *crypto.Encryptor
}

func NewService(repo *Repository, enc *crypto.Encryptor) *Service {
	return &Service{repo: repo, enc: enc}
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
	return s.repo.SetEnabled(id, enabled)
}

func (s *Service) IsEnabled(id uint) (bool, error) {
	return s.repo.IsEnabled(id)
}

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
