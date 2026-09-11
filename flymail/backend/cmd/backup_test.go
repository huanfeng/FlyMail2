package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flymail/internal/database"
	"flymail/modules/auth"
)

// seedDB 造一个带管理员的库，返回数据目录。
func seedDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := runDBInit(dir, "", "admin", "secret123"); err != nil {
		t.Fatalf("初始化数据库失败: %v", err)
	}
	return dir
}

// adminExists 用于确认恢复后的库里还有数据，而不仅仅是「文件存在」。
func adminExists(t *testing.T, dbPath string) bool {
	t.Helper()
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	defer closeDB(db)
	u, err := auth.NewRepository(db).GetByUsername("admin")
	return err == nil && u != nil && u.PasswordHash != ""
}

func TestDBBackup_DefaultPath(t *testing.T) {
	dir := seedDB(t)

	if err := runDBBackup(dir, "", ""); err != nil {
		t.Fatalf("备份失败: %v", err)
	}

	matches, _ := filepath.Glob(filepath.Join(dir, "backups", "flymail-*.db"))
	if len(matches) != 1 {
		t.Fatalf("期望在 backups/ 下生成 1 个备份，实际 %d 个", len(matches))
	}
	if fi, err := os.Stat(matches[0]); err != nil || fi.Size() == 0 {
		t.Fatalf("备份文件为空或不可读: %v", err)
	}
	// 备份必须是一个可用的库，而不只是字节搬运。
	if !adminExists(t, matches[0]) {
		t.Error("备份里没有管理员记录")
	}
}

// TestDBBackup_WhileOpen 覆盖本命令存在的理由：服务运行中也能备份。
// 这里保持一个打开的连接并持续写入，模拟正在跑的服务。
func TestDBBackup_WhileOpen(t *testing.T) {
	dir := seedDB(t)

	db, err := database.Open(filepath.Join(dir, "flymail.db"))
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	defer closeDB(db)
	if err := db.Exec("CREATE TABLE probe (n INTEGER)").Error; err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	if err := db.Exec("INSERT INTO probe VALUES (1)").Error; err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	out := filepath.Join(t.TempDir(), "hot.db")
	if err := runDBBackup(dir, "", out); err != nil {
		t.Fatalf("持有连接时备份失败: %v", err)
	}
	if !adminExists(t, out) {
		t.Error("热备份内容不完整")
	}
}

func TestDBBackup_RefusesExistingFile(t *testing.T) {
	dir := seedDB(t)
	out := filepath.Join(t.TempDir(), "taken.db")
	if err := os.WriteFile(out, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runDBBackup(dir, "", out)
	if err == nil {
		t.Fatal("目标文件已存在时不应覆盖")
	}
	if !strings.Contains(err.Error(), "已存在") {
		t.Errorf("错误信息应点明文件已存在，实际: %v", err)
	}
}

func TestDBBackup_MissingDatabase(t *testing.T) {
	if err := runDBBackup(t.TempDir(), "", ""); err == nil {
		t.Fatal("库不存在时应报错而不是产出一个空备份")
	}
}

func TestDBRestore_RoundTrip(t *testing.T) {
	dir := seedDB(t)
	backup := filepath.Join(t.TempDir(), "b.db")
	if err := runDBBackup(dir, "", backup); err != nil {
		t.Fatalf("备份失败: %v", err)
	}

	// 模拟「删库」：换成一个内容不同的新库。
	dbPath := filepath.Join(dir, "flymail.db")
	if err := os.Remove(dbPath); err != nil {
		t.Fatal(err)
	}
	if err := runDBInit(dir, "", "other", "pw123456"); err != nil {
		t.Fatalf("重建数据库失败: %v", err)
	}

	if err := runDBRestore(dir, "", backup, true); err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if !adminExists(t, dbPath) {
		t.Error("恢复后原管理员记录丢失")
	}
}

// TestDBRestore_RequiresForce 恢复是不可逆操作，不带 --force 必须拒绝。
func TestDBRestore_RequiresForce(t *testing.T) {
	dir := seedDB(t)
	backup := filepath.Join(t.TempDir(), "b.db")
	if err := runDBBackup(dir, "", backup); err != nil {
		t.Fatal(err)
	}

	err := runDBRestore(dir, "", backup, false)
	if err == nil {
		t.Fatal("覆盖现有库应要求 --force")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("错误信息应提示 --force，实际: %v", err)
	}
}

// TestDBRestore_KeepsOldDatabase 覆盖前必须留一份，否则误操作无法回退。
func TestDBRestore_KeepsOldDatabase(t *testing.T) {
	dir := seedDB(t)
	backup := filepath.Join(t.TempDir(), "b.db")
	if err := runDBBackup(dir, "", backup); err != nil {
		t.Fatal(err)
	}

	if err := runDBRestore(dir, "", backup, true); err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	kept, _ := filepath.Glob(filepath.Join(dir, "flymail.db.bak-*"))
	if len(kept) != 1 {
		t.Fatalf("期望保留 1 份原库，实际 %d 份", len(kept))
	}
}

// TestDBRestore_RejectsForeignDatabase 一个完好但不属于 FlyMail 的 SQLite 文件，
// integrity_check 会通过，必须靠表结构挡下来——否则服务会带着空库起来。
func TestDBRestore_RejectsForeignDatabase(t *testing.T) {
	dir := seedDB(t)
	foreign := filepath.Join(t.TempDir(), "foreign.db")
	db, err := database.Open(foreign)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE unrelated (id INTEGER)").Error; err != nil {
		t.Fatal(err)
	}
	closeDB(db)

	err = runDBRestore(dir, "", foreign, true)
	if err == nil {
		t.Fatal("非 FlyMail 数据库应被拒绝")
	}
	if !strings.Contains(err.Error(), "FlyMail") {
		t.Errorf("错误信息应说明原因，实际: %v", err)
	}
}

func TestDBRestore_RejectsGarbage(t *testing.T) {
	dir := seedDB(t)
	junk := filepath.Join(t.TempDir(), "junk.db")
	if err := os.WriteFile(junk, []byte("这不是数据库"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runDBRestore(dir, "", junk, true); err == nil {
		t.Fatal("非数据库文件应被拒绝")
	}
	// 被拒绝时原库必须原封不动。
	if !adminExists(t, filepath.Join(dir, "flymail.db")) {
		t.Error("校验失败不应破坏现有数据库")
	}
}

func TestDBRestore_MissingBackup(t *testing.T) {
	dir := seedDB(t)
	if err := runDBRestore(dir, "", filepath.Join(dir, "nope.db"), true); err == nil {
		t.Fatal("备份文件不存在时应报错")
	}
}

// TestDBRestore_ClearsStaleSidecars 旧库的 -wal/-shm 残留若留在原地，
// SQLite 会把它们当成新库的未提交事务重放，直接损坏刚恢复的数据。
func TestDBRestore_ClearsStaleSidecars(t *testing.T) {
	dir := seedDB(t)
	backup := filepath.Join(t.TempDir(), "b.db")
	if err := runDBBackup(dir, "", backup); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "flymail.db")
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.WriteFile(dbPath+suffix, []byte("stale"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := runDBRestore(dir, "", backup, true); err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(dbPath + suffix); !os.IsNotExist(err) {
			t.Errorf("残留的 %s 未被清理", suffix)
		}
	}
}

func TestHumanSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{512, "512 B"},
		{2048, "2.0 KB"},
		{3 * 1024 * 1024, "3.0 MB"},
	}
	for _, c := range cases {
		if got := humanSize(c.in); got != c.want {
			t.Errorf("humanSize(%d) = %q，期望 %q", c.in, got, c.want)
		}
	}
}
