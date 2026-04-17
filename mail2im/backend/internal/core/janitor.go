package core

import (
	"log"
	"mail2im/internal/models"
	"time"
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

	log.Println("Janitor started")

	// Run once immediately
	j.RunCleanup()

	for {
		select {
		case <-ticker.C:
			j.RunCleanup()
		case <-j.stopChan:
			log.Println("Janitor stopped")
			return
		}
	}
}

func (j *Janitor) Stop() {
	close(j.stopChan)
}

func (j *Janitor) RunCleanup() {
	log.Println("Running cleanup task...")

	// 1. Cleanup Attachments
	if Attachments != nil {
		if err := Attachments.Cleanup(j.RetentionDays); err != nil {
			log.Printf("Failed to cleanup attachments: %v", err)
		} else {
			log.Println("Attachments cleanup completed")
		}
	}

	// 2. Cleanup Logs (Optional, if we had a method for it)
	// For now, we just log
	threshold := time.Now().AddDate(0, 0, -j.RetentionDays)
	if err := DB.Where("created_at < ?", threshold).Delete(&models.ForwardLog{}).Error; err != nil {
		log.Printf("Failed to cleanup logs: %v", err)
	} else {
		log.Println("Logs cleanup completed")
	}
}
