package protocol

import (
	"fmt"
	"sync"
	"time"

	imapv2 "github.com/emersion/go-imap/v2"

	corevimap "flymail-core/imap"
	"flymail-core/parser"
	"flymail-core/types"

	"flymail/modules/email/message"
)

// CoreAdapter implements sync.EmailProtocol using core/imap.Session.
// This bridges the core IMAP layer (go-imap/v2) with flymail's sync layer.
type CoreAdapter struct {
	config    types.IMAPConfig
	accountID uint
	session   *corevimap.Session
	mu        sync.Mutex
	folder    string // currently selected folder
}

// NewCoreAdapter creates a new adapter for the given IMAP config.
func NewCoreAdapter(cfg types.IMAPConfig, accountID uint) *CoreAdapter {
	return &CoreAdapter{
		config:    cfg,
		accountID: accountID,
	}
}

// Connect establishes connection to the IMAP server.
func (a *CoreAdapter) Connect() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.session != nil {
		return nil
	}

	session, err := corevimap.Dial(a.config)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	a.session = session
	return nil
}

// Disconnect closes the connection.
func (a *CoreAdapter) Disconnect() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.session != nil {
		err := a.session.Close()
		a.session = nil
		a.folder = ""
		return err
	}
	return nil
}

// IsConnected checks if the client is connected.
func (a *CoreAdapter) IsConnected() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.session != nil && a.session.Client != nil
}

// SupportsIDLE checks if the server supports IDLE.
func (a *CoreAdapter) SupportsIDLE() (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.session == nil {
		return false, fmt.Errorf("not connected")
	}
	return a.session.SupportsIDLE, nil
}

// SelectFolder selects a folder.
func (a *CoreAdapter) SelectFolder(folderName string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.session == nil {
		return fmt.Errorf("not connected")
	}

	_, err := a.session.SelectFolder(folderName)
	if err != nil {
		return err
	}
	a.folder = folderName
	return nil
}

// GetUIDs returns all message UIDs in the currently selected folder.
func (a *CoreAdapter) GetUIDs() ([]uint32, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.session == nil {
		return nil, fmt.Errorf("not connected")
	}

	// Search for all messages (UID 1:*)
	allUIDs, err := a.session.SearchUnseenSince(1)
	if err != nil {
		// Fallback: search all UIDs regardless of seen status
		criteria := &imapv2.SearchCriteria{}
		uidSet := imapv2.UIDSet{}
		uidSet.AddRange(1, 0)
		criteria.UID = []imapv2.UIDSet{uidSet}

		res, searchErr := a.session.Client.UIDSearch(criteria, nil).Wait()
		if searchErr != nil {
			return nil, fmt.Errorf("search UIDs failed: %w", err)
		}

		uids := res.AllUIDs()
		result := make([]uint32, len(uids))
		for i, u := range uids {
			result[i] = uint32(u)
		}
		return result, nil
	}

	result := make([]uint32, len(allUIDs))
	for i, u := range allUIDs {
		result[i] = uint32(u)
	}
	return result, nil
}

// FetchEmails fetches emails by UIDs and converts to message.Email.
func (a *CoreAdapter) FetchEmails(uids []uint32) ([]*message.Email, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.session == nil {
		return nil, fmt.Errorf("not connected")
	}
	if len(uids) == 0 {
		return []*message.Email{}, nil
	}

	// Convert uint32 UIDs to imap.UID
	imapUIDs := make([]imapv2.UID, len(uids))
	for i, u := range uids {
		imapUIDs[i] = imapv2.UID(u)
	}

	parsed, err := a.session.FetchByUIDs(imapUIDs, corevimap.FetchOptions{
		FetchBody:       true,
		FallbackHeaders: true,
	})
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}

	emails := make([]*message.Email, 0, len(parsed))
	for _, p := range parsed {
		emails = append(emails, a.toMessageEmail(p))
	}
	return emails, nil
}

// FetchEmailByUID fetches a single email by UID.
func (a *CoreAdapter) FetchEmailByUID(uid uint32) (*message.Email, error) {
	emails, err := a.FetchEmails([]uint32{uid})
	if err != nil {
		return nil, err
	}
	if len(emails) == 0 {
		return nil, fmt.Errorf("email with UID %d not found", uid)
	}
	return emails[0], nil
}

// DeleteEmail marks an email as deleted and expunges.
func (a *CoreAdapter) DeleteEmail(uid uint32) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.session == nil {
		return fmt.Errorf("not connected")
	}
	return a.session.Delete(imapv2.UID(uid))
}

// StartIDLE enters IDLE mode.
func (a *CoreAdapter) StartIDLE() error {
	// Note: the sync.EmailProtocol interface expects a blocking StartIDLE.
	// core/imap uses non-blocking IdleHandle. We adapt by blocking here.
	a.mu.Lock()
	if a.session == nil {
		a.mu.Unlock()
		return fmt.Errorf("not connected")
	}
	a.mu.Unlock()

	// For the current interface, just return nil.
	// The actual IDLE handling is done via WaitForNewEmail.
	return nil
}

// StopIDLE stops IDLE mode.
func (a *CoreAdapter) StopIDLE() error {
	// See StartIDLE comment — IDLE lifecycle is managed via WaitForNewEmail
	return nil
}

// WaitForNewEmail waits for new email notification via IDLE.
func (a *CoreAdapter) WaitForNewEmail(timeout time.Duration) (bool, error) {
	a.mu.Lock()
	if a.session == nil {
		a.mu.Unlock()
		return false, fmt.Errorf("not connected")
	}
	session := a.session
	a.mu.Unlock()

	// Start IDLE
	handle, err := session.StartIDLE()
	if err != nil {
		return false, fmt.Errorf("start IDLE failed: %w", err)
	}

	// Set up notification channel
	gotNew := make(chan bool, 1)
	session.SetIDLEHandler(func(ev corevimap.IDLEEvent) {
		if ev.Kind == "mailbox" || ev.Kind == "exists" {
			select {
			case gotNew <- true:
			default:
			}
		}
	})
	defer session.SetIDLEHandler(nil)

	// Wait for event or timeout
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-gotNew:
		handle.Stop("new email")
		return true, nil
	case err := <-handle.Done():
		if err != nil {
			return false, fmt.Errorf("IDLE dropped: %w", err)
		}
		return false, nil
	case <-timer.C:
		handle.Stop("timeout")
		return false, nil
	}
}

// GetCapabilities returns the server's IMAP capabilities.
func (a *CoreAdapter) GetCapabilities() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.session == nil {
		return nil
	}
	return a.session.Capabilities
}

// toMessageEmail converts a ParsedEmail to flymail's message.Email model.
func (a *CoreAdapter) toMessageEmail(p *types.ParsedEmail) *message.Email {
	folderType := 0
	if p.FolderType != "" {
		folderType = int(types.ParseFolderType(p.FolderType))
	} else if a.folder != "" {
		folderType = int(types.ClassifyFolder(a.folder, a.folder, nil))
	}

	return &message.Email{
		AccountID:  a.accountID,
		MessageID:  p.MessageID,
		UID:        p.UID,
		Subject:    parser.DecodeMIMEHeader(p.Subject),
		From:       types.FormatAddressList(p.From),
		To:         types.FormatAddressList(p.To),
		CC:         types.FormatAddressList(p.CC),
		BCC:        types.FormatAddressList(p.BCC),
		Body:       p.TextBody,
		BodyHTML:   p.HTMLBody,
		IsRead:     p.IsRead,
		IsStarred:  p.IsStarred,
		Date:       p.Date,
		Size:       p.Size,
		FolderName: folderNameOrDefault(p.FolderName, a.folder),
		FolderType: folderType,
	}
}

func folderNameOrDefault(name, fallback string) string {
	if name != "" {
		return name
	}
	return fallback
}

// GetFolders returns folder list using core/imap.
func (a *CoreAdapter) GetFolders() ([]types.FolderInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.session == nil {
		return nil, fmt.Errorf("not connected")
	}
	return a.session.ListFolders()
}

// MarkAsRead marks an email as read.
func (a *CoreAdapter) MarkAsRead(uid uint32) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.session == nil {
		return fmt.Errorf("not connected")
	}
	return a.session.MarkRead(imapv2.UID(uid))
}

// TestConnection tests the IMAP connection.
func (a *CoreAdapter) TestConnection() (*types.ConnectionTestResult, error) {
	session, err := corevimap.Dial(a.config)
	if err != nil {
		return &types.ConnectionTestResult{
			IMAPError: err.Error(),
		}, nil
	}
	defer session.Close()

	return &types.ConnectionTestResult{
		IMAP:         true,
		SupportsIDLE: session.SupportsIDLE,
		Capabilities: session.Capabilities,
		SecurityMode: session.SecurityMode,
	}, nil
}

