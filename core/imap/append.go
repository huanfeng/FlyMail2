package imap

import (
	"fmt"

	imapv2 "github.com/emersion/go-imap/v2"
)

// Append uploads a message into the given mailbox with the given flags
// (e.g. \Seen for sent mail).
func (s *Session) Append(mailbox string, flags []imapv2.Flag, msg []byte) error {
	if s.Client == nil {
		return fmt.Errorf("not connected")
	}
	opts := &imapv2.AppendOptions{Flags: flags}
	cmd := s.Client.Append(mailbox, int64(len(msg)), opts)
	if _, err := cmd.Write(msg); err != nil {
		return fmt.Errorf("append write failed: %w", err)
	}
	if err := cmd.Close(); err != nil {
		return fmt.Errorf("append close failed: %w", err)
	}
	if _, err := cmd.Wait(); err != nil {
		return fmt.Errorf("append failed: %w", err)
	}
	return nil
}
