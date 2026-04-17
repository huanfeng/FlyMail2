package dispatcher

import (
	"mail2im/internal/core"
	"regexp"
)

type StrategyConfig struct {
	QuietEnabled    bool
	QuietHoursStart string // "22:00"
	QuietHoursEnd   string // "08:00"
	BlockPatterns   []string
}

type StrategyEngine struct {
	Config StrategyConfig
}

func NewStrategyEngine(config StrategyConfig) *StrategyEngine {
	return &StrategyEngine{Config: config}
}

func (s *StrategyEngine) ShouldSend(event core.Event) bool {
	// Note: mail type filtering (spam/trash/draft/sent) is now handled by
	// MailType.Action in the dispatcher, not here.

	// 1. Critical events always pass
	if event.Priority == core.PriorityCritical {
		return true
	}

	// 2. Check Block Patterns
	if s.matchesBlockPattern(event) {
		return false
	}

	return true
}

func (s *StrategyEngine) matchesBlockPattern(event core.Event) bool {
	for _, pattern := range s.Config.BlockPatterns {
		matched, err := regexp.MatchString(pattern, event.Source)
		if err == nil && matched {
			return true
		}

		// Check subject if available
		if payload, ok := event.Payload.(map[string]any); ok {
			if subject, ok := payload["subject"].(string); ok {
				matched, err := regexp.MatchString(pattern, subject)
				if err == nil && matched {
					return true
				}
			}
		}
	}
	return false
}
