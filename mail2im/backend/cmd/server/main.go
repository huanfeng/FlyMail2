package main

import (
	"fmt"
	"io"
	"log"
	"mail2im/internal/api"
	appconfig "mail2im/internal/config"
	"mail2im/internal/core"
	"mail2im/internal/dispatcher"
	"mail2im/internal/dispatcher/channels"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/emersion/go-message"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"golang.org/x/text/encoding/htmlindex"

	"flymail-core/logger"
	"go.uber.org/zap"
)

var (
	AppVersion = "dev"
	GitCommit  = "unknown"
	BuildTime  = "unknown"
)

const AppName = "Mail2IM"

// customUsage prints the help message
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
	// Register CharsetReader for non-UTF-8 emails
	message.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		enc, err := htmlindex.Get(charset)
		if err != nil {
			return nil, err
		}
		return enc.NewDecoder().Reader(input), nil
	}

	// Define flags
	pflag.String("data-root", "./mail2im_data", "Data storage root directory")
	pflag.String("reset-password", "", "Reset default user password. Optional: provide a specific password.")
	// Define NoOptDefVal for reset-password to allow it to be used as a boolean flag (which sets it to a sentinel value)
	// BUT standard pflag doesn't support "optional argument" well in the way -flag [value].
	// It supports -flag (bool) or -flag value.
	// The trick for optional value is usually using Lookup and NoOptDefVal.
	// If we set NoOptDefVal, when user provides just `--reset-password`, it takes that value.
	// If user provides `--reset-password=123`, it takes 123.
	// We use a specific sentinel value to indicate "random generation".
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

	// Viper setup
	viper.SetEnvPrefix("MAIL2IM") // e.g. MAIL2IM_DATA_ROOT
	viper.AutomaticEnv()
	viper.BindPFlag("data_root", pflag.Lookup("data-root"))

	dataRoot := viper.GetString("data_root")
	if dataRoot == "" {
		dataRoot = os.Getenv("DATA_ROOT") // Fallback to old env var if needed
	}
	if dataRoot == "" {
		dataRoot = "./mail2im_data"
	}

	// Ensure data root directory exists
	if err := os.MkdirAll(dataRoot, 0755); err != nil {
		log.Fatalf("Failed to create data root directory %s: %v", dataRoot, err)
	}

	// Auth setup
	core.InitAuth()

	// Ensure config directory exists under data root
	configDir := filepath.Join(dataRoot, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		log.Fatalf("Failed to create config directory %s: %v", configDir, err)
	}

	// Initialize Database - use dataRoot
	dbPath := filepath.Join(dataRoot, "mail2im.db")
	core.InitDB(dbPath)

	// Handle Reset Password
	// Check if flag was explicitly set
	if pflag.Lookup("reset-password").Changed {
		val := pflag.Lookup("reset-password").Value.String()

		var targetPwd string
		if val == randomPasswordSentinel {
			// User used -reset-password without value (or with default sentinel)
			targetPwd = "" // Trigger random generation in core
		} else {
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

	// Provider defaults - use dataRoot
	providerConfigPath := filepath.Join(configDir, "providers.json")
	appconfig.LoadProviderConfig(providerConfigPath)

	// Initialize Core Services
	core.InitEventBus()
	core.InitDebugService()
	// Initialize Attachment Manager - use dataRoot
	attachmentPath := filepath.Join(dataRoot, "attachments")
	core.InitAttachmentManager(attachmentPath)
	core.InitJanitor(30) // Retention 30 days

	dispatcher.InitDispatcher()
	dispatcher.Instance.Register(channels.NewConsoleChannel(core.PriorityLow))

	// Start Watcher
	go core.StartWatcher() // Legacy, will be replaced

	// Setup Router
	r := gin.Default()

	// Config from Env
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// CORS configuration
	corsConfig := cors.DefaultConfig()
	if os.Getenv("ENV") == "production" {
		corsOrigins := os.Getenv("CORS_ORIGINS")
		if corsOrigins != "" {
			corsConfig.AllowOrigins = strings.Split(corsOrigins, ",")
		}
	} else {
		// Dev mode: Allow all origins to avoid port issues (5173 vs 5174 etc)
		corsConfig.AllowAllOrigins = true
	}

	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Origin", "Content-Type", "Authorization"}
	corsConfig.ExposeHeaders = []string{"Content-Length"}
	corsConfig.AllowCredentials = true
	corsConfig.MaxAge = 12 * time.Hour

	r.Use(cors.New(corsConfig))

	apiGroup := r.Group("/api")

	// Public Routes
	apiGroup.GET("/public/emails/:token", api.GetSharedEmail)

	authGroup := apiGroup.Group("/auth")
	authGroup.POST("/setup", api.SetupUser)
	authGroup.POST("/login", api.Login)
	authGroup.POST("/refresh", api.RefreshToken)

	authProtected := apiGroup.Group("/auth")
	authProtected.Use(api.AuthMiddleware())
	authProtected.GET("/me", api.GetMe)
	authProtected.PUT("/profile", api.UpdateProfile)

	protected := apiGroup.Group("/")
	protected.Use(api.AuthMiddleware())

	protected.GET("/config", api.GetSettings)
	protected.POST("/config", api.UpdateSettings)
	protected.POST("/config/export", api.ExportConfig)
	protected.POST("/config/import", api.ImportConfig)
	protected.GET("/logs", api.GetLogs)
	protected.DELETE("/logs/:id", api.DeleteLog)
	protected.DELETE("/logs", api.DeleteAllLogs)
	protected.GET("/debug/stats", api.GetDebugStats)
	protected.POST("/debug/test-event", api.TriggerTestEvent)

	protected.GET("/proxies", api.GetProxies)
	protected.POST("/proxies", api.CreateProxy)
	protected.PUT("/proxies/:id", api.UpdateProxy)
	protected.DELETE("/proxies/:id", api.DeleteProxy)

	protected.GET("/accounts", api.GetAccounts)
	protected.GET("/accounts/:id", api.GetAccount)
	protected.POST("/accounts", api.CreateAccount)
	protected.POST("/accounts/batch", api.CreateAccounts)
	protected.PUT("/accounts/:id", api.UpdateAccount)
	protected.POST("/accounts/test", api.TestAccountConnection)
	protected.DELETE("/accounts/:id", api.DeleteAccount)
	protected.GET("/providers", api.GetProviders)

	// Mailboxes
	protected.GET("/accounts/:id/mailboxes", api.GetMailboxes)
	protected.POST("/accounts/:id/mailboxes/sync", api.SyncMailboxes)
	protected.PUT("/mailboxes/:mailbox_id", api.UpdateMailbox)

	// Emails
	protected.GET("/emails", api.GetEmails)
	protected.GET("/emails/:id", api.GetEmail)
	protected.GET("/emails/:id/html", api.GetEmailHTML)
	protected.DELETE("/emails/:id", api.DeleteEmail)
	protected.POST("/emails/:id/share", api.GenerateShareLink)

	// Channels
	protected.GET("/channels", api.GetChannels)
	protected.POST("/channels", api.CreateChannel)
	protected.PUT("/channels/:id", api.UpdateChannel)
	protected.DELETE("/channels/:id", api.DeleteChannel)
	protected.POST("/channels/test", api.TestChannel)

	// Classification
	protected.GET("/mailtypes", api.GetMailTypes)
	protected.POST("/mailtypes", api.CreateMailType)
	protected.PUT("/mailtypes/:id", api.UpdateMailType)
	protected.DELETE("/mailtypes/:id", api.DeleteMailType)

	protected.GET("/rules", api.GetFolderRules)
	protected.POST("/rules", api.CreateFolderRule)
	protected.PUT("/rules/:id", api.UpdateFolderRule)
	protected.DELETE("/rules/:id", api.DeleteFolderRule)

	// Notification Policy
	protected.GET("/notification-policy", api.GetNotificationPolicy)
	protected.PUT("/notification-policy/:key", api.UpdateNotificationPolicy)

	// Templates
	protected.GET("/templates", api.GetTemplates)
	protected.POST("/templates", api.CreateTemplate)
	protected.PUT("/templates/:id", api.UpdateTemplate)
	protected.DELETE("/templates/:id", api.DeleteTemplate)
	protected.POST("/templates/preview", api.PreviewTemplate)
	protected.GET("/templates/variables", api.GetTemplateVariables)
	protected.GET("/templates/defaults", api.GetDefaultTemplates)

	logger.Info("Server starting", zap.String("port", port))
	if err := r.Run(":" + port); err != nil {
		logger.Fatal("Server failed to start", zap.Error(err))
	}
}
