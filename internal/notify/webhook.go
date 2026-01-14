package notify

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// WebhookSender sends notifications via HTTP webhooks
type WebhookSender struct{}

func (s *WebhookSender) Type() ChannelType {
	return ChannelWebhook
}

// WebhookPayload is the JSON payload sent to webhook endpoints
type WebhookPayload struct {
	// Notification metadata
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`

	// Alert info
	Title     string            `json:"title"`
	Message   string            `json:"message"`
	Severity  string            `json:"severity"`
	Source    string            `json:"source"`
	SourceType string           `json:"source_type"`
	SourceID  string            `json:"source_id"`
	URL       string            `json:"url,omitempty"`
	DedupKey  string            `json:"dedup_key,omitempty"`

	// Metric info (for alerts)
	Value     float64           `json:"value,omitempty"`
	Threshold float64           `json:"threshold,omitempty"`

	// Labels and annotations
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

func (s *WebhookSender) Send(channel *Channel, notification *Notification) error {
	var config WebhookConfig
	if err := channel.GetConfig(&config); err != nil {
		return fmt.Errorf("invalid webhook config: %w", err)
	}

	// Build payload
	payload := WebhookPayload{
		Type:       string(notification.Type),
		Timestamp:  notification.Timestamp,
		Title:      notification.Title,
		Message:    notification.Message,
		Severity:   string(notification.Severity),
		Source:     notification.Source,
		SourceType: notification.SourceType,
		SourceID:   notification.SourceID,
		URL:        notification.URL,
		DedupKey:   notification.DedupKey,
		Value:      notification.Value,
		Threshold:  notification.Threshold,
		Labels:     notification.Labels,
		Annotations: notification.Annotations,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Determine method
	method := config.Method
	if method == "" {
		method = "POST"
	}

	// Create request
	req, err := http.NewRequest(method, config.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "dogwatch/1.0")
	req.Header.Set("X-Dogwatch-Event", string(notification.Type))

	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	// Set basic auth if configured
	if config.Username != "" {
		req.SetBasicAuth(config.Username, config.Password)
	}

	// Create client with timeout
	timeout := time.Duration(config.Timeout) * time.Second
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	transport := &http.Transport{}
	if config.TLSInsecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (s *WebhookSender) ValidateConfig(config json.RawMessage) error {
	var c WebhookConfig
	if err := json.Unmarshal(config, &c); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	if c.URL == "" {
		return fmt.Errorf("url is required")
	}

	// Validate method
	if c.Method != "" && c.Method != "POST" && c.Method != "PUT" && c.Method != "PATCH" {
		return fmt.Errorf("method must be POST, PUT, or PATCH")
	}

	return nil
}
