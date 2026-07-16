package schedmessages

import (
	"encoding/json"
	"errors"
	"time"

	cc "github.com/dia-bot/dia/internal/features/customcommands"
)

// MessageSpec is the composed message a schedule posts, mirroring the
// dashboard's shared message editor output (web/src/lib/schedules.ts).
// Every string renders as a Go template against the schedule scope.
type MessageSpec struct {
	Content    string            `json:"content,omitempty"`
	Embeds     []cc.EmbedSpec    `json:"embeds,omitempty"`
	Components []cc.ComponentRow `json:"components,omitempty"`
	// ButtonActions maps a composed button's custom_id_suffix to the saved
	// automation its click runs (sched:act:<id>:<suffix> routes it here).
	ButtonActions map[string]string `json:"button_actions,omitempty"`
}

// Empty reports whether the spec has nothing to send.
func (m MessageSpec) Empty() bool {
	return m.Content == "" && len(m.Embeds) == 0 && len(m.Components) == 0
}

// DecodeSpec parses a schedule's spec column (broken/empty JSON yields zero).
func DecodeSpec(raw json.RawMessage) MessageSpec {
	var m MessageSpec
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	return m
}

// ScheduleDef is the cadence stored on scheduled_messages.schedule. All times
// are UTC; the dashboard mirrors this shape exactly.
type ScheduleDef struct {
	Kind string `json:"kind"` // once | every | daily | weekly
	// At is the RFC 3339 send time for kind=once.
	At string `json:"at,omitempty"`
	// EveryMinutes is the interval for kind=every (min 5).
	EveryMinutes int `json:"every_minutes,omitempty"`
	// Time is the "HH:MM" UTC send time for daily/weekly.
	Time string `json:"time,omitempty"`
	// Weekdays are the send days for kind=weekly (0=Sunday … 6=Saturday).
	Weekdays []int `json:"weekdays,omitempty"`
}

// DecodeSchedule parses a schedule column (broken/empty JSON yields zero).
func DecodeSchedule(raw json.RawMessage) ScheduleDef {
	var d ScheduleDef
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &d)
	}
	return d
}

// minEvery floors interval schedules so a typo can't spam a channel.
const minEvery = 5 * time.Minute

// Validate rejects definitions NextRun couldn't schedule.
func (d ScheduleDef) Validate() error {
	switch d.Kind {
	case "once":
		if _, err := time.Parse(time.RFC3339, d.At); err != nil {
			return errors.New("pick a valid date and time")
		}
	case "every":
		if time.Duration(d.EveryMinutes)*time.Minute < minEvery {
			return errors.New("the interval must be at least 5 minutes")
		}
	case "daily", "weekly":
		if _, err := time.Parse("15:04", d.Time); err != nil {
			return errors.New("pick a valid time of day (HH:MM, UTC)")
		}
		if d.Kind == "weekly" {
			if len(d.Weekdays) == 0 {
				return errors.New("pick at least one weekday")
			}
			for _, w := range d.Weekdays {
				if w < 0 || w > 6 {
					return errors.New("invalid weekday")
				}
			}
		}
	default:
		return errors.New("unknown schedule kind")
	}
	return nil
}

// NextRun computes the next send strictly after `after` (UTC). ok=false means
// the schedule is finished (a one-off in the past).
func (d ScheduleDef) NextRun(after time.Time) (time.Time, bool) {
	after = after.UTC()
	switch d.Kind {
	case "once":
		at, err := time.Parse(time.RFC3339, d.At)
		if err != nil || !at.After(after) {
			return time.Time{}, false
		}
		return at.UTC(), true
	case "every":
		step := time.Duration(d.EveryMinutes) * time.Minute
		if step < minEvery {
			step = minEvery
		}
		return after.Add(step), true
	case "daily", "weekly":
		tod, err := time.Parse("15:04", d.Time)
		if err != nil {
			return time.Time{}, false
		}
		days := d.Weekdays
		if d.Kind == "daily" {
			days = []int{0, 1, 2, 3, 4, 5, 6}
		}
		allowed := map[int]bool{}
		for _, w := range days {
			allowed[w] = true
		}
		// Walk day by day from today until a candidate lands after `after`
		// (worst case 8 iterations).
		for i := 0; i < 8; i++ {
			day := after.AddDate(0, 0, i)
			cand := time.Date(day.Year(), day.Month(), day.Day(), tod.Hour(), tod.Minute(), 0, 0, time.UTC)
			if cand.After(after) && allowed[int(cand.Weekday())] {
				return cand, true
			}
		}
		return time.Time{}, false
	}
	return time.Time{}, false
}
