package config

import (
	"strings"
	"testing"
)

// No init() needed — flymail-core/provider.init() loads embedded defaults automatically.

func TestGetProvider_Gmail(t *testing.T) {
	p := GetProvider("gmail")
	if p == nil {
		t.Fatal("expected gmail provider to exist")
	}
	if p.Name != "Gmail" {
		t.Errorf("expected name 'Gmail', got %q", p.Name)
	}
}

func TestGetProvider_NotFound(t *testing.T) {
	p := GetProvider("nonexistent")
	if p != nil {
		t.Error("expected nil for nonexistent provider")
	}
}

func TestGuessProviderByDomain(t *testing.T) {
	tests := []struct {
		email   string
		wantID  string
		wantNil bool
	}{
		{"user@gmail.com", "gmail", false},
		{"user@outlook.com", "outlook", false},
		{"user@163.com", "163", false},
		{"user@unknown-domain.com", "", true},
		{"invalid-email", "", true},
	}

	for _, tt := range tests {
		p := GuessProviderByDomain(tt.email)
		if tt.wantNil {
			if p != nil {
				t.Errorf("GuessProviderByDomain(%q) = %v, want nil", tt.email, p.ID)
			}
			continue
		}
		if p == nil {
			t.Errorf("GuessProviderByDomain(%q) = nil, want %q", tt.email, tt.wantID)
			continue
		}
		if p.ID != tt.wantID {
			t.Errorf("GuessProviderByDomain(%q).ID = %q, want %q", tt.email, p.ID, tt.wantID)
		}
	}
}

func TestLookupFolderType_GmailMappings(t *testing.T) {
	tests := []struct {
		name       string
		folderName string
		folderPath string
		want       string
	}{
		{"INBOX", "INBOX", "INBOX", "primary"},
		{"Spam folder", "[Gmail]/Spam", "[Gmail]/Spam", "spam"},
		{"Sent Mail", "[Gmail]/Sent Mail", "[Gmail]/Sent Mail", "sent"},
		{"Trash", "[Gmail]/Trash", "[Gmail]/Trash", "trash"},
		{"Drafts", "[Gmail]/Drafts", "[Gmail]/Drafts", "draft"},
		{"Important", "[Gmail]/Important", "[Gmail]/Important", "important"},
		{"Unknown folder", "CustomFolder", "CustomFolder", ""},
	}

	for _, tt := range tests {
		got := LookupFolderType("gmail", tt.folderName, tt.folderPath)
		if got != tt.want {
			t.Errorf("LookupFolderType(gmail, %q, %q) = %q, want %q",
				tt.folderName, tt.folderPath, got, tt.want)
		}
	}
}

func TestLookupFolderType_CaseInsensitive(t *testing.T) {
	// "INBOX" should match even with different case
	got := LookupFolderType("gmail", "inbox", "inbox")
	if got != "primary" {
		t.Errorf("case-insensitive lookup: got %q, want %q", got, "primary")
	}
}

func TestLookupFolderType_UnknownProvider(t *testing.T) {
	got := LookupFolderType("nonexistent", "INBOX", "INBOX")
	if got != "" {
		t.Errorf("unknown provider should return empty, got %q", got)
	}
}

func TestLookupFolderType_EmptyProvider(t *testing.T) {
	got := LookupFolderType("", "INBOX", "INBOX")
	if got != "" {
		t.Errorf("empty provider should return empty, got %q", got)
	}
}

func TestLookupFolderType_PathMatchFallback(t *testing.T) {
	// Test that folder path is also checked when name doesn't match
	got := LookupFolderType("gmail", "SomeDecodedName", "[Gmail]/Spam")
	if got != "spam" {
		t.Errorf("path fallback: got %q, want %q", got, "spam")
	}
}

func TestProviders_AllHaveFolderMappings(t *testing.T) {
	providers := Providers()
	if len(providers) == 0 {
		t.Fatal("expected at least one provider")
	}

	for _, p := range providers {
		if len(p.FolderMappings) == 0 {
			t.Errorf("provider %q has no folder mappings", p.ID)
		}
		// Every provider should map some form of Inbox to "primary"
		hasInbox := false
		for k, v := range p.FolderMappings {
			if strings.EqualFold(k, "INBOX") && v == "primary" {
				hasInbox = true
				break
			}
		}
		if !hasInbox {
			t.Errorf("provider %q missing Inbox→primary mapping", p.ID)
		}
	}
}
