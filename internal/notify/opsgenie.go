package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OpsGenieSender sends notifications to OpsGenie
type OpsGenieSender struct{}

func (s *OpsGenieSender) Type() ChannelType {
	return ChannelOpsGenie
}

const (
	opsGenieUSURL = "https://api.opsgenie.com/v2/alerts"
	opsGenieEUURL = "https://api.eu.opsgenie.com/v2/alerts"
)

// OpsGenieAlert represents an OpsGenie alert
type OpsGenieAlert struct {
	Message     string            `json:"message"`
	Alias       string            `json:"alias,omitempty"`
	Description string            `json:"description,omitempty"`
	Responders  []OpsGenieTarget  `json:"responders,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Entity      string            `json:"entity,omitempty"`
	Source      string            `json:"source,omitempty"`
	Priority    string            `json:"priority,omitempty"` // P1-P5
	Note        string            `json:"note,omitempty"`
	Details     map[string]string `json:"details,omitempty"`
}

// OpsGenieTarget represents a responder target
type OpsGenieTarget struct {
	Type string `json:"type"` // team, user, escalation, schedule
	Name string `json:"name,omitempty"`
	ID   string `json:"id,omitempty"`
}

// OpsGenieCloseRequest closes an alert
type OpsGenieCloseRequest struct {
	Source string `json:"source,omitempty"`
	Note   string `json:"note,omitempty"`
}

func (s *OpsGenieSender) Send(channel *Channel, notification *Notification) error {
	var config OpsGenieConfig
	if err := channel.GetConfig(&config); err != nil {
		return fmt.Errorf("invalid opsgenie config: %w", err)
	}

	// Determine API URL based on region
	apiURL := opsGenieUSURL
	if config.Region == "eu" {
		apiURL = opsGenieEUURL
	}

	// Handle resolve by closing the alert
	if notification.Type == NotificationResolved {
		return s.closeAlert(apiURL, config, notification)
	}

	// Build alert
	alert := OpsGenieAlert{
		Message:     notification.Title,
		Alias:       notification.SourceID, // Use source ID for deduplication
		Description: notification.Message,
		Source:      "dogwatch",
		Entity:      notification.Source,
		Priority:    s.mapPriority(notification.Severity, config.Priority),
		Tags:        config.Tags,
		Details:     make(map[string]string),
	}

	// Add details
	if notification.Value != 0 {
		alert.Details["value"] = fmt.Sprintf("%.2f", notification.Value)
	}
	if notification.Threshold != 0 {
		alert.Details["threshold"] = fmt.Sprintf("%.2f", notification.Threshold)
	}
	alert.Details["type"] = string(notification.Type)
	alert.Details["severity"] = string(notification.Severity)

	for k, v := range notification.Labels {
		alert.Details[k] = v
	}

	// Add responders from config
	for _, responder := range config.Responders {
		alert.Responders = append(alert.Responders, OpsGenieTarget{
			Type: "team",
			Name: responder,
		})
	}

	// Send alert
	body, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("failed to marshal alert: %w", err)
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "GenieKey "+config.APIKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode >= 400 {
		return fmt.Errorf("opsgenie returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (s *OpsGenieSender) closeAlert(apiURL string, config OpsGenieConfig, notification *Notification) error {
	if notification.SourceID == "" {
		return fmt.Errorf("source_id required to close alert")
	}

	closeURL := fmt.Sprintf("%s/%s/close?identifierType=alias", apiURL, notification.SourceID)

	closeReq := OpsGenieCloseRequest{
		Source: "dogwatch",
		Note:   "Alert resolved",
	}

	body, err := json.Marshal(closeReq)
	if err != nil {
		return fmt.Errorf("failed to marshal close request: %w", err)
	}

	req, err := http.NewRequest("POST", closeURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "GenieKey "+config.APIKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// 404 is ok - alert may already be closed
	if resp.StatusCode >= 400 && resp.StatusCode != 404 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("opsgenie returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (s *OpsGenieSender) mapPriority(severity Severity, defaultPriority string) string {
	switch severity {
	case SeverityCritical:
		return "P1"
	case SeverityHigh:
		return "P2"
	case SeverityWarning:
		return "P3"
	case SeverityInfo:
		return "P4"
	default:
		if defaultPriority != "" {
			return defaultPriority
		}
		return "P3"
	}
}

func (s *OpsGenieSender) ValidateConfig(config json.RawMessage) error {
	var c OpsGenieConfig
	if err := json.Unmarshal(config, &c); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	if c.APIKey == "" {
		return fmt.Errorf("api_key is required")
	}

	if c.Region != "" && c.Region != "us" && c.Region != "eu" {
		return fmt.Errorf("region must be 'us' or 'eu'")
	}

	if c.Priority != "" {
		valid := map[string]bool{"P1": true, "P2": true, "P3": true, "P4": true, "P5": true}
		if !valid[c.Priority] {
			return fmt.Errorf("priority must be P1-P5")
		}
	}

	return nil
}
