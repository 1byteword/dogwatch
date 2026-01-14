package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DiscordSender sends notifications to Discord webhooks
type DiscordSender struct{}

func (s *DiscordSender) Type() ChannelType {
	return ChannelDiscord
}

// DiscordMessage represents a Discord webhook message
type DiscordMessage struct {
	Username  string          `json:"username,omitempty"`
	AvatarURL string          `json:"avatar_url,omitempty"`
	Content   string          `json:"content,omitempty"`
	Embeds    []DiscordEmbed  `json:"embeds,omitempty"`
}

// DiscordEmbed represents an embedded rich content
type DiscordEmbed struct {
	Title       string               `json:"title,omitempty"`
	Description string               `json:"description,omitempty"`
	URL         string               `json:"url,omitempty"`
	Color       int                  `json:"color,omitempty"`
	Timestamp   string               `json:"timestamp,omitempty"`
	Footer      *DiscordFooter       `json:"footer,omitempty"`
	Thumbnail   *DiscordImage        `json:"thumbnail,omitempty"`
	Author      *DiscordAuthor       `json:"author,omitempty"`
	Fields      []DiscordField       `json:"fields,omitempty"`
}

// DiscordFooter represents the footer of an embed
type DiscordFooter struct {
	Text    string `json:"text"`
	IconURL string `json:"icon_url,omitempty"`
}

// DiscordImage represents an image
type DiscordImage struct {
	URL string `json:"url"`
}

// DiscordAuthor represents the author of an embed
type DiscordAuthor struct {
	Name    string `json:"name"`
	URL     string `json:"url,omitempty"`
	IconURL string `json:"icon_url,omitempty"`
}

// DiscordField represents a field in an embed
type DiscordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

func (s *DiscordSender) Send(channel *Channel, notification *Notification) error {
	var config DiscordConfig
	if err := channel.GetConfig(&config); err != nil {
		return fmt.Errorf("invalid discord config: %w", err)
	}

	// Build Discord message
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

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("discord returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (s *DiscordSender) buildMessage(config DiscordConfig, notification *Notification) *DiscordMessage {
	// Determine color (Discord uses decimal colors)
	color := s.mapColor(notification.Severity)
	if notification.Type == NotificationResolved {
		color = 0x22c55e // Green
	}

	// Build title with emoji
	emoji := notification.Type.Emoji()
	title := fmt.Sprintf("%s %s", emoji, notification.Title)

	// Build fields
	var fields []DiscordField

	if notification.Source != "" {
		fields = append(fields, DiscordField{
			Name:   "Source",
			Value:  notification.Source,
			Inline: true,
		})
	}

	if notification.Severity != "" {
		fields = append(fields, DiscordField{
			Name:   "Severity",
			Value:  string(notification.Severity),
			Inline: true,
		})
	}

	if notification.Value != 0 {
		fields = append(fields, DiscordField{
			Name:   "Value",
			Value:  fmt.Sprintf("%.2f", notification.Value),
			Inline: true,
		})
	}

	if notification.Threshold != 0 {
		fields = append(fields, DiscordField{
			Name:   "Threshold",
			Value:  fmt.Sprintf("%.2f", notification.Threshold),
			Inline: true,
		})
	}

	// Add labels as fields
	for k, v := range notification.Labels {
		fields = append(fields, DiscordField{
			Name:   k,
			Value:  v,
			Inline: true,
		})
	}

	msg := &DiscordMessage{
		Username:  config.Username,
		AvatarURL: config.AvatarURL,
		Embeds: []DiscordEmbed{
			{
				Title:       title,
				Description: notification.Message,
				URL:         notification.URL,
				Color:       color,
				Timestamp:   notification.Timestamp.Format(time.RFC3339),
				Fields:      fields,
				Footer: &DiscordFooter{
					Text: "dogwatch",
				},
			},
		},
	}

	// Set defaults
	if msg.Username == "" {
		msg.Username = "dogwatch"
	}

	return msg
}

func (s *DiscordSender) mapColor(severity Severity) int {
	switch severity {
	case SeverityCritical:
		return 0xdc2626 // Red
	case SeverityHigh:
		return 0xea580c // Orange
	case SeverityWarning:
		return 0xca8a04 // Yellow
	case SeverityInfo:
		return 0x2563eb // Blue
	default:
		return 0x6b7280 // Gray
	}
}

func (s *DiscordSender) ValidateConfig(config json.RawMessage) error {
	var c DiscordConfig
	if err := json.Unmarshal(config, &c); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	if c.WebhookURL == "" {
		return fmt.Errorf("webhook_url is required")
	}

	return nil
}
