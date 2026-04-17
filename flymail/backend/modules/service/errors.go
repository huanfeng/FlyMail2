package service

import "errors"

// Common service errors
var (
	// Account errors
	ErrAccountNotFound     = errors.New("account not found")
	ErrAccountExists       = errors.New("account already exists")
	ErrAccountUnauthorized = errors.New("unauthorized access to account")

	// Email errors
	ErrEmailNotFound   = errors.New("email not found")
	ErrEmailSendFailed = errors.New("failed to send email")
	ErrEmailSyncFailed = errors.New("failed to sync emails")

	// Connection errors
	ErrConnectionFailed     = errors.New("connection failed")
	ErrAuthenticationFailed = errors.New("authentication failed")

	// Task errors
	ErrTaskNotFound      = errors.New("task not found")
	ErrTaskAlreadyExists = errors.New("task already exists")

	// User errors
	ErrUserNotFound       = errors.New("user not found")
	ErrUserAlreadyExists  = errors.New("user already exists")
	ErrInvalidCredentials = errors.New("invalid credentials")
)
