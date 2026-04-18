package core

import (
	"mail2im/internal/models"
	"time"

	"flymail-core/logger"

	"go.uber.org/zap"
)

type Janitor struct {
	RetentionDays int
	stopChan      chan struct{}
}

var Cleaner *Janitor

func InitJanitor(retentionDays int) {
	Cleaner = &Janitor{
		RetentionDays: retentionDays,
		stopChan:      make(chan struct{}),
	}
	go Cleaner.Start()
}

func (j *Janitor) Start() {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	logger.Info("Janitor started")

	// Run once immediately
	j.RunCleanup()

	for {
		select {
		case <-ticker.C:
			j.RunCleanup()
		case <-j.stopChan:
			logger.Info("Janitor stopped")
			return
		}
	}
}

func (j *Janitor) Stop() {
	close(j.stopChan)
}

func (j *Janitor) RunCleanup() {
	logger.Info("Running cleanup task...")

	// 1. Cleanup Attachments
	if Attachments != nil {
		if err := Attachments.Cleanup(j.RetentionDays); err != nil {
			logger.Error("Failed to cleanup attachments", zap.Error(err))
		} else {
			logger.Info("Attachments cleanup completed")
		}
	}

	// 2. Cleanup Logs (Optional, if we had a method for it)
	// For now, we just log
	threshold := time.Now().AddDate(0, 0, -j.RetentionDays)
	if err := DB.Where("created_at < ?", threshold).Delete(&models.ForwardLog{}).Error; err != nil {
		logger.Error("Failed to cleanup logs", zap.Error(err))
	} else {
		logger.Info("Logs cleanup completed")
	}
}
