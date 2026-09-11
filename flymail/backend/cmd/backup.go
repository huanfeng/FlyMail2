package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"flymail/internal/config"
	"flymail/internal/database"

	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

// 备份与恢复。
//
// 为什么内建而不是让用户拿 sqlite3 CLI 操作：运行镜像是 alpine + 静态二进制，
// 里面没有 sqlite3，也不该为了备份往镜像里塞一个。而直接 cp 一个正在被写入的库文件
// 会拿到撕裂的快照——SQLite 的一致性保证只在事务边界上成立，cp 不理解事务边界。
//
// 备份只覆盖数据库。附件是普通文件，直接拷 <数据目录>/attachments 即可，
// 没有一致性问题（附件写入后不再修改）。

var dbBackupCmd = &cobra.Command{
	Use:   "backup",
	Short: "热备份数据库（服务运行中也可执行）",
	Long: "用 SQLite 的 VACUUM INTO 生成一份一致的数据库副本。\n" +
		"附件不在备份范围内，请另行拷贝 <数据目录>/attachments。",
	RunE: func(cmd *cobra.Command, args []string) error {
		output, _ := cmd.Flags().GetString("output")
		return runDBBackup(dataDir, configFile, output)
	},
}

var dbRestoreCmd = &cobra.Command{
	Use:   "restore <备份文件>",
	Short: "从备份文件恢复数据库",
	Long: "校验备份文件后覆盖当前数据库。执行前请先停止服务，\n" +
		"否则运行中的进程仍持有旧文件句柄，恢复结果会被它的后续写入破坏。",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")
		return runDBRestore(dataDir, configFile, args[0], force)
	},
}

func init() {
	dbBackupCmd.Flags().String("output", "", "备份文件路径（默认 <数据目录>/backups/flymail-<时间戳>.db）")
	dbRestoreCmd.Flags().Bool("force", false, "确认覆盖现有数据库（现有库会先改名保留）")
	dbCmd.AddCommand(dbBackupCmd, dbRestoreCmd)
}

// backupStamp 是备份文件名与保留副本用的时间戳格式。
const backupStamp = "20060102-150405"

func runDBBackup(dir, cfgFile, output string) error {
	cfg, err := config.Load(config.LoadOptions{DataDir: dir, ConfigFile: cfgFile})
	if err != nil {
		return err
	}
	src := cfg.DBPath()
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("数据库不存在：%s", src)
	}

	if output == "" {
		output = filepath.Join(cfg.DataDir, "backups",
			fmt.Sprintf("flymail-%s.db", time.Now().Format(backupStamp)))
	}
	// VACUUM INTO 要求目标文件不存在，先把话说清楚，别让用户去猜 SQLite 的原文报错。
	if _, err := os.Stat(output); err == nil {
		return fmt.Errorf("备份文件已存在：%s", output)
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}

	// 不走 database.Migrate：备份一个旧版本的库时，不应顺手改它的结构。
	db, err := database.Open(src)
	if err != nil {
		return err
	}
	defer closeDB(db)

	// VACUUM INTO 在一个读事务里把整库写出为一个整理过的新文件，期间不阻塞其他读者，
	// 因此服务运行中也能执行。
	if err := db.Exec("VACUUM INTO ?", output).Error; err != nil {
		return fmt.Errorf("备份失败：%w", err)
	}

	size := int64(0)
	if fi, err := os.Stat(output); err == nil {
		size = fi.Size()
	}
	fmt.Printf("已备份到 %s（%s）\n", output, humanSize(size))
	fmt.Println("提示：附件未包含在内，如需完整备份请一并拷贝",
		filepath.Join(cfg.DataDir, "attachments"))
	return nil
}

func runDBRestore(dir, cfgFile, src string, force bool) error {
	cfg, err := config.Load(config.LoadOptions{DataDir: dir, ConfigFile: cfgFile})
	if err != nil {
		return err
	}
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("备份文件不存在：%s", src)
	}
	if err := verifyBackup(src); err != nil {
		return err
	}

	dst := cfg.DBPath()
	if _, err := os.Stat(dst); err == nil {
		if !force {
			return fmt.Errorf("数据库已存在：%s\n"+
				"确认服务已停止后，加 --force 重新执行（现有库会改名保留，不会被直接删除）", dst)
		}
		kept := fmt.Sprintf("%s.bak-%s", dst, time.Now().Format(backupStamp))
		if err := os.Rename(dst, kept); err != nil {
			return fmt.Errorf("保留现有数据库失败：%w", err)
		}
		fmt.Printf("现有数据库已改名保留为 %s\n", kept)
	} else if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return err
	}

	if err := copyFile(src, dst); err != nil {
		return fmt.Errorf("写入数据库失败：%w", err)
	}
	// 清理可能残留的旁文件：它们属于被替换掉的那个库，留在原地会被 SQLite 当成
	// 新库的未提交事务重放，直接损坏刚恢复的数据。
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		os.Remove(dst + suffix)
	}

	fmt.Printf("已从 %s 恢复到 %s\n", src, dst)
	fmt.Println("提示：附件不在备份范围内，如有需要请单独恢复 attachments 目录；随后重启服务。")
	return nil
}

// verifyBackup 确认这是一个完好的 FlyMail 数据库，避免把服务恢复成一个打不开的文件。
func verifyBackup(path string) error {
	db, err := database.Open(path)
	if err != nil {
		return fmt.Errorf("备份文件无法打开：%w", err)
	}
	defer closeDB(db)

	var result string
	if err := db.Raw("PRAGMA integrity_check").Scan(&result).Error; err != nil {
		return fmt.Errorf("备份文件校验失败：%w", err)
	}
	if result != "ok" {
		return fmt.Errorf("备份文件已损坏：%s", result)
	}

	// 结构检查：integrity_check 只保证「是个完好的 SQLite 文件」，不保证是 FlyMail 的库。
	var n int64
	if err := db.Raw(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name IN ('admin_users','accounts','messages')",
	).Scan(&n).Error; err != nil {
		return fmt.Errorf("备份文件校验失败：%w", err)
	}
	if n < 3 {
		return fmt.Errorf("这不是一个 FlyMail 数据库：%s", path)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// closeDB 释放连接。Windows 下不关闭会占住文件，后续的改名与删除都会失败。
func closeDB(db *gorm.DB) {
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.Close()
	}
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}
