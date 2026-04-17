package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// LoadOptions defines options for loading configuration
type LoadOptions struct {
	EnvPrefix  string         // environment variable prefix
	ConfigPath string         // path to config file
	Defaults   map[string]any // default values
}

// LoadConfig loads configuration from file and environment into the target struct.
// If the config file does not exist, it creates one with the provided defaults.
func LoadConfig(opts LoadOptions, target any) error {
	v := viper.New()

	// Set defaults
	for key, val := range opts.Defaults {
		v.SetDefault(key, val)
	}

	// Set environment variable prefix and auto-read
	if opts.EnvPrefix != "" {
		v.SetEnvPrefix(opts.EnvPrefix)
		v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
		v.AutomaticEnv()
	}

	// Set config file path
	configPath := opts.ConfigPath
	if configPath == "" {
		configPath = "./config.yaml"
	}
	v.SetConfigFile(configPath)

	// Try to read config file
	if err := v.ReadInConfig(); err != nil {
		var pathError *fs.PathError
		if errors.As(err, &pathError) && errors.Is(pathError.Err, fs.ErrNotExist) {
			// Config file does not exist, create it with defaults
			if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
				return fmt.Errorf("failed to create config directory: %w", err)
			}

			if err := v.SafeWriteConfigAs(configPath); err != nil {
				return fmt.Errorf("failed to create default config: %w", err)
			}

			fmt.Printf("Created default config file at %s\n", configPath)
		} else {
			return fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Unmarshal into target struct
	if err := v.Unmarshal(target); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return nil
}

// GenerateRandomSecret generates a random 64-character hex string suitable for JWT secrets
func GenerateRandomSecret() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "default-insecure-secret-replace-me-immediately"
	}
	return hex.EncodeToString(bytes)
}
