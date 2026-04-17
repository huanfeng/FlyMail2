package provider

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

// ServerEndpoint holds the host and port for a mail server connection.
type ServerEndpoint struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// ProviderConfig describes a mail provider's IMAP/SMTP server presets,
// supported domains, and folder-to-type mappings.
type ProviderConfig struct {
	ID             string                    `json:"id"`
	Name           string                    `json:"name"`
	Domains        []string                  `json:"domains"`
	Servers        map[string]ServerEndpoint `json:"servers"`          // keys: ssl, starttls, none
	FolderMappings map[string]string         `json:"folder_mappings"` // folder name/path → mail type
}

// providerList is the top-level JSON wrapper.
type providerList struct {
	Providers []ProviderConfig `json:"providers"`
}

//go:embed providers.json
var defaultData []byte

var (
	mu        sync.RWMutex
	providers []ProviderConfig
	domainIdx map[string]*ProviderConfig // domain → provider (lazy built)
)

func init() {
	_ = LoadProvidersFromEmbed()
}

// LoadProvidersFromEmbed loads the built-in embedded provider data.
func LoadProvidersFromEmbed() error {
	return loadFromBytes(defaultData)
}

// LoadProviders reads provider presets from a JSON file at the given path.
// If the file cannot be read or parsed, an error is returned and the
// existing in-memory data is left unchanged.
func LoadProviders(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("provider: read %s: %w", path, err)
	}
	return loadFromBytes(data)
}

func loadFromBytes(data []byte) error {
	var list providerList
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("provider: parse json: %w", err)
	}

	idx := make(map[string]*ProviderConfig, len(list.Providers)*2)
	for i := range list.Providers {
		p := &list.Providers[i]
		for _, d := range p.Domains {
			idx[strings.ToLower(d)] = p
		}
	}

	mu.Lock()
	providers = list.Providers
	domainIdx = idx
	mu.Unlock()
	return nil
}

// GetProvider looks up a provider preset by email domain (e.g. "gmail.com").
// The lookup is case-insensitive.
func GetProvider(domain string) (*ProviderConfig, bool) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := domainIdx[strings.ToLower(domain)]
	return p, ok
}

// GetProviderByID returns the provider with the given ID, or nil.
func GetProviderByID(id string) *ProviderConfig {
	mu.RLock()
	defer mu.RUnlock()
	for i := range providers {
		if providers[i].ID == id {
			return &providers[i]
		}
	}
	return nil
}

// Providers returns a copy of the current provider list.
func Providers() []ProviderConfig {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]ProviderConfig, len(providers))
	copy(out, providers)
	return out
}

// LookupFolderType returns the mail type for a folder name/path using the
// provider's folder mappings. Returns empty string if no mapping is found.
func LookupFolderType(providerID, folderName, folderPath string) string {
	p := GetProviderByID(providerID)
	if p == nil || len(p.FolderMappings) == 0 {
		return ""
	}

	if t, ok := p.FolderMappings[folderName]; ok {
		return t
	}
	if t, ok := p.FolderMappings[folderPath]; ok {
		return t
	}
	for k, v := range p.FolderMappings {
		if strings.EqualFold(k, folderName) || strings.EqualFold(k, folderPath) {
			return v
		}
	}
	return ""
}
