package imap

import (
	"bytes"
	"fmt"
	"strings"

	imapv2 "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"flymail-core/parser"
	"flymail-core/types"
)

// FetchOptions controls what to fetch.
type FetchOptions struct {
	// FetchBody requests the full RFC 5322 body for parsing text/html/attachments.
	// When false, only envelope metadata is returned (faster).
	FetchBody bool

	// FallbackHeaders fills envelope fields from headers if missing.
	FallbackHeaders bool
}

// FetchByUIDs fetches messages by specific UIDs from the currently selected folder.
func (s *Session) FetchByUIDs(uids []imapv2.UID, opts FetchOptions) ([]*types.ParsedEmail, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("not connected")
	}
	if len(uids) == 0 {
		return nil, nil
	}

	var uidSet imapv2.UIDSet
	uidSet.AddNum(uids...)

	return s.doFetch(uidSet, opts)
}

// FetchByUIDRange fetches messages in a UID range [from, to].
// If to is 0, fetches from `from` to the end (*).
func (s *Session) FetchByUIDRange(from, to imapv2.UID, opts FetchOptions) ([]*types.ParsedEmail, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("not connected")
	}

	var uidSet imapv2.UIDSet
	uidSet.AddRange(from, to)

	return s.doFetch(uidSet, opts)
}

// SearchUnseenSince searches for unseen messages with UID >= startUID.
func (s *Session) SearchUnseenSince(startUID imapv2.UID) ([]imapv2.UID, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("not connected")
	}

	uidSet := imapv2.UIDSet{}
	uidSet.AddRange(startUID, 0)

	criteria := &imapv2.SearchCriteria{
		UID:     []imapv2.UIDSet{uidSet},
		NotFlag: []imapv2.Flag{imapv2.FlagSeen},
	}

	res, err := s.Client.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	return res.AllUIDs(), nil
}

func (s *Session) doFetch(uidSet imapv2.UIDSet, opts FetchOptions) ([]*types.ParsedEmail, error) {
	var bodySection *imapv2.FetchItemBodySection
	if opts.FetchBody {
		bodySection = &imapv2.FetchItemBodySection{}
	}

	fetchOpts := &imapv2.FetchOptions{
		UID:          true,
		Envelope:     true,
		InternalDate: true,
		Flags:        true,
		RFC822Size:   true,
	}
	if bodySection != nil {
		fetchOpts.BodySection = []*imapv2.FetchItemBodySection{bodySection}
	}

	fetchCmd := s.Client.Fetch(uidSet, fetchOpts)

	var emails []*types.ParsedEmail
	for {
		msg := fetchCmd.Next()
		if msg == nil {
			break
		}

		buf, err := msg.Collect()
		if err != nil {
			continue
		}

		email := convertMessage(buf, bodySection, opts.FallbackHeaders)
		if email != nil {
			emails = append(emails, email)
		}
	}

	if err := fetchCmd.Close(); err != nil {
		return emails, fmt.Errorf("fetch close: %w", err)
	}
	return emails, nil
}

func convertMessage(buf *imapclient.FetchMessageBuffer, bodySection *imapv2.FetchItemBodySection, fallbackHeaders bool) *types.ParsedEmail {
	if buf == nil {
		return nil
	}

	email := &types.ParsedEmail{
		UID:    uint32(buf.UID),
		SeqNum: buf.SeqNum,
		Date:   buf.InternalDate,
		Size:   buf.RFC822Size,
	}

	// Flags
	for _, f := range buf.Flags {
		email.Flags = append(email.Flags, string(f))
		switch f {
		case imapv2.FlagSeen:
			email.IsRead = true
		case imapv2.FlagFlagged:
			email.IsStarred = true
		}
	}

	// Envelope
	if env := buf.Envelope; env != nil {
		email.Subject = parser.DecodeMIMEHeader(env.Subject)
		email.MessageID = strings.Trim(env.MessageID, "<>")
		email.From = ConvertIMAPAddresses(env.From)
		email.To = ConvertIMAPAddresses(env.To)
		email.CC = ConvertIMAPAddresses(env.Cc)
		email.BCC = ConvertIMAPAddresses(env.Bcc)
		email.ReplyTo = ConvertIMAPAddresses(env.ReplyTo)

		if email.Date.IsZero() && !env.Date.IsZero() {
			email.Date = env.Date
		}
	}

	// Body parsing
	if bodySection != nil {
		body := buf.FindBodySection(bodySection)
		if body != nil {
			parser.ParseBody(bytes.NewReader(body), email, fallbackHeaders)
		}
	}

	return email
}

// ConvertIMAPAddresses converts go-imap/v2 addresses to core types.Address.
func ConvertIMAPAddresses(addrs []imapv2.Address) []types.Address {
	if len(addrs) == 0 {
		return nil
	}
	result := make([]types.Address, 0, len(addrs))
	for _, a := range addrs {
		addr := fmt.Sprintf("%s@%s", a.Mailbox, a.Host)
		name := parser.DecodeMIMEHeader(strings.TrimSpace(a.Name))
		result = append(result, types.Address{Name: name, Email: addr})
	}
	return result
}
