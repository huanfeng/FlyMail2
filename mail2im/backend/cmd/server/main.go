package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/pflag"

	"mail2im/internal/app"
	appconfig "mail2im/internal/config"
	"mail2im/internal/core"
)

var (
	AppVersion = "dev"
	GitCommit  = "unknown"
	BuildTime  = "unknown"
)

const AppName = "Mail2IM"

func customUsage() {
	fmt.Printf("%s %s\n", AppName, AppVersion)
	fmt.Printf("Commit: %s\n", GitCommit)
	fmt.Printf("Build Time: %s\n", BuildTime)
	fmt.Println("\nUsage:")
	fmt.Println("  mail2im [flags]")
	fmt.Println("\nFlags:")
	pflag.PrintDefaults()
	fmt.Println()
}

func main() {
	// Define flags
	pflag.String("data-root", "./mail2im_data", "Data storage root directory")
	pflag.String("reset-password", "", "Reset default user password")
	const randomPasswordSentinel = "__RANDOM__"
	pflag.Lookup("reset-password").NoOptDefVal = randomPasswordSentinel

	showVersion := pflag.Bool("version", false, "Print version information")
	showHelp := pflag.BoolP("help", "h", false, "Print this help message")

	pflag.Usage = customUsage
	pflag.Parse()

	if *showHelp {
		customUsage()
		return
	}

	if *showVersion {
		fmt.Printf("%s %s\n", AppName, AppVersion)
		fmt.Printf("Commit: %s\n", GitCommit)
		fmt.Printf("Build Time: %s\n", BuildTime)
		return
	}

	// Load config
	if dataRoot := pflag.Lookup("data-root").Value.String(); dataRoot != "./mail2im_data" {
		os.Setenv("MAIL2IM_DATA_ROOT", dataRoot)
	}

	appCfg, err := appconfig.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Handle --reset-password (needs DB init first)
	if pflag.Lookup("reset-password").Changed {
		core.InitCrypto(appCfg.AppSecret)
		core.InitDB(appCfg.DataRoot + "/mail2im.db")
		val := pflag.Lookup("reset-password").Value.String()

		var targetPwd string
		if val != randomPasswordSentinel {
			targetPwd = val
		}

		user, pwd, err := core.ResetDefaultUserPassword(targetPwd)
		if err != nil {
			log.Fatalf("Failed to reset password: %v", err)
		}
		fmt.Println("========================================")
		fmt.Println("Password reset successfully")
		fmt.Printf("Username: %s\n", user.Username)
		if targetPwd == "" {
			fmt.Printf("Password: %s\n", pwd)
		} else {
			fmt.Printf("Password: %s (set manually)\n", pwd)
		}
		fmt.Println("========================================")
		return
	}

	// Build server config
	cfg := app.ServerConfig{
		Port:     appCfg.Port,
		DataRoot: appCfg.DataRoot,
	}

	// Start server
	server := app.NewServer(cfg)
	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	server.Stop()
}
