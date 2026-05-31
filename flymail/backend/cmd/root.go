package cmd

import "github.com/spf13/cobra"

var (
	dataDir    string
	configFile string
)

var rootCmd = &cobra.Command{
	Use:   "flymail",
	Short: "FlyMail 自托管邮箱客户端",
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dataDir, "data-dir", "", "数据目录（默认 ./data 或 FLYMAIL_DATA_DIR）")
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "配置文件路径")
}

func Execute() error { return rootCmd.Execute() }
