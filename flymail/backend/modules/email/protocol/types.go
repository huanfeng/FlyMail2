package protocol

import "time"

// EmailData represents parsed email data
type EmailData struct {
	EmailID    string
	Subject    string
	From       string
	To         []string
	Cc         []string
	Bcc        []string
	Date       time.Time
	Body       string
	HTMLBody   string
	Headers    map[string]string
	Size       int64
	IsRead     bool
	IsStarred  bool
	FolderName string
	UID        uint32 // IMAP UID
}
