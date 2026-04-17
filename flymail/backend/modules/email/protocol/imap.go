package protocol

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"mime"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap"
	idle "github.com/emersion/go-imap-idle"
	"github.com/emersion/go-imap/client"
	gomessage "github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset" // Import charset support
	"github.com/emersion/go-message/mail"
	"go.uber.org/zap"
	"golang.org/x/text/encoding/simplifiedchinese"

	emailmessage "flymail/modules/email/message"
	"flymail/pkg/logger"
)

func init() {
	// Register additional charset support
	originalCharsetReader := gomessage.CharsetReader
	gomessage.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		// Handle euc-cn as gb18030 (which is backward compatible)
		switch strings.ToLower(charset) {
		case "euc-cn":
			return simplifiedchinese.GB18030.NewDecoder().Reader(input), nil
		}

		// Fall back to the original charset reader
		if originalCharsetReader != nil {
			return originalCharsetReader(charset, input)
		}
		return nil, fmt.Errorf("unhandled charset %q", charset)
	}
}

// IMAPClient implements EmailProtocol interface for IMAP
type IMAPClient struct {
	host           string
	port           int
	username       string
	password       string
	useSSL         bool
	accountID      uint
	conn           *client.Client
	idleClient     *idle.Client
	mu             sync.Mutex
	currentFolder  string
	currentMailbox *imap.MailboxStatus
}

// NewIMAPClient creates a new IMAP client
func NewIMAPClient(host string, port int, username, password string, useSSL bool, accountID uint) *IMAPClient {
	return &IMAPClient{
		host:      host,
		port:      port,
		username:  username,
		password:  password,
		useSSL:    useSSL,
		accountID: accountID,
	}
}

// Connect establishes connection to the IMAP server
func (c *IMAPClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		// Already connected
		return nil
	}

	addr := fmt.Sprintf("%s:%d", c.host, c.port)
	var conn *client.Client
	var err error

	if c.useSSL {
		conn, err = client.DialTLS(addr, &tls.Config{
			ServerName: c.host,
		})
	} else {
		conn, err = client.Dial(addr)
		if err == nil && conn != nil {
			// Try to upgrade to TLS
			if ok, _ := conn.SupportStartTLS(); ok {
				if err := conn.StartTLS(&tls.Config{
					ServerName: c.host,
				}); err != nil {
					logger.Warn("Failed to upgrade to TLS", zap.Error(err))
				}
			}
		}
	}

	if err != nil {
		return fmt.Errorf("failed to connect to IMAP server: %w", err)
	}

	// Login
	if err := conn.Login(c.username, c.password); err != nil {
		conn.Logout()
		return fmt.Errorf("IMAP login failed: %w", err)
	}

	c.conn = conn
	c.idleClient = idle.NewClient(conn)

	return nil
}

// Disconnect closes the connection to the IMAP server
func (c *IMAPClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		err := c.conn.Logout()
		c.conn = nil
		c.idleClient = nil
		c.currentFolder = ""
		c.currentMailbox = nil
		return err
	}
	return nil
}

// IsConnected checks if the client is connected
func (c *IMAPClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil && c.conn.State() == imap.AuthenticatedState
}

// SupportsIDLE checks if the server supports IDLE
func (c *IMAPClient) SupportsIDLE() (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return false, fmt.Errorf("not connected")
	}

	supported, err := c.idleClient.SupportIdle()
	return supported, err
}

// SelectFolder selects a folder
func (c *IMAPClient) SelectFolder(folderName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	mbox, err := c.conn.Select(folderName, false)
	if err != nil {
		return fmt.Errorf("failed to select folder %s: %w", folderName, err)
	}

	c.currentFolder = folderName
	c.currentMailbox = mbox
	return nil
}

// GetUIDs gets all message UIDs in the current folder
func (c *IMAPClient) GetUIDs() ([]uint32, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil, fmt.Errorf("not connected")
	}

	if c.currentMailbox == nil || c.currentMailbox.Messages == 0 {
		return []uint32{}, nil
	}

	// Create sequence set for all messages
	seqSet := new(imap.SeqSet)
	seqSet.AddRange(1, c.currentMailbox.Messages)

	// Fetch UIDs
	messages := make(chan *imap.Message, 100)
	done := make(chan error, 1)

	go func() {
		done <- c.conn.Fetch(seqSet, []imap.FetchItem{imap.FetchUid}, messages)
	}()

	var uids []uint32
	for msg := range messages {
		uids = append(uids, msg.Uid)
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("failed to fetch UIDs: %w", err)
	}

	return uids, nil
}

// FetchEmails fetches emails by UIDs
func (c *IMAPClient) FetchEmails(uids []uint32) ([]*emailmessage.Email, error) {
	if len(uids) == 0 {
		return []*emailmessage.Email{}, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil, fmt.Errorf("not connected")
	}

	var emails []*emailmessage.Email

	// Process in batches to avoid timeouts
	const batchSize = 50
	for i := 0; i < len(uids); i += batchSize {
		end := i + batchSize
		if end > len(uids) {
			end = len(uids)
		}

		batch := uids[i:end]
		batchEmails, err := c.fetchEmailBatch(batch)
		if err != nil {
			return nil, err
		}

		emails = append(emails, batchEmails...)
	}

	return emails, nil
}

// fetchEmailBatch fetches a batch of emails
func (c *IMAPClient) fetchEmailBatch(uids []uint32) ([]*emailmessage.Email, error) {
	seqSet := new(imap.SeqSet)
	for _, uid := range uids {
		seqSet.AddNum(uid)
	}

	items := []imap.FetchItem{
		imap.FetchEnvelope,
		imap.FetchFlags,
		imap.FetchInternalDate,
		imap.FetchRFC822Size,
		imap.FetchUid,
		imap.FetchBodyStructure,
		"BODY[]",
	}

	messages := make(chan *imap.Message, len(uids))
	done := make(chan error, 1)

	go func() {
		done <- c.conn.UidFetch(seqSet, items, messages)
	}()

	var emails []*emailmessage.Email
	for msg := range messages {
		email := c.parseIMAPMessage(msg)
		if email != nil {
			emails = append(emails, email)
		}
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("failed to fetch emails: %w", err)
	}

	return emails, nil
}

// FetchEmailByUID fetches a single email by UID
func (c *IMAPClient) FetchEmailByUID(uid uint32) (*emailmessage.Email, error) {
	emails, err := c.FetchEmails([]uint32{uid})
	if err != nil {
		return nil, err
	}

	if len(emails) == 0 {
		return nil, fmt.Errorf("email with UID %d not found", uid)
	}

	return emails[0], nil
}

// DeleteEmail deletes an email by UID
func (c *IMAPClient) DeleteEmail(uid uint32) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	// Mark as deleted
	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uid)

	item := imap.FormatFlagsOp(imap.AddFlags, true)
	flags := []interface{}{imap.DeletedFlag}

	if err := c.conn.UidStore(seqSet, item, flags, nil); err != nil {
		return fmt.Errorf("failed to mark email as deleted: %w", err)
	}

	// Expunge to permanently delete
	if err := c.conn.Expunge(nil); err != nil {
		return fmt.Errorf("failed to expunge: %w", err)
	}

	return nil
}

// StartIDLE starts IDLE mode
func (c *IMAPClient) StartIDLE() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.idleClient == nil {
		return fmt.Errorf("IDLE not supported")
	}

	err := c.idleClient.IdleWithFallback(nil, 0)
	return err
}

// StopIDLE stops IDLE mode
func (c *IMAPClient) StopIDLE() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.idleClient == nil {
		return fmt.Errorf("IDLE not supported")
	}

	// TODO: Implement proper IDLE stop
	return nil
}

// WaitForNewEmail waits for new email notification via IDLE
func (c *IMAPClient) WaitForNewEmail(timeout time.Duration) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Wait for update or timeout
	select {
	case <-ctx.Done():
		return false, nil // Timeout, no new emails
	default:
		// TODO: Implement proper IDLE update handling
		// The idle client API has changed, need to update this implementation
		return false, nil
	}
}

// parseIMAPMessage converts an IMAP message to our Email model
func (c *IMAPClient) parseIMAPMessage(msg *imap.Message) *emailmessage.Email {
	if msg == nil || msg.Envelope == nil {
		return nil
	}

	email := &emailmessage.Email{
		AccountID:  c.accountID,
		UID:        msg.Uid,
		MessageID:  msg.Envelope.MessageId,
		Subject:    decodeRFC2047(msg.Envelope.Subject),
		Date:       msg.Envelope.Date,
		FolderName: c.currentFolder,
		Size:       int64(msg.Size),
		FolderType: determineFolderType(c.currentFolder),
	}

	// Parse addresses
	if len(msg.Envelope.From) > 0 {
		email.From = formatAddresses(msg.Envelope.From)
	}
	if len(msg.Envelope.To) > 0 {
		email.To = formatAddresses(msg.Envelope.To)
	}
	if len(msg.Envelope.Cc) > 0 {
		email.CC = formatAddresses(msg.Envelope.Cc)
	}
	if len(msg.Envelope.Bcc) > 0 {
		email.BCC = formatAddresses(msg.Envelope.Bcc)
	}

	// Parse flags
	for _, flag := range msg.Flags {
		switch flag {
		case imap.SeenFlag:
			email.IsRead = true
		case imap.FlaggedFlag:
			email.IsStarred = true
		}
	}

	// Parse body
	section := &imap.BodySectionName{}
	if body := msg.GetBody(section); body != nil {
		if err := c.parseBodyFromReader(body, email); err != nil {
			logger.Error("Failed to parse email body",
				zap.Uint32("uid", msg.Uid),
				zap.Error(err))
		}
	}

	return email
}

// parseBody parses the email body and attachments
func (c *IMAPClient) parseBodyFromReader(body io.Reader, email *emailmessage.Email) error {
	if body == nil {
		return fmt.Errorf("no body found")
	}

	// Create mail reader
	mr, err := mail.CreateReader(body)
	if err != nil {
		return fmt.Errorf("failed to create mail reader: %w", err)
	}
	defer mr.Close()

	// Process parts
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read part: %w", err)
		}

		switch h := p.Header.(type) {
		case *mail.InlineHeader:
			// Text content
			b, _ := io.ReadAll(p.Body)
			contentType, _, _ := h.ContentType()

			if strings.HasPrefix(contentType, "text/plain") {
				email.Body = string(b)
			} else if strings.HasPrefix(contentType, "text/html") {
				email.BodyHTML = string(b)
			}

		case *mail.AttachmentHeader:
			// Attachment
			filename, _ := h.Filename()
			if filename != "" {
				email.Attachments = append(email.Attachments, emailmessage.Attachment{
					Filename:    filename,
					ContentType: h.Get("Content-Type"),
					Size:        0, // Size will be determined when saving
				})
			}
		}
	}

	return nil
}

// formatAddresses formats email addresses
func formatAddresses(addrs []*imap.Address) string {
	var result []string
	for _, addr := range addrs {
		if addr == nil {
			continue
		}

		formatted := fmt.Sprintf("%s@%s", addr.MailboxName, addr.HostName)
		if addr.PersonalName != "" {
			name := decodeRFC2047(addr.PersonalName)
			formatted = fmt.Sprintf("%s <%s>", name, formatted)
		}
		result = append(result, formatted)
	}
	return strings.Join(result, ", ")
}

// determineFolderType determines the folder type based on name
func determineFolderType(folderName string) int {
	lowerName := strings.ToLower(folderName)

	switch lowerName {
	case "inbox":
		return 1 // FolderTypeInbox
	case "sent", "sent mail", "sent items":
		return 2 // FolderTypeSent
	case "drafts":
		return 3 // FolderTypeDrafts
	case "trash", "deleted items":
		return 4 // FolderTypeTrash
	case "spam", "junk":
		return 5 // FolderTypeSpam
	case "archive":
		return 6 // FolderTypeArchive
	default:
		return 0 // FolderTypeCustom
	}
}

// decodeRFC2047 decodes RFC 2047 encoded strings
func decodeRFC2047(s string) string {
	dec := new(mime.WordDecoder)
	decoded, err := dec.DecodeHeader(s)
	if err != nil {
		return s
	}
	return decoded
}
