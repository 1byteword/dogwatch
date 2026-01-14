package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SlackSender sends notifications to Slack
type SlackSender struct{}

func (s *SlackSender) Type() ChannelType {
	return ChannelSlack
}

// SlackMessage represents a Slack message with Block Kit
type SlackMessage struct {
	Channel     string            `json:"channel,omitempty"`
	Username    string            `json:"username,omitempty"`
	IconEmoji   string            `json:"icon_emoji,omitempty"`
	IconURL     string            `json:"icon_url,omitempty"`
	Text        string            `json:"text,omitempty"` // Fallback text
	Attachments []SlackAttachment `json:"attachments,omitempty"`
	Blocks      []SlackBlock      `json:"blocks,omitempty"`
}

// SlackAttachment represents a Slack attachment
type SlackAttachment struct {
	Color      string       `json:"color,omitempty"`
	Title      string       `json:"title,omitempty"`
	TitleLink  string       `json:"title_link,omitempty"`
	Text       string       `json:"text,omitempty"`
	Fields     []SlackField `json:"fields,omitempty"`
	Footer     string       `json:"footer,omitempty"`
	FooterIcon string       `json:"footer_icon,omitempty"`
	Ts         int64        `json:"ts,omitempty"`
}

// SlackField represents a field in Slack attachment
type SlackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// SlackBlock represents a Block Kit block
type SlackBlock struct {
	Type      string           `json:"type"`
	Text      *SlackText       `json:"text,omitempty"`
	Fields    []SlackText      `json:"fields,omitempty"`
	Elements  []SlackElement   `json:"elements,omitempty"`
	Accessory *SlackElement    `json:"accessory,omitempty"`
}

// SlackText represents text in Block Kit
type SlackText struct {
	Type  string `json:"type"` // plain_text or mrkdwn
	Text  string `json:"text"`
	Emoji bool   `json:"emoji,omitempty"`
}

// SlackElement represents an interactive element
type SlackElement struct {
	Type     string     `json:"type"`
	Text     *SlackText `json:"text,omitempty"`
	URL      string     `json:"url,omitempty"`
	ActionID string     `json:"action_id,omitempty"`
}

func (s *SlackSender) Send(channel *Channel, notification *Notification) error {
	var config SlackConfig
	if err := channel.GetConfig(&config); err != nil {
		return fmt.Errorf("invalid slack config: %w", err)
	}

	// Build Slack message
	msg := s.buildMessage(config, notification)

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Determine endpoint
	url := config.WebhookURL
	if url == "" && config.BotToken != "" {
		url = "https://slack.com/api/chat.postMessage"
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Add bot token if using API
	if config.BotToken != "" {
		req.Header.Set("Authorization", "Bearer "+config.BotToken)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode >= 400 {
		return fmt.Errorf("slack returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Check Slack API response for errors
	if config.BotToken != "" {
		var slackResp struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(respBody, &slackResp); err == nil && !slackResp.OK {
			return fmt.Errorf("slack API error: %s", slackResp.Error)
		}
	}

	return nil
}

func (s *SlackSender) buildMessage(config SlackConfig, notification *Notification) *SlackMessage {
	// Determine color based on type and severity
	color := notification.Severity.Color()
	if notification.Type == NotificationResolved {
		color = "#22c55e" // Green for resolved
	}

	// Build title with emoji
	emoji := notification.Type.Emoji()
	title := fmt.Sprintf("%s %s", emoji, notification.Title)

	// Build fields
	var fields []SlackField

	if notification.Source != "" {
		fields = append(fields, SlackField{
			Title: "Source",
			Value: notification.Source,
			Short: true,
		})
	}

	if notification.Severity != "" {
		fields = append(fields, SlackField{
			Title: "Severity",
			Value: string(notification.Severity),
			Short: true,
		})
	}

	if notification.Value != 0 {
		fields = append(fields, SlackField{
			Title: "Value",
			Value: fmt.Sprintf("%.2f", notification.Value),
			Short: true,
		})
	}

	if notification.Threshold != 0 {
		fields = append(fields, SlackField{
			Title: "Threshold",
			Value: fmt.Sprintf("%.2f", notification.Threshold),
			Short: true,
		})
	}

	// Add labels as fields
	for key, value := range notification.Labels {
		fields = append(fields, SlackField{
			Title: key,
			Value: value,
			Short: true,
		})
	}

	msg := &SlackMessage{
		Channel:   config.Channel,
		Username:  config.Username,
		IconEmoji: config.IconEmoji,
		IconURL:   config.IconURL,
		Text:      title, // Fallback for notifications
		Attachments: []SlackAttachment{
			{
				Color:     color,
				Title:     title,
				TitleLink: notification.URL,
				Text:      notification.Message,
				Fields:    fields,
				Footer:    "dogwatch",
				Ts:        notification.Timestamp.Unix(),
			},
		},
	}

	// Set defaults
	if msg.Username == "" {
		msg.Username = "dogwatch"
	}
	if msg.IconEmoji == "" {
		msg.IconEmoji = ":dog:"
	}

	return msg
}

func (s *SlackSender) ValidateConfig(config json.RawMessage) error {
	var c SlackConfig
	if err := json.Unmarshal(config, &c); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	if c.WebhookURL == "" && c.BotToken == "" {
		return fmt.Errorf("either webhook_url or bot_token is required")
	}

	if c.BotToken != "" && c.Channel == "" {
		return fmt.Errorf("channel is required when using bot_token")
	}

	return nil
}
