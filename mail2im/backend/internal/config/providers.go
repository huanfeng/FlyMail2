package config

import (
	_ "embed"
	"encoding/json"
	"log"
	"os"
	"strings"
)

type ServerEndpoint struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type Provider struct {
	ID             string                    `json:"id"`
	Name           string                    `json:"name"`
	Domains        []string                  `json:"domains"`
	Servers        map[string]ServerEndpoint `json:"servers"`          // ssl, starttls, none
	FolderMappings map[string]string         `json:"folder_mappings"` // folder name/path → mail type key
}

type ProviderConfig struct {
	Providers []Provider `json:"providers"`
}

var providerConfig ProviderConfig

//go:embed defaults/providers.json
var defaultProviderConfig []byte

func LoadProviderConfig(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[warn] unable to read provider config at %s: %v, using built-in defaults", path, err)
		loadDefaultProviderConfig()
		return
	}

	if err := json.Unmarshal(data, &providerConfig); err != nil {
		log.Printf("[warn] failed to parse provider config at %s: %v, using built-in defaults", path, err)
		loadDefaultProviderConfig()
		return
	}
}

func loadDefaultProviderConfig() {
	if len(defaultProviderConfig) == 0 {
		return
	}

	if err := json.Unmarshal(defaultProviderConfig, &providerConfig); err != nil {
		log.Printf("[warn] failed to load embedded provider config: %v", err)
	}
}

func Providers() []Provider {
	return providerConfig.Providers
}

func GetProvider(id string) *Provider {
	for _, p := range providerConfig.Providers {
		if p.ID == id {
			return &p
		}
	}
	return nil
}

func GuessProviderByDomain(email string) *Provider {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return nil
	}
	domain := parts[1]
	for _, p := range providerConfig.Providers {
		for _, d := range p.Domains {
			if strings.EqualFold(d, domain) {
				return &p
			}
		}
	}
	return nil
}

// LookupFolderType returns the mail type for a folder name/path using the
// provider's built-in folder mappings. It checks both the decoded name and
// the raw IMAP path. Returns empty string if no mapping is found.
func LookupFolderType(providerID, folderName, folderPath string) string {
	p := GetProvider(providerID)
	if p == nil || len(p.FolderMappings) == 0 {
		return ""
	}

	// Exact match on folder name
	if t, ok := p.FolderMappings[folderName]; ok {
		return t
	}
	// Exact match on raw path
	if t, ok := p.FolderMappings[folderPath]; ok {
		return t
	}
	// Case-insensitive match
	for k, v := range p.FolderMappings {
		if strings.EqualFold(k, folderName) || strings.EqualFold(k, folderPath) {
			return v
		}
	}
	return ""
}
