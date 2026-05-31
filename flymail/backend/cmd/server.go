package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"flymail/internal/app"
	"flymail/internal/config"

	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "启动 FlyMail 服务",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(config.LoadOptions{DataDir: dataDir, ConfigFile: configFile})
		if err != nil {
			return err
		}
		if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
			return err
		}
		a, err := app.New(cfg)
		if err != nil {
			return err
		}
		addr, err := a.Start("")
		if err != nil {
			return err
		}
		fmt.Printf("FlyMail 已启动：http://%s\n", addr)

		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		<-stop
		fmt.Println("正在关闭…")
		return a.Shutdown()
	},
}

func init() { rootCmd.AddCommand(serverCmd) }
