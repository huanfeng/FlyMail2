package config

import (
	"flymail-core/provider"
	"log"
	"strings"
)

// Provider is a type alias so existing callers keep working with the same field names.
type Provider = provider.ProviderConfig

// ServerEndpoint is a type alias for the core type.
type ServerEndpoint = provider.ServerEndpoint

// LoadProviderConfig loads provider presets from a JSON file.
// Falls back to the embedded defaults on error (handled by flymail-core/provider init).
func LoadProviderConfig(path string) {
	if err := provider.LoadProviders(path); err != nil {
		log.Printf("[warn] %v, using built-in defaults", err)
		// embedded defaults are already loaded by provider.init()
	}
}

// Providers returns all known provider presets.
func Providers() []Provider {
	return provider.Providers()
}

// GetProvider looks up a provider by ID.
func GetProvider(id string) *Provider {
	return provider.GetProviderByID(id)
}

// GuessProviderByDomain extracts the domain from an email address and looks up
// the matching provider preset.
func GuessProviderByDomain(email string) *Provider {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return nil
	}
	p, ok := provider.GetProvider(parts[1])
	if !ok {
		return nil
	}
	return p
}

// LookupFolderType delegates to the core provider package.
func LookupFolderType(providerID, folderName, folderPath string) string {
	return provider.LookupFolderType(providerID, folderName, folderPath)
}
