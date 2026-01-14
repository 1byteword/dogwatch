package notify

import (
	"encoding/json"
	"time"
)

// ChannelType represents the type of notification channel
type ChannelType string

const (
	ChannelWebhook   ChannelType = "webhook"
	ChannelSlack     ChannelType = "slack"
	ChannelEmail     ChannelType = "email"
	ChannelPagerDuty ChannelType = "pagerduty"
	ChannelOpsGenie  ChannelType = "opsgenie"
	ChannelMSTeams   ChannelType = "msteams"
	ChannelDiscord   ChannelType = "discord"
)

// Channel represents a notification channel configuration
type Channel struct {
	ID          string          `json:"id"`
	OrgID       string          `json:"org_id"`
	Name        string          `json:"name"`
	Type        ChannelType     `json:"type"`
	Config      json.RawMessage `json:"config"`
	Enabled     bool            `json:"enabled"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	LastUsedAt  *time.Time      `json:"last_used_at,omitempty"`
	LastError   string          `json:"last_error,omitempty"`
	SuccessRate float64         `json:"success_rate"` // Last 100 sends
}

// NotificationType indicates what kind of notification this is
type NotificationType string

const (
	NotificationAlert     NotificationType = "alert"
	NotificationResolved  NotificationType = "resolved"
	NotificationIncident  NotificationType = "incident"
	NotificationEscalated NotificationType = "escalated"
	NotificationAcked     NotificationType = "acknowledged"
	NotificationTest      NotificationType = "test"
)

// Severity levels
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

// Notification represents a notification to be sent
type Notification struct {
	Type        NotificationType  `json:"type"`
	Title       string            `json:"title"`
	Message     string            `json:"message"`
	Severity    Severity          `json:"severity"`
	Source      string            `json:"source"`       // Alert rule name, watch name, etc.
	SourceType  string            `json:"source_type"`  // alert, watch, incident, synthetic
	SourceID    string            `json:"source_id"`    // ID of the source
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	URL         string            `json:"url,omitempty"` // Link to dashboard
	Value       float64           `json:"value,omitempty"`
	Threshold   float64           `json:"threshold,omitempty"`
	Timestamp   time.Time         `json:"timestamp"`
	DedupKey    string            `json:"dedup_key,omitempty"` // For deduplication
}

// NotificationLog records a sent notification
type NotificationLog struct {
	ID           string           `json:"id"`
	ChannelID    string           `json:"channel_id"`
	ChannelName  string           `json:"channel_name"`
	ChannelType  ChannelType      `json:"channel_type"`
	Notification *Notification    `json:"notification"`
	Status       NotificationStatus `json:"status"`
	Error        string           `json:"error,omitempty"`
	SentAt       time.Time        `json:"sent_at"`
	ResponseTime int64            `json:"response_time_ms"`
}

// NotificationStatus represents the status of a sent notification
type NotificationStatus string

const (
	StatusPending   NotificationStatus = "pending"
	StatusSent      NotificationStatus = "sent"
	StatusFailed    NotificationStatus = "failed"
	StatusDelivered NotificationStatus = "delivered"
)

// WebhookConfig for webhook notifications
type WebhookConfig struct {
	URL         string            `json:"url"`
	Method      string            `json:"method,omitempty"` // POST, PUT - defaults to POST
	Headers     map[string]string `json:"headers,omitempty"`
	Username    string            `json:"username,omitempty"`    // Basic auth
	Password    string            `json:"password,omitempty"`    // Basic auth
	TLSInsecure bool              `json:"tls_insecure,omitempty"`
	Timeout     int               `json:"timeout,omitempty"` // Seconds, defaults to 10
}

// SlackConfig for Slack notifications
type SlackConfig struct {
	WebhookURL string `json:"webhook_url,omitempty"` // Incoming webhook
	BotToken   string `json:"bot_token,omitempty"`   // For API-based sending
	Channel    string `json:"channel,omitempty"`     // #channel or @user
	Username   string `json:"username,omitempty"`
	IconEmoji  string `json:"icon_emoji,omitempty"`
	IconURL    string `json:"icon_url,omitempty"`
}

// EmailConfig for SMTP email notifications
type EmailConfig struct {
	SMTPHost     string   `json:"smtp_host"`
	SMTPPort     int      `json:"smtp_port"`
	Username     string   `json:"username,omitempty"`
	Password     string   `json:"password,omitempty"`
	From         string   `json:"from"`
	To           []string `json:"to"`
	TLS          bool     `json:"tls"`
	StartTLS     bool     `json:"starttls"`
	SkipVerify   bool     `json:"skip_verify,omitempty"`
	Subject      string   `json:"subject,omitempty"` // Template for subject
}

// PagerDutyConfig for PagerDuty Events API v2
type PagerDutyConfig struct {
	IntegrationKey string `json:"integration_key"` // Events API v2 routing key
	ServiceKey     string `json:"service_key,omitempty"` // Legacy service key
	DefaultSeverity string `json:"default_severity,omitempty"` // critical, error, warning, info
}

// OpsGenieConfig for OpsGenie Alerts API
type OpsGenieConfig struct {
	APIKey      string   `json:"api_key"`
	Region      string   `json:"region,omitempty"` // us or eu, defaults to us
	Priority    string   `json:"priority,omitempty"` // P1-P5
	Tags        []string `json:"tags,omitempty"`
	Responders  []string `json:"responders,omitempty"` // Team names or user emails
}

// MSTeamsConfig for Microsoft Teams webhooks
type MSTeamsConfig struct {
	WebhookURL string `json:"webhook_url"`
	Title      string `json:"title,omitempty"` // Card title
}

// DiscordConfig for Discord webhooks
type DiscordConfig struct {
	WebhookURL string `json:"webhook_url"`
	Username   string `json:"username,omitempty"`
	AvatarURL  string `json:"avatar_url,omitempty"`
}

// Sender interface for notification senders
type Sender interface {
	// Type returns the channel type this sender handles
	Type() ChannelType
	// Send sends a notification via this channel
	Send(channel *Channel, notification *Notification) error
	// ValidateConfig validates the channel configuration
	ValidateConfig(config json.RawMessage) error
}

// GetConfig unmarshals the channel config into the appropriate struct
func (c *Channel) GetConfig(v interface{}) error {
	return json.Unmarshal(c.Config, v)
}

// SetConfig marshals the config struct into the channel
func (c *Channel) SetConfig(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.Config = data
	return nil
}

// SeverityColor returns a color code for the severity level
func (s Severity) Color() string {
	switch s {
	case SeverityCritical:
		return "#dc2626" // Red
	case SeverityHigh:
		return "#ea580c" // Orange
	case SeverityWarning:
		return "#ca8a04" // Yellow
	case SeverityInfo:
		return "#2563eb" // Blue
	default:
		return "#6b7280" // Gray
	}
}

// SeverityEmoji returns an emoji for the severity level
func (s Severity) Emoji() string {
	switch s {
	case SeverityCritical:
		return "\U0001F6A8" // rotating light
	case SeverityHigh:
		return "\U000026A0" // warning
	case SeverityWarning:
		return "\U0001F7E1" // yellow circle
	case SeverityInfo:
		return "\U0001F535" // blue circle
	default:
		return "\U00002139" // info
	}
}

// NotificationTypeEmoji returns an emoji for the notification type
func (t NotificationType) Emoji() string {
	switch t {
	case NotificationAlert:
		return "\U0001F6A8" // rotating light
	case NotificationResolved:
		return "\U00002705" // checkmark
	case NotificationIncident:
		return "\U0001F525" // fire
	case NotificationEscalated:
		return "\U00002B06" // up arrow
	case NotificationAcked:
		return "\U0001F44D" // thumbs up
	case NotificationTest:
		return "\U0001F9EA" // test tube
	default:
		return "\U0001F514" // bell
	}
}
