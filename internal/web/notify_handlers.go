package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"dogwatch/internal/notify"
	"dogwatch/internal/rbac"
)

var notifyService *notify.Service

// SetNotifyService sets the notification service for handlers
func SetNotifyService(svc *notify.Service) {
	notifyService = svc
}

// handleNotifyChannels handles /api/notify/channels
func (s *Server) handleNotifyChannels(w http.ResponseWriter, r *http.Request) {
	if notifyService == nil {
		http.Error(w, `{"error":"notification service not configured"}`, http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case "GET":
		s.listNotifyChannels(w, r)
	case "POST":
		s.createNotifyChannel(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleNotifyChannel handles /api/notify/channels/{id}
func (s *Server) handleNotifyChannel(w http.ResponseWriter, r *http.Request) {
	if notifyService == nil {
		http.Error(w, `{"error":"notification service not configured"}`, http.StatusServiceUnavailable)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/notify/channels/")

	// Check for /test suffix
	if strings.HasSuffix(id, "/test") {
		id = strings.TrimSuffix(id, "/test")
		if r.Method == "POST" {
			s.testNotifyChannel(w, r, id)
			return
		}
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	switch r.Method {
	case "GET":
		s.getNotifyChannel(w, r, id)
	case "PUT":
		s.updateNotifyChannel(w, r, id)
	case "DELETE":
		s.deleteNotifyChannel(w, r, id)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleNotifyHistory handles /api/notify/history
func (s *Server) handleNotifyHistory(w http.ResponseWriter, r *http.Request) {
	if notifyService == nil {
		http.Error(w, `{"error":"notification service not configured"}`, http.StatusServiceUnavailable)
		return
	}

	if r.Method != "GET" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	channelID := r.URL.Query().Get("channel_id")
	logs, err := notifyService.GetStore().ListLogs(100, channelID)
	if err != nil {
		http.Error(w, `{"error":"failed to list logs"}`, http.StatusInternalServerError)
		return
	}

	if logs == nil {
		logs = []*notify.NotificationLog{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

func (s *Server) listNotifyChannels(w http.ResponseWriter, r *http.Request) {
	// Get org ID from context if available
	orgID := ""
	if user := rbac.GetUserFromContext(r.Context()); user != nil {
		orgID = user.OrgID
	}

	channels, err := notifyService.GetStore().ListChannels(orgID)
	if err != nil {
		http.Error(w, `{"error":"failed to list channels"}`, http.StatusInternalServerError)
		return
	}

	if channels == nil {
		channels = []*notify.Channel{}
	}

	// Mask sensitive config fields
	for _, ch := range channels {
		maskSensitiveConfig(ch)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(channels)
}

func (s *Server) createNotifyChannel(w http.ResponseWriter, r *http.Request) {
	var channel notify.Channel
	if err := json.NewDecoder(r.Body).Decode(&channel); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	// Set org ID from context
	if user := rbac.GetUserFromContext(r.Context()); user != nil {
		channel.OrgID = user.OrgID
	}

	// Validate and create
	if err := notifyService.CreateChannel(&channel); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	// Mask sensitive fields before returning
	maskSensitiveConfig(&channel)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(channel)
}

func (s *Server) getNotifyChannel(w http.ResponseWriter, r *http.Request, id string) {
	channel, err := notifyService.GetStore().GetChannel(id)
	if err != nil {
		http.Error(w, `{"error":"failed to get channel"}`, http.StatusInternalServerError)
		return
	}
	if channel == nil {
		http.Error(w, `{"error":"channel not found"}`, http.StatusNotFound)
		return
	}

	maskSensitiveConfig(channel)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(channel)
}

func (s *Server) updateNotifyChannel(w http.ResponseWriter, r *http.Request, id string) {
	// Get existing channel
	existing, err := notifyService.GetStore().GetChannel(id)
	if err != nil {
		http.Error(w, `{"error":"failed to get channel"}`, http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.Error(w, `{"error":"channel not found"}`, http.StatusNotFound)
		return
	}

	var channel notify.Channel
	if err := json.NewDecoder(r.Body).Decode(&channel); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	// Preserve ID and OrgID
	channel.ID = id
	channel.OrgID = existing.OrgID
	channel.CreatedAt = existing.CreatedAt

	// Validate and update
	if err := notifyService.UpdateChannel(&channel); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	maskSensitiveConfig(&channel)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(channel)
}

func (s *Server) deleteNotifyChannel(w http.ResponseWriter, r *http.Request, id string) {
	if err := notifyService.GetStore().DeleteChannel(id); err != nil {
		http.Error(w, `{"error":"failed to delete channel"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) testNotifyChannel(w http.ResponseWriter, r *http.Request, id string) {
	if err := notifyService.TestChannel(id); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "test notification sent"})
}

// maskSensitiveConfig masks sensitive fields in channel config
func maskSensitiveConfig(channel *notify.Channel) {
	if channel == nil || channel.Config == nil {
		return
	}

	var config map[string]interface{}
	if err := json.Unmarshal(channel.Config, &config); err != nil {
		return
	}

	// List of sensitive fields to mask
	sensitiveFields := []string{
		"password", "api_key", "apikey", "secret", "token",
		"bot_token", "integration_key", "service_key",
	}

	for _, field := range sensitiveFields {
		if _, ok := config[field]; ok {
			config[field] = "********"
		}
	}

	// Re-marshal
	masked, err := json.Marshal(config)
	if err != nil {
		return
	}
	channel.Config = masked
}

// GetNotifyService returns the notification service
func GetNotifyService() *notify.Service {
	return notifyService
}
