package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"flymail/pkg/logger"
	"flymail/shared/config"
	"flymail/shared/database"
	"flymail/shared/server"

	"go.uber.org/zap"
)

var rootCmd = &cobra.Command{
	Use:   "flymail",
	Short: "FlyMail - Self-hosted email client backend",
	Long:  `FlyMail is a self-hosted single-user email client backend with multi-account support.`,
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the FlyMail server",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Printf("Failed to load config: %v\n", err)
			os.Exit(1)
		}

		// Initialize logger
		loggerCfg := &logger.Config{
			Level:       cfg.Logger.Level,
			Development: cfg.Logger.Development,
			OutputPaths: cfg.Logger.OutputPaths,
		}

		// Copy rotation config if present
		if cfg.Logger.Rotation != nil {
			loggerCfg.Rotation = &logger.RotationConfig{
				MaxSize:    cfg.Logger.Rotation.MaxSize,
				MaxBackups: cfg.Logger.Rotation.MaxBackups,
				MaxAge:     cfg.Logger.Rotation.MaxAge,
				Compress:   cfg.Logger.Rotation.Compress,
			}
		}
		if err := logger.Init(loggerCfg); err != nil {
			fmt.Printf("Failed to initialize logger: %v\n", err)
			os.Exit(1)
		}
		defer logger.Sync()

		logger.Info("Starting FlyMail server")

		if err := database.Init(cfg); err != nil {
			logger.Fatal("Failed to initialize database", zap.Error(err))
		}

		srv, err := server.New(cfg)
		if err != nil {
			logger.Fatal("Failed to create server", zap.Error(err))
		}

		if err := srv.Start(); err != nil {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	},
}

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Database operations",
}

var dbInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize database",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Printf("Failed to load config: %v\n", err)
			os.Exit(1)
		}

		if err := database.Init(cfg); err != nil {
			fmt.Printf("Failed to initialize database: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Database initialized successfully")
	},
}

var dbResetAdminPasswordCmd = &cobra.Command{
	Use:   "reset-admin-password",
	Short: "Reset admin password",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Printf("Failed to load config: %v\n", err)
			os.Exit(1)
		}

		if err := database.ResetAdminPassword(cfg); err != nil {
			fmt.Printf("Failed to reset admin password: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Admin password reset successfully")
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(dbCmd)
	dbCmd.AddCommand(dbInitCmd)
	dbCmd.AddCommand(dbResetAdminPasswordCmd)

	// Global flags
	rootCmd.PersistentFlags().String("config", "./data/config.yaml", "config file path")
	rootCmd.PersistentFlags().String("data-dir", "./data", "data directory")

	viper.BindPFlag("config", rootCmd.PersistentFlags().Lookup("config"))
	viper.BindPFlag("data_dir", rootCmd.PersistentFlags().Lookup("data-dir"))
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
