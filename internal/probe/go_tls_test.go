package probe

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGoTLSProbe_IsGoBinary(t *testing.T) {
	probe := NewGoTLSProbe()

	// Test with current Go binary (go test runner)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go binary not found in PATH")
	}

	isGo, version := probe.IsGoBinary(goBin)
	if !isGo {
		t.Error("Expected go binary to be detected as Go")
	}
	t.Logf("Detected Go version: %s", version)

	// Test with a non-Go binary (like /bin/ls)
	lsBin := "/bin/ls"
	if _, err := os.Stat(lsBin); err != nil {
		lsBin = "/usr/bin/ls"
	}
	isGo, _ = probe.IsGoBinary(lsBin)
	if isGo {
		t.Error("Expected /bin/ls to NOT be detected as Go")
	}
}

func TestGoTLSProbe_AnalyzeBinary(t *testing.T) {
	probe := NewGoTLSProbe()

	// Find a Go binary to analyze
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go binary not found in PATH")
	}

	info, err := probe.AnalyzeBinary(goBin)
	if err != nil {
		t.Fatalf("Failed to analyze Go binary: %v", err)
	}

	t.Logf("Binary: %s", info.Path)
	t.Logf("Go Version: %s", info.GoVersion)
	t.Logf("Valid: %v", info.IsValid)
	t.Logf("TLS Symbols found: %d", len(info.TLSSymbols))

	for sym, offset := range info.TLSSymbols {
		t.Logf("  %s @ 0x%x", sym, offset)
	}
}

func TestGoTLSProbe_ScanProcesses(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("Scanning processes requires root")
	}

	probe := NewGoTLSProbe()

	results, err := probe.ScanProcesses()
	if err != nil {
		t.Fatalf("Failed to scan processes: %v", err)
	}

	t.Logf("Found %d Go processes with TLS", len(results))
	for _, info := range results {
		t.Logf("  %s (Go %s)", info.Path, info.GoVersion)
	}
}

func TestGoTLSProbe_Stats(t *testing.T) {
	probe := NewGoTLSProbe()

	// Analyze something first
	goBin, err := exec.LookPath("go")
	if err == nil {
		probe.AnalyzeBinary(goBin)
	}

	stats := probe.Stats()

	if stats["analyzed_binaries"].(int) > 0 {
		t.Logf("Stats: %+v", stats)
	}
}

func TestGetConnStructOffsets(t *testing.T) {
	tests := []struct {
		version string
	}{
		{"1.20.1"},
		{"1.21.0"},
		{"1.22.3"},
		{"1.19.5"},
		{"1.18.10"},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			offsets := GetConnStructOffsets(tt.version)

			// Just verify offsets are reasonable (non-zero for most)
			t.Logf("Go %s: ConnOffset=%d, ConfigOffset=%d, BufOffset=%d, HandshakeDone=%d",
				tt.version, offsets.ConnOffset, offsets.ConfigOffset,
				offsets.BufOffset, offsets.HandshakeDone)

			if offsets.ConfigOffset == 0 {
				t.Error("ConfigOffset should not be 0")
			}
		})
	}
}

func TestGoTLSProbe_FindGoBinariesInDirectory(t *testing.T) {
	probe := NewGoTLSProbe()

	// Create a temp directory with a fake "binary"
	tmpDir := t.TempDir()

	// We can't easily create a real Go binary, but we can test the walk logic
	// by testing with /usr/bin if we have access
	results, err := probe.FindGoBinariesInDirectory(tmpDir)
	if err != nil {
		t.Fatalf("Failed to scan directory: %v", err)
	}

	// Empty dir should return empty results
	if len(results) != 0 {
		t.Errorf("Expected 0 results from empty dir, got %d", len(results))
	}
}

func TestGoTLSSymbols(t *testing.T) {
	// Verify our target symbols are correct
	expected := []string{
		"crypto/tls.(*Conn).Read",
		"crypto/tls.(*Conn).Write",
		"crypto/tls.(*Conn).Handshake",
		"crypto/tls.(*Conn).Close",
	}

	if len(GoTLSSymbols) != len(expected) {
		t.Errorf("Expected %d symbols, got %d", len(expected), len(GoTLSSymbols))
	}

	for i, sym := range expected {
		if GoTLSSymbols[i] != sym {
			t.Errorf("Symbol mismatch at %d: expected %s, got %s", i, sym, GoTLSSymbols[i])
		}
	}
}

// Integration test that builds and analyzes a simple Go binary
func TestGoTLSProbe_IntegrationWithRealBinary(t *testing.T) {
	// Check if we can build Go code
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not in PATH")
	}

	// Create a minimal Go program that uses TLS
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "main.go")
	binFile := filepath.Join(tmpDir, "testbin")

	src := `package main

import (
	"crypto/tls"
	"net"
)

func main() {
	// Just reference TLS to ensure symbols are included
	var _ tls.Conn
	var _ net.Conn
}
`
	if err := os.WriteFile(srcFile, []byte(src), 0644); err != nil {
		t.Fatalf("Failed to write source: %v", err)
	}

	// Build the binary
	cmd := exec.Command("go", "build", "-o", binFile, srcFile)
	cmd.Dir = tmpDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build: %v\n%s", err, output)
	}

	// Analyze the built binary
	probe := NewGoTLSProbe()

	isGo, version := probe.IsGoBinary(binFile)
	if !isGo {
		t.Error("Built binary should be detected as Go")
	}
	t.Logf("Built binary detected as Go %s", version)

	info, err := probe.AnalyzeBinary(binFile)
	if err != nil {
		t.Logf("Analysis returned error (may be expected for minimal binary): %v", err)
	} else {
		t.Logf("Analysis result: Valid=%v, TLS symbols=%d", info.IsValid, len(info.TLSSymbols))
		for sym, offset := range info.TLSSymbols {
			t.Logf("  Found: %s @ 0x%x", sym, offset)
		}
	}
}
