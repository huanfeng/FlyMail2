package imap

import (
	"fmt"

	imapv2 "github.com/emersion/go-imap/v2"

	"flymail-core/types"
)

// SelectedFolder holds state after selecting a mailbox.
type SelectedFolder struct {
	Path        string
	NumMessages uint32
	UIDValidity uint32
	UIDNext     uint32
}

// ListFolders returns all folders from the server.
func (s *Session) ListFolders() ([]types.FolderInfo, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("not connected")
	}

	listCmd := s.Client.List("", "*", nil)
	var folders []types.FolderInfo

	for {
		mbox := listCmd.Next()
		if mbox == nil {
			break
		}

		attrs := make([]string, 0, len(mbox.Attrs))
		for _, a := range mbox.Attrs {
			attrs = append(attrs, string(a))
		}

		// Decode UTF-7 folder name; fall back to raw name on error
		name := mbox.Mailbox
		if decoded, err := DecodeUTF7(mbox.Mailbox); err == nil {
			name = decoded
		}

		folders = append(folders, types.FolderInfo{
			Name:       name,
			Path:       mbox.Mailbox,
			Delimiter:  string(mbox.Delim),
			Attributes: attrs,
		})
	}

	if err := listCmd.Close(); err != nil {
		return nil, fmt.Errorf("list folders failed: %w", err)
	}

	return folders, nil
}

// SelectFolder selects a mailbox and returns its status.
func (s *Session) SelectFolder(path string) (*SelectedFolder, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("not connected")
	}

	data, err := s.Client.Select(path, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("select %s failed: %w", path, err)
	}

	sf := &SelectedFolder{
		Path:        path,
		NumMessages: data.NumMessages,
	}
	if data.UIDValidity != 0 {
		sf.UIDValidity = data.UIDValidity
	}
	if data.UIDNext != 0 {
		sf.UIDNext = uint32(data.UIDNext)
	}

	return sf, nil
}

// StatusItem selects which status items to query.
type StatusItem int

const (
	StatusNumMessages StatusItem = iota
	StatusUIDNext
	StatusUIDValidity
	StatusUnseen
)

// FolderStatusResult holds results from a STATUS command.
type FolderStatusResult struct {
	NumMessages *uint32
	UIDNext     *uint32
	UIDValidity *uint32
	Unseen      *uint32
}

// FolderStatus returns status info for a folder without selecting it.
func (s *Session) FolderStatus(path string, items ...StatusItem) (*FolderStatusResult, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("not connected")
	}

	opts := &imapv2.StatusOptions{}
	for _, item := range items {
		switch item {
		case StatusNumMessages:
			opts.NumMessages = true
		case StatusUIDNext:
			opts.UIDNext = true
		case StatusUIDValidity:
			opts.UIDValidity = true
		case StatusUnseen:
			opts.NumUnseen = true
		}
	}

	data, err := s.Client.Status(path, opts).Wait()
	if err != nil {
		return nil, fmt.Errorf("status %s failed: %w", path, err)
	}

	result := &FolderStatusResult{}
	if data.NumMessages != nil {
		v := *data.NumMessages
		result.NumMessages = &v
	}
	if data.UIDNext != 0 {
		v := uint32(data.UIDNext)
		result.UIDNext = &v
	}
	if data.UIDValidity != 0 {
		v := data.UIDValidity
		result.UIDValidity = &v
	}
	if data.NumUnseen != nil {
		v := *data.NumUnseen
		result.Unseen = &v
	}

	return result, nil
}
