package protocol

import (
	"io"
	"net/mail"
	"strings"
	"time"

	"flymail/shared/store/model"
)

type ParsedEmail struct {
	EmailID  string
	Subject  string
	From     string
	To       string
	CC       string
	BCC      string
	Date     time.Time
	Body     string
	BodyHTML string
	Size     int64
}

func ParseEmail(rawEmail string) (*ParsedEmail, error) {
	msg, err := mail.ReadMessage(strings.NewReader(rawEmail))
	if err != nil {
		return nil, err
	}

	parsed := &ParsedEmail{
		EmailID: msg.Header.Get("Message-ID"),
		Subject: msg.Header.Get("Subject"),
		From:    msg.Header.Get("From"),
		To:      msg.Header.Get("To"),
		CC:      msg.Header.Get("CC"),
		BCC:     msg.Header.Get("BCC"),
		Size:    int64(len(rawEmail)),
	}

	// Parse date
	if dateStr := msg.Header.Get("Date"); dateStr != "" {
		if date, err := mail.ParseDate(dateStr); err == nil {
			parsed.Date = date
		} else {
			parsed.Date = time.Now()
		}
	} else {
		parsed.Date = time.Now()
	}

	// Read body
	body, err := io.ReadAll(msg.Body)
	if err != nil {
		return nil, err
	}

	contentType := msg.Header.Get("Content-Type")
	if strings.Contains(strings.ToLower(contentType), "html") {
		parsed.BodyHTML = string(body)
	} else {
		parsed.Body = string(body)
	}

	return parsed, nil
}

func (p *ParsedEmail) ToEmail(accountID uint) *model.Email {
	return &model.Email{
		AccountID: accountID,
		MessageID: p.EmailID,
		Subject:   p.Subject,
		From:      p.From,
		To:        p.To,
		CC:        p.CC,
		BCC:       p.BCC,
		Body:      p.Body,
		BodyHTML:  p.BodyHTML,
		Date:      p.Date,
		Size:      p.Size,
		IsRead:    false,
		IsStarred: false,
	}
}
