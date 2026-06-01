package cmd

import (
	"fmt"
	"os"

	"flymail/internal/config"
	"flymail/internal/database"
	"flymail/modules/auth"

	"github.com/spf13/cobra"
)

var dbCmd = &cobra.Command{Use: "db", Short: "数据库管理"}

var dbInitCmd = &cobra.Command{
	Use:   "init",
	Short: "初始化数据库并设置管理员账户",
	RunE: func(cmd *cobra.Command, args []string) error {
		username, _ := cmd.Flags().GetString("admin-user")
		password, _ := cmd.Flags().GetString("admin-pass")
		if password == "" {
			// 开发便利：未指定密码时默认 admin（配合 admin-user 默认 admin = admin/admin）。
			password = "admin"
			fmt.Println("⚠ 未指定 --admin-pass，使用默认开发密码 \"admin\"；生产环境请用 db reset-admin-password 修改")
		}
		return runDBInit(dataDir, configFile, username, password)
	},
}

var dbResetPwCmd = &cobra.Command{
	Use:   "reset-admin-password",
	Short: "重置管理员密码",
	RunE: func(cmd *cobra.Command, args []string) error {
		username, _ := cmd.Flags().GetString("admin-user")
		password, _ := cmd.Flags().GetString("admin-pass")
		if password == "" {
			return fmt.Errorf("--admin-pass 不能为空")
		}
		return runResetAdminPassword(dataDir, configFile, username, password)
	},
}

func init() {
	for _, c := range []*cobra.Command{dbInitCmd, dbResetPwCmd} {
		c.Flags().String("admin-user", "admin", "管理员用户名")
		c.Flags().String("admin-pass", "", "管理员密码")
	}
	dbCmd.AddCommand(dbInitCmd, dbResetPwCmd)
	rootCmd.AddCommand(dbCmd)
}

func openForCLI(dir, cfgFile string) (*config.Config, *auth.Service, func(), error) {
	cfg, err := config.Load(config.LoadOptions{DataDir: dir, ConfigFile: cfgFile})
	if err != nil {
		return nil, nil, nil, err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, nil, nil, err
	}
	db, err := database.Open(cfg.DBPath())
	if err != nil {
		return nil, nil, nil, err
	}
	if err := database.Migrate(db); err != nil {
		return nil, nil, nil, err
	}
	svc := auth.NewService(auth.NewRepository(db), auth.Options{
		JWTSecret:      cfg.Auth.JWTSecret,
		AccessTTLMin:   cfg.Auth.AccessTokenTTL,
		RefreshTTLHour: cfg.Auth.RefreshTokenTTL,
	})
	close := func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	}
	return cfg, svc, close, nil
}

func runDBInit(dir, cfgFile, username, password string) error {
	_, svc, close, err := openForCLI(dir, cfgFile)
	if err != nil {
		return err
	}
	defer close()
	if err := svc.SetAdminPassword(username, password); err != nil {
		return err
	}
	fmt.Printf("数据库已初始化，管理员 %q 已创建\n", username)
	return nil
}

func runResetAdminPassword(dir, cfgFile, username, password string) error {
	_, svc, close, err := openForCLI(dir, cfgFile)
	if err != nil {
		return err
	}
	defer close()
	if err := svc.SetAdminPassword(username, password); err != nil {
		return err
	}
	fmt.Printf("管理员 %q 密码已重置\n", username)
	return nil
}
