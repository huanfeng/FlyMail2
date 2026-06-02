package message_test

import (
	"testing"
	"time"

	"flymail/modules/email/message"

	coreimap "flymail-core/imap"
	"flymail-core/types"

	imapv2 "github.com/emersion/go-imap/v2"
)

// recordingFetcher 在 fakeFetcher 基础上记录最后一次 Fetch 调用的入参，
// 用于断言增量同步实际请求的区间。
type recordingFetcher struct {
	uidValidity uint32
	uidNext     uint32
	numMessages uint32
	emails      map[uint32]*types.ParsedEmail

	// statusUIDNext 为 STATUS(UIDNEXT) 兜底返回值；nil 表示服务商不报 UIDNEXT。
	statusUIDNext *uint32

	// 记录最后一次调用入参。
	uidFetchCalled bool
	uidFrom        imapv2.UID
	uidTo          imapv2.UID
	seqFetchCalled bool
	seqFrom        uint32
	seqTo          uint32
}

func (f *recordingFetcher) SelectFolder(path string) (*coreimap.SelectedFolder, error) {
	return &coreimap.SelectedFolder{
		Path:        path,
		NumMessages: f.numMessages,
		UIDValidity: f.uidValidity,
		UIDNext:     f.uidNext,
	}, nil
}

func (f *recordingFetcher) FolderStatus(path string, items ...coreimap.StatusItem) (*coreimap.FolderStatusResult, error) {
	v := f.uidValidity
	return &coreimap.FolderStatusResult{UIDValidity: &v, UIDNext: f.statusUIDNext}, nil
}

func (f *recordingFetcher) FetchByUIDRange(from, to imapv2.UID, opts coreimap.FetchOptions) ([]*types.ParsedEmail, error) {
	f.uidFetchCalled = true
	f.uidFrom, f.uidTo = from, to
	var out []*types.ParsedEmail
	for uid, e := range f.emails {
		if imapv2.UID(uid) >= from && (to == 0 || imapv2.UID(uid) <= to) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *recordingFetcher) FetchBySeqRange(from, to uint32, opts coreimap.FetchOptions) ([]*types.ParsedEmail, error) {
	f.seqFetchCalled = true
	f.seqFrom, f.seqTo = from, to
	var out []*types.ParsedEmail
	for uid, e := range f.emails {
		if uid >= from && (to == 0 || uid <= to) {
			out = append(out, e)
		}
	}
	return out, nil
}

// 预置文件夹内连续 UID 的邮件（uid in [from,to]）。
func seedFolder(t *testing.T, repo *message.Repository, from, to uint32) {
	t.Helper()
	for uid := from; uid <= to; uid++ {
		if err := repo.Upsert(&message.Message{AccountID: 1, FolderID: 1, UID: uid, Date: time.Now()}); err != nil {
			t.Fatalf("seed upsert uid=%d: %v", uid, err)
		}
	}
}

func mkEmails(from, to uint32) map[uint32]*types.ParsedEmail {
	m := map[uint32]*types.ParsedEmail{}
	for uid := from; uid <= to; uid++ {
		m[uid] = &types.ParsedEmail{UID: uid, Subject: "m", Date: time.Now()}
	}
	return m
}

// 场景 1：已知 UIDNEXT，anchor=prevUIDNext，抓 [prevUIDNext, uidNext-1]。
// 本地无邮件；SELECT 返回 UIDNext=11、NumMessages=10；prevUIDNext=6。
// 期望：FetchByUIDRange 收到 from=6 to=10；state.UIDNext=11；newCount=5。
func TestIncrementalSyncKnownUIDNext(t *testing.T) {
	svc, repo := newMsgService(t)
	f := &recordingFetcher{
		uidValidity: 1,
		uidNext:     11,
		numMessages: 10,
		emails:      mkEmails(6, 10), // 服务器侧新邮件 uid 6..10
	}
	state, newCount, err := svc.IncrementalSync(1, 1, "INBOX", 1, 6, 5, f)
	if err != nil {
		t.Fatalf("IncrementalSync: %v", err)
	}
	if !f.uidFetchCalled {
		t.Fatalf("应调用 FetchByUIDRange")
	}
	if f.uidFrom != 6 || f.uidTo != 10 {
		t.Errorf("FetchByUIDRange 入参 = [%d,%d], 期望 [6,10]", f.uidFrom, f.uidTo)
	}
	if f.seqFetchCalled {
		t.Errorf("不应调用 FetchBySeqRange")
	}
	if state.UIDNext != 11 {
		t.Errorf("state.UIDNext = %d, 期望 11", state.UIDNext)
	}
	if newCount != 5 {
		t.Errorf("newCount = %d, 期望 5", newCount)
	}
	if cnt, _ := repo.CountByFolder(1); cnt != 5 {
		t.Errorf("本地邮件数 = %d, 期望 5", cnt)
	}
}

// 场景 2：无 UIDNEXT（163）。SELECT 返回 UIDNext=0、NumMessages=12；STATUS(UIDNext)=nil；prevTotal=10。
// 期望：FetchBySeqRange 收到 from=11 to=12；newCount=2；state.UIDNext = 本地 maxUID+1。
func TestIncrementalSyncNoUIDNext(t *testing.T) {
	svc, repo := newMsgService(t)
	// 本地已有 uid 1..10。
	seedFolder(t, repo, 1, 10)
	f := &recordingFetcher{
		uidValidity:   1,
		uidNext:       0,
		numMessages:   12,
		statusUIDNext: nil,
		emails:        mkEmails(11, 12), // 序号 11、12 对应 uid 11、12
	}
	state, newCount, err := svc.IncrementalSync(1, 1, "INBOX", 1, 0, 10, f)
	if err != nil {
		t.Fatalf("IncrementalSync: %v", err)
	}
	if !f.seqFetchCalled {
		t.Fatalf("应调用 FetchBySeqRange")
	}
	if f.seqFrom != 11 || f.seqTo != 12 {
		t.Errorf("FetchBySeqRange 入参 = [%d,%d], 期望 [11,12]", f.seqFrom, f.seqTo)
	}
	if f.uidFetchCalled {
		t.Errorf("不应调用 FetchByUIDRange")
	}
	if newCount != 2 {
		t.Errorf("newCount = %d, 期望 2", newCount)
	}
	if state.UIDNext != 13 { // 本地 maxUID(12)+1
		t.Errorf("state.UIDNext = %d, 期望 13 (maxUID+1)", state.UIDNext)
	}
}

// 场景 3：无新邮件。已知 UIDNEXT 且 prevUIDNext == uidNext → 不调用任何 Fetch；newCount=0。
func TestIncrementalSyncNoNewMessages(t *testing.T) {
	svc, repo := newMsgService(t)
	seedFolder(t, repo, 1, 10)
	f := &recordingFetcher{
		uidValidity: 1,
		uidNext:     11,
		numMessages: 10,
		emails:      map[uint32]*types.ParsedEmail{},
	}
	state, newCount, err := svc.IncrementalSync(1, 1, "INBOX", 1, 11, 10, f)
	if err != nil {
		t.Fatalf("IncrementalSync: %v", err)
	}
	if f.uidFetchCalled || f.seqFetchCalled {
		t.Errorf("无新邮件时不应触发任何 Fetch (uid=%v seq=%v)", f.uidFetchCalled, f.seqFetchCalled)
	}
	if newCount != 0 {
		t.Errorf("newCount = %d, 期望 0", newCount)
	}
	if state.UIDNext != 11 {
		t.Errorf("state.UIDNext = %d, 期望 11", state.UIDNext)
	}
}
