package core

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type TimeWindow struct {
	Enabled bool
	Start   string // HH:MM
	End     string // HH:MM
}

func parseBoolVal(val, defaultVal string) bool {
	if strings.TrimSpace(val) == "" {
		val = defaultVal
	}
	b, err := strconv.ParseBool(strings.TrimSpace(val))
	return err == nil && b
}

func ParseBoolSetting(key string, defaultVal bool) bool {
	v := GetSystemSettingWithDefault(key, strconv.FormatBool(defaultVal))
	return parseBoolVal(v, strconv.FormatBool(defaultVal))
}

// GetSystemLocation returns the system timezone configured in settings, defaulting to UTC.
func GetSystemLocation() *time.Location {
	tz := GetSystemSettingWithDefault("timezone", "UTC")
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC
	}
	return loc
}

func NowInSystemTZ() time.Time {
	return time.Now().In(GetSystemLocation())
}

// IsInWindow checks if now is within the HH:MM window (can cross midnight).
func IsInWindow(now time.Time, window TimeWindow, loc *time.Location) bool {
	if !window.Enabled {
		return false
	}
	if loc == nil {
		loc = time.UTC
	}

	startParts := strings.Split(window.Start, ":")
	endParts := strings.Split(window.End, ":")
	if len(startParts) != 2 || len(endParts) != 2 {
		return false
	}

	sh, _ := strconv.Atoi(startParts[0])
	sm, _ := strconv.Atoi(startParts[1])
	eh, _ := strconv.Atoi(endParts[0])
	em, _ := strconv.Atoi(endParts[1])

	year, month, day := now.In(loc).Date()
	start := time.Date(year, month, day, sh, sm, 0, 0, loc)
	end := time.Date(year, month, day, eh, em, 0, 0, loc)

	if window.Start == window.End {
		return false
	}

	if end.After(start) {
		return now.In(loc).After(start) && now.In(loc).Before(end)
	}
	// Cross-midnight
	return now.In(loc).After(start) || now.In(loc).Before(end)
}

func ListTimezones() []string {
	paths := []string{
		"/usr/share/zoneinfo/zone1970.tab",
		"/usr/share/zoneinfo/zone.tab",
	}

	seen := make(map[string]struct{})
	var zones []string

	for _, p := range paths {
		f, err := os.Open(filepath.Clean(p))
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 3 {
				continue
			}
			rawZones := strings.Fields(parts[2])
			for _, zone := range rawZones {
				if _, ok := seen[zone]; !ok {
					seen[zone] = struct{}{}
					zones = append(zones, zone)
				}
			}
		}
		f.Close()
		if len(zones) > 0 {
			break
		}
	}

	if len(zones) == 0 {
		return []string{"UTC"}
	}

	sort.Strings(zones)
	return zones
}

func GetNightWindow() TimeWindow {
	return TimeWindow{
		Enabled: ParseBoolSetting("night_enabled", false),
		Start:   GetSystemSettingWithDefault("night_start", ""),
		End:     GetSystemSettingWithDefault("night_end", ""),
	}
}

func GetGlobalQuietWindow() TimeWindow {
	return TimeWindow{
		Enabled: ParseBoolSetting("quiet_enabled", false),
		Start:   GetSystemSettingWithDefault("quiet_start", ""),
		End:     GetSystemSettingWithDefault("quiet_end", ""),
	}
}
