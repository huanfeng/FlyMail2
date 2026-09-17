package folder

import (
	"path/filepath"
	"testing"

	"flymail-core/types"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// fakeLister 按给定的路径列表冒充一次 IMAP LIST。
type fakeLister struct {
	paths []string
	err   error
}

func (f fakeLister) ListFolders() ([]types.FolderInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]types.FolderInfo, 0, len(f.paths))
	for _, p := range f.paths {
		out = append(out, types.FolderInfo{Name: p, Path: p, Delimiter: "/"})
	}
	return out, nil
}

func newFolderDB(t *testing.T) (*gorm.DB, *Service) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "t.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&Folder{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// 邮件相关的表用裸 SQL 建：folder 包不能 import message（会成环），
	// 而清理逻辑本来就是按表名写的。
	for _, ddl := range []string{
		`CREATE TABLE messages (id INTEGER PRIMARY KEY, account_id INTEGER, folder_id INTEGER)`,
		`CREATE TABLE attachments (id INTEGER PRIMARY KEY, message_id INTEGER)`,
		`CREATE TABLE message_bodies (id INTEGER PRIMARY KEY, message_id INTEGER)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	return db, NewService(NewRepository(db))
}

// seedFolderWithMail 建一个文件夹并塞一封带正文和附件的邮件，返回文件夹 ID。
func seedFolderWithMail(t *testing.T, db *gorm.DB, accountID uint, path string) uint {
	t.Helper()
	f := &Folder{AccountID: accountID, Path: path, DisplayName: path, Type: "custom", Selectable: true}
	if err := db.Create(f).Error; err != nil {
		t.Fatalf("create folder: %v", err)
	}
	if err := db.Exec(`INSERT INTO messages (account_id, folder_id) VALUES (?, ?)`, accountID, f.ID).Error; err != nil {
		t.Fatalf("seed message: %v", err)
	}
	var mid uint
	if err := db.Raw(`SELECT id FROM messages WHERE folder_id = ?`, f.ID).Scan(&mid).Error; err != nil {
		t.Fatalf("read message id: %v", err)
	}
	for _, tbl := range []string{"attachments", "message_bodies"} {
		if err := db.Exec(`INSERT INTO `+tbl+` (message_id) VALUES (?)`, mid).Error; err != nil {
			t.Fatalf("seed %s: %v", tbl, err)
		}
	}
	return f.ID
}

func count(t *testing.T, db *gorm.DB, table, where string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.Raw(`SELECT COUNT(*) FROM `+table+` WHERE `+where, args...).Scan(&n).Error; err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// 服务端已经删掉的文件夹，本地要跟着消失。
//
// ── 缘起 ─────────────────────────────────────────────────────────────────────
//
// SyncFolders 原先只 Upsert、从不删除。用户在网页端删掉一个文件夹之后，本地那行
// 一直留着，每一轮同步都会去 SELECT 它、每一轮都失败——测试账户上两天刷了
// 8876 次 `NO SELECT failed. No such mailbox`，侧栏里还挂着点进去就报错的入口。
func TestSyncFoldersPrunesMissing(t *testing.T) {
	db, svc := newFolderDB(t)
	seedFolderWithMail(t, db, 1, "INBOX")
	goneID := seedFolderWithMail(t, db, 1, "项目 A")

	// 服务端这一轮只报了 INBOX——「项目 A」已经被用户删掉了
	if err := svc.SyncFolders(1, fakeLister{paths: []string{"INBOX"}}); err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}

	if n := count(t, db, "folders", "id = ?", goneID); n != 0 {
		t.Errorf("消失的文件夹还留着")
	}
	if n := count(t, db, "folders", "path = ?", "INBOX"); n != 1 {
		t.Errorf("INBOX 被误删了")
	}
	// 邮件与它的子表要一并清掉，否则就是换一种孤儿
	if n := count(t, db, "messages", "folder_id = ?", goneID); n != 0 {
		t.Errorf("文件夹没了，它的 %d 封邮件还留着", n)
	}
	if n := count(t, db, "attachments", "1=1"); n != 1 {
		t.Errorf("附件表剩 %d 行，应当只剩 INBOX 那封的", n)
	}
	if n := count(t, db, "message_bodies", "1=1"); n != 1 {
		t.Errorf("正文表剩 %d 行，应当只剩 INBOX 那封的", n)
	}
}

// ⚠⚠ 这是这段代码最要紧的一条：LIST 返回空时一个都不能删。
//
// 判断依据是一次 IMAP LIST，而 LIST 会因为连接抖动、权限变化、服务端故障返回空
// 结果。照单全收的话，一次抖动就能把这个账户的本地邮件**全部删光**——用户毫无
// 察觉，本地也没有任何地方能恢复。
//
// 留着陈旧文件夹的代价只是日志里多几行错误；删错的代价是数据没了。两者不对称，
// 所以往安全一侧倒：空列表一律跳过。
func TestSyncFoldersRefusesToPruneOnEmptyList(t *testing.T) {
	db, svc := newFolderDB(t)
	seedFolderWithMail(t, db, 1, "INBOX")
	seedFolderWithMail(t, db, 1, "项目 A")

	if err := svc.SyncFolders(1, fakeLister{paths: nil}); err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}

	if n := count(t, db, "folders", "account_id = ?", 1); n != 2 {
		t.Errorf("服务端返回空列表时删掉了文件夹，只剩 %d 个——一次连接抖动就会清空用户的本地邮件", n)
	}
	if n := count(t, db, "messages", "account_id = ?", 1); n != 2 {
		t.Errorf("服务端返回空列表时删掉了邮件，只剩 %d 封", n)
	}
}

// LIST 本身失败时更不能删，而且要把错误抛上去。
func TestSyncFoldersKeepsEverythingWhenListFails(t *testing.T) {
	db, svc := newFolderDB(t)
	seedFolderWithMail(t, db, 1, "INBOX")

	err := svc.SyncFolders(1, fakeLister{err: errBoom})
	if err == nil {
		t.Error("LIST 失败却没有报错，调用方会以为同步成功了")
	}
	if n := count(t, db, "folders", "account_id = ?", 1); n != 1 {
		t.Errorf("LIST 失败时动了本地文件夹，剩 %d 个", n)
	}
}

// 清理只影响当前账户。
//
// 多账户共用一张表，按 account_id 过滤漏掉的话，同步一个账户会删掉别人的文件夹。
func TestSyncFoldersOnlyTouchesItsOwnAccount(t *testing.T) {
	db, svc := newFolderDB(t)
	seedFolderWithMail(t, db, 1, "INBOX")
	seedFolderWithMail(t, db, 2, "INBOX")
	otherID := seedFolderWithMail(t, db, 2, "项目 A")

	// 账户 1 同步，账户 2 的「项目 A」不该受影响
	if err := svc.SyncFolders(1, fakeLister{paths: []string{"INBOX"}}); err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if n := count(t, db, "folders", "id = ?", otherID); n != 1 {
		t.Error("同步账户 1 删掉了账户 2 的文件夹")
	}
	if n := count(t, db, "messages", "account_id = ?", 2); n != 2 {
		t.Errorf("同步账户 1 删掉了账户 2 的邮件，剩 %d 封", n)
	}
}

// 新增的文件夹照常入库，清理不该把刚 Upsert 的那个也带走。
func TestSyncFoldersKeepsNewlyAdded(t *testing.T) {
	db, svc := newFolderDB(t)
	seedFolderWithMail(t, db, 1, "INBOX")

	if err := svc.SyncFolders(1, fakeLister{paths: []string{"INBOX", "新文件夹"}}); err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if n := count(t, db, "folders", "account_id = ?", 1); n != 2 {
		t.Errorf("同步后应有 2 个文件夹，实际 %d 个", n)
	}
	if n := count(t, db, "folders", "path = ?", "新文件夹"); n != 1 {
		t.Error("新文件夹没有入库")
	}
}

var errBoom = errBoomType{}

type errBoomType struct{}

func (errBoomType) Error() string { return "boom" }
