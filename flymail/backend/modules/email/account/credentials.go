package account

import (
	"time"

	"flymail-core/types"
)

// secret 返回该账户用于认证的凭据：OAuth 账户返回访问令牌（必要时先自动刷新），
// 密码账户返回解密后的密码。第二个返回值标明它是不是令牌。
//
// IMAPConfig 与 SMTPConfig 是全仓唯一的凭据出口，把两种认证方式的差异收敛在这里之后，
// 同步引擎与发送流程都不需要知道账户用的是密码还是令牌。
func (s *Service) secret(a *Account) (value string, isToken bool, err error) {
	if a.IsOAuth() {
		tok, err := s.AccessToken(a.ID)
		return tok, true, err
	}
	pw, err := s.enc.Decrypt(a.PasswordEnc)
	return pw, false, err
}

// IMAPConfig 取出账户并解密凭证，构建 core 的 IMAP 配置（供同步引擎使用）。
func (s *Service) IMAPConfig(id uint) (types.IMAPConfig, error) {
	a, err := s.repo.GetByID(id)
	if err != nil {
		return types.IMAPConfig{}, err
	}
	secret, isToken, err := s.secret(a)
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
	cfg := types.IMAPConfig{
		Host:         a.IMAPHost,
		Port:         a.IMAPPort,
		Username:     a.LoginName(),
		Security:     parseSecurity(a.IMAPSecurity),
		Proxy:        proxy,
		ClientName:   "FlyMail",
		ClientVendor: "FlyMail",
	}
	if isToken {
		cfg.AccessToken = secret
	} else {
		cfg.Password = secret
	}
	return cfg, nil
}

// SMTPConfig 取出账户并解密凭证，构建 core 的 SMTP 配置（供发送使用）。
func (s *Service) SMTPConfig(id uint) (types.SMTPConfig, error) {
	a, err := s.repo.GetByID(id)
	if err != nil {
		return types.SMTPConfig{}, err
	}
	secret, isToken, err := s.secret(a)
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
	cfg := types.SMTPConfig{
		Host:     a.SMTPHost,
		Port:     a.SMTPPort,
		Username: a.LoginName(),
		Security: parseSecurity(a.SMTPSecurity),
		Proxy:    proxy,
	}
	if isToken {
		cfg.AccessToken = secret
	} else {
		cfg.Password = secret
	}
	return cfg, nil
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
