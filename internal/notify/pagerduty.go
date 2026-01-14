package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// PagerDutySender sends notifications to PagerDuty Events API v2
type PagerDutySender struct{}

func (s *PagerDutySender) Type() ChannelType {
	return ChannelPagerDuty
}

const pagerDutyEventsURL = "https://events.pagerduty.com/v2/enqueue"

// PagerDutyEvent represents a PagerDuty Events API v2 event
type PagerDutyEvent struct {
	RoutingKey  string             `json:"routing_key"`
	EventAction string             `json:"event_action"` // trigger, acknowledge, resolve
	DedupKey    string             `json:"dedup_key,omitempty"`
	Payload     PagerDutyPayload   `json:"payload"`
	Links       []PagerDutyLink    `json:"links,omitempty"`
	Images      []PagerDutyImage   `json:"images,omitempty"`
}

// PagerDutyPayload is the event payload
type PagerDutyPayload struct {
	Summary       string                 `json:"summary"`
	Severity      string                 `json:"severity"` // critical, error, warning, info
	Source        string                 `json:"source"`
	Component     string                 `json:"component,omitempty"`
	Group         string                 `json:"group,omitempty"`
	Class         string                 `json:"class,omitempty"`
	Timestamp     string                 `json:"timestamp,omitempty"`
	CustomDetails map[string]interface{} `json:"custom_details,omitempty"`
}

// PagerDutyLink represents a link in the event
type PagerDutyLink struct {
	Href string `json:"href"`
	Text string `json:"text,omitempty"`
}

// PagerDutyImage represents an image in the event
type PagerDutyImage struct {
	Src  string `json:"src"`
	Href string `json:"href,omitempty"`
	Alt  string `json:"alt,omitempty"`
}

func (s *PagerDutySender) Send(channel *Channel, notification *Notification) error {
	var config PagerDutyConfig
	if err := channel.GetConfig(&config); err != nil {
		return fmt.Errorf("invalid pagerduty config: %w", err)
	}

	// Determine event action
	eventAction := "trigger"
	if notification.Type == NotificationResolved {
		eventAction = "resolve"
	} else if notification.Type == NotificationAcked {
		eventAction = "acknowledge"
	}

	// Map severity
	severity := s.mapSeverity(notification.Severity, config.DefaultSeverity)

	// Build custom details
	customDetails := make(map[string]interface{})
	if notification.Value != 0 {
		customDetails["value"] = notification.Value
	}
	if notification.Threshold != 0 {
		customDetails["threshold"] = notification.Threshold
	}
	if notification.SourceType != "" {
		customDetails["source_type"] = notification.SourceType
	}
	for k, v := range notification.Labels {
		customDetails[k] = v
	}

	// Build event
	event := PagerDutyEvent{
		RoutingKey:  config.IntegrationKey,
		EventAction: eventAction,
		DedupKey:    notification.DedupKey,
		Payload: PagerDutyPayload{
			Summary:       fmt.Sprintf("%s: %s", notification.Title, notification.Message),
			Severity:      severity,
			Source:        notification.Source,
			Component:     notification.SourceType,
			Class:         string(notification.Type),
			Timestamp:     notification.Timestamp.Format(time.RFC3339),
			CustomDetails: customDetails,
		},
	}

	// Add link if URL provided
	if notification.URL != "" {
		event.Links = []PagerDutyLink{
			{
				Href: notification.URL,
				Text: "View in dogwatch",
			},
		}
	}

	// If no dedup key, generate one from source ID
	if event.DedupKey == "" && notification.SourceID != "" {
		event.DedupKey = notification.SourceID
	}

	// Send to PagerDuty
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	req, err := http.NewRequest("POST", pagerDutyEventsURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Routing-Key", config.IntegrationKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	// Check response
	if resp.StatusCode >= 400 {
		return fmt.Errorf("pagerduty returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response for error message
	var pdResp struct {
		Status   string `json:"status"`
		Message  string `json:"message"`
		DedupKey string `json:"dedup_key"`
	}
	if err := json.Unmarshal(respBody, &pdResp); err == nil && pdResp.Status != "success" {
		return fmt.Errorf("pagerduty error: %s", pdResp.Message)
	}

	return nil
}

func (s *PagerDutySender) mapSeverity(severity Severity, defaultSeverity string) string {
	switch severity {
	case SeverityCritical:
		return "critical"
	case SeverityHigh:
		return "error"
	case SeverityWarning:
		return "warning"
	case SeverityInfo:
		return "info"
	default:
		if defaultSeverity != "" {
			return defaultSeverity
		}
		return "warning"
	}
}

func (s *PagerDutySender) ValidateConfig(config json.RawMessage) error {
	var c PagerDutyConfig
	if err := json.Unmarshal(config, &c); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	if c.IntegrationKey == "" {
		return fmt.Errorf("integration_key is required")
	}

	// Validate default severity if provided
	if c.DefaultSeverity != "" {
		valid := map[string]bool{"critical": true, "error": true, "warning": true, "info": true}
		if !valid[c.DefaultSeverity] {
			return fmt.Errorf("default_severity must be one of: critical, error, warning, info")
		}
	}

	return nil
}
