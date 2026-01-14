# Real Notifications Implementation Plan

## Current State

**What exists:**
- `watch/notifier.go` - Working webhook and Slack notifications for watches
- `alerting/router.go` - Stubbed receivers (Webhook, Slack, Email, PagerDuty, OpsGenie, OnCall) that only log
- `incidents/pager.go` - Slack and webhook notifications for incidents
- `/api/watch/channels` - API endpoints for watch notification channels

**The problem:** Alerting receivers are stubs, no unified channel management, no Email/PagerDuty/Teams actual implementation.

---

## Implementation Plan

### 1. Create Unified Notification Service (`internal/notify/`)

**Files to create:**
- `notify/service.go` - Main notification service
- `notify/channels.go` - Channel type definitions
- `notify/store.go` - SQLite storage for channels
- `notify/webhook.go` - Webhook sender
- `notify/slack.go` - Slack sender (webhook + API)
- `notify/email.go` - SMTP email sender
- `notify/pagerduty.go` - PagerDuty Events API v2
- `notify/opsgenie.go` - OpsGenie Alerts API
- `notify/msteams.go` - Microsoft Teams webhooks
- `notify/discord.go` - Discord webhooks

**Channel Model:**
```go
type Channel struct {
    ID          string
    OrgID       string          // Multi-tenant support
    Name        string
    Type        ChannelType     // webhook, slack, email, pagerduty, opsgenie, msteams, discord
    Config      json.RawMessage // Type-specific config
    Enabled     bool
    CreatedAt   time.Time
    UpdatedAt   time.Time
    LastUsedAt  *time.Time
    LastError   string          // Last send error if any
}

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
```

**Notification Interface:**
```go
type Notification struct {
    Type        NotificationType // alert, incident, resolved, test
    Title       string
    Message     string
    Severity    string
    Source      string           // alert rule name, watch name, etc.
    Labels      map[string]string
    URL         string           // Link to dashboard
    Timestamp   time.Time
}

type Sender interface {
    Send(channel *Channel, notification *Notification) error
    Test(channel *Channel) error
}
```

### 2. Channel Configurations

**Webhook:**
```go
type WebhookConfig struct {
    URL         string
    Method      string            // POST, PUT
    Headers     map[string]string
    BasicAuth   *BasicAuth        // Optional
    TLSInsecure bool
}
```

**Slack:**
```go
type SlackConfig struct {
    WebhookURL  string // Incoming webhook
    // OR
    BotToken    string // For API-based (better)
    Channel     string // #channel or @user
    Username    string
    IconEmoji   string
}
```

**Email (SMTP):**
```go
type EmailConfig struct {
    SMTPHost    string
    SMTPPort    int
    Username    string
    Password    string  // Encrypted at rest
    From        string
    To          []string
    TLS         bool
}
```

**PagerDuty:**
```go
type PagerDutyConfig struct {
    IntegrationKey string // Events API v2 routing key
    Severity       string // Default severity mapping
}
```

**OpsGenie:**
```go
type OpsGenieConfig struct {
    APIKey   string
    Region   string // us or eu
    Priority string // P1-P5
    Tags     []string
}
```

**Microsoft Teams:**
```go
type MSTeamsConfig struct {
    WebhookURL string
}
```

**Discord:**
```go
type DiscordConfig struct {
    WebhookURL string
    Username   string
}
```

### 3. API Endpoints

```
GET    /api/notify/channels          - List all channels
POST   /api/notify/channels          - Create channel
GET    /api/notify/channels/{id}     - Get channel
PUT    /api/notify/channels/{id}     - Update channel
DELETE /api/notify/channels/{id}     - Delete channel
POST   /api/notify/channels/{id}/test - Test channel
GET    /api/notify/history           - Notification history
```

### 4. UI Updates (`index.html`)

- Add "Notification Channels" widget or modal accessible from settings
- Channel list with type icons, status, last used
- Create/edit channel forms for each type
- Test button for each channel
- Notification history view

### 5. Integration Points

**A. Replace alerting/router.go stub receivers:**
```go
// Instead of:
func (r *SlackReceiver) Send(group *AlertGroup) error {
    log.Printf("[alert-slack] Would send...")
    return nil
}

// Inject actual NotifyService and use it:
func (r *SlackReceiver) Send(group *AlertGroup) error {
    return r.notifyService.SendToChannel(r.channelID, buildNotification(group))
}
```

**B. Hook into incidents/pager.go:**
- Replace direct Slack/webhook sends with NotifyService
- Use configured channels from escalation rules

**C. Alert Rule Channel References:**
- Alert rules already have `NotifyChannels []string`
- Wire these to the new channel IDs

### 6. Notification Templating

Simple Go templates for message formatting:
```go
type NotificationTemplate struct {
    Title   string // "{{.Severity}}: {{.Title}}"
    Body    string // Rich template
}
```

### 7. Notification History/Logging

```go
type NotificationLog struct {
    ID           string
    ChannelID    string
    ChannelType  string
    Notification *Notification
    Status       string    // sent, failed, delivered
    Error        string
    SentAt       time.Time
    ResponseTime time.Duration
}
```

---

## Implementation Order

1. **Create notify package core** - service.go, channels.go, store.go
2. **Implement Webhook sender** - webhook.go (simplest, test the architecture)
3. **Implement Slack sender** - slack.go (with rich formatting)
4. **Add API endpoints** - handlers in web/
5. **Add UI** - channel management modal
6. **Implement Email sender** - email.go (SMTP)
7. **Implement PagerDuty** - pagerduty.go (Events API v2)
8. **Implement OpsGenie** - opsgenie.go
9. **Implement MS Teams** - msteams.go
10. **Implement Discord** - discord.go
11. **Wire into alerting router** - replace stubs
12. **Wire into incident pager** - use channels
13. **Add notification history UI**

---

## Estimated Scope

- ~1000-1500 lines of Go code
- ~300-400 lines of HTML/JS for UI
- 7 new files in `internal/notify/`
- 1 new handler file `web/notify_handlers.go`
- Updates to `index.html` for UI
