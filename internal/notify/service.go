package notify

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// Service manages notification sending
type Service struct {
	store   *Store
	senders map[ChannelType]Sender
	baseURL string // Base URL for links in notifications
	mu      sync.RWMutex
}

// NewService creates a new notification service
func NewService(store *Store, baseURL string) *Service {
	s := &Service{
		store:   store,
		senders: make(map[ChannelType]Sender),
		baseURL: baseURL,
	}

	// Register built-in senders
	s.RegisterSender(&WebhookSender{})
	s.RegisterSender(&SlackSender{})
	s.RegisterSender(&EmailSender{})
	s.RegisterSender(&PagerDutySender{})
	s.RegisterSender(&OpsGenieSender{})
	s.RegisterSender(&MSTeamsSender{})
	s.RegisterSender(&DiscordSender{})

	return s
}

// RegisterSender registers a sender for a channel type
func (s *Service) RegisterSender(sender Sender) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.senders[sender.Type()] = sender
}

// GetStore returns the notification store
func (s *Service) GetStore() *Store {
	return s.store
}

// Send sends a notification to a specific channel
func (s *Service) Send(channelID string, notification *Notification) error {
	channel, err := s.store.GetChannel(channelID)
	if err != nil {
		return fmt.Errorf("failed to get channel: %w", err)
	}
	if channel == nil {
		return fmt.Errorf("channel not found: %s", channelID)
	}
	if !channel.Enabled {
		return fmt.Errorf("channel is disabled: %s", channel.Name)
	}

	return s.sendToChannel(channel, notification)
}

// SendToChannels sends a notification to multiple channels
func (s *Service) SendToChannels(channelIDs []string, notification *Notification) []error {
	var errors []error
	var wg sync.WaitGroup

	errCh := make(chan error, len(channelIDs))

	for _, id := range channelIDs {
		wg.Add(1)
		go func(channelID string) {
			defer wg.Done()
			if err := s.Send(channelID, notification); err != nil {
				errCh <- fmt.Errorf("channel %s: %w", channelID, err)
			}
		}(id)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		errors = append(errors, err)
	}

	return errors
}

// SendToType sends a notification to all channels of a specific type
func (s *Service) SendToType(channelType ChannelType, notification *Notification) []error {
	channels, err := s.store.ListChannelsByType(channelType)
	if err != nil {
		return []error{err}
	}

	var errors []error
	for _, channel := range channels {
		if err := s.sendToChannel(channel, notification); err != nil {
			errors = append(errors, fmt.Errorf("channel %s: %w", channel.Name, err))
		}
	}

	return errors
}

// sendToChannel sends a notification to a specific channel
func (s *Service) sendToChannel(channel *Channel, notification *Notification) error {
	s.mu.RLock()
	sender, ok := s.senders[channel.Type]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no sender registered for channel type: %s", channel.Type)
	}

	// Add base URL to notification if not set
	if notification.URL == "" && s.baseURL != "" {
		notification.URL = s.baseURL
	}

	// Set timestamp if not set
	if notification.Timestamp.IsZero() {
		notification.Timestamp = time.Now()
	}

	// Send notification and track timing
	start := time.Now()
	err := sender.Send(channel, notification)
	duration := time.Since(start)

	// Log the notification
	logEntry := &NotificationLog{
		ChannelID:    channel.ID,
		ChannelName:  channel.Name,
		ChannelType:  channel.Type,
		Notification: notification,
		SentAt:       time.Now(),
		ResponseTime: duration.Milliseconds(),
	}

	if err != nil {
		logEntry.Status = StatusFailed
		logEntry.Error = err.Error()
		s.store.RecordFailure(channel.ID, err.Error())
		log.Printf("[notify] Failed to send to %s (%s): %v", channel.Name, channel.Type, err)
	} else {
		logEntry.Status = StatusSent
		s.store.RecordSuccess(channel.ID)
		log.Printf("[notify] Sent to %s (%s) in %dms", channel.Name, channel.Type, duration.Milliseconds())
	}

	// Store the log entry
	if logErr := s.store.CreateLog(logEntry); logErr != nil {
		log.Printf("[notify] Failed to log notification: %v", logErr)
	}

	return err
}

// TestChannel sends a test notification to a channel
func (s *Service) TestChannel(channelID string) error {
	channel, err := s.store.GetChannel(channelID)
	if err != nil {
		return fmt.Errorf("failed to get channel: %w", err)
	}
	if channel == nil {
		return fmt.Errorf("channel not found: %s", channelID)
	}

	notification := &Notification{
		Type:     NotificationTest,
		Title:    "Test Notification",
		Message:  "This is a test notification from dogwatch. If you received this, your notification channel is configured correctly!",
		Severity: SeverityInfo,
		Source:   "dogwatch",
		SourceType: "test",
		Timestamp: time.Now(),
	}

	return s.sendToChannel(channel, notification)
}

// ValidateChannelConfig validates a channel configuration
func (s *Service) ValidateChannelConfig(channelType ChannelType, config json.RawMessage) error {
	s.mu.RLock()
	sender, ok := s.senders[channelType]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("unknown channel type: %s", channelType)
	}

	return sender.ValidateConfig(config)
}

// CreateChannel creates a new channel with validation
func (s *Service) CreateChannel(channel *Channel) error {
	// Validate config
	if err := s.ValidateChannelConfig(channel.Type, channel.Config); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	return s.store.CreateChannel(channel)
}

// UpdateChannel updates a channel with validation
func (s *Service) UpdateChannel(channel *Channel) error {
	// Validate config
	if err := s.ValidateChannelConfig(channel.Type, channel.Config); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	return s.store.UpdateChannel(channel)
}

func (s *Service) Close() error {
	return s.store.Close()
}

// Helper functions for creating notifications

// NewAlertNotification creates a notification for an alert
func NewAlertNotification(ruleName, ruleID string, value, threshold float64, severity Severity, labels map[string]string) *Notification {
	return &Notification{
		Type:       NotificationAlert,
		Title:      fmt.Sprintf("Alert: %s", ruleName),
		Message:    fmt.Sprintf("Alert triggered: value %.2f exceeds threshold %.2f", value, threshold),
		Severity:   severity,
		Source:     ruleName,
		SourceType: "alert",
		SourceID:   ruleID,
		Value:      value,
		Threshold:  threshold,
		Labels:     labels,
		Timestamp:  time.Now(),
	}
}

// NewResolvedNotification creates a notification for a resolved alert
func NewResolvedNotification(ruleName, ruleID string, labels map[string]string) *Notification {
	return &Notification{
		Type:       NotificationResolved,
		Title:      fmt.Sprintf("Resolved: %s", ruleName),
		Message:    "Alert has been resolved",
		Severity:   SeverityInfo,
		Source:     ruleName,
		SourceType: "alert",
		SourceID:   ruleID,
		Labels:     labels,
		Timestamp:  time.Now(),
	}
}

// NewIncidentNotification creates a notification for an incident
func NewIncidentNotification(title, incidentID string, severity Severity, description string) *Notification {
	return &Notification{
		Type:       NotificationIncident,
		Title:      title,
		Message:    description,
		Severity:   severity,
		Source:     "incident",
		SourceType: "incident",
		SourceID:   incidentID,
		Timestamp:  time.Now(),
	}
}

// NewEscalationNotification creates a notification for an escalation
func NewEscalationNotification(title, incidentID string, level int, severity Severity) *Notification {
	return &Notification{
		Type:       NotificationEscalated,
		Title:      fmt.Sprintf("Escalation Level %d: %s", level, title),
		Message:    fmt.Sprintf("Incident escalated to level %d", level),
		Severity:   severity,
		Source:     "escalation",
		SourceType: "incident",
		SourceID:   incidentID,
		Timestamp:  time.Now(),
	}
}
