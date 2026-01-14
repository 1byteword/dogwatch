package notify

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html/template"
	"net/smtp"
	"strings"
)

// EmailSender sends notifications via SMTP email
type EmailSender struct{}

func (s *EmailSender) Type() ChannelType {
	return ChannelEmail
}

// Email template
var emailTemplate = template.Must(template.New("email").Parse(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 0; padding: 20px; background: #f5f5f5; }
        .container { max-width: 600px; margin: 0 auto; background: white; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.1); }
        .header { padding: 20px; color: white; }
        .header.critical { background: #dc2626; }
        .header.high { background: #ea580c; }
        .header.warning { background: #ca8a04; }
        .header.info { background: #2563eb; }
        .header.resolved { background: #22c55e; }
        .header h1 { margin: 0; font-size: 20px; }
        .content { padding: 20px; }
        .message { color: #374151; line-height: 1.6; }
        .details { margin-top: 20px; border-top: 1px solid #e5e7eb; padding-top: 20px; }
        .detail-row { display: flex; margin-bottom: 8px; }
        .detail-label { font-weight: 600; color: #6b7280; width: 120px; }
        .detail-value { color: #111827; }
        .labels { margin-top: 15px; }
        .label { display: inline-block; background: #e5e7eb; color: #374151; padding: 4px 8px; border-radius: 4px; font-size: 12px; margin-right: 4px; margin-bottom: 4px; }
        .footer { padding: 15px 20px; background: #f9fafb; border-top: 1px solid #e5e7eb; font-size: 12px; color: #6b7280; }
        .button { display: inline-block; background: #2563eb; color: white; text-decoration: none; padding: 10px 20px; border-radius: 6px; margin-top: 15px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header {{.HeaderClass}}">
            <h1>{{.Emoji}} {{.Title}}</h1>
        </div>
        <div class="content">
            <div class="message">{{.Message}}</div>

            <div class="details">
                {{if .Source}}
                <div class="detail-row">
                    <span class="detail-label">Source:</span>
                    <span class="detail-value">{{.Source}}</span>
                </div>
                {{end}}
                {{if .Severity}}
                <div class="detail-row">
                    <span class="detail-label">Severity:</span>
                    <span class="detail-value">{{.Severity}}</span>
                </div>
                {{end}}
                {{if .Value}}
                <div class="detail-row">
                    <span class="detail-label">Value:</span>
                    <span class="detail-value">{{.Value}}</span>
                </div>
                {{end}}
                {{if .Threshold}}
                <div class="detail-row">
                    <span class="detail-label">Threshold:</span>
                    <span class="detail-value">{{.Threshold}}</span>
                </div>
                {{end}}
                <div class="detail-row">
                    <span class="detail-label">Time:</span>
                    <span class="detail-value">{{.Timestamp}}</span>
                </div>
            </div>

            {{if .Labels}}
            <div class="labels">
                {{range $key, $value := .Labels}}
                <span class="label">{{$key}}: {{$value}}</span>
                {{end}}
            </div>
            {{end}}

            {{if .URL}}
            <a href="{{.URL}}" class="button">View in Dashboard</a>
            {{end}}
        </div>
        <div class="footer">
            This notification was sent by dogwatch
        </div>
    </div>
</body>
</html>
`))

type emailData struct {
	Title       string
	Emoji       string
	Message     string
	Source      string
	Severity    string
	Value       string
	Threshold   string
	Timestamp   string
	Labels      map[string]string
	URL         string
	HeaderClass string
}

func (s *EmailSender) Send(channel *Channel, notification *Notification) error {
	var config EmailConfig
	if err := channel.GetConfig(&config); err != nil {
		return fmt.Errorf("invalid email config: %w", err)
	}

	// Build email
	subject := s.buildSubject(config, notification)
	body, err := s.buildBody(notification)
	if err != nil {
		return fmt.Errorf("failed to build email body: %w", err)
	}

	// Build message
	var msg bytes.Buffer
	msg.WriteString(fmt.Sprintf("From: %s\r\n", config.From))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(config.To, ", ")))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/html; charset=utf-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)

	// Connect to SMTP server
	addr := fmt.Sprintf("%s:%d", config.SMTPHost, config.SMTPPort)

	var auth smtp.Auth
	if config.Username != "" {
		auth = smtp.PlainAuth("", config.Username, config.Password, config.SMTPHost)
	}

	// Send with TLS or StartTLS
	if config.TLS {
		return s.sendWithTLS(addr, config, auth, msg.Bytes())
	}

	return smtp.SendMail(addr, auth, config.From, config.To, msg.Bytes())
}

func (s *EmailSender) sendWithTLS(addr string, config EmailConfig, auth smtp.Auth, msg []byte) error {
	tlsConfig := &tls.Config{
		ServerName:         config.SMTPHost,
		InsecureSkipVerify: config.SkipVerify,
	}

	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("TLS connection failed: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, config.SMTPHost)
	if err != nil {
		return fmt.Errorf("SMTP client creation failed: %w", err)
	}
	defer client.Close()

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP auth failed: %w", err)
		}
	}

	if err := client.Mail(config.From); err != nil {
		return fmt.Errorf("MAIL command failed: %w", err)
	}

	for _, to := range config.To {
		if err := client.Rcpt(to); err != nil {
			return fmt.Errorf("RCPT command failed for %s: %w", to, err)
		}
	}

	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA command failed: %w", err)
	}

	if _, err := wc.Write(msg); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	if err := wc.Close(); err != nil {
		return fmt.Errorf("close failed: %w", err)
	}

	return client.Quit()
}

func (s *EmailSender) buildSubject(config EmailConfig, notification *Notification) string {
	if config.Subject != "" {
		// Simple template replacement
		subject := config.Subject
		subject = strings.ReplaceAll(subject, "{{.Title}}", notification.Title)
		subject = strings.ReplaceAll(subject, "{{.Severity}}", string(notification.Severity))
		subject = strings.ReplaceAll(subject, "{{.Source}}", notification.Source)
		return subject
	}

	// Default subject
	prefix := ""
	switch notification.Type {
	case NotificationAlert:
		prefix = "[ALERT]"
	case NotificationResolved:
		prefix = "[RESOLVED]"
	case NotificationIncident:
		prefix = "[INCIDENT]"
	case NotificationEscalated:
		prefix = "[ESCALATED]"
	case NotificationTest:
		prefix = "[TEST]"
	}

	return fmt.Sprintf("%s %s", prefix, notification.Title)
}

func (s *EmailSender) buildBody(notification *Notification) (string, error) {
	headerClass := string(notification.Severity)
	if notification.Type == NotificationResolved {
		headerClass = "resolved"
	}

	data := emailData{
		Title:       notification.Title,
		Emoji:       notification.Type.Emoji(),
		Message:     notification.Message,
		Source:      notification.Source,
		Severity:    string(notification.Severity),
		Timestamp:   notification.Timestamp.Format("2006-01-02 15:04:05 MST"),
		Labels:      notification.Labels,
		URL:         notification.URL,
		HeaderClass: headerClass,
	}

	if notification.Value != 0 {
		data.Value = fmt.Sprintf("%.2f", notification.Value)
	}
	if notification.Threshold != 0 {
		data.Threshold = fmt.Sprintf("%.2f", notification.Threshold)
	}

	var buf bytes.Buffer
	if err := emailTemplate.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (s *EmailSender) ValidateConfig(config json.RawMessage) error {
	var c EmailConfig
	if err := json.Unmarshal(config, &c); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	if c.SMTPHost == "" {
		return fmt.Errorf("smtp_host is required")
	}
	if c.SMTPPort == 0 {
		return fmt.Errorf("smtp_port is required")
	}
	if c.From == "" {
		return fmt.Errorf("from is required")
	}
	if len(c.To) == 0 {
		return fmt.Errorf("at least one recipient (to) is required")
	}

	return nil
}
