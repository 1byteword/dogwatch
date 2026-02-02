package probe

import (
	"strings"
	"testing"
)

func TestParseKernelVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected KernelVersion
		wantErr  bool
	}{
		{"5.15.0-generic", KernelVersion{5, 15, 0}, false},
		{"4.4.0", KernelVersion{4, 4, 0}, false},
		{"6.8.0-90-generic", KernelVersion{6, 8, 0}, false},
		{"5.10.102", KernelVersion{5, 10, 102}, false},
		{"4.19", KernelVersion{4, 19, 0}, false},
		{"5.4.0-150-generic", KernelVersion{5, 4, 0}, false},
		{"invalid", KernelVersion{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseKernelVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseKernelVersion(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.expected {
				t.Errorf("ParseKernelVersion(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestKernelVersionAtLeast(t *testing.T) {
	tests := []struct {
		version  KernelVersion
		major    int
		minor    int
		patch    int
		expected bool
	}{
		{KernelVersion{5, 15, 0}, 5, 8, 0, true},   // 5.15 >= 5.8
		{KernelVersion{5, 15, 0}, 5, 15, 0, true},  // exact match
		{KernelVersion{5, 15, 0}, 5, 16, 0, false}, // 5.15 < 5.16
		{KernelVersion{4, 4, 0}, 5, 0, 0, false},   // 4.4 < 5.0
		{KernelVersion{6, 0, 0}, 5, 8, 0, true},    // 6.0 >= 5.8
		{KernelVersion{5, 8, 1}, 5, 8, 0, true},    // patch version higher
		{KernelVersion{5, 8, 0}, 5, 8, 1, false},   // patch version lower
	}

	for _, tt := range tests {
		t.Run(tt.version.String(), func(t *testing.T) {
			got := tt.version.AtLeast(tt.major, tt.minor, tt.patch)
			if got != tt.expected {
				t.Errorf("%v.AtLeast(%d, %d, %d) = %v, want %v",
					tt.version, tt.major, tt.minor, tt.patch, got, tt.expected)
			}
		})
	}
}

func TestGetKernelVersion(t *testing.T) {
	v, err := GetKernelVersion()
	if err != nil {
		t.Fatalf("GetKernelVersion() error: %v", err)
	}

	// Basic sanity checks
	if v.Major < 3 || v.Major > 10 {
		t.Errorf("Unexpected major version: %d", v.Major)
	}
	if v.Minor < 0 || v.Minor > 99 {
		t.Errorf("Unexpected minor version: %d", v.Minor)
	}

	t.Logf("Detected kernel version: %s", v)
}

func TestDetectBPFFeatures(t *testing.T) {
	f := DetectBPFFeatures()

	// Verify kernel version was detected
	if f.KernelVersion.Major == 0 {
		t.Error("Kernel version not detected")
	}

	// Log detected features for debugging
	t.Logf("Kernel Version: %s", f.KernelVersion)
	t.Logf("Has BTF: %v", f.HasBTF)
	t.Logf("Has Ring Buffer: %v", f.HasRingBuffer)
	t.Logf("Has Perf Buffer: %v", f.HasPerfBuffer)
	t.Logf("Has Kprobe: %v", f.HasKprobe)
	t.Logf("Has Tracepoint: %v", f.HasTracepoint)
	t.Logf("Has Uprobe: %v", f.HasUprobe)

	// At minimum, hash maps and array maps should be available
	if !f.HasHashMap {
		t.Error("Hash map should be available")
	}
	if !f.HasArrayMap {
		t.Error("Array map should be available")
	}
}

func TestCheckProbeCapabilities(t *testing.T) {
	caps := CheckProbeCapabilities()

	// Log capabilities
	t.Logf("TCP Connect Probe: %v", caps.CanUseTCPConnect)
	t.Logf("HTTP Probe: %v", caps.CanUseHTTPProbe)
	t.Logf("SSL Probe: %v", caps.CanUseSSLProbe)
	t.Logf("DB Probe: %v", caps.CanUseDBProbe)
	t.Logf("Profiler: %v", caps.CanUseProfiler)
	t.Logf("Using Ring Buffer: %v", caps.UseRingBuffer)

	for _, w := range caps.Warnings {
		t.Logf("Warning: %s", w)
	}
	for _, m := range caps.MissingFeatures {
		t.Logf("Missing: %s", m)
	}
}

func TestCheckTracepointExists(t *testing.T) {
	// syscalls tracepoints should exist on most systems
	exists := CheckTracepointExists("syscalls", "sys_enter_read")
	t.Logf("sys_enter_read tracepoint exists: %v", exists)

	// Non-existent tracepoint
	exists = CheckTracepointExists("nonexistent", "fake_tracepoint")
	if exists {
		t.Error("Non-existent tracepoint should not exist")
	}
}

func TestCheckKprobeTarget(t *testing.T) {
	// tcp_connect should exist on most systems with networking
	exists := CheckKprobeTarget("tcp_connect")
	t.Logf("tcp_connect kprobe target exists: %v", exists)

	// Non-existent function
	exists = CheckKprobeTarget("totally_fake_function_name_12345")
	if exists {
		t.Error("Non-existent function should not be found")
	}
}

func TestCompatibilityReport(t *testing.T) {
	report := CompatibilityReport()

	// Verify report contains expected sections
	if !strings.Contains(report, "Kernel Version:") {
		t.Error("Report should contain kernel version")
	}
	if !strings.Contains(report, "BTF Support:") {
		t.Error("Report should contain BTF support status")
	}
	if !strings.Contains(report, "Probe Capabilities:") {
		t.Error("Report should contain probe capabilities")
	}

	t.Logf("Compatibility Report:\n%s", report)
}

func TestKernelVersionString(t *testing.T) {
	v := KernelVersion{5, 15, 42}
	expected := "5.15.42"
	if v.String() != expected {
		t.Errorf("KernelVersion.String() = %q, want %q", v.String(), expected)
	}
}
