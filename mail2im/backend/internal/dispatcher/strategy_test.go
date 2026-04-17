package dispatcher

import (
	"mail2im/internal/core"
	"testing"
)

func TestShouldSend_CriticalAlwaysPasses(t *testing.T) {
	engine := NewStrategyEngine(StrategyConfig{
		BlockPatterns: []string{".*blocked.*"},
	})

	event := core.Event{
		Type:     core.EventEmailReceived,
		Priority: core.PriorityCritical,
		Source:   "blocked-source",
	}

	if !engine.ShouldSend(event) {
		t.Error("critical events should always pass, even with matching block pattern")
	}
}

func TestShouldSend_NormalEventPasses(t *testing.T) {
	engine := NewStrategyEngine(StrategyConfig{})

	event := core.Event{
		Type:     core.EventEmailReceived,
		Priority: core.PriorityNormal,
		Source:   "test",
	}

	if !engine.ShouldSend(event) {
		t.Error("normal event with no block patterns should pass")
	}
}

func TestShouldSend_BlockPatternMatchesSource(t *testing.T) {
	engine := NewStrategyEngine(StrategyConfig{
		BlockPatterns: []string{"spam-source"},
	})

	event := core.Event{
		Type:     core.EventEmailReceived,
		Priority: core.PriorityNormal,
		Source:   "spam-source",
	}

	if engine.ShouldSend(event) {
		t.Error("event matching block pattern on source should be blocked")
	}
}

func TestShouldSend_BlockPatternMatchesSubject(t *testing.T) {
	engine := NewStrategyEngine(StrategyConfig{
		BlockPatterns: []string{"(?i)unsubscribe"},
	})

	event := core.Event{
		Type:     core.EventEmailReceived,
		Priority: core.PriorityNormal,
		Source:   "legit-source",
		Payload: map[string]any{
			"subject": "Please Unsubscribe Me",
		},
	}

	if engine.ShouldSend(event) {
		t.Error("event matching block pattern on subject should be blocked")
	}
}

func TestShouldSend_BlockPatternNoMatch(t *testing.T) {
	engine := NewStrategyEngine(StrategyConfig{
		BlockPatterns: []string{"spam", "ads"},
	})

	event := core.Event{
		Type:     core.EventEmailReceived,
		Priority: core.PriorityNormal,
		Source:   "normal-source",
		Payload: map[string]any{
			"subject": "Hello World",
		},
	}

	if !engine.ShouldSend(event) {
		t.Error("event not matching any block pattern should pass")
	}
}

func TestShouldSend_MultipleBlockPatterns(t *testing.T) {
	engine := NewStrategyEngine(StrategyConfig{
		BlockPatterns: []string{"pattern1", "pattern2", "pattern3"},
	})

	tests := []struct {
		name   string
		source string
		want   bool
	}{
		{"matches first", "pattern1", false},
		{"matches second", "pattern2", false},
		{"matches third", "pattern3", false},
		{"matches none", "clean-source", true},
	}

	for _, tt := range tests {
		event := core.Event{
			Type:     core.EventEmailReceived,
			Priority: core.PriorityNormal,
			Source:   tt.source,
		}
		if got := engine.ShouldSend(event); got != tt.want {
			t.Errorf("%s: ShouldSend() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestShouldSend_InvalidRegexIgnored(t *testing.T) {
	engine := NewStrategyEngine(StrategyConfig{
		BlockPatterns: []string{"[invalid-regex"},
	})

	event := core.Event{
		Type:     core.EventEmailReceived,
		Priority: core.PriorityNormal,
		Source:   "test",
	}

	// Invalid regex should not block (regexp.MatchString returns error)
	if !engine.ShouldSend(event) {
		t.Error("invalid regex pattern should not block events")
	}
}

func TestShouldSend_PayloadWithoutSubject(t *testing.T) {
	engine := NewStrategyEngine(StrategyConfig{
		BlockPatterns: []string{"block-me"},
	})

	event := core.Event{
		Type:     core.EventEmailReceived,
		Priority: core.PriorityNormal,
		Source:   "clean",
		Payload:  map[string]any{"other_field": "block-me"},
	}

	// Should not block because "block-me" is not in source or subject
	if !engine.ShouldSend(event) {
		t.Error("block pattern should only check source and subject, not other payload fields")
	}
}
