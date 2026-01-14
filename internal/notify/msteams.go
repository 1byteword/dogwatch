package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// MSTeamsSender sends notifications to Microsoft Teams
type MSTeamsSender struct{}

func (s *MSTeamsSender) Type() ChannelType {
	return ChannelMSTeams
}

// TeamsMessage represents a Microsoft Teams message card
type TeamsMessage struct {
	Type       string         `json:"@type"`
	Context    string         `json:"@context"`
	ThemeColor string         `json:"themeColor,omitempty"`
	Summary    string         `json:"summary"`
	Title      string         `json:"title,omitempty"`
	Sections   []TeamsSection `json:"sections,omitempty"`
	Actions    []TeamsAction  `json:"potentialAction,omitempty"`
}

// TeamsSection represents a section in the message
type TeamsSection struct {
	ActivityTitle    string       `json:"activityTitle,omitempty"`
	ActivitySubtitle string       `json:"activitySubtitle,omitempty"`
	ActivityImage    string       `json:"activityImage,omitempty"`
	ActivityText     string       `json:"activityText,omitempty"`
	Text             string       `json:"text,omitempty"`
	Facts            []TeamsFact  `json:"facts,omitempty"`
	Markdown         bool         `json:"markdown,omitempty"`
}

// TeamsFact represents a fact (key-value pair)
type TeamsFact struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// TeamsAction represents an action button
type TeamsAction struct {
	Type    string              `json:"@type"`
	Name    string              `json:"name"`
	Targets []TeamsActionTarget `json:"targets,omitempty"`
}

// TeamsActionTarget is the target for an action
type TeamsActionTarget struct {
	OS  string `json:"os"`
	URI string `json:"uri"`
}

func (s *MSTeamsSender) Send(channel *Channel, notification *Notification) error {
	var config MSTeamsConfig
	if err := channel.GetConfig(&config); err != nil {
		return fmt.Errorf("invalid msteams config: %w", err)
	}

	// Build message card
	msg := s.buildMessage(config, notification)

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	req, err := http.NewRequest("POST", config.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Teams returns "1" for success
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode >= 400 {
		return fmt.Errorf("teams returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (s *MSTeamsSender) buildMessage(config MSTeamsConfig, notification *Notification) *TeamsMessage {
	// Determine theme color
	themeColor := s.mapColor(notification.Severity)
	if notification.Type == NotificationResolved {
		themeColor = "22c55e" // Green
	}

	// Build title
	emoji := notification.Type.Emoji()
	title := config.Title
	if title == "" {
		title = fmt.Sprintf("%s %s", emoji, notification.Title)
	}

	// Build facts
	var facts []TeamsFact

	if notification.Source != "" {
		facts = append(facts, TeamsFact{Name: "Source", Value: notification.Source})
	}
	if notification.Severity != "" {
		facts = append(facts, TeamsFact{Name: "Severity", Value: string(notification.Severity)})
	}
	if notification.Value != 0 {
		facts = append(facts, TeamsFact{Name: "Value", Value: fmt.Sprintf("%.2f", notification.Value)})
	}
	if notification.Threshold != 0 {
		facts = append(facts, TeamsFact{Name: "Threshold", Value: fmt.Sprintf("%.2f", notification.Threshold)})
	}

	facts = append(facts, TeamsFact{
		Name:  "Time",
		Value: notification.Timestamp.Format("2006-01-02 15:04:05 MST"),
	})

	// Add labels as facts
	for k, v := range notification.Labels {
		facts = append(facts, TeamsFact{Name: k, Value: v})
	}

	msg := &TeamsMessage{
		Type:       "MessageCard",
		Context:    "http://schema.org/extensions",
		ThemeColor: themeColor,
		Summary:    notification.Title,
		Title:      title,
		Sections: []TeamsSection{
			{
				Text:     notification.Message,
				Facts:    facts,
				Markdown: true,
			},
		},
	}

	// Add action button if URL provided
	if notification.URL != "" {
		msg.Actions = []TeamsAction{
			{
				Type: "OpenUri",
				Name: "View in Dashboard",
				Targets: []TeamsActionTarget{
					{OS: "default", URI: notification.URL},
				},
			},
		}
	}

	return msg
}

func (s *MSTeamsSender) mapColor(severity Severity) string {
	switch severity {
	case SeverityCritical:
		return "dc2626" // Red
	case SeverityHigh:
		return "ea580c" // Orange
	case SeverityWarning:
		return "ca8a04" // Yellow
	case SeverityInfo:
		return "2563eb" // Blue
	default:
		return "6b7280" // Gray
	}
}

func (s *MSTeamsSender) ValidateConfig(config json.RawMessage) error {
	var c MSTeamsConfig
	if err := json.Unmarshal(config, &c); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	if c.WebhookURL == "" {
		return fmt.Errorf("webhook_url is required")
	}

	return nil
}
