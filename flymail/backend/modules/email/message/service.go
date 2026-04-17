package message

import (
	"context"

	"go.uber.org/zap"

	"flymail-core/logger"
)

// Service interface for email message operations
type Service interface {
	// Email operations
	GetEmails(ctx context.Context, userID uint, filter *EmailFilter) (*EmailList, error)
	GetEmail(ctx context.Context, userID uint, emailID uint) (*Email, error)
	UpdateEmailStatus(ctx context.Context, userID uint, emailID uint, updates map[string]interface{}) error
	DeleteEmail(ctx context.Context, userID uint, emailID uint, deleteFromServer bool) error
	// Sync operations
	CreateOrUpdateEmail(ctx context.Context, email *Email) error
	CreateEmailsBatch(ctx context.Context, emails []*Email) error
	GetLatestUID(ctx context.Context, accountID uint, folderName string) (uint32, error)
	GetUIDList(ctx context.Context, accountID uint, folderName string) ([]uint32, error)
	DeleteByUIDs(ctx context.Context, accountID uint, folderName string, uids []uint32) error
	// Stats operations
	GetAccountStats(ctx context.Context, accountID uint) (total, unread int64, err error)
	GetFolderStats(ctx context.Context, accountID uint) (map[string]struct{ Total, Unread int64 }, error)
	// Attachment operations
	GetAttachments(ctx context.Context, emailID uint) ([]*Attachment, error)
	GetAttachment(ctx context.Context, attachmentID uint) (*Attachment, error)
	// Dependency injection
	SetEmailDeleter(deleter EmailDeleter)
}

// EmailDeleter interface for server-side deletion
type EmailDeleter interface {
	DeleteEmailFromServer(ctx context.Context, accountID uint, folderName string, uid uint32) error
}

// service implements Service interface
type service struct {
	repo         Repository
	emailDeleter EmailDeleter
}

// NewService creates a new email message service
func NewService(repo Repository) Service {
	return &service{
		repo: repo,
	}
}

// SetEmailDeleter sets the email deleter for server-side deletion
func (s *service) SetEmailDeleter(deleter EmailDeleter) {
	s.emailDeleter = deleter
}

// GetEmails retrieves paginated emails
func (s *service) GetEmails(ctx context.Context, userID uint, filter *EmailFilter) (*EmailList, error) {
	// Set default pagination
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}

	emails, total, err := s.repo.GetList(ctx, userID, filter)
	if err != nil {
		logger.Error("Failed to get emails", zap.Error(err))
		return nil, err
	}

	return &EmailList{
		Emails: emails,
		Total:  total,
	}, nil
}

// GetEmail retrieves a specific email
func (s *service) GetEmail(ctx context.Context, userID uint, emailID uint) (*Email, error) {
	email, err := s.repo.GetByID(ctx, emailID, userID)
	if err != nil {
		logger.Error("Failed to get email", zap.Uint("emailID", emailID), zap.Error(err))
		return nil, err
	}

	// Load attachments
	attachments, err := s.repo.GetAttachmentsByEmailID(ctx, emailID)
	if err != nil {
		logger.Error("Failed to get attachments", zap.Uint("emailID", emailID), zap.Error(err))
		// Don't fail the request if attachments can't be loaded
	} else {
		// Convert []*Attachment to []Attachment
		for _, att := range attachments {
			if att != nil {
				email.Attachments = append(email.Attachments, *att)
			}
		}
	}

	return email, nil
}

// UpdateEmailStatus updates email status fields
func (s *service) UpdateEmailStatus(ctx context.Context, userID uint, emailID uint, updates map[string]interface{}) error {
	// Verify the email belongs to the user
	_, err := s.repo.GetByID(ctx, emailID, userID)
	if err != nil {
		return err
	}

	// Update the email
	if err := s.repo.UpdateFields(ctx, emailID, updates); err != nil {
		logger.Error("Failed to update email status", zap.Uint("emailID", emailID), zap.Error(err))
		return err
	}

	return nil
}

// DeleteEmail deletes an email
func (s *service) DeleteEmail(ctx context.Context, userID uint, emailID uint, deleteFromServer bool) error {
	// Get the email first
	email, err := s.repo.GetByID(ctx, emailID, userID)
	if err != nil {
		return err
	}

	// Delete from server if requested
	if deleteFromServer && s.emailDeleter != nil {
		if err := s.emailDeleter.DeleteEmailFromServer(ctx, email.AccountID, email.FolderName, email.UID); err != nil {
			logger.Error("Failed to delete email from server",
				zap.Uint("emailID", emailID),
				zap.Uint32("uid", email.UID),
				zap.Error(err))
			// Continue with local deletion even if server deletion fails
		}
	}

	// Delete attachments first
	if err := s.repo.DeleteAttachmentsByEmailID(ctx, emailID); err != nil {
		logger.Error("Failed to delete attachments", zap.Uint("emailID", emailID), zap.Error(err))
	}

	// Delete the email
	if err := s.repo.Delete(ctx, emailID); err != nil {
		logger.Error("Failed to delete email", zap.Uint("emailID", emailID), zap.Error(err))
		return err
	}

	return nil
}

// CreateOrUpdateEmail creates or updates an email
func (s *service) CreateOrUpdateEmail(ctx context.Context, email *Email) error {
	// Check if email already exists
	existing, err := s.repo.GetByUID(ctx, email.AccountID, email.FolderName, email.UID)
	if err != nil {
		return err
	}

	if existing != nil {
		// Update existing email
		email.EmailID = existing.EmailID
		email.CreatedAt = existing.CreatedAt
		return s.repo.Update(ctx, email)
	}

	// Create new email
	return s.repo.Create(ctx, email)
}

// CreateEmailsBatch creates multiple emails in batch
func (s *service) CreateEmailsBatch(ctx context.Context, emails []*Email) error {
	if len(emails) == 0 {
		return nil
	}

	// Filter out existing emails
	var newEmails []*Email
	for _, email := range emails {
		existing, err := s.repo.GetByUID(ctx, email.AccountID, email.FolderName, email.UID)
		if err != nil {
			return err
		}
		if existing == nil {
			newEmails = append(newEmails, email)
		}
	}

	if len(newEmails) == 0 {
		return nil
	}

	return s.repo.CreateBatch(ctx, newEmails)
}

// GetLatestUID gets the latest UID for a folder
func (s *service) GetLatestUID(ctx context.Context, accountID uint, folderName string) (uint32, error) {
	return s.repo.GetLatestUID(ctx, accountID, folderName)
}

// GetUIDList gets all UIDs for a folder
func (s *service) GetUIDList(ctx context.Context, accountID uint, folderName string) ([]uint32, error) {
	return s.repo.GetUIDList(ctx, accountID, folderName)
}

// DeleteByUIDs deletes emails by UIDs
func (s *service) DeleteByUIDs(ctx context.Context, accountID uint, folderName string, uids []uint32) error {
	return s.repo.DeleteByUIDs(ctx, accountID, folderName, uids)
}

// GetAccountStats gets statistics for an account
func (s *service) GetAccountStats(ctx context.Context, accountID uint) (total, unread int64, err error) {
	total, err = s.repo.CountByAccount(ctx, accountID)
	if err != nil {
		return 0, 0, err
	}

	unread, err = s.repo.CountUnreadByAccount(ctx, accountID)
	if err != nil {
		return 0, 0, err
	}

	return total, unread, nil
}

// GetFolderStats gets statistics for all folders in an account
func (s *service) GetFolderStats(ctx context.Context, accountID uint) (map[string]struct{ Total, Unread int64 }, error) {
	return s.repo.GetFolderStats(ctx, accountID)
}

// GetAttachments gets all attachments for an email
func (s *service) GetAttachments(ctx context.Context, emailID uint) ([]*Attachment, error) {
	return s.repo.GetAttachmentsByEmailID(ctx, emailID)
}

// GetAttachment gets a specific attachment
func (s *service) GetAttachment(ctx context.Context, attachmentID uint) (*Attachment, error) {
	return s.repo.GetAttachmentByID(ctx, attachmentID)
}
