package parser

import (
	"io"
	"strings"

	"github.com/emersion/go-message/mail"

	"flymail-core/types"
)

func init() {
	RegisterCharsets()
}

// ParseBody reads an RFC 5322 message body from r and populates
// text/html body fields and attachment metadata on the target ParsedEmail.
//
// Envelope fields (Subject, From, To, etc.) are NOT set here — the caller
// typically gets those from the IMAP ENVELOPE response which is more reliable.
// If fallbackHeaders is true, missing envelope fields will be filled from
// the message headers as a fallback.
func ParseBody(r io.Reader, email *types.ParsedEmail, fallbackHeaders bool) error {
	mr, err := mail.CreateReader(r)
	if err != nil {
		return err
	}

	// Optionally fill envelope fields from headers
	if fallbackHeaders {
		fillFromHeaders(mr, email)
	}

	// Walk MIME parts
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Skip malformed parts instead of aborting
			continue
		}

		switch h := p.Header.(type) {
		case *mail.InlineHeader:
			ct, _, _ := h.ContentType()
			b, readErr := io.ReadAll(p.Body)
			if readErr != nil {
				continue
			}

			switch {
			case strings.HasPrefix(ct, "text/plain") && email.TextBody == "":
				email.TextBody = string(b)
			case strings.HasPrefix(ct, "text/html") && email.HTMLBody == "":
				email.HTMLBody = string(b)
			}

		case *mail.AttachmentHeader:
			filename, _ := h.Filename()
			ct, _, _ := h.ContentType()
			contentID := h.Get("Content-Id")

			// Read through body to get size (we don't store content here)
			n, _ := io.Copy(io.Discard, p.Body)

			att := types.Attachment{
				Filename:    filename,
				ContentType: ct,
				Size:        n,
				ContentID:   strings.Trim(contentID, "<>"),
			}
			email.Attachments = append(email.Attachments, att)
		}
	}

	return nil
}

// fillFromHeaders populates ParsedEmail envelope fields from message headers
// only when those fields are still empty.
func fillFromHeaders(mr *mail.Reader, email *types.ParsedEmail) {
	if email.Subject == "" {
		if subj, err := mr.Header.Text("Subject"); err == nil && subj != "" {
			email.Subject = DecodeMIMEHeader(subj)
		}
	}

	if email.MessageID == "" {
		if mid, err := mr.Header.Text("Message-ID"); err == nil {
			email.MessageID = strings.Trim(mid, "<>")
		}
	}

	if len(email.From) == 0 {
		if addrs, err := mr.Header.AddressList("From"); err == nil {
			for _, a := range addrs {
				email.From = append(email.From, types.Address{
					Name:  DecodeMIMEHeader(a.Name),
					Email: a.Address,
				})
			}
		}
	}

	if len(email.To) == 0 {
		if addrs, err := mr.Header.AddressList("To"); err == nil {
			for _, a := range addrs {
				email.To = append(email.To, types.Address{
					Name:  DecodeMIMEHeader(a.Name),
					Email: a.Address,
				})
			}
		}
	}

	if len(email.CC) == 0 {
		if addrs, err := mr.Header.AddressList("Cc"); err == nil {
			for _, a := range addrs {
				email.CC = append(email.CC, types.Address{
					Name:  DecodeMIMEHeader(a.Name),
					Email: a.Address,
				})
			}
		}
	}

	if len(email.BCC) == 0 {
		if addrs, err := mr.Header.AddressList("Bcc"); err == nil {
			for _, a := range addrs {
				email.BCC = append(email.BCC, types.Address{
					Name:  DecodeMIMEHeader(a.Name),
					Email: a.Address,
				})
			}
		}
	}
}
