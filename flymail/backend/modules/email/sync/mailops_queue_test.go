package sync

// 覆盖「本地先改 + 回写队列异步补」这条路径：
// 邮件操作接口必须立刻返回、不建 IMAP 连接，服务器侧动作以队列记录的形式留下。

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	coreimap "flymail-core/imap"
	"flymail-core/types"
	imapv2 "github.com/emersion/go-imap/v2"

	"flymail/internal/database"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"
)

// wbRecSession 在 mgrFakeSession 基础上记录 SELECT/DELETE/MOVE 的实际参数。
type wbRecSession struct {
	mgrFakeSession
	selected []string
	deleted  []imapv2.UID
	moved    []imapv2.UID
	moveTo   string
}

func (s *wbRecSession) SelectFolder(path string) (*coreimap.SelectedFolder, error) {
	s.selected = append(s.selected, path)
	return &coreimap.SelectedFolder{Path: path}, nil
}

func (s *wbRecSession) Delete(uids ...imapv2.UID) error {
	s.deleted = append(s.deleted, uids...)
	return nil
}

func (s *wbRecSession) Move(mailbox string, uids ...imapv2.UID) error {
	s.moveTo = mailbox
	s.moved = append(s.moved, uids...)
	return nil
}

// wbFakeOrch 记录入队的回写操作；其余编排能力在本测试中不涉及。
type wbFakeOrch struct {
	ops []*WritebackOp
}

func (o *wbFakeOrch) TriggerSync(context.Context, uint) error                       { return nil }
func (o *wbFakeOrch) ForegroundOp(context.Context, uint, func(Session) error) error { return nil }
func (o *wbFakeOrch) BackgroundOp(uint, func(Session) error) bool                   { return true }
func (o *wbFakeOrch) EnqueueWriteback(op *WritebackOp)                              { o.ops = append(o.ops, op) }

type wbFakeAccounts struct{}

func (wbFakeAccounts) IMAPConfig(uint) (types.IMAPConfig, error) { return types.IMAPConfig{}, nil }
func (wbFakeAccounts) TouchLastSync(uint, time.Time) error       { return nil }
func (wbFakeAccounts) IsEnabled(uint) (bool, error)              { return true, nil }

// newQueuedService 组装一个接了 fake 编排器的 Service：任何建连接的尝试都会被记下。
func newQueuedService(t *testing.T) (*Service, *folder.Repository, *message.Repository, *wbFakeOrch) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	frepo := folder.NewRepository(db)
	mrepo := message.NewRepository(db)
	svc := NewService(wbFakeAccounts{}, folder.NewService(frepo), message.NewService(mrepo, message.NewBodyRepository(db)))
	orch := &wbFakeOrch{}
	svc.orch = orch
	// dial 一旦被调用即为回归：操作不该在请求线程里建连接。
	svc.SetDial(func(types.IMAPConfig) (Session, error) {
		t.Error("操作不应在请求路径上建立 IMAP 连接")
		return &wbRecSession{}, nil
	})
	return svc, frepo, mrepo, orch
}

func qSeedFolder(t *testing.T, frepo *folder.Repository, accountID uint, path, ftype string) uint {
	t.Helper()
	f := &folder.Folder{AccountID: accountID, Path: path, DisplayName: path, Type: ftype, Selectable: true}
	if err := frepo.UpsertByPath(f); err != nil {
		t.Fatalf("seed folder %s: %v", path, err)
	}
	return f.ID
}

func qSeedMsg(t *testing.T, mrepo *message.Repository, accountID, folderID uint, uid uint32) uint {
	t.Helper()
	m := &message.Message{AccountID: accountID, FolderID: folderID, UID: uid, Date: time.Now()}
	if err := mrepo.Upsert(m); err != nil {
		t.Fatalf("seed msg: %v", err)
	}
	return m.ID
}

func TestDeleteMessageEnqueuesMoveToTrash(t *testing.T) {
	svc, frepo, mrepo, orch := newQueuedService(t)
	inbox := qSeedFolder(t, frepo, 1, "INBOX", "inbox")
	qSeedFolder(t, frepo, 1, "Trash", "trash")
	id := qSeedMsg(t, mrepo, 1, inbox, 7)

	if err := svc.DeleteMessage(id); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	// 本地立即生效
	if _, err := mrepo.GetByID(id); err == nil {
		t.Error("本地行应已删除")
	}
	// 服务器侧只留下一条队列记录
	if len(orch.ops) != 1 {
		t.Fatalf("入队 %d 条，期望 1 条", len(orch.ops))
	}
	op := orch.ops[0]
	if op.Op != wbOpMove || op.TargetPath != "Trash" || op.FolderPath != "INBOX" || op.UIDs != "7" {
		t.Errorf("回写记录不对: %+v", op)
	}
}

func TestDeleteInTrashEnqueuesExpunge(t *testing.T) {
	svc, frepo, mrepo, orch := newQueuedService(t)
	trash := qSeedFolder(t, frepo, 1, "Trash", "trash")
	id := qSeedMsg(t, mrepo, 1, trash, 3)

	if err := svc.DeleteMessage(id); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if len(orch.ops) != 1 || orch.ops[0].Op != wbOpExpunge {
		t.Fatalf("回收站内删除应入队 expunge，实际 %+v", orch.ops)
	}
}

func TestBatchDeleteMergesUIDsPerFolder(t *testing.T) {
	svc, frepo, mrepo, orch := newQueuedService(t)
	inbox := qSeedFolder(t, frepo, 1, "INBOX", "inbox")
	qSeedFolder(t, frepo, 1, "Trash", "trash")
	ids := []uint{
		qSeedMsg(t, mrepo, 1, inbox, 11),
		qSeedMsg(t, mrepo, 1, inbox, 12),
		qSeedMsg(t, mrepo, 1, inbox, 13),
	}

	if err := svc.BatchDelete(ids); err != nil {
		t.Fatalf("BatchDelete: %v", err)
	}
	// 同一文件夹的三封合并成一条记录，服务器侧只需一次 SELECT + 一次 MOVE
	if len(orch.ops) != 1 {
		t.Fatalf("入队 %d 条，期望合并成 1 条", len(orch.ops))
	}
	if orch.ops[0].UIDs != "11,12,13" {
		t.Errorf("UIDs = %q, 期望 11,12,13", orch.ops[0].UIDs)
	}
	for _, id := range ids {
		if _, err := mrepo.GetByID(id); err == nil {
			t.Errorf("邮件 %d 的本地行应已删除", id)
		}
	}
}

func TestBatchSetReadUpdatesLocalAndEnqueues(t *testing.T) {
	svc, frepo, mrepo, orch := newQueuedService(t)
	inbox := qSeedFolder(t, frepo, 1, "INBOX", "inbox")
	ids := []uint{qSeedMsg(t, mrepo, 1, inbox, 21), qSeedMsg(t, mrepo, 1, inbox, 22)}

	if err := svc.BatchSetRead(ids, true); err != nil {
		t.Fatalf("BatchSetRead: %v", err)
	}
	for _, id := range ids {
		m, err := mrepo.GetByID(id)
		if err != nil || !m.Seen {
			t.Errorf("邮件 %d 本地应已标记已读 (err=%v)", id, err)
		}
	}
	if len(orch.ops) != 1 || orch.ops[0].Op != wbOpRead || orch.ops[0].UIDs != "21,22" {
		t.Fatalf("回写记录不对: %+v", orch.ops)
	}
}

func TestMoveMessageEnqueuesWithTargetPath(t *testing.T) {
	svc, frepo, mrepo, orch := newQueuedService(t)
	inbox := qSeedFolder(t, frepo, 1, "INBOX", "inbox")
	dst := qSeedFolder(t, frepo, 1, "Work", "custom")
	id := qSeedMsg(t, mrepo, 1, inbox, 5)

	if err := svc.MoveMessage(id, dst); err != nil {
		t.Fatalf("MoveMessage: %v", err)
	}
	if len(orch.ops) != 1 {
		t.Fatalf("入队 %d 条，期望 1 条", len(orch.ops))
	}
	if op := orch.ops[0]; op.Op != wbOpMove || op.TargetPath != "Work" || op.FolderPath != "INBOX" {
		t.Errorf("回写记录不对: %+v", op)
	}
}

// ── applyWriteback / UID 序列化 ────────────────────────────────────────────

func TestOpUIDsParsing(t *testing.T) {
	cases := []struct {
		name string
		op   WritebackOp
		want []imapv2.UID
	}{
		{"多 UID", WritebackOp{UIDs: "1,2,3"}, []imapv2.UID{1, 2, 3}},
		{"旧数据只有单 UID", WritebackOp{UID: 9}, []imapv2.UID{9}},
		{"UIDs 优先于 UID", WritebackOp{UID: 9, UIDs: "4,5"}, []imapv2.UID{4, 5}},
		{"跳过脏片段", WritebackOp{UIDs: "1,,x,0,2"}, []imapv2.UID{1, 2}},
		{"全空", WritebackOp{}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := opUIDs(c.op)
			if len(got) != len(c.want) {
				t.Fatalf("opUIDs = %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("opUIDs = %v, want %v", got, c.want)
				}
			}
		})
	}
}

func TestJoinUIDs(t *testing.T) {
	if got := joinUIDs([]uint32{3, 1, 2}); got != "3,1,2" {
		t.Errorf("joinUIDs = %q", got)
	}
	if got := joinUIDs(nil); got != "" {
		t.Errorf("joinUIDs(nil) = %q", got)
	}
}

func TestApplyWritebackMoveAndExpunge(t *testing.T) {
	sess := &wbRecSession{}
	err := applyWriteback(sess, WritebackOp{FolderPath: "INBOX", UIDs: "1,2", Op: wbOpMove, TargetPath: "Trash"})
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if sess.moveTo != "Trash" || len(sess.moved) != 2 {
		t.Errorf("move 参数不对: to=%q uids=%v", sess.moveTo, sess.moved)
	}
	if len(sess.selected) != 1 || sess.selected[0] != "INBOX" {
		t.Errorf("应先 SELECT 源文件夹，实际 %v", sess.selected)
	}

	sess2 := &wbRecSession{}
	if err := applyWriteback(sess2, WritebackOp{FolderPath: "Trash", UIDs: "8", Op: wbOpExpunge}); err != nil {
		t.Fatalf("expunge: %v", err)
	}
	if len(sess2.deleted) != 1 || sess2.deleted[0] != 8 {
		t.Errorf("expunge 参数不对: %v", sess2.deleted)
	}
}

// 空 UID 的记录直接判成功：否则会永远重试到放弃，白占队列。
func TestApplyWritebackEmptyIsNoop(t *testing.T) {
	sess := &wbRecSession{}
	if err := applyWriteback(sess, WritebackOp{FolderPath: "INBOX", Op: wbOpRead}); err != nil {
		t.Fatalf("空操作应直接成功: %v", err)
	}
	if len(sess.selected) != 0 {
		t.Error("空操作不该 SELECT 文件夹")
	}
}
