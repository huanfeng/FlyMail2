package sync

import (
	"testing"
	"time"

	"flymail/internal/fts"
	"flymail/modules/email/folder"

	imapv2 "github.com/emersion/go-imap/v2"
)

func TestIMAPCriteriaMapping(t *testing.T) {
	q := fts.Parse(`发票 "hello world" from:zhang to:li subject:报销 is:read is:starred before:2026-06-01 after:2026-01-01 has:attachment`)
	c := imapCriteria(q)
	if len(c.Text) != 2 || c.Text[0] != "发票" || c.Text[1] != "hello world" {
		t.Errorf("Text = %v", c.Text)
	}
	if len(c.Header) != 3 || c.Header[0].Key != "From" || c.Header[1].Key != "To" || c.Header[2].Key != "Subject" {
		t.Errorf("Header = %+v", c.Header)
	}
	if len(c.Flag) != 2 || c.Flag[0] != imapv2.FlagSeen || c.Flag[1] != imapv2.FlagFlagged {
		t.Errorf("Flag = %v", c.Flag)
	}
	if !c.SentBefore.Equal(time.Date(2026, 6, 1, 0, 0, 0, 0, time.Local)) || !c.SentSince.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)) {
		t.Errorf("dates = %v / %v", c.SentBefore, c.SentSince)
	}

	// 反向标志
	c2 := imapCriteria(fts.Parse("is:unread is:unstarred"))
	if len(c2.NotFlag) != 2 || len(c2.Flag) != 0 {
		t.Errorf("NotFlag = %v Flag = %v", c2.NotFlag, c2.Flag)
	}
}

func TestRemoteSearchFolders(t *testing.T) {
	all := []folder.Folder{
		{ID: 1, Path: "INBOX", DisplayName: "收件箱", Type: "inbox", Selectable: true},
		{ID: 2, Path: "Sent", DisplayName: "已发送", Type: "sent", Selectable: true},
		{ID: 3, Path: "[Gmail]", DisplayName: "[Gmail]", Type: "custom", Selectable: false},
		{ID: 4, Path: "Projects/发票", DisplayName: "发票", Type: "custom", Selectable: true},
	}
	ids := func(fs []folder.Folder) []uint {
		out := []uint{}
		for _, f := range fs {
			out = append(out, f.ID)
		}
		return out
	}
	if got := ids(remoteSearchFolders(all, "")); len(got) != 3 {
		t.Errorf("no qualifier -> %v (不可选中的应被剔除)", got)
	}
	if got := ids(remoteSearchFolders(all, "INBOX")); len(got) != 1 || got[0] != 1 {
		t.Errorf("in:INBOX -> %v", got)
	}
	if got := ids(remoteSearchFolders(all, "发票")); len(got) != 1 || got[0] != 4 {
		t.Errorf("in:发票 -> %v", got)
	}
	if got := ids(remoteSearchFolders(all, "已发送")); len(got) != 1 || got[0] != 2 {
		t.Errorf("in:已发送 -> %v", got)
	}
}
