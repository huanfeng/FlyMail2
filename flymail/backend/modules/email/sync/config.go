package sync

import "time"

// Config represents the configuration for email monitoring
type Config struct {
	// EnableIDLE enables IMAP IDLE support if server supports it
	EnableIDLE bool `json:"enable_idle"`
	// PollInterval is the default interval for polling when IDLE is not available
	PollInterval time.Duration `json:"poll_interval"`
	// DayTimeStart is the start hour of daytime (0-23)
	DayTimeStart int `json:"day_time_start"`
	// DayTimeEnd is the end hour of daytime (0-23)
	DayTimeEnd int `json:"day_time_end"`
	// DayTimePollInterval is the polling interval during daytime
	DayTimePollInterval time.Duration `json:"day_time_poll_interval"`
	// NightTimePollInterval is the polling interval during nighttime
	NightTimePollInterval time.Duration `json:"night_time_poll_interval"`
	// RetryInterval is the interval to retry after an error
	RetryInterval time.Duration `json:"retry_interval"`
	// MaxRetries is the maximum number of retries before giving up
	MaxRetries int `json:"max_retries"`
}

// DefaultConfig returns the default monitor configuration
func DefaultConfig() *Config {
	return &Config{
		EnableIDLE:            true,
		PollInterval:          5 * time.Minute,
		DayTimeStart:          8,  // 8 AM
		DayTimeEnd:            22, // 10 PM
		DayTimePollInterval:   1 * time.Minute,
		NightTimePollInterval: 10 * time.Minute,
		RetryInterval:         30 * time.Second,
		MaxRetries:            3,
	}
}

// GetCurrentPollInterval returns the polling interval based on current time
func (c *Config) GetCurrentPollInterval() time.Duration {
	hour := time.Now().Hour()
	if hour >= c.DayTimeStart && hour < c.DayTimeEnd {
		return c.DayTimePollInterval
	}
	return c.NightTimePollInterval
}
