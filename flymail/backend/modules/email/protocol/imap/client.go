package imap

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset" // Import charset support for GBK, GB2312, etc.
	"github.com/emersion/go-message/mail"
	"go.uber.org/zap"
	"golang.org/x/text/encoding/simplifiedchinese"

	"flymail/modules/email/protocol"
	"flymail-core/logger"
	"flymail/pkg/utils"
)

func init() {
	// Register additional charset support
	// The default charset package should handle most cases, but we add custom support for edge cases
	originalCharsetReader := message.CharsetReader
	message.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
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

// IMAPClient represents an IMAP client
type IMAPClient struct {
	host     string
	port     int
	username string
	password string
	useSSL   bool
}

const (
	// BatchSize is the number of emails to fetch in each batch
	// This prevents timeouts and memory issues when fetching large numbers of emails
	BatchSize = 100 // Reduced from 500 to prevent timeouts on slow connections
)

// IMAPSession represents a reusable IMAP connection session
type IMAPSession struct {
	client *IMAPClient
	conn   *client.Client
}

// NewSession creates a new IMAP session with a persistent connection
func (c *IMAPClient) NewSession() (*IMAPSession, error) {
	conn, err := c.connect()
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &IMAPSession{
		client: c,
		conn:   conn,
	}, nil
}

// Close closes the IMAP session
func (s *IMAPSession) Close() error {
	if s.conn != nil {
		return s.conn.Logout()
	}
	return nil
}

// FetchEmailsFromFolder fetches emails from a specific folder using the existing connection
func (s *IMAPSession) FetchEmailsFromFolder(folderName string, limit int) ([]*protocol.EmailData, error) {
	if s.conn == nil {
		return nil, fmt.Errorf("session connection is nil")
	}

	// Select the specified folder
	mbox, err := s.conn.Select(folderName, false)
	if err != nil {
		return nil, fmt.Errorf("failed to select folder %s: %w", folderName, err)
	}

	// If folder is empty, return empty result
	if mbox.Messages == 0 {
		return []*protocol.EmailData{}, nil
	}

	// First, we need to get the UIDs of the messages we want to fetch
	// If limit is specified, we need to find the most recent messages by UID
	var uids []uint32

	if limit > 0 && mbox.Messages > uint32(limit) {
		// Search for all messages to get their UIDs, then take the most recent ones
		criteria := imap.NewSearchCriteria()
		criteria.WithoutFlags = []string{} // Search all messages

		allUids, err := s.conn.UidSearch(criteria)
		if err != nil {
			return nil, fmt.Errorf("failed to search for UIDs: %w", err)
		}

		// Take the last 'limit' UIDs (most recent)
		if len(allUids) > limit {
			uids = allUids[len(allUids)-limit:]
		} else {
			uids = allUids
		}
	} else {
		// Fetch all messages - use UID range
		// We'll fetch using UID range 1:* which means all messages
		uids = nil // Signal to use range-based fetching
	}

	logger.Debug("Preparing to fetch emails from session using UIDs",
		zap.String("folder", folderName),
		zap.Int("uids_count", len(uids)),
		zap.Uint32("total_in_mailbox", mbox.Messages),
		zap.Int("limit", limit),
	)

	var allEmails []*protocol.EmailData

	if uids == nil {
		// Use UID range for all messages
		seqset := new(imap.SeqSet)
		seqset.AddRange(1, 0) // 1:* means all messages

		messages := make(chan *imap.Message, 10)
		done := make(chan error, 1)

		go func() {
			done <- s.conn.UidFetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid, imap.FetchRFC822Size}, messages)
		}()

		// Process messages
		for msg := range messages {
			emailData, err := s.client.parseMessage(msg)
			if err != nil {
				logger.Warn("Failed to parse message", zap.Error(err))
				continue
			}

			// Fetch body separately if needed
			if emailData.Body == "" && emailData.HTMLBody == "" && msg.Uid > 0 {
				bodyData, err := s.FetchEmailBody(folderName, msg.Uid)
				if err != nil {
					logger.Warn("Failed to fetch email body",
						zap.Uint32("uid", msg.Uid),
						zap.String("subject", emailData.Subject),
						zap.Error(err))
				} else {
					emailData.Body = bodyData.Body
					emailData.HTMLBody = bodyData.HTMLBody
				}
			}

			emailData.FolderName = folderName
			allEmails = append(allEmails, emailData)
		}

		if err := <-done; err != nil {
			return nil, fmt.Errorf("failed to fetch messages: %w", err)
		}
	} else {
		// Fetch specific UIDs in batches
		for i := 0; i < len(uids); i += BatchSize {
			// Calculate batch end
			batchEnd := i + BatchSize
			if batchEnd > len(uids) {
				batchEnd = len(uids)
			}

			batchUids := uids[i:batchEnd]

			logger.Debug("Fetching batch of emails by UIDs",
				zap.Int("batch_start", i),
				zap.Int("batch_end", batchEnd),
				zap.Int("batch_size", len(batchUids)),
			)

			// Create UID set for this batch
			seqset := new(imap.SeqSet)
			for _, uid := range batchUids {
				seqset.AddNum(uid)
			}

			messages := make(chan *imap.Message, 10)
			done := make(chan error, 1)

			go func() {
				done <- s.conn.UidFetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid, imap.FetchRFC822Size}, messages)
			}()

			// Process messages
			batchEmails := make([]*protocol.EmailData, 0)
			for msg := range messages {
				emailData, err := s.client.parseMessage(msg)
				if err != nil {
					logger.Warn("Failed to parse message", zap.Error(err))
					continue
				}

				// Fetch body separately if needed
				if emailData.Body == "" && emailData.HTMLBody == "" && msg.Uid > 0 {
					bodyData, err := s.FetchEmailBody(folderName, msg.Uid)
					if err != nil {
						logger.Warn("Failed to fetch email body",
							zap.Uint32("uid", msg.Uid),
							zap.String("subject", emailData.Subject),
							zap.Error(err))
					} else {
						emailData.Body = bodyData.Body
						emailData.HTMLBody = bodyData.HTMLBody
					}
				}

				emailData.FolderName = folderName
				batchEmails = append(batchEmails, emailData)
			}

			if err := <-done; err != nil {
				return nil, fmt.Errorf("failed to fetch messages: %w", err)
			}

			// Append batch to all emails
			allEmails = append(allEmails, batchEmails...)

			logger.Debug("Batch fetch completed",
				zap.Int("batch_emails_count", len(batchEmails)),
				zap.Int("total_emails_so_far", len(allEmails)),
			)
		}
	}

	logger.Debug("Completed fetching emails from session",
		zap.String("folder", folderName),
		zap.Int("total_emails_fetched", len(allEmails)),
	)

	return allEmails, nil
}

// FetchEmailsFromFolderWithContext fetches emails from a specific folder with context support
func (s *IMAPSession) FetchEmailsFromFolderWithContext(ctx context.Context, folderName string, limit int) ([]*protocol.EmailData, error) {
	// For better control, implement context checking within the fetch process
	if s.conn == nil {
		return nil, fmt.Errorf("session connection is nil")
	}

	// Select the specified folder
	mbox, err := s.conn.Select(folderName, false)
	if err != nil {
		return nil, fmt.Errorf("failed to select folder %s: %w", folderName, err)
	}

	// If folder is empty, return empty result
	if mbox.Messages == 0 {
		return []*protocol.EmailData{}, nil
	}

	// First, we need to get the UIDs of the messages we want to fetch
	var uids []uint32

	if limit > 0 && mbox.Messages > uint32(limit) {
		// Search for all messages to get their UIDs
		criteria := imap.NewSearchCriteria()
		criteria.WithoutFlags = []string{} // Search all messages

		allUids, err := s.conn.UidSearch(criteria)
		if err != nil {
			return nil, fmt.Errorf("failed to search for UIDs: %w", err)
		}

		// Take the last 'limit' UIDs (most recent)
		if len(allUids) > limit {
			uids = allUids[len(allUids)-limit:]
		} else {
			uids = allUids
		}
	}

	logger.Debug("Preparing to fetch emails with context",
		zap.String("folder", folderName),
		zap.Int("uids_count", len(uids)),
		zap.Uint32("total_in_mailbox", mbox.Messages),
		zap.Int("limit", limit))

	var allEmails []*protocol.EmailData

	// Fetch emails with context checking
	if len(uids) == 0 {
		// Fetch all using range
		from := uint32(1)
		to := mbox.Messages

		if limit > 0 && mbox.Messages > uint32(limit) {
			from = to - uint32(limit) + 1
		}

		for currentFrom := from; currentFrom <= to; {
			// Check context
			select {
			case <-ctx.Done():
				logger.Warn("Context cancelled during batch fetch",
					zap.String("folder", folderName),
					zap.Int("fetched", len(allEmails)))
				return allEmails, ctx.Err()
			default:
			}

			batchTo := currentFrom + BatchSize - 1
			if batchTo > to {
				batchTo = to
			}

			logger.Debug("Fetching batch with context check",
				zap.Uint32("from", currentFrom),
				zap.Uint32("to", batchTo))

			// Fetch batch - use regular Fetch with sequence numbers
			seqset := new(imap.SeqSet)
			seqset.AddRange(currentFrom, batchTo)

			messages := make(chan *imap.Message, 10)
			done := make(chan error, 1)

			go func() {
				done <- s.conn.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid, imap.FetchRFC822Size}, messages)
			}()

			// Process messages with timeout
			batchEmails := make([]*protocol.EmailData, 0)
			for msg := range messages {
				emailData, err := s.client.parseMessage(msg)
				if err != nil {
					logger.Warn("Failed to parse message", zap.Error(err))
					continue
				}

				// Fetch body if needed
				if emailData.Body == "" && emailData.HTMLBody == "" && msg.Uid > 0 {
					bodyData, err := s.FetchEmailBody(folderName, msg.Uid)
					if err != nil {
						logger.Warn("Failed to fetch email body",
							zap.Uint32("uid", msg.Uid),
							zap.String("subject", emailData.Subject),
							zap.Error(err))
					} else {
						emailData.Body = bodyData.Body
						emailData.HTMLBody = bodyData.HTMLBody
					}
				}
				emailData.FolderName = folderName
				batchEmails = append(batchEmails, emailData)
			}

			if err := <-done; err != nil {
				logger.Error("Failed to fetch batch",
					zap.Uint32("from", currentFrom),
					zap.Uint32("to", batchTo),
					zap.Error(err))
				if len(allEmails) > 0 {
					// Return partial results
					return allEmails, nil
				}
				return nil, fmt.Errorf("failed to fetch messages: %w", err)
			}

			allEmails = append(allEmails, batchEmails...)
			currentFrom = batchTo + 1

			logger.Debug("Batch completed",
				zap.String("folder", folderName),
				zap.Int("batch_size", len(batchEmails)),
				zap.Int("total_fetched", len(allEmails)))
		}
	} else {
		// Fetch specific UIDs
		for i := 0; i < len(uids); i += BatchSize {
			// Check context
			select {
			case <-ctx.Done():
				logger.Warn("Context cancelled during UID fetch",
					zap.String("folder", folderName),
					zap.Int("fetched", len(allEmails)))
				return allEmails, ctx.Err()
			default:
			}

			batchEnd := i + BatchSize
			if batchEnd > len(uids) {
				batchEnd = len(uids)
			}

			batchUids := uids[i:batchEnd]
			seqset := new(imap.SeqSet)
			for _, uid := range batchUids {
				seqset.AddNum(uid)
			}

			messages := make(chan *imap.Message, 10)
			done := make(chan error, 1)

			go func() {
				done <- s.conn.UidFetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid, imap.FetchRFC822Size}, messages)
			}()

			// Process messages
			for msg := range messages {
				emailData, err := s.client.parseMessage(msg)
				if err != nil {
					logger.Warn("Failed to parse message", zap.Error(err))
					continue
				}

				emailData.FolderName = folderName
				allEmails = append(allEmails, emailData)
			}

			if err := <-done; err != nil {
				logger.Error("Failed to fetch UID batch", zap.Error(err))
				if len(allEmails) > 0 {
					return allEmails, nil
				}
				return nil, err
			}
		}
	}

	return allEmails, nil
}

// FetchEmailsFromFolderSince fetches emails from a specific folder since a specific date using the existing connection
func (s *IMAPSession) FetchEmailsFromFolderSince(folderName string, since time.Time) ([]*protocol.EmailData, error) {
	if s.conn == nil {
		return nil, fmt.Errorf("session connection is nil")
	}

	// Select the specified folder
	_, err := s.conn.Select(folderName, false)
	if err != nil {
		return nil, fmt.Errorf("failed to select folder %s: %w", folderName, err)
	}

	// Search for messages since date
	criteria := imap.NewSearchCriteria()
	criteria.Since = since

	ids, err := s.conn.Search(criteria)
	if err != nil {
		return nil, fmt.Errorf("failed to search messages: %w", err)
	}

	if len(ids) == 0 {
		return []*protocol.EmailData{}, nil
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(ids...)

	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)

	go func() {
		done <- s.conn.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid, imap.FetchRFC822Size}, messages)
	}()

	var emails []*protocol.EmailData
	for msg := range messages {
		emailData, err := s.client.parseMessage(msg)
		if err != nil {
			logger.Warn("Failed to parse message", zap.Error(err))
			continue
		}

		// Fetch body separately if needed
		if emailData.Body == "" && emailData.HTMLBody == "" && msg.Uid > 0 {
			bodyData, err := s.FetchEmailBody(folderName, msg.Uid)
			if err != nil {
				logger.Warn("Failed to fetch email body",
					zap.Uint32("uid", msg.Uid),
					zap.String("subject", emailData.Subject),
					zap.Error(err))
			} else {
				emailData.Body = bodyData.Body
				emailData.HTMLBody = bodyData.HTMLBody
			}
		}

		// Set the folder name
		emailData.FolderName = folderName
		emails = append(emails, emailData)
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("failed to fetch messages: %w", err)
	}

	return emails, nil
}

// GetFolders returns a list of folders with detailed information using the existing connection
func (s *IMAPSession) GetFolders() ([]*FolderInfo, error) {
	if s.conn == nil {
		return nil, fmt.Errorf("session connection is nil")
	}

	// List mailboxes
	mailboxes := make(chan *imap.MailboxInfo, 10)
	done := make(chan error, 1)
	go func() {
		done <- s.conn.List("", "*", mailboxes)
	}()

	var folders []*FolderInfo
	for mbox := range mailboxes {
		// Decode UTF-7 encoded mailbox name
		decodedName, err := utils.DecodeUTF7(mbox.Name)
		if err != nil {
			logger.Warn("Failed to decode mailbox name",
				zap.String("raw_name", mbox.Name),
				zap.Error(err))
			decodedName = mbox.Name // Fallback to raw name
		}

		folder := &FolderInfo{
			Name:       decodedName,
			RawName:    mbox.Name,
			Delimiter:  mbox.Delimiter,
			Flags:      mbox.Attributes,
			Attributes: mbox.Attributes,
		}
		folders = append(folders, folder)
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("failed to list mailboxes: %w", err)
	}

	return folders, nil
}

// GetFolderStatus returns the status of a specific folder including UIDVALIDITY
func (s *IMAPSession) GetFolderStatus(folderName string) (*imap.MailboxStatus, error) {
	if s.conn == nil {
		return nil, fmt.Errorf("session connection is nil")
	}

	// Select the folder to get its status
	mbox, err := s.conn.Select(folderName, false)
	if err != nil {
		return nil, fmt.Errorf("failed to select folder %s: %w", folderName, err)
	}

	// Log the UIDVALIDITY for debugging
	logger.Debug("Folder status after SELECT",
		zap.String("folder", folderName),
		zap.Uint32("uid_validity", mbox.UidValidity),
		zap.Uint32("uid_next", mbox.UidNext),
		zap.Uint32("messages", mbox.Messages))

	// If UIDVALIDITY is missing, try to get it via STATUS command
	if mbox.UidValidity == 0 {
		logger.Warn("UIDVALIDITY not provided in SELECT response, trying STATUS command",
			zap.String("folder", folderName))

		items := []imap.StatusItem{imap.StatusUidValidity, imap.StatusUidNext, imap.StatusMessages}
		status, err := s.conn.Status(folderName, items)
		if err == nil && status != nil {
			if status.UidValidity > 0 {
				mbox.UidValidity = status.UidValidity
				logger.Debug("Got UIDVALIDITY from STATUS command",
					zap.String("folder", folderName),
					zap.Uint32("uid_validity", status.UidValidity))
			}
			if status.UidNext > 0 {
				mbox.UidNext = status.UidNext
			}
		} else if err != nil {
			logger.Error("Failed to get folder status via STATUS command",
				zap.String("folder", folderName),
				zap.Error(err))
		}
	}

	// Some IMAP servers don't provide UidNext in SELECT response
	// Try to get it via STATUS command if not available
	if mbox.UidNext == 0 && mbox.Messages > 0 {
		items := []imap.StatusItem{imap.StatusUidNext}
		status, err := s.conn.Status(folderName, items)
		if err == nil && status != nil {
			mbox.UidNext = status.UidNext
		}
	}

	// Final validation
	if mbox.UidValidity == 0 {
		logger.Error("Failed to get valid UIDVALIDITY for folder",
			zap.String("folder", folderName),
			zap.Uint32("messages", mbox.Messages))
	}

	return mbox, nil
}

// GetFolderStatusWithContext returns the status of a specific folder including UIDVALIDITY with context support
func (s *IMAPSession) GetFolderStatusWithContext(ctx context.Context, folderName string) (*imap.MailboxStatus, error) {
	if s.conn == nil {
		return nil, fmt.Errorf("session connection is nil")
	}

	// Create a channel to receive the result
	type result struct {
		mbox *imap.MailboxStatus
		err  error
	}
	resultChan := make(chan result, 1)

	go func() {
		mbox, err := s.GetFolderStatus(folderName)
		resultChan <- result{mbox, err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-resultChan:
		return res.mbox, res.err
	}
}

// FetchEmailsByUIDRange fetches emails within a UID range
func (s *IMAPSession) FetchEmailsByUIDRange(folderName string, uidFrom, uidTo uint32) ([]*protocol.EmailData, error) {
	if s.conn == nil {
		return nil, fmt.Errorf("session connection is nil")
	}

	// Select the specified folder
	mbox, err := s.conn.Select(folderName, false)
	if err != nil {
		return nil, fmt.Errorf("failed to select folder %s: %w", folderName, err)
	}

	// If the folder is empty, return empty result
	if mbox.Messages == 0 {
		return []*protocol.EmailData{}, nil
	}

	// Create UID set
	seqset := new(imap.SeqSet)
	if uidTo == 0 {
		// Fetch from uidFrom to the end
		seqset.AddRange(uidFrom, 0)
	} else {
		seqset.AddRange(uidFrom, uidTo)
	}

	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)

	go func() {
		// Use UidFetch instead of Fetch for UID-based fetching
		// Remove BODY.PEEK[] from initial fetch to avoid body structure parsing errors
		done <- s.conn.UidFetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid}, messages)
	}()

	var emails []*protocol.EmailData
	for msg := range messages {
		// Skip messages with UIDs less than what we requested
		if msg.Uid < uidFrom {
			logger.Warn("Server returned UID less than requested in FetchEmailsByUIDRange",
				zap.Uint32("requested_from", uidFrom),
				zap.Uint32("returned_uid", msg.Uid))
			continue
		}

		// If uidTo is specified, skip messages beyond it
		if uidTo > 0 && msg.Uid > uidTo {
			logger.Warn("Server returned UID greater than requested range",
				zap.Uint32("requested_to", uidTo),
				zap.Uint32("returned_uid", msg.Uid))
			continue
		}

		emailData, err := s.client.parseMessage(msg)
		if err != nil {
			logger.Warn("Failed to parse message", zap.Error(err))
			continue
		}

		// Fetch body separately if needed
		if emailData.Body == "" && emailData.HTMLBody == "" && msg.Uid > 0 {
			bodyData, err := s.FetchEmailBody(folderName, msg.Uid)
			if err != nil {
				logger.Warn("Failed to fetch email body",
					zap.Uint32("uid", msg.Uid),
					zap.String("subject", emailData.Subject),
					zap.Error(err))
			} else {
				emailData.Body = bodyData.Body
				emailData.HTMLBody = bodyData.HTMLBody
			}
		}

		emailData.FolderName = folderName
		emails = append(emails, emailData)
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("failed to fetch messages by UID: %w", err)
	}

	return emails, nil
}

// FetchEmailsByUIDRangeWithContext fetches emails within a UID range with context support
func (s *IMAPSession) FetchEmailsByUIDRangeWithContext(ctx context.Context, folderName string, uidFrom, uidTo uint32) ([]*protocol.EmailData, error) {
	if s.conn == nil {
		return nil, fmt.Errorf("session connection is nil")
	}

	// Select the specified folder
	mbox, err := s.conn.Select(folderName, false)
	if err != nil {
		return nil, fmt.Errorf("failed to select folder %s: %w", folderName, err)
	}

	// If the folder is empty, return empty result
	if mbox.Messages == 0 {
		return []*protocol.EmailData{}, nil
	}

	logger.Debug("Fetching emails by UID range with context",
		zap.String("folder", folderName),
		zap.Uint32("uid_from", uidFrom),
		zap.Uint32("uid_to", uidTo))

	var allEmails []*protocol.EmailData

	// Process in batches
	currentUID := uidFrom
	for currentUID <= uidTo || (uidTo == 0 && currentUID > 0) {
		// Check context
		select {
		case <-ctx.Done():
			logger.Warn("Context cancelled during UID range fetch",
				zap.String("folder", folderName),
				zap.Uint32("current_uid", currentUID),
				zap.Int("fetched", len(allEmails)))
			return allEmails, ctx.Err()
		default:
		}

		// Calculate batch end
		batchEndUID := currentUID + uint32(BatchSize) - 1
		if uidTo > 0 && batchEndUID > uidTo {
			batchEndUID = uidTo
		}

		// Create UID set for batch
		seqset := new(imap.SeqSet)
		if uidTo == 0 {
			seqset.AddRange(currentUID, 0) // Fetch from currentUID to the end
		} else {
			seqset.AddRange(currentUID, batchEndUID)
		}

		messages := make(chan *imap.Message, 10)
		done := make(chan error, 1)

		go func() {
			done <- s.conn.UidFetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid, imap.FetchRFC822Size}, messages)
		}()

		batchEmails := make([]*protocol.EmailData, 0)
		lastUID := currentUID - 1     // Initialize to one less than current to detect no progress
		actualHighestUID := uint32(0) // Track actual highest UID returned

		for msg := range messages {
			// Skip messages with UIDs less than what we requested
			if msg.Uid < currentUID {
				logger.Warn("Server returned UID less than requested",
					zap.Uint32("requested_from", currentUID),
					zap.Uint32("returned_uid", msg.Uid))
				continue
			}

			emailData, err := s.client.parseMessage(msg)
			if err != nil {
				logger.Warn("Failed to parse message", zap.Error(err))
				continue
			}

			// Fetch body if needed
			if emailData.Body == "" && emailData.HTMLBody == "" && msg.Uid > 0 {
				bodyData, err := s.FetchEmailBody(folderName, msg.Uid)
				if err != nil {
					logger.Warn("Failed to fetch email body",
						zap.Uint32("uid", msg.Uid),
						zap.String("subject", emailData.Subject),
						zap.Error(err))
				} else {
					emailData.Body = bodyData.Body
					emailData.HTMLBody = bodyData.HTMLBody
				}
			}
			emailData.FolderName = folderName
			batchEmails = append(batchEmails, emailData)

			// Track the highest UID we've seen
			if msg.Uid > actualHighestUID {
				actualHighestUID = msg.Uid
			}
		}

		// Update lastUID only if we got valid messages
		if actualHighestUID > 0 {
			lastUID = actualHighestUID
		}

		if err := <-done; err != nil {
			logger.Error("Failed to fetch UID range batch",
				zap.Uint32("from", currentUID),
				zap.Uint32("to", batchEndUID),
				zap.Error(err))
			if len(allEmails) > 0 {
				// Return partial results
				return allEmails, nil
			}
			return nil, fmt.Errorf("failed to fetch messages by UID: %w", err)
		}

		allEmails = append(allEmails, batchEmails...)

		logger.Debug("UID range batch completed",
			zap.String("folder", folderName),
			zap.Int("batch_size", len(batchEmails)),
			zap.Int("total_fetched", len(allEmails)),
			zap.Uint32("last_uid", lastUID))

		// If we got no messages or uidTo is specified and we've reached it, we're done
		if len(batchEmails) == 0 || (uidTo > 0 && lastUID >= uidTo) {
			break
		}

		// Check if we're making progress
		if lastUID <= currentUID {
			logger.Warn("No progress in UID range fetch, stopping to prevent infinite loop",
				zap.Uint32("current_uid", currentUID),
				zap.Uint32("last_uid", lastUID),
				zap.String("folder", folderName))
			break
		}

		// Move to next batch
		currentUID = lastUID + 1
	}

	return allEmails, nil
}

// FetchEmailBody fetches only the body of a specific email by UID
func (s *IMAPSession) FetchEmailBody(folderName string, uid uint32) (*protocol.EmailData, error) {
	if s.conn == nil {
		return nil, fmt.Errorf("session connection is nil")
	}

	// Create UID set for single email
	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)

	messages := make(chan *imap.Message, 1)
	done := make(chan error, 1)

	go func() {
		done <- s.conn.UidFetch(seqset, []imap.FetchItem{"BODY.PEEK[]"}, messages)
	}()

	emailData := &protocol.EmailData{}

	for msg := range messages {
		// Parse body sections
		for _, section := range msg.Body {
			if section == nil {
				continue
			}

			// Create a mail reader
			mr, err := mail.CreateReader(section)
			if err != nil {
				logger.Debug("Failed to create mail reader for body",
					zap.Uint32("uid", uid),
					zap.Error(err))
				continue
			}

			// Read each part
			for {
				p, err := mr.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					logger.Debug("Failed to read part", zap.Error(err))
					break
				}

				switch h := p.Header.(type) {
				case *mail.InlineHeader:
					ct, _, _ := h.ContentType()
					b, err := io.ReadAll(p.Body)
					if err != nil {
						logger.Debug("Failed to read body content", zap.Error(err))
						continue
					}

					switch ct {
					case "text/plain":
						emailData.Body = string(b)
					case "text/html":
						emailData.HTMLBody = string(b)
					}
				}
			}
		}
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("failed to fetch body: %w", err)
	}

	return emailData, nil
}

// debugWriter is a writer that logs IMAP protocol debug output
type debugWriter struct{}

func (debugWriter) Write(p []byte) (n int, err error) {
	logger.Debug("IMAP protocol", zap.String("data", string(p)))
	return len(p), nil
}

// NewIMAPClient creates a new IMAP client
func NewIMAPClient(host string, port int, username, password string, useSSL bool) *IMAPClient {
	return &IMAPClient{
		host:     host,
		port:     port,
		username: username,
		password: password,
		useSSL:   useSSL,
	}
}

// TestConnection tests the IMAP connection
func (c *IMAPClient) TestConnection() error {
	logger.Debug("Testing IMAP connection",
		zap.String("host", c.host),
		zap.Int("port", c.port),
		zap.String("username", c.username),
		zap.Bool("ssl", c.useSSL),
	)

	conn, err := c.connect()
	if err != nil {
		logger.Error("IMAP connection test failed", zap.Error(err))
		return err
	}
	defer conn.Logout()

	logger.Info("IMAP connection test successful")
	return nil
}

// FetchEmails fetches emails from the IMAP server with default limit
func (c *IMAPClient) FetchEmails() ([]*protocol.EmailData, error) {
	// Default to fetching last 100 messages from INBOX
	return c.FetchEmailsFromFolder("INBOX", 100)
}

// FetchEmailsWithLimit fetches emails from INBOX with specified limit
func (c *IMAPClient) FetchEmailsWithLimit(limit int) ([]*protocol.EmailData, error) {
	return c.FetchEmailsFromFolder("INBOX", limit)
}

// FetchEmailsFromFolder fetches emails from a specific folder with specified limit
// If limit is 0, it fetches all emails. If limit is > 0, it fetches the most recent emails up to the limit.
// Large fetches are done in batches to prevent timeouts and memory issues.
func (c *IMAPClient) FetchEmailsFromFolder(folderName string, limit int) ([]*protocol.EmailData, error) {
	conn, err := c.connect()
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Logout()

	// Select the specified folder
	mbox, err := conn.Select(folderName, false)
	if err != nil {
		return nil, fmt.Errorf("failed to select INBOX: %w", err)
	}

	// Calculate range based on limit
	from := uint32(1)
	to := mbox.Messages

	// If limit is 0, fetch all emails
	// If limit > 0, fetch only the most recent emails
	if limit > 0 && mbox.Messages > uint32(limit) {
		from = to - uint32(limit) + 1
	}

	totalToFetch := to - from + 1
	logger.Debug("Preparing to fetch emails",
		zap.String("folder", folderName),
		zap.Uint32("from", from),
		zap.Uint32("to", to),
		zap.Uint32("total_in_mailbox", mbox.Messages),
		zap.Uint32("total_to_fetch", totalToFetch),
		zap.Int("limit", limit),
	)

	var allEmails []*protocol.EmailData

	// Fetch emails in batches to prevent timeouts and memory issues
	for currentFrom := from; currentFrom <= to; {
		// Calculate batch range
		batchTo := currentFrom + BatchSize - 1
		if batchTo > to {
			batchTo = to
		}

		logger.Debug("Fetching batch of emails",
			zap.Uint32("batch_from", currentFrom),
			zap.Uint32("batch_to", batchTo),
			zap.Uint32("batch_size", batchTo-currentFrom+1),
		)

		// Fetch current batch
		seqset := new(imap.SeqSet)
		seqset.AddRange(currentFrom, batchTo)

		messages := make(chan *imap.Message, 10)
		done := make(chan error, 1)

		go func() {
			done <- conn.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid, imap.FetchRFC822Size}, messages)
		}()

		// Process messages in this batch
		batchEmails := make([]*protocol.EmailData, 0, batchTo-currentFrom+1)
		for msg := range messages {
			emailData, err := c.parseMessage(msg)
			if err != nil {
				logger.Warn("Failed to parse message", zap.Error(err))
				continue
			}
			// Set the folder name
			emailData.FolderName = folderName
			batchEmails = append(batchEmails, emailData)
		}

		if err := <-done; err != nil {
			logger.Error("Failed to fetch batch",
				zap.Uint32("batch_from", currentFrom),
				zap.Uint32("batch_to", batchTo),
				zap.Error(err),
			)
			// Continue with partial results rather than failing completely
			if len(allEmails) > 0 {
				logger.Warn("Returning partial results due to batch fetch error",
					zap.Int("fetched_count", len(allEmails)))
				break
			}
			return nil, fmt.Errorf("failed to fetch messages: %w", err)
		}

		allEmails = append(allEmails, batchEmails...)

		logger.Debug("Batch fetch completed",
			zap.Int("batch_size", len(batchEmails)),
			zap.Int("total_fetched", len(allEmails)),
		)

		// Move to next batch
		currentFrom = batchTo + 1
	}

	logger.Debug("Email fetch completed",
		zap.Int("total_fetched", len(allEmails)),
		zap.Int("requested_limit", limit),
	)

	return allEmails, nil
}

// FetchEmailsSince fetches emails from INBOX since a specific date
func (c *IMAPClient) FetchEmailsSince(since time.Time) ([]*protocol.EmailData, error) {
	return c.FetchEmailsFromFolderSince("INBOX", since)
}

// FetchEmailsFromFolderSince fetches emails from a specific folder since a specific date
func (c *IMAPClient) FetchEmailsFromFolderSince(folderName string, since time.Time) ([]*protocol.EmailData, error) {
	conn, err := c.connect()
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Logout()

	// Select the specified folder
	_, err = conn.Select(folderName, false)
	if err != nil {
		return nil, fmt.Errorf("failed to select folder %s: %w", folderName, err)
	}

	// Search for messages since date
	criteria := imap.NewSearchCriteria()
	criteria.Since = since

	ids, err := conn.Search(criteria)
	if err != nil {
		return nil, fmt.Errorf("failed to search messages: %w", err)
	}

	if len(ids) == 0 {
		return []*protocol.EmailData{}, nil
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(ids...)

	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)

	go func() {
		done <- conn.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchBodyStructure, imap.FetchUid, imap.FetchRFC822Size, "BODY.PEEK[]"}, messages)
	}()

	var emails []*protocol.EmailData
	for msg := range messages {
		emailData, err := c.parseMessage(msg)
		if err != nil {
			logger.Warn("Failed to parse message", zap.Error(err))
			continue
		}
		// Set the folder name
		emailData.FolderName = folderName
		emails = append(emails, emailData)
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("failed to fetch messages: %w", err)
	}

	return emails, nil
}

// FolderInfo represents detailed folder information
type FolderInfo struct {
	Name       string   // Decoded UTF-8 name
	RawName    string   // Original UTF-7 name
	Delimiter  string   // Hierarchy delimiter
	Flags      []string // Folder flags
	Attributes []string // Folder attributes
}

// GetFolders returns a list of folders with detailed information
func (c *IMAPClient) GetFolders() ([]*FolderInfo, error) {
	conn, err := c.connect()
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Logout()

	// List mailboxes
	mailboxes := make(chan *imap.MailboxInfo, 10)
	done := make(chan error, 1)
	go func() {
		done <- conn.List("", "*", mailboxes)
	}()

	var folders []*FolderInfo
	for m := range mailboxes {
		// Decode UTF-7 folder name
		decodedName, err := utils.DecodeUTF7(m.Name)
		if err != nil {
			logger.Warn("Failed to decode folder name",
				zap.String("raw_name", m.Name),
				zap.Error(err))
			decodedName = m.Name // Fallback to raw name
		}

		folderInfo := &FolderInfo{
			Name:       decodedName,
			RawName:    m.Name,
			Delimiter:  m.Delimiter,
			Attributes: m.Attributes,
		}

		// Extract flags from attributes
		for _, attr := range m.Attributes {
			folderInfo.Flags = append(folderInfo.Flags, string(attr))
		}

		folders = append(folders, folderInfo)
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("failed to list folders: %w", err)
	}

	return folders, nil
}

// MarkAsRead marks an email as read
func (c *IMAPClient) MarkAsRead(uid uint32) error {
	conn, err := c.connect()
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Logout()

	// Select INBOX
	_, err = conn.Select("INBOX", false)
	if err != nil {
		return fmt.Errorf("failed to select INBOX: %w", err)
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uid)

	// Add the \Seen flag
	item := imap.FormatFlagsOp(imap.AddFlags, true)
	flags := []interface{}{imap.SeenFlag}
	if err := conn.Store(seqSet, item, flags, nil); err != nil {
		return fmt.Errorf("failed to mark as read: %w", err)
	}

	return nil
}

// DeleteEmail deletes an email
func (c *IMAPClient) DeleteEmail(uid uint32) error {
	conn, err := c.connect()
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Logout()

	// Select INBOX
	_, err = conn.Select("INBOX", false)
	if err != nil {
		return fmt.Errorf("failed to select INBOX: %w", err)
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uid)

	// Add the \Deleted flag
	item := imap.FormatFlagsOp(imap.AddFlags, true)
	flags := []interface{}{imap.DeletedFlag}
	if err := conn.Store(seqSet, item, flags, nil); err != nil {
		return fmt.Errorf("failed to mark as deleted: %w", err)
	}

	// Expunge to permanently delete
	if err := conn.Expunge(nil); err != nil {
		return fmt.Errorf("failed to expunge: %w", err)
	}

	return nil
}

// Connect establishes connection to IMAP server and returns the client
// This method is exported to allow external packages to get a persistent connection
func (c *IMAPClient) Connect() (*client.Client, error) {
	return c.connect()
}

// GetCapabilities returns the server's capabilities
func (c *IMAPClient) GetCapabilities() ([]string, error) {
	conn, err := c.connect()
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Logout()

	// Get capabilities using the client's Capability method
	capsMap, err := conn.Capability()
	if err != nil {
		return nil, fmt.Errorf("failed to get capabilities: %w", err)
	}

	// Convert map to slice
	capabilities := make([]string, 0, len(capsMap))
	for cap := range capsMap {
		capabilities = append(capabilities, cap)
	}

	// If no capabilities found, use default
	if len(capabilities) == 0 {
		capabilities = []string{"IMAP4rev1"}
	}

	logger.Info("IMAP server capabilities",
		zap.String("server", c.host),
		zap.Strings("capabilities", capabilities))

	return capabilities, nil
}

// CheckIDLESupport checks if the server supports IDLE extension
func (c *IMAPClient) CheckIDLESupport() (bool, error) {
	// Use GetCapabilities to check for IDLE support
	capabilities, err := c.GetCapabilities()
	if err != nil {
		return false, err
	}

	// Check if IDLE is in the capabilities list
	supportsIDLE := false
	for _, cap := range capabilities {
		if strings.ToUpper(cap) == "IDLE" {
			supportsIDLE = true
			break
		}
	}

	logger.Info("IDLE support check",
		zap.String("server", c.host),
		zap.Bool("supports_idle", supportsIDLE))

	return supportsIDLE, nil
}

// connect establishes connection to IMAP server
func (c *IMAPClient) connect() (*client.Client, error) {
	addr := fmt.Sprintf("%s:%d", c.host, c.port)

	logger.Debug("Connecting to IMAP server",
		zap.String("address", addr),
		zap.Bool("ssl", c.useSSL),
	)

	var conn *client.Client
	var err error

	if c.useSSL {
		// Connect with TLS
		tlsConfig := &tls.Config{
			ServerName: c.host,
		}
		logger.Debug("Using TLS connection")
		conn, err = client.DialTLS(addr, tlsConfig)
	} else {
		// Connect without TLS
		logger.Debug("Using plain connection")
		conn, err = client.Dial(addr)
	}

	if err != nil {
		logger.Error("Failed to dial IMAP server",
			zap.String("address", addr),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to dial: %w", err)
	}

	logger.Debug("Connected to IMAP server, attempting login",
		zap.String("username", c.username),
	)

	// Enable debug logging if in debug mode
	if logger.Logger != nil && logger.Logger.Core().Enabled(zap.DebugLevel) {
		conn.SetDebug(debugWriter{})
	}

	// Login
	if err := conn.Login(c.username, c.password); err != nil {
		logger.Error("IMAP login failed",
			zap.String("username", c.username),
			zap.Error(err),
		)
		conn.Logout()
		return nil, fmt.Errorf("failed to login: %w", err)
	}

	logger.Info("Successfully logged in to IMAP server",
		zap.String("username", c.username),
	)

	// For 163 mail servers (163.com, 126.com, yeah.net), send ID command
	if c.is163Server() {
		if err := c.sendImapID(conn); err != nil {
			logger.Warn("Failed to send IMAP ID command", zap.Error(err))
			// Don't fail the connection, just log the warning
		}
	}

	return conn, nil
}

// is163Server checks if the server is a 163 mail server
func (c *IMAPClient) is163Server() bool {
	host := strings.ToLower(c.host)
	return strings.Contains(host, "163.com") ||
		strings.Contains(host, "126.com") ||
		strings.Contains(host, "yeah.net") ||
		strings.Contains(host, "yeah.com")
}

// idCommand implements the ID extension command
type idCommand struct{}

// Command returns the IMAP command
func (idCommand) Command() *imap.Command {
	// ID ("name" "FlyMail" "version" "1.0.0" "vendor" "FlyMail")
	return &imap.Command{
		Name: "ID",
		Arguments: []interface{}{
			[]interface{}{
				"name", "FlyMail",
				"version", "1.0.0",
				"vendor", "FlyMail",
			},
		},
	}
}

// sendImapID sends IMAP ID extension command for 163 mail servers
func (c *IMAPClient) sendImapID(conn *client.Client) error {
	logger.Debug("Sending IMAP ID command for 163 server")

	// For 163 mail servers, we need to send an ID command
	// This is a workaround for 163's requirement

	// Execute the ID command
	status, err := conn.Execute(idCommand{}, nil)
	if err != nil {
		return fmt.Errorf("failed to execute IMAP ID command: %w", err)
	}

	if status.Type != imap.StatusRespOk {
		return fmt.Errorf("IMAP ID command failed: %s", status.Info)
	}

	logger.Debug("Successfully sent IMAP ID command for 163 server")
	return nil
}

// parseMessage parses an IMAP message to protocol.EmailData
func (c *IMAPClient) parseMessage(msg *imap.Message) (*protocol.EmailData, error) {
	email := &protocol.EmailData{
		Headers: make(map[string]string),
	}

	// Extract UID
	email.UID = msg.Uid

	// Extract size
	email.Size = int64(msg.Size)

	// Parse envelope
	if msg.Envelope != nil {
		email.Subject = msg.Envelope.Subject
		email.Date = msg.Envelope.Date

		// Message-ID
		email.EmailID = strings.Trim(msg.Envelope.MessageId, "<>")

		// From
		if len(msg.Envelope.From) > 0 {
			email.From = formatAddress(msg.Envelope.From[0])
		}

		// To
		for _, addr := range msg.Envelope.To {
			email.To = append(email.To, formatAddress(addr))
		}

		// Cc
		for _, addr := range msg.Envelope.Cc {
			email.Cc = append(email.Cc, formatAddress(addr))
		}

		// Bcc
		for _, addr := range msg.Envelope.Bcc {
			email.Bcc = append(email.Bcc, formatAddress(addr))
		}
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

	// Debug log flags
	if len(msg.Flags) > 0 {
		logger.Debug("Email flags",
			zap.String("subject", email.Subject),
			zap.Strings("flags", msg.Flags),
			zap.Bool("is_read", email.IsRead),
			zap.Bool("is_starred", email.IsStarred),
		)
	}

	// Parse body
	for _, section := range msg.Body {
		if section == nil {
			continue
		}

		r := section
		if r == nil {
			continue
		}

		// Create a mail reader
		mr, err := mail.CreateReader(r)
		if err != nil {
			logger.Warn("Failed to create mail reader",
				zap.Error(err),
				zap.String("subject", email.Subject),
				zap.String("from", email.From),
				zap.Time("date", email.Date))
			continue
		}

		// Read each part
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				logger.Warn("Failed to read part", zap.Error(err))
				break
			}

			switch h := p.Header.(type) {
			case *mail.InlineHeader:
				// This is the message's text (plain-text or HTML)
				ct, _, _ := h.ContentType()
				b, err := io.ReadAll(p.Body)
				if err != nil {
					logger.Warn("Failed to read body", zap.Error(err))
					continue
				}

				switch ct {
				case "text/plain":
					email.Body = string(b)
				case "text/html":
					email.HTMLBody = string(b)
				}
			case *mail.AttachmentHeader:
				// This is an attachment (TODO: handle attachments)
				filename, _ := h.Filename()
				logger.Debug("Found attachment", zap.String("filename", filename))
			}
		}
	}

	return email, nil
}

// formatAddress formats an IMAP address to string
func formatAddress(addr *imap.Address) string {
	if addr.PersonalName != "" {
		return fmt.Sprintf("%s <%s@%s>", addr.PersonalName, addr.MailboxName, addr.HostName)
	}
	return fmt.Sprintf("%s@%s", addr.MailboxName, addr.HostName)
}
