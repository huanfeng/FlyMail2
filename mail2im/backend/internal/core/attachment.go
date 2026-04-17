package core

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

type AttachmentManager struct {
	StoragePath string
	mu          sync.RWMutex
}

var Attachments *AttachmentManager

func InitAttachmentManager(storagePath string) {
	if err := os.MkdirAll(storagePath, 0755); err != nil {
		panic(fmt.Sprintf("Failed to create attachment storage: %v", err))
	}
	Attachments = &AttachmentManager{
		StoragePath: storagePath,
	}
}

func (m *AttachmentManager) Save(filename string, content io.Reader) (string, error) {
	// Generate unique ID
	id := uuid.New().String()
	ext := filepath.Ext(filename)
	storedFilename := fmt.Sprintf("%s%s", id, ext)
	path := filepath.Join(m.StoragePath, storedFilename)

	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, content); err != nil {
		return "", err
	}

	return storedFilename, nil
}

func (m *AttachmentManager) GetPath(filename string) string {
	return filepath.Join(m.StoragePath, filename)
}

// Cleanup removes files older than retention period
func (m *AttachmentManager) Cleanup(retentionDays int) error {
	entries, err := os.ReadDir(m.StoragePath)
	if err != nil {
		return err
	}

	threshold := time.Now().AddDate(0, 0, -retentionDays)

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(threshold) {
			os.Remove(filepath.Join(m.StoragePath, entry.Name()))
		}
	}
	return nil
}
