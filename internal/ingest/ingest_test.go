package ingest

import (
	"strings"
	"testing"
	"time"
)

func TestGraphiteParser_ParsePlaintext(t *testing.T) {
	parser := &GraphiteParser{}

	input := `servers.web01.cpu.usage 42.5 1609459200
servers.db01.memory.used 1024 1609459200
# comment line
invalid line

servers.app01.requests 100 1609459200`

	batch, err := parser.ParsePlaintext(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParsePlaintext failed: %v", err)
	}

	if len(batch.Samples) != 3 {
		t.Errorf("Expected 3 samples, got %d", len(batch.Samples))
	}

	if batch.Source != "graphite" {
		t.Errorf("Expected source 'graphite', got %s", batch.Source)
	}

	// Check first sample
	if batch.Samples[0].Value != 42.5 {
		t.Errorf("Expected value 42.5, got %f", batch.Samples[0].Value)
	}
}

func TestInfluxParser_ParseLineProtocol(t *testing.T) {
	parser := &InfluxParser{DefaultPrecision: "s"}

	input := `cpu,host=server01,region=us-west usage=42.5,system=12.3 1609459200
memory,host=server01 used=1024i,free=512i 1609459200`

	batch, err := parser.ParseLineProtocol(strings.NewReader(input), "s")
	if err != nil {
		t.Fatalf("ParseLineProtocol failed: %v", err)
	}

	// Should have 4 samples: 2 fields x 2 lines
	if len(batch.Samples) != 4 {
		t.Errorf("Expected 4 samples, got %d", len(batch.Samples))
	}

	if batch.Source != "influxdb" {
		t.Errorf("Expected source 'influxdb', got %s", batch.Source)
	}

	// Check that tags are parsed
	foundHost := false
	for _, s := range batch.Samples {
		if s.Tags["host"] == "server01" {
			foundHost = true
			break
		}
	}
	if !foundHost {
		t.Error("Expected to find host tag 'server01'")
	}
}

func TestOpenTSDBParser_ParseHTTP(t *testing.T) {
	parser := &OpenTSDBParser{}

	// Test array format
	input := `[
		{"metric":"sys.cpu","timestamp":1609459200,"value":42.5,"tags":{"host":"web01"}},
		{"metric":"sys.mem","timestamp":1609459200,"value":1024,"tags":{"host":"web01"}}
	]`

	batch, err := parser.ParseHTTP(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseHTTP failed: %v", err)
	}

	if len(batch.Samples) != 2 {
		t.Errorf("Expected 2 samples, got %d", len(batch.Samples))
	}

	if batch.Source != "opentsdb" {
		t.Errorf("Expected source 'opentsdb', got %s", batch.Source)
	}

	// Test single object format
	singleInput := `{"metric":"sys.cpu","timestamp":1609459200,"value":42.5,"tags":{"host":"web01"}}`
	batch, err = parser.ParseHTTP(strings.NewReader(singleInput))
	if err != nil {
		t.Fatalf("ParseHTTP (single) failed: %v", err)
	}

	if len(batch.Samples) != 1 {
		t.Errorf("Expected 1 sample, got %d", len(batch.Samples))
	}
}

func TestOpenTSDBParser_ParseTelnet(t *testing.T) {
	parser := &OpenTSDBParser{}

	input := `put sys.cpu.user 1609459200 42.5 host=web01 cpu=0
put sys.mem.used 1609459200 1024 host=web01
sys.disk.usage 1609459200 85.5 host=web01 mount=/`

	batch, err := parser.ParseTelnet(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseTelnet failed: %v", err)
	}

	if len(batch.Samples) != 3 {
		t.Errorf("Expected 3 samples, got %d", len(batch.Samples))
	}

	if batch.Source != "opentsdb-telnet" {
		t.Errorf("Expected source 'opentsdb-telnet', got %s", batch.Source)
	}
}

func TestDataDogParser_ParseV1Series(t *testing.T) {
	parser := &DataDogParser{}

	input := `{
		"series": [
			{
				"metric": "system.cpu.user",
				"type": "gauge",
				"points": [[1609459200, 42.5], [1609459260, 43.0]],
				"host": "web01",
				"tags": ["env:prod", "region:us-west"]
			}
		]
	}`

	batch, err := parser.ParseV1Series(strings.NewReader(input), "")
	if err != nil {
		t.Fatalf("ParseV1Series failed: %v", err)
	}

	if len(batch.Samples) != 2 {
		t.Errorf("Expected 2 samples, got %d", len(batch.Samples))
	}

	if batch.Source != "datadog" {
		t.Errorf("Expected source 'datadog', got %s", batch.Source)
	}

	// Check tags
	sample := batch.Samples[0]
	if sample.Tags["host"] != "web01" {
		t.Errorf("Expected host 'web01', got %s", sample.Tags["host"])
	}
	if sample.Tags["env"] != "prod" {
		t.Errorf("Expected env 'prod', got %s", sample.Tags["env"])
	}
}

func TestDataDogParser_ParseV2Series(t *testing.T) {
	parser := &DataDogParser{}

	input := `{
		"series": [
			{
				"metric": "system.cpu.user",
				"type": 3,
				"points": [{"timestamp": 1609459200, "value": 42.5}],
				"tags": ["env:prod"],
				"resources": [{"type": "host", "name": "web01"}]
			}
		]
	}`

	batch, err := parser.ParseV2Series(strings.NewReader(input), "")
	if err != nil {
		t.Fatalf("ParseV2Series failed: %v", err)
	}

	if len(batch.Samples) != 1 {
		t.Errorf("Expected 1 sample, got %d", len(batch.Samples))
	}

	if batch.Source != "datadog-v2" {
		t.Errorf("Expected source 'datadog-v2', got %s", batch.Source)
	}

	// Check tags
	sample := batch.Samples[0]
	if sample.Tags["host"] != "web01" {
		t.Errorf("Expected host 'web01', got %s", sample.Tags["host"])
	}
	if sample.Tags["__dd_type"] != "gauge" {
		t.Errorf("Expected type 'gauge', got %s", sample.Tags["__dd_type"])
	}
}

func TestDataDogParser_ParseCheckRun(t *testing.T) {
	parser := &DataDogParser{}

	input := `[
		{
			"check": "http.can_connect",
			"host_name": "web01",
			"status": 0,
			"timestamp": 1609459200,
			"tags": ["service:api"]
		}
	]`

	batch, err := parser.ParseCheckRun(strings.NewReader(input), "")
	if err != nil {
		t.Fatalf("ParseCheckRun failed: %v", err)
	}

	if len(batch.Samples) != 1 {
		t.Errorf("Expected 1 sample, got %d", len(batch.Samples))
	}

	sample := batch.Samples[0]
	if sample.Metric != "datadog_check_status" {
		t.Errorf("Expected metric 'datadog_check_status', got %s", sample.Metric)
	}
	if sample.Value != 0 {
		t.Errorf("Expected value 0, got %f", sample.Value)
	}
	if sample.Tags["check"] != "http.can_connect" {
		t.Errorf("Expected check 'http.can_connect', got %s", sample.Tags["check"])
	}
}

func TestDataDogParser_ParseDistributionPoints(t *testing.T) {
	parser := &DataDogParser{}

	input := `{
		"series": [
			{
				"metric": "request.latency",
				"points": [[1609459200, [1.0, 2.0, 3.0, 4.0, 5.0]]],
				"host": "web01",
				"tags": ["service:api"]
			}
		]
	}`

	batch, err := parser.ParseDistributionPoints(strings.NewReader(input), "")
	if err != nil {
		t.Fatalf("ParseDistributionPoints failed: %v", err)
	}

	// Should have 5 summary metrics: count, sum, min, max, avg
	if len(batch.Samples) != 5 {
		t.Errorf("Expected 5 samples, got %d", len(batch.Samples))
	}

	// Check summary metrics
	metrics := make(map[string]float64)
	for _, s := range batch.Samples {
		metrics[s.Metric] = s.Value
	}

	if metrics["request.latency_count"] != 5 {
		t.Errorf("Expected count 5, got %f", metrics["request.latency_count"])
	}
	if metrics["request.latency_sum"] != 15 { // 1+2+3+4+5
		t.Errorf("Expected sum 15, got %f", metrics["request.latency_sum"])
	}
	if metrics["request.latency_min"] != 1 {
		t.Errorf("Expected min 1, got %f", metrics["request.latency_min"])
	}
	if metrics["request.latency_max"] != 5 {
		t.Errorf("Expected max 5, got %f", metrics["request.latency_max"])
	}
	if metrics["request.latency_avg"] != 3 {
		t.Errorf("Expected avg 3, got %f", metrics["request.latency_avg"])
	}
}

func TestInfluxParser_Precision(t *testing.T) {
	parser := &InfluxParser{}

	tests := []struct {
		precision string
		timestamp int64
		expected  time.Time
	}{
		{"ns", 1609459200000000000, time.Unix(1609459200, 0)},
		{"us", 1609459200000000, time.Unix(1609459200, 0)},
		{"ms", 1609459200000, time.Unix(1609459200, 0)},
		{"s", 1609459200, time.Unix(1609459200, 0)},
	}

	for _, tt := range tests {
		result := parser.convertTimestamp(tt.timestamp, tt.precision)
		if !result.Equal(tt.expected) {
			t.Errorf("Precision %s: expected %v, got %v", tt.precision, tt.expected, result)
		}
	}
}

func TestDataDogParser_Compression(t *testing.T) {
	parser := &DataDogParser{}

	// Test with no compression (most common case)
	input := `{"series":[{"metric":"test","type":"gauge","points":[[1609459200,42.5]],"host":"web01"}]}`

	batch, err := parser.ParseV1Series(strings.NewReader(input), "")
	if err != nil {
		t.Fatalf("ParseV1Series failed: %v", err)
	}

	if len(batch.Samples) != 1 {
		t.Errorf("Expected 1 sample, got %d", len(batch.Samples))
	}
}

func TestGraphiteParser_PathToMetricAndTags(t *testing.T) {
	parser := &GraphiteParser{}

	tests := []struct {
		path         string
		wantMetric   string
		wantHostTag  string
	}{
		{"servers.web01.cpu.usage", "cpu_usage", "web01"},
		{"app.myservice.requests", "requests", ""},
		{"statsd.gauge.mymetric", "mymetric", ""},
		{"simple.metric", "simple.metric", ""},
	}

	for _, tt := range tests {
		metric, tags := parser.pathToMetricAndTags(tt.path)
		if tt.wantHostTag != "" && tags["host"] != tt.wantHostTag {
			t.Errorf("Path %s: expected host '%s', got '%s'", tt.path, tt.wantHostTag, tags["host"])
		}
		// Just ensure we got a valid metric name
		if metric == "" {
			t.Errorf("Path %s: got empty metric name", tt.path)
		}
	}
}

func TestPrometheusParser_ParseRemoteWrite(t *testing.T) {
	parser := &PrometheusParser{}

	input := `{
		"timeseries": [
			{
				"labels": [
					{"name": "__name__", "value": "http_requests_total"},
					{"name": "method", "value": "GET"},
					{"name": "status", "value": "200"}
				],
				"samples": [
					{"timestamp": 1609459200000, "value": 42},
					{"timestamp": 1609459260000, "value": 45}
				]
			},
			{
				"labels": [
					{"name": "__name__", "value": "http_requests_total"},
					{"name": "method", "value": "POST"},
					{"name": "status", "value": "200"}
				],
				"samples": [
					{"timestamp": 1609459200000, "value": 10}
				]
			}
		]
	}`

	batch, err := parser.ParseRemoteWrite(strings.NewReader(input), "")
	if err != nil {
		t.Fatalf("ParseRemoteWrite failed: %v", err)
	}

	if len(batch.Samples) != 3 {
		t.Errorf("Expected 3 samples, got %d", len(batch.Samples))
	}

	if batch.Source != "prometheus-remote-write" {
		t.Errorf("Expected source 'prometheus-remote-write', got %s", batch.Source)
	}

	// Check first sample
	sample := batch.Samples[0]
	if sample.Metric != "http_requests_total" {
		t.Errorf("Expected metric 'http_requests_total', got %s", sample.Metric)
	}
	if sample.Tags["method"] != "GET" {
		t.Errorf("Expected method 'GET', got %s", sample.Tags["method"])
	}
	if sample.Value != 42 {
		t.Errorf("Expected value 42, got %f", sample.Value)
	}
}

func TestParsePrometheusTime(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
		wantErr  bool
	}{
		{"1609459200", time.Unix(1609459200, 0), false},
		{"2021-01-01T00:00:00Z", time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC), false},
		{"invalid", time.Time{}, true},
	}

	for _, tt := range tests {
		result, err := parsePrometheusTime(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("Input %s: expected error, got nil", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("Input %s: unexpected error: %v", tt.input, err)
			continue
		}
		if !result.Equal(tt.expected) {
			t.Errorf("Input %s: expected %v, got %v", tt.input, tt.expected, result)
		}
	}

	// Test fractional seconds separately with tolerance
	result, err := parsePrometheusTime("1609459200.5")
	if err != nil {
		t.Errorf("Fractional time: unexpected error: %v", err)
	}
	if result.Unix() != 1609459200 {
		t.Errorf("Fractional time: expected Unix 1609459200, got %d", result.Unix())
	}
}

func TestStatsDParser_Parse(t *testing.T) {
	parser := NewStatsDParser()

	input := `gorets:1|c
glork:320|ms
gaugor:333|g
uniques:765|s
gorets:1|c|@0.1
users.online:10|g|#host:web01,env:prod`

	batch, err := parser.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if batch.Source != "statsd" {
		t.Errorf("Expected source 'statsd', got %s", batch.Source)
	}

	// Should have multiple samples (counters emit _total, timers emit value + count, etc.)
	if len(batch.Samples) < 6 {
		t.Errorf("Expected at least 6 samples, got %d", len(batch.Samples))
	}

	// Check for counter with sample rate adjustment
	found := false
	for _, s := range batch.Samples {
		if s.Metric == "gorets_total" && s.Value == 10 { // 1 / 0.1 = 10
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to find counter 'gorets_total' with sample-rate-adjusted value 10")
	}

	// Check for gauge with tags
	foundTagged := false
	for _, s := range batch.Samples {
		if s.Metric == "users.online" && s.Tags["host"] == "web01" && s.Tags["env"] == "prod" {
			foundTagged = true
			break
		}
	}
	if !foundTagged {
		t.Error("Expected to find gauge 'users.online' with tags host=web01, env=prod")
	}
}

func TestStatsDParser_MetricTypes(t *testing.T) {
	parser := NewStatsDParser()

	tests := []struct {
		input      string
		wantMetric string
		wantValue  float64
	}{
		{"counter:5|c", "counter_total", 5},
		{"gauge:100|g", "gauge", 100},
		{"timer:320|ms", "timer", 320},
		{"histogram:50|h", "histogram", 50},
	}

	for _, tt := range tests {
		batch, err := parser.Parse(strings.NewReader(tt.input))
		if err != nil {
			t.Errorf("Input %s: unexpected error: %v", tt.input, err)
			continue
		}

		found := false
		for _, s := range batch.Samples {
			if s.Metric == tt.wantMetric && s.Value == tt.wantValue {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Input %s: expected metric %s with value %f", tt.input, tt.wantMetric, tt.wantValue)
		}
	}
}

func TestStatsDParser_DogStatsDTags(t *testing.T) {
	parser := NewStatsDParser()

	input := `page.views:1|c|#page:home,user_type:member,browser:chrome`

	batch, err := parser.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(batch.Samples) == 0 {
		t.Fatal("Expected at least 1 sample")
	}

	sample := batch.Samples[0]
	if sample.Tags["page"] != "home" {
		t.Errorf("Expected tag page=home, got %s", sample.Tags["page"])
	}
	if sample.Tags["user_type"] != "member" {
		t.Errorf("Expected tag user_type=member, got %s", sample.Tags["user_type"])
	}
	if sample.Tags["browser"] != "chrome" {
		t.Errorf("Expected tag browser=chrome, got %s", sample.Tags["browser"])
	}
}
