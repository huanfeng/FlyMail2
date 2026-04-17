package service

// ServiceRegistry holds all service instances
type ServiceRegistry struct {
	Auth    AuthService
	Email   EmailService
	Account AccountService
	Monitor MonitorService
	Setting SettingService
}

// NewServiceRegistry creates a new service registry
func NewServiceRegistry(
	auth AuthService,
	email EmailService,
	account AccountService,
	monitor MonitorService,
	setting SettingService,
) *ServiceRegistry {
	return &ServiceRegistry{
		Auth:    auth,
		Email:   email,
		Account: account,
		Monitor: monitor,
		Setting: setting,
	}
}

// GetAuthService returns the auth service
func (r *ServiceRegistry) GetAuthService() AuthService {
	return r.Auth
}

// GetAccountService returns the account service
func (r *ServiceRegistry) GetAccountService() AccountService {
	return r.Account
}

// GetEmailService returns the email service
func (r *ServiceRegistry) GetEmailService() EmailService {
	return r.Email
}

// GetSettingService returns the setting service
func (r *ServiceRegistry) GetSettingService() SettingService {
	return r.Setting
}

// GetMonitorService returns the monitor service
func (r *ServiceRegistry) GetMonitorService() MonitorService {
	return r.Monitor
}
