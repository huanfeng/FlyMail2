package imap

import (
	"fmt"

	imapv2 "github.com/emersion/go-imap/v2"
)

// MarkRead adds the \Seen flag to the given UIDs.
func (s *Session) MarkRead(uids ...imapv2.UID) error {
	return s.storeFlags(uids, imapv2.StoreFlagsAdd, imapv2.FlagSeen)
}

// MarkUnread removes the \Seen flag from the given UIDs.
func (s *Session) MarkUnread(uids ...imapv2.UID) error {
	return s.storeFlags(uids, imapv2.StoreFlagsDel, imapv2.FlagSeen)
}

// MarkStarred adds the \Flagged flag to the given UIDs.
func (s *Session) MarkStarred(uids ...imapv2.UID) error {
	return s.storeFlags(uids, imapv2.StoreFlagsAdd, imapv2.FlagFlagged)
}

// MarkUnstarred removes the \Flagged flag from the given UIDs.
func (s *Session) MarkUnstarred(uids ...imapv2.UID) error {
	return s.storeFlags(uids, imapv2.StoreFlagsDel, imapv2.FlagFlagged)
}

// Delete marks messages as \Deleted and expunges them.
func (s *Session) Delete(uids ...imapv2.UID) error {
	if err := s.storeFlags(uids, imapv2.StoreFlagsAdd, imapv2.FlagDeleted); err != nil {
		return err
	}
	return s.Client.Expunge().Close()
}

func (s *Session) storeFlags(uids []imapv2.UID, op imapv2.StoreFlagsOp, flags ...imapv2.Flag) error {
	if s.Client == nil {
		return fmt.Errorf("not connected")
	}
	if len(uids) == 0 {
		return nil
	}

	var uidSet imapv2.UIDSet
	uidSet.AddNum(uids...)

	storeCmd := s.Client.Store(uidSet, &imapv2.StoreFlags{
		Op:    op,
		Flags: flags,
	}, nil)

	return storeCmd.Close()
}
