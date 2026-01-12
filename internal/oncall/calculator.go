package oncall

import (
	"fmt"
	"sort"
	"time"
)

// Calculator computes on-call assignments
type Calculator struct {
	store *Store
}

// NewCalculator creates a new on-call calculator
func NewCalculator(store *Store) *Calculator {
	return &Calculator{store: store}
}

// GetCurrentOnCall returns who is currently on-call for a schedule
func (c *Calculator) GetCurrentOnCall(scheduleID string) (*OnCallEntry, error) {
	sched, err := c.store.GetSchedule(scheduleID)
	if err != nil {
		return nil, err
	}

	return c.getOnCallAt(sched, time.Now())
}

// GetOnCallAt returns who is on-call at a specific time
func (c *Calculator) GetOnCallAt(scheduleID string, at time.Time) (*OnCallEntry, error) {
	sched, err := c.store.GetSchedule(scheduleID)
	if err != nil {
		return nil, err
	}

	return c.getOnCallAt(sched, at)
}

// GetCalendar returns on-call entries for a time range
func (c *Calculator) GetCalendar(scheduleID string, start, end time.Time) ([]OnCallEntry, error) {
	sched, err := c.store.GetSchedule(scheduleID)
	if err != nil {
		return nil, err
	}

	return c.generateCalendar(sched, start, end)
}

// getOnCallAt computes who is on-call at a specific time
func (c *Calculator) getOnCallAt(sched *Schedule, at time.Time) (*OnCallEntry, error) {
	// Load timezone
	loc, err := time.LoadLocation(sched.Timezone)
	if err != nil {
		loc = time.UTC
	}
	at = at.In(loc)

	// Check for active override first (highest priority)
	for _, override := range sched.Overrides {
		if at.After(override.StartTime) && at.Before(override.EndTime) {
			return &OnCallEntry{
				User: User{
					ID:   override.UserID,
					Name: override.UserName,
				},
				StartTime:  override.StartTime,
				EndTime:    override.EndTime,
				IsOverride: true,
			}, nil
		}
	}

	// Sort layers by priority (highest first)
	layers := make([]Layer, len(sched.Layers))
	copy(layers, sched.Layers)
	sort.Slice(layers, func(i, j int) bool {
		return layers[i].Priority > layers[j].Priority
	})

	// Find the first active layer
	for _, layer := range layers {
		if !c.isLayerActive(layer, at, loc) {
			continue
		}

		user, shiftStart, shiftEnd := c.getUserOnCall(layer, at, loc)
		if user != nil {
			return &OnCallEntry{
				User:      *user,
				StartTime: shiftStart,
				EndTime:   shiftEnd,
				LayerID:   layer.ID,
				LayerName: layer.Name,
			}, nil
		}
	}

	return nil, nil
}

// isLayerActive checks if a layer is active at the given time based on restrictions
func (c *Calculator) isLayerActive(layer Layer, at time.Time, loc *time.Location) bool {
	// Check if within layer date range
	if at.Before(layer.StartDate) {
		return false
	}
	if layer.EndDate != nil && at.After(*layer.EndDate) {
		return false
	}

	// Check restrictions
	if len(layer.Restrictions) == 0 {
		return true
	}

	for _, r := range layer.Restrictions {
		if c.matchesRestriction(r, at, loc) {
			return true
		}
	}

	return false
}

// matchesRestriction checks if a time matches a restriction
func (c *Calculator) matchesRestriction(r Restriction, at time.Time, loc *time.Location) bool {
	at = at.In(loc)

	startHour, startMin := parseTime(r.StartTime)
	endHour, endMin := parseTime(r.EndTime)

	currentMins := at.Hour()*60 + at.Minute()
	startMins := startHour*60 + startMin
	endMins := endHour*60 + endMin

	switch r.Type {
	case "daily":
		// Simple time range check
		if endMins > startMins {
			return currentMins >= startMins && currentMins < endMins
		}
		// Overnight restriction (e.g., 22:00-06:00)
		return currentMins >= startMins || currentMins < endMins

	case "weekly":
		weekday := int(at.Weekday())

		// Check if within day range
		if r.StartDay <= r.EndDay {
			if weekday < r.StartDay || weekday > r.EndDay {
				return false
			}
		} else {
			// Wraps around (e.g., Fri-Mon)
			if weekday < r.StartDay && weekday > r.EndDay {
				return false
			}
		}

		// Check time within the day
		if r.StartDay == r.EndDay {
			// Same day, simple time check
			if endMins > startMins {
				return currentMins >= startMins && currentMins < endMins
			}
			return currentMins >= startMins || currentMins < endMins
		}

		// Multi-day restriction
		if weekday == r.StartDay {
			return currentMins >= startMins
		}
		if weekday == r.EndDay {
			return currentMins < endMins
		}
		return true // Full day in between
	}

	return true
}

// getUserOnCall determines which user is on-call for a layer at a given time
func (c *Calculator) getUserOnCall(layer Layer, at time.Time, loc *time.Location) (*User, time.Time, time.Time) {
	if len(layer.Users) == 0 {
		return nil, time.Time{}, time.Time{}
	}

	at = at.In(loc)
	shiftDuration := time.Duration(layer.ShiftDuration)
	if shiftDuration == 0 {
		// Default shift durations based on rotation type
		switch layer.RotationType {
		case "daily":
			shiftDuration = 24 * time.Hour
		case "weekly":
			shiftDuration = 7 * 24 * time.Hour
		default:
			shiftDuration = 24 * time.Hour
		}
	}

	// Parse handoff time
	handoffHour, handoffMin := parseTime(layer.HandoffTime)

	// Calculate the reference point (first handoff)
	startDate := layer.StartDate.In(loc)

	// Find the first handoff time on or after start date
	firstHandoff := time.Date(
		startDate.Year(), startDate.Month(), startDate.Day(),
		handoffHour, handoffMin, 0, 0, loc,
	)

	// For weekly rotation, align to handoff day
	if layer.RotationType == "weekly" {
		for int(firstHandoff.Weekday()) != layer.HandoffDay {
			firstHandoff = firstHandoff.AddDate(0, 0, 1)
		}
	}

	// If first handoff is before start date, move to next occurrence
	for firstHandoff.Before(startDate) {
		firstHandoff = firstHandoff.Add(shiftDuration)
	}

	// Calculate how many complete shifts have passed since first handoff
	if at.Before(firstHandoff) {
		// Before the first handoff, use the first user
		return &layer.Users[0], firstHandoff.Add(-shiftDuration), firstHandoff
	}

	elapsed := at.Sub(firstHandoff)
	shiftIndex := int(elapsed / shiftDuration)
	userIndex := shiftIndex % len(layer.Users)

	shiftStart := firstHandoff.Add(time.Duration(shiftIndex) * shiftDuration)
	shiftEnd := shiftStart.Add(shiftDuration)

	return &layer.Users[userIndex], shiftStart, shiftEnd
}

// generateCalendar creates a calendar of on-call entries
func (c *Calculator) generateCalendar(sched *Schedule, start, end time.Time) ([]OnCallEntry, error) {
	loc, err := time.LoadLocation(sched.Timezone)
	if err != nil {
		loc = time.UTC
	}

	var entries []OnCallEntry

	// First, add all overrides in the range
	for _, override := range sched.Overrides {
		if override.EndTime.Before(start) || override.StartTime.After(end) {
			continue
		}

		entryStart := override.StartTime
		if entryStart.Before(start) {
			entryStart = start
		}
		entryEnd := override.EndTime
		if entryEnd.After(end) {
			entryEnd = end
		}

		entries = append(entries, OnCallEntry{
			User: User{
				ID:   override.UserID,
				Name: override.UserName,
			},
			StartTime:  entryStart,
			EndTime:    entryEnd,
			IsOverride: true,
		})
	}

	// Sort layers by priority
	layers := make([]Layer, len(sched.Layers))
	copy(layers, sched.Layers)
	sort.Slice(layers, func(i, j int) bool {
		return layers[i].Priority > layers[j].Priority
	})

	// Generate entries for each layer
	for _, layer := range layers {
		layerEntries := c.generateLayerCalendar(layer, start, end, loc)
		entries = append(entries, layerEntries...)
	}

	// Sort by start time
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].StartTime.Before(entries[j].StartTime)
	})

	// Merge overlapping entries (higher priority/override wins)
	return c.mergeEntries(entries, sched.Overrides), nil
}

// generateLayerCalendar generates calendar entries for a single layer
func (c *Calculator) generateLayerCalendar(layer Layer, start, end time.Time, loc *time.Location) []OnCallEntry {
	if len(layer.Users) == 0 {
		return nil
	}

	var entries []OnCallEntry
	current := start

	for current.Before(end) {
		user, shiftStart, shiftEnd := c.getUserOnCall(layer, current, loc)
		if user == nil {
			current = current.Add(time.Hour)
			continue
		}

		// Clamp to requested range
		if shiftStart.Before(start) {
			shiftStart = start
		}
		if shiftEnd.After(end) {
			shiftEnd = end
		}

		// Check if layer is active during this shift
		if c.isLayerActive(layer, current, loc) {
			entries = append(entries, OnCallEntry{
				User:      *user,
				StartTime: shiftStart,
				EndTime:   shiftEnd,
				LayerID:   layer.ID,
				LayerName: layer.Name,
			})
		}

		// Move to end of this shift
		current = shiftEnd
	}

	return entries
}

// mergeEntries merges overlapping entries, with overrides taking precedence
func (c *Calculator) mergeEntries(entries []OnCallEntry, overrides []Override) []OnCallEntry {
	if len(entries) == 0 {
		return entries
	}

	// Simple approach: keep entries, mark non-override entries that overlap with overrides
	var result []OnCallEntry

	for _, entry := range entries {
		// Check if this entry overlaps with any override
		overlapsWithOverride := false
		if !entry.IsOverride {
			for _, override := range overrides {
				if entry.StartTime.Before(override.EndTime) && entry.EndTime.After(override.StartTime) {
					overlapsWithOverride = true
					break
				}
			}
		}

		if !overlapsWithOverride || entry.IsOverride {
			result = append(result, entry)
		}
	}

	return result
}

// GetNextHandoff returns when the next handoff occurs
func (c *Calculator) GetNextHandoff(scheduleID string) (*OnCallEntry, error) {
	sched, err := c.store.GetSchedule(scheduleID)
	if err != nil {
		return nil, err
	}

	loc, err := time.LoadLocation(sched.Timezone)
	if err != nil {
		loc = time.UTC
	}

	now := time.Now().In(loc)

	// Look ahead up to 30 days
	end := now.AddDate(0, 0, 30)

	calendar, err := c.generateCalendar(sched, now, end)
	if err != nil {
		return nil, err
	}

	// Find the first entry that starts after now
	for _, entry := range calendar {
		if entry.StartTime.After(now) {
			return &entry, nil
		}
	}

	return nil, nil
}

// Helper function to parse HH:MM time strings
func parseTime(s string) (hour, min int) {
	if s == "" {
		return 0, 0
	}
	_, err := fmt.Sscanf(s, "%d:%d", &hour, &min)
	if err != nil {
		return 0, 0
	}
	return
}

