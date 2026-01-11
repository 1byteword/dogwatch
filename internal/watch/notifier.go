package watch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Notifier sends notifications via various channels
type Notifier struct {
	client *http.Client
}

// NewNotifier creates a new notifier
func NewNotifier() *Notifier {
	return &Notifier{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Send dispatches a notification to a channel
func (n *Notifier) Send(channel *Channel, watch *Watch, event *Event) error {
	switch channel.Type {
	case "webhook":
		return n.sendWebhook(channel, watch, event)
	case "slack":
		return n.sendSlack(channel, watch, event)
	default:
		return fmt.Errorf("unknown channel type: %s", channel.Type)
	}
}

// WebhookPayload is the payload sent to webhook endpoints
type WebhookPayload struct {
	Event     string    `json:"event"`
	Watch     string    `json:"watch"`
	WatchID   string    `json:"watch_id"`
	State     State     `json:"state"`
	PrevState State     `json:"prev_state"`
	Metric    string    `json:"metric"`
	Value     float64   `json:"value"`
	Threshold float64   `json:"threshold"`
	Operator  string    `json:"operator"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

func (n *Notifier) sendWebhook(channel *Channel, watch *Watch, event *Event) error {
	var config WebhookConfig
	if err := json.Unmarshal(channel.Config, &config); err != nil {
		return fmt.Errorf("invalid webhook config: %w", err)
	}

	payload := WebhookPayload{
		Event:     string(event.ToState),
		Watch:     watch.Name,
		WatchID:   watch.ID,
		State:     event.ToState,
		PrevState: event.FromState,
		Metric:    string(watch.Metric),
		Value:     event.Value,
		Threshold: watch.Threshold,
		Operator:  string(watch.Operator),
		Message:   event.Message,
		Timestamp: event.Timestamp,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	method := config.Method
	if method == "" {
		method = "POST"
	}

	req, err := http.NewRequest(method, config.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "dogwatch/1.0")

	for k, v := range config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// SlackMessage represents a Slack message
type SlackMessage struct {
	Channel     string            `json:"channel,omitempty"`
	Username    string            `json:"username,omitempty"`
	IconEmoji   string            `json:"icon_emoji,omitempty"`
	Attachments []SlackAttachment `json:"attachments"`
}

// SlackAttachment represents a Slack attachment
type SlackAttachment struct {
	Color      string       `json:"color"`
	Title      string       `json:"title"`
	Text       string       `json:"text"`
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

func (n *Notifier) sendSlack(channel *Channel, watch *Watch, event *Event) error {
	var config SlackConfig
	if err := json.Unmarshal(channel.Config, &config); err != nil {
		return fmt.Errorf("invalid slack config: %w", err)
	}

	color := "#36a64f" // green for OK
	emoji := ":white_check_mark:"
	title := fmt.Sprintf("%s Recovered", watch.Name)

	if event.ToState == StateAlerting {
		color = "#dc3545" // red for alerting
		emoji = ":rotating_light:"
		title = fmt.Sprintf("%s Alert", watch.Name)
	}

	msg := SlackMessage{
		Channel:   config.Channel,
		Username:  config.Username,
		IconEmoji: ":dog:",
		Attachments: []SlackAttachment{
			{
				Color: color,
				Title: fmt.Sprintf("%s %s", emoji, title),
				Text:  event.Message,
				Fields: []SlackField{
					{Title: "Metric", Value: string(watch.Metric), Short: true},
					{Title: "Value", Value: fmt.Sprintf("%.2f", event.Value), Short: true},
					{Title: "Threshold", Value: fmt.Sprintf("%s %.2f", watch.Operator, watch.Threshold), Short: true},
					{Title: "State", Value: string(event.ToState), Short: true},
				},
				Footer:     "dogwatch",
				FooterIcon: "https://example.com/dogwatch.png",
				Ts:         event.Timestamp.Unix(),
			},
		},
	}

	if msg.Username == "" {
		msg.Username = "dogwatch"
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	resp, err := n.client.Post(config.WebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("slack returned status %d", resp.StatusCode)
	}

	return nil
}

// TestChannel sends a test notification
func (n *Notifier) TestChannel(channel *Channel) error {
	testWatch := &Watch{
		ID:        "test",
		Name:      "Test Watch",
		Metric:    MetricCPU,
		Operator:  OpGreaterThan,
		Threshold: 80,
	}

	testEvent := &Event{
		ID:        "test",
		WatchID:   "test",
		WatchName: "Test Watch",
		FromState: StateOK,
		ToState:   StateAlerting,
		Value:     85.5,
		Threshold: 80,
		Message:   "This is a test notification from dogwatch",
		Timestamp: time.Now(),
	}

	return n.Send(channel, testWatch, testEvent)
}
