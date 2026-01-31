package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"time"
)

// MyOnCallView represents the "My On-Call" dashboard data
type MyOnCallView struct {
	UserID       string            `json:"user_id"`
	UserEmail    string            `json:"user_email"`
	Timestamp    time.Time         `json:"timestamp"`

	// Current status
	IsCurrentlyOnCall  bool               `json:"is_currently_oncall"`
	CurrentShifts      []CurrentShift     `json:"current_shifts,omitempty"`

	// Upcoming shifts
	UpcomingShifts     []UpcomingShift    `json:"upcoming_shifts"`

	// Stats
	TotalSchedules     int                `json:"total_schedules"`
	TotalHoursThisWeek float64            `json:"total_hours_this_week"`
	TotalHoursThisMonth float64           `json:"total_hours_this_month"`

	// Quick actions available
	CanTakeOverride    bool               `json:"can_take_override"`
}

// CurrentShift represents an active on-call shift
type CurrentShift struct {
	ScheduleID   string    `json:"schedule_id"`
	ScheduleName string    `json:"schedule_name"`
	LayerName    string    `json:"layer_name,omitempty"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	TimeRemaining string   `json:"time_remaining"`
	IsOverride   bool      `json:"is_override"`
}

// UpcomingShift represents a future on-call shift
type UpcomingShift struct {
	ScheduleID   string    `json:"schedule_id"`
	ScheduleName string    `json:"schedule_name"`
	LayerName    string    `json:"layer_name,omitempty"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	StartsIn     string    `json:"starts_in"`
	Duration     string    `json:"duration"`
}

// handleMyOnCall returns the current user's on-call view (JSON)
func (s *Server) handleMyOnCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get user from query param or header (in real app, would use auth)
	userID := r.URL.Query().Get("user_id")
	userEmail := r.URL.Query().Get("email")
	if userID == "" {
		userID = r.Header.Get("X-User-ID")
	}
	if userEmail == "" {
		userEmail = r.Header.Get("X-User-Email")
	}

	view := s.buildMyOnCallView(userID, userEmail)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(view)
}

// handleMyOnCallPage serves the HTML page
func (s *Server) handleMyOnCallPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := r.URL.Query().Get("user_id")
	userEmail := r.URL.Query().Get("email")

	view := s.buildMyOnCallView(userID, userEmail)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl := template.Must(template.New("myoncall").Parse(myOnCallTemplate))
	tmpl.Execute(w, view)
}

// buildMyOnCallView builds the My On-Call view for a user
func (s *Server) buildMyOnCallView(userID, userEmail string) *MyOnCallView {
	view := &MyOnCallView{
		UserID:         userID,
		UserEmail:      userEmail,
		Timestamp:      time.Now(),
		CurrentShifts:  []CurrentShift{},
		UpcomingShifts: []UpcomingShift{},
	}

	if s.oncallStore == nil || s.oncallCalculator == nil {
		return view
	}

	schedules, err := s.oncallStore.ListSchedules()
	if err != nil {
		return view
	}

	view.TotalSchedules = len(schedules)
	now := time.Now()
	weekStart := now.AddDate(0, 0, -int(now.Weekday()))
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	weekEnd := weekStart.AddDate(0, 0, 7)
	monthEnd := monthStart.AddDate(0, 1, 0)

	// Look ahead 30 days for upcoming shifts
	lookAhead := now.AddDate(0, 0, 30)

	for _, sched := range schedules {
		// Check if user is in this schedule
		userInSchedule := false
		for _, layer := range sched.Layers {
			for _, user := range layer.Users {
				if (userID != "" && user.ID == userID) || (userEmail != "" && user.Email == userEmail) {
					userInSchedule = true
					break
				}
			}
			if userInSchedule {
				break
			}
		}

		if !userInSchedule {
			continue
		}

		// Get current on-call
		current, err := s.oncallCalculator.GetCurrentOnCall(sched.ID)
		if err == nil && current != nil {
			isCurrentUser := (userID != "" && current.User.ID == userID) ||
				(userEmail != "" && current.User.Email == userEmail)

			if isCurrentUser {
				view.IsCurrentlyOnCall = true
				view.CurrentShifts = append(view.CurrentShifts, CurrentShift{
					ScheduleID:    sched.ID,
					ScheduleName:  sched.Name,
					LayerName:     current.LayerName,
					StartTime:     current.StartTime,
					EndTime:       current.EndTime,
					TimeRemaining: formatDurationHuman(current.EndTime.Sub(now)),
					IsOverride:    current.IsOverride,
				})
			}
		}

		// Get upcoming shifts (calendar)
		calendar, err := s.oncallCalculator.GetCalendar(sched.ID, now, lookAhead)
		if err != nil {
			continue
		}

		for _, entry := range calendar {
			isCurrentUser := (userID != "" && entry.User.ID == userID) ||
				(userEmail != "" && entry.User.Email == userEmail)

			if !isCurrentUser {
				continue
			}

			// Skip if it's the current shift (already shown above)
			if entry.StartTime.Before(now) && entry.EndTime.After(now) {
				continue
			}

			// Only future shifts
			if entry.StartTime.After(now) {
				view.UpcomingShifts = append(view.UpcomingShifts, UpcomingShift{
					ScheduleID:   sched.ID,
					ScheduleName: sched.Name,
					LayerName:    entry.LayerName,
					StartTime:    entry.StartTime,
					EndTime:      entry.EndTime,
					StartsIn:     formatDurationHuman(entry.StartTime.Sub(now)),
					Duration:     formatDurationHuman(entry.EndTime.Sub(entry.StartTime)),
				})
			}

			// Calculate hours for stats
			shiftStart := entry.StartTime
			shiftEnd := entry.EndTime

			// This week
			if shiftEnd.After(weekStart) && shiftStart.Before(weekEnd) {
				start := shiftStart
				if start.Before(weekStart) {
					start = weekStart
				}
				end := shiftEnd
				if end.After(weekEnd) {
					end = weekEnd
				}
				view.TotalHoursThisWeek += end.Sub(start).Hours()
			}

			// This month
			if shiftEnd.After(monthStart) && shiftStart.Before(monthEnd) {
				start := shiftStart
				if start.Before(monthStart) {
					start = monthStart
				}
				end := shiftEnd
				if end.After(monthEnd) {
					end = monthEnd
				}
				view.TotalHoursThisMonth += end.Sub(start).Hours()
			}
		}
	}

	// Sort upcoming shifts by start time
	sort.Slice(view.UpcomingShifts, func(i, j int) bool {
		return view.UpcomingShifts[i].StartTime.Before(view.UpcomingShifts[j].StartTime)
	})

	// Limit to next 10 upcoming shifts
	if len(view.UpcomingShifts) > 10 {
		view.UpcomingShifts = view.UpcomingShifts[:10]
	}

	view.CanTakeOverride = len(schedules) > 0

	return view
}

// formatDurationHuman formats a duration in human-readable form
func formatDurationHuman(d time.Duration) string {
	if d < 0 {
		return "now"
	}
	if d < time.Minute {
		return "< 1 min"
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 min"
		}
		return fmt.Sprintf("%d mins", mins)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		mins := int(d.Minutes()) % 60
		if hours == 1 {
			if mins > 0 {
				return fmt.Sprintf("1h %dm", mins)
			}
			return "1 hour"
		}
		if mins > 0 {
			return fmt.Sprintf("%dh %dm", hours, mins)
		}
		return fmt.Sprintf("%d hours", hours)
	}
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	if days == 1 {
		if hours > 0 {
			return fmt.Sprintf("1 day %dh", hours)
		}
		return "1 day"
	}
	if hours > 0 {
		return fmt.Sprintf("%d days %dh", days, hours)
	}
	return fmt.Sprintf("%d days", days)
}

const myOnCallTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>My On-Call | Dogwatch</title>
    <style>
        :root {
            --bg-primary: #0d1117;
            --bg-secondary: #161b22;
            --bg-tertiary: #21262d;
            --text-primary: #c9d1d9;
            --text-secondary: #8b949e;
            --text-muted: #6e7681;
            --border-color: #30363d;
            --accent-green: #3fb950;
            --accent-yellow: #d29922;
            --accent-red: #f85149;
            --accent-blue: #58a6ff;
        }

        * { box-sizing: border-box; margin: 0; padding: 0; }

        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: var(--bg-primary);
            color: var(--text-primary);
            min-height: 100vh;
            line-height: 1.5;
        }

        .container {
            max-width: 900px;
            margin: 0 auto;
            padding: 24px;
        }

        header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 24px;
            padding-bottom: 16px;
            border-bottom: 1px solid var(--border-color);
        }

        h1 { font-size: 24px; font-weight: 600; }
        h2 { font-size: 18px; font-weight: 600; margin-bottom: 16px; }

        .status-banner {
            padding: 20px;
            border-radius: 8px;
            margin-bottom: 24px;
            display: flex;
            align-items: center;
            gap: 16px;
        }

        .status-banner.oncall {
            background: linear-gradient(135deg, #1a4d2e 0%, #0d1117 100%);
            border: 1px solid var(--accent-green);
        }

        .status-banner.not-oncall {
            background: var(--bg-secondary);
            border: 1px solid var(--border-color);
        }

        .status-icon {
            font-size: 48px;
        }

        .status-text h3 {
            font-size: 20px;
            margin-bottom: 4px;
        }

        .status-text p {
            color: var(--text-secondary);
        }

        .stats-grid {
            display: grid;
            grid-template-columns: repeat(3, 1fr);
            gap: 16px;
            margin-bottom: 24px;
        }

        .stat-card {
            background: var(--bg-secondary);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            padding: 16px;
            text-align: center;
        }

        .stat-value {
            font-size: 32px;
            font-weight: 600;
            color: var(--accent-blue);
        }

        .stat-label {
            font-size: 12px;
            color: var(--text-secondary);
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }

        .section {
            background: var(--bg-secondary);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            padding: 20px;
            margin-bottom: 24px;
        }

        .current-shift {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 12px;
            background: var(--bg-tertiary);
            border-radius: 6px;
            margin-bottom: 8px;
        }

        .current-shift:last-child {
            margin-bottom: 0;
        }

        .shift-info h4 {
            font-weight: 600;
            margin-bottom: 4px;
        }

        .shift-info p {
            font-size: 13px;
            color: var(--text-secondary);
        }

        .time-remaining {
            text-align: right;
        }

        .time-remaining .value {
            font-size: 18px;
            font-weight: 600;
            color: var(--accent-yellow);
        }

        .time-remaining .label {
            font-size: 11px;
            color: var(--text-muted);
            text-transform: uppercase;
        }

        .upcoming-list {
            display: flex;
            flex-direction: column;
            gap: 8px;
        }

        .upcoming-shift {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 12px;
            background: var(--bg-tertiary);
            border-radius: 6px;
        }

        .shift-schedule {
            font-weight: 500;
        }

        .shift-time {
            font-size: 13px;
            color: var(--text-secondary);
        }

        .starts-in {
            text-align: right;
            font-size: 13px;
            color: var(--accent-blue);
        }

        .empty-state {
            text-align: center;
            padding: 32px;
            color: var(--text-secondary);
        }

        .override-badge {
            display: inline-block;
            padding: 2px 8px;
            font-size: 11px;
            background: var(--accent-yellow);
            color: black;
            border-radius: 4px;
            margin-left: 8px;
        }

        .refresh-btn {
            padding: 8px 16px;
            background: var(--bg-tertiary);
            border: 1px solid var(--border-color);
            color: var(--text-primary);
            border-radius: 6px;
            cursor: pointer;
        }

        .refresh-btn:hover {
            background: var(--bg-secondary);
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <div>
                <h1>📟 My On-Call</h1>
            </div>
            <div>
                <button class="refresh-btn" onclick="location.reload()">Refresh</button>
            </div>
        </header>

        {{if .IsCurrentlyOnCall}}
        <div class="status-banner oncall">
            <div class="status-icon">🟢</div>
            <div class="status-text">
                <h3>You're On-Call</h3>
                <p>You are currently on-call for {{len .CurrentShifts}} schedule(s)</p>
            </div>
        </div>
        {{else}}
        <div class="status-banner not-oncall">
            <div class="status-icon">⚪</div>
            <div class="status-text">
                <h3>Not Currently On-Call</h3>
                <p>You have no active on-call shifts right now</p>
            </div>
        </div>
        {{end}}

        <div class="stats-grid">
            <div class="stat-card">
                <div class="stat-value">{{printf "%.0f" .TotalHoursThisWeek}}</div>
                <div class="stat-label">Hours This Week</div>
            </div>
            <div class="stat-card">
                <div class="stat-value">{{printf "%.0f" .TotalHoursThisMonth}}</div>
                <div class="stat-label">Hours This Month</div>
            </div>
            <div class="stat-card">
                <div class="stat-value">{{len .UpcomingShifts}}</div>
                <div class="stat-label">Upcoming Shifts</div>
            </div>
        </div>

        {{if .CurrentShifts}}
        <div class="section">
            <h2>Current Shifts</h2>
            {{range .CurrentShifts}}
            <div class="current-shift">
                <div class="shift-info">
                    <h4>{{.ScheduleName}}{{if .IsOverride}}<span class="override-badge">Override</span>{{end}}</h4>
                    <p>{{if .LayerName}}{{.LayerName}} · {{end}}Ends {{.EndTime.Format "Mon Jan 2, 3:04 PM"}}</p>
                </div>
                <div class="time-remaining">
                    <div class="value">{{.TimeRemaining}}</div>
                    <div class="label">remaining</div>
                </div>
            </div>
            {{end}}
        </div>
        {{end}}

        <div class="section">
            <h2>Upcoming Shifts</h2>
            {{if .UpcomingShifts}}
            <div class="upcoming-list">
                {{range .UpcomingShifts}}
                <div class="upcoming-shift">
                    <div>
                        <div class="shift-schedule">{{.ScheduleName}}</div>
                        <div class="shift-time">{{.StartTime.Format "Mon Jan 2, 3:04 PM"}} - {{.EndTime.Format "3:04 PM"}} ({{.Duration}})</div>
                    </div>
                    <div class="starts-in">in {{.StartsIn}}</div>
                </div>
                {{end}}
            </div>
            {{else}}
            <div class="empty-state">
                No upcoming shifts in the next 30 days
            </div>
            {{end}}
        </div>
    </div>

    <script>
        // Auto-refresh every 60 seconds
        setTimeout(() => location.reload(), 60000);
    </script>
</body>
</html>`
