package entity

import (
	"testing"
	"time"
)

func TestNewSynthesizer(t *testing.T) {
	cfg := DefaultConfig()
	s := NewSynthesizer(cfg)

	if s == nil {
		t.Fatal("expected non-nil synthesizer")
	}

	if s.signalWindow != cfg.SignalWindow {
		t.Errorf("expected signal window %v, got %v", cfg.SignalWindow, s.signalWindow)
	}
}

func TestProcessSpan_DiscoveryService(t *testing.T) {
	s := NewSynthesizer(DefaultConfig())

	span := TraceSpan{
		TraceID:       "trace-123",
		SpanID:        "span-456",
		ServiceName:   "payment-service",
		OperationName: "processPayment",
		SpanKind:      "SERVER",
		Duration:      100 * time.Millisecond,
		StatusCode:    200,
		Timestamp:     time.Now(),
	}

	s.ProcessSpan(span)

	// Check service was created
	entities := s.ListEntities(TypeService)
	if len(entities) != 1 {
		t.Fatalf("expected 1 service entity, got %d", len(entities))
	}

	if entities[0].Name != "payment-service" {
		t.Errorf("expected service name 'payment-service', got %s", entities[0].Name)
	}
}

func TestProcessSpan_DiscoverDatabase(t *testing.T) {
	s := NewSynthesizer(DefaultConfig())

	span := TraceSpan{
		TraceID:       "trace-123",
		SpanID:        "span-456",
		ServiceName:   "payment-service",
		OperationName: "SELECT * FROM payments",
		SpanKind:      "CLIENT",
		Duration:      50 * time.Millisecond,
		StatusCode:    200,
		Timestamp:     time.Now(),
		Attributes: map[string]string{
			"db.system": "postgresql",
			"db.name":   "payments_db",
		},
	}

	s.ProcessSpan(span)

	// Check database was created
	dbEntities := s.ListEntities(TypeDatabase)
	if len(dbEntities) != 1 {
		t.Fatalf("expected 1 database entity, got %d", len(dbEntities))
	}

	if dbEntities[0].Name != "payments_db" {
		t.Errorf("expected db name 'payments_db', got %s", dbEntities[0].Name)
	}

	// Check relationship was created
	svcEntity := s.GetEntityByName(TypeService, "payment-service")
	if svcEntity == nil {
		t.Fatal("expected service entity to exist")
	}

	rels := svcEntity.GetRelationships()
	found := false
	for _, r := range rels {
		if r.Type == RelDependsOn && r.TargetType == TypeDatabase {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected DEPENDS_ON relationship to database")
	}
}

func TestProcessMetric_DiscoverHost(t *testing.T) {
	s := NewSynthesizer(DefaultConfig())

	metric := MetricData{
		Name:  "cpu_percent",
		Value: 65.5,
		Labels: map[string]string{
			"host": "server-01",
		},
		Timestamp: time.Now(),
	}

	s.ProcessMetric(metric)

	entities := s.ListEntities(TypeHost)
	if len(entities) != 1 {
		t.Fatalf("expected 1 host entity, got %d", len(entities))
	}

	if entities[0].Name != "server-01" {
		t.Errorf("expected host name 'server-01', got %s", entities[0].Name)
	}

	// Check saturation was set
	signals := entities[0].GetSignals()
	if signals.Saturation != 65.5 {
		t.Errorf("expected saturation 65.5, got %f", signals.Saturation)
	}
}

func TestProcessK8sPod(t *testing.T) {
	s := NewSynthesizer(DefaultConfig())

	pod := K8sPod{
		Name:       "payment-service-abc123",
		Namespace:  "production",
		NodeName:   "node-1",
		Containers: []string{"payment-api", "sidecar"},
		Labels: map[string]string{
			"app": "payment-service",
		},
	}

	s.ProcessK8sPod(pod)

	// Check container entities were created
	containers := s.ListEntities(TypeContainer)
	if len(containers) != 2 {
		t.Fatalf("expected 2 container entities, got %d", len(containers))
	}

	// Check host entity was created
	hosts := s.ListEntities(TypeHost)
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host entity, got %d", len(hosts))
	}

	// Check service entity was created from app label
	services := s.ListEntities(TypeService)
	if len(services) != 1 {
		t.Fatalf("expected 1 service entity, got %d", len(services))
	}
}

func TestGetRelatedEntities(t *testing.T) {
	s := NewSynthesizer(DefaultConfig())

	// Create service
	span1 := TraceSpan{
		ServiceName: "api-gateway",
		SpanKind:    "SERVER",
		Duration:    10 * time.Millisecond,
		Timestamp:   time.Now(),
	}
	s.ProcessSpan(span1)

	// Create downstream service call
	span2 := TraceSpan{
		ServiceName:   "api-gateway",
		OperationName: "callPayment",
		SpanKind:      "CLIENT",
		Duration:      50 * time.Millisecond,
		Timestamp:     time.Now(),
		Attributes: map[string]string{
			"peer.service": "payment-service",
		},
	}
	s.ProcessSpan(span2)

	// Get related entities
	apiEntity := s.GetEntityByName(TypeService, "api-gateway")
	related := s.GetRelatedEntities(apiEntity.ID)

	if len(related) != 1 {
		t.Fatalf("expected 1 related entity, got %d", len(related))
	}

	if related[0].Name != "payment-service" {
		t.Errorf("expected related entity 'payment-service', got %s", related[0].Name)
	}
}

func TestEntityHealth(t *testing.T) {
	e := &Entity{
		Signals: GoldenSignals{
			ErrorRate:  0.5,
			Saturation: 50,
			TotalCount: 100,
		},
	}

	health := e.ComputeHealth()
	if health != HealthHealthy {
		t.Errorf("expected healthy, got %s", health)
	}

	// High error rate
	e.Signals.ErrorRate = 6
	health = e.ComputeHealth()
	if health != HealthUnhealthy {
		t.Errorf("expected unhealthy with 6%% error rate, got %s", health)
	}

	// Reset error rate, high saturation
	e.Signals.ErrorRate = 0.5
	e.Signals.Saturation = 96
	health = e.ComputeHealth()
	if health != HealthUnhealthy {
		t.Errorf("expected unhealthy with 96%% saturation, got %s", health)
	}

	// Degraded
	e.Signals.Saturation = 85
	health = e.ComputeHealth()
	if health != HealthDegraded {
		t.Errorf("expected degraded with 85%% saturation, got %s", health)
	}
}

func TestIsExternalHost(t *testing.T) {
	tests := []struct {
		host     string
		expected bool
	}{
		{"localhost", false},
		{"127.0.0.1", false},
		{"10.0.0.1", false},
		{"192.168.1.1", false},
		{"service.local", false},
		{"myapp.svc.cluster.local", false},
		{"api.stripe.com", true},
		{"s3.amazonaws.com", true},
		{"github.com", true},
	}

	for _, tt := range tests {
		result := isExternalHost(tt.host)
		if result != tt.expected {
			t.Errorf("isExternalHost(%q) = %v, expected %v", tt.host, result, tt.expected)
		}
	}
}

func TestMakeID(t *testing.T) {
	id := makeID(TypeService, "my-service")
	expected := "SERVICE:my-service"
	if id != expected {
		t.Errorf("makeID() = %q, expected %q", id, expected)
	}
}

func TestFormatDisplayName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"payment-service", "Payment Service"},
		{"user_api", "User Api"},
		{"simple", "Simple"},
		{"UPPERCASE", "Uppercase"},
	}

	for _, tt := range tests {
		result := formatDisplayName(tt.input)
		if result != tt.expected {
			t.Errorf("formatDisplayName(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestListEntities_TypeFilter(t *testing.T) {
	s := NewSynthesizer(DefaultConfig())

	// Create various entities
	s.ProcessSpan(TraceSpan{ServiceName: "svc1", SpanKind: "SERVER", Timestamp: time.Now()})
	s.ProcessMetric(MetricData{Name: "cpu", Labels: map[string]string{"host": "host1"}})
	s.ProcessMetric(MetricData{Name: "cpu", Labels: map[string]string{"container": "container1"}})

	// List all
	all := s.ListEntities("")
	if len(all) != 3 {
		t.Errorf("expected 3 entities, got %d", len(all))
	}

	// List services only
	services := s.ListEntities(TypeService)
	if len(services) != 1 {
		t.Errorf("expected 1 service, got %d", len(services))
	}

	// List hosts only
	hosts := s.ListEntities(TypeHost)
	if len(hosts) != 1 {
		t.Errorf("expected 1 host, got %d", len(hosts))
	}
}
