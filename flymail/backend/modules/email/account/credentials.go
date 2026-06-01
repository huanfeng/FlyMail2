package account

import (
	"time"

	"flymail-core/types"
)

// IMAPConfig 取出账户并解密凭证，构建 core 的 IMAP 配置（供同步引擎使用）。
func (s *Service) IMAPConfig(id uint) (types.IMAPConfig, error) {
	a, err := s.repo.GetByID(id)
	if err != nil {
		return types.IMAPConfig{}, err
	}
	pw, err := s.enc.Decrypt(a.PasswordEnc)
	if err != nil {
		return types.IMAPConfig{}, err
	}
	var proxy *types.ProxyConfig
	if a.ProxyHost != "" {
		ppw, err := s.enc.Decrypt(a.ProxyPasswordEnc)
		if err != nil {
			return types.IMAPConfig{}, err
		}
		proxy = &types.ProxyConfig{
			Type: a.ProxyType, Host: a.ProxyHost, Port: a.ProxyPort,
			Username: a.ProxyUsername, Password: ppw,
		}
	}
	return types.IMAPConfig{
		Host:         a.IMAPHost,
		Port:         a.IMAPPort,
		Username:     a.LoginName(),
		Password:     pw,
		Security:     parseSecurity(a.IMAPSecurity),
		Proxy:        proxy,
		ClientName:   "FlyMail",
		ClientVendor: "FlyMail",
	}, nil
}

// SMTPConfig 取出账户并解密凭证，构建 core 的 SMTP 配置（供发送使用）。
func (s *Service) SMTPConfig(id uint) (types.SMTPConfig, error) {
	a, err := s.repo.GetByID(id)
	if err != nil {
		return types.SMTPConfig{}, err
	}
	pw, err := s.enc.Decrypt(a.PasswordEnc)
	if err != nil {
		return types.SMTPConfig{}, err
	}
	var proxy *types.ProxyConfig
	if a.ProxyHost != "" {
		ppw, err := s.enc.Decrypt(a.ProxyPasswordEnc)
		if err != nil {
			return types.SMTPConfig{}, err
		}
		proxy = &types.ProxyConfig{
			Type: a.ProxyType, Host: a.ProxyHost, Port: a.ProxyPort,
			Username: a.ProxyUsername, Password: ppw,
		}
	}
	return types.SMTPConfig{
		Host:     a.SMTPHost,
		Port:     a.SMTPPort,
		Username: a.LoginName(),
		Password: pw,
		Security: parseSecurity(a.SMTPSecurity),
		Proxy:    proxy,
	}, nil
}

// TouchLastSync 更新账户的最后同步时间。
func (s *Service) TouchLastSync(id uint, t time.Time) error {
	a, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	a.LastSyncAt = &t
	return s.repo.Update(a)
}
