package probe

import (
	"net"
	"testing"
	"time"
)

func TestNewFDTracker(t *testing.T) {
	cfg := DefaultFDTrackerConfig()
	tracker := NewFDTracker(cfg)
	if tracker == nil {
		t.Fatal("expected non-nil tracker")
	}
	if tracker.maxEntries != 65536 {
		t.Errorf("expected maxEntries=65536, got %d", tracker.maxEntries)
	}
	if tracker.cleanupAge != 5*time.Minute {
		t.Errorf("expected cleanupAge=5m, got %v", tracker.cleanupAge)
	}
}

func TestFDTrackerStoreLookup(t *testing.T) {
	tracker := NewFDTracker(DefaultFDTrackerConfig())

	meta := &SocketMeta{
		LocalIP:    net.ParseIP("127.0.0.1"),
		LocalPort:  8080,
		RemoteIP:   net.ParseIP("10.0.0.1"),
		RemotePort: 54321,
		Protocol:   ProtoTCP,
		Family:     afINET,
		State:      1,
		Inode:      12345,
		FirstSeen:  time.Now(),
		LastSeen:   time.Now(),
		Direction:  "inbound",
	}

	tracker.storeFD(1000, 5, meta)

	// Lookup should find it
	result := tracker.Lookup(1000, 5)
	if result == nil {
		t.Fatal("expected to find tracked FD")
	}
	if result.LocalPort != 8080 {
		t.Errorf("expected LocalPort=8080, got %d", result.LocalPort)
	}
	if result.RemotePort != 54321 {
		t.Errorf("expected RemotePort=54321, got %d", result.RemotePort)
	}
	if result.Direction != "inbound" {
		t.Errorf("expected Direction=inbound, got %s", result.Direction)
	}

	// Lookup for unknown FD should return nil
	result = tracker.Lookup(1000, 99)
	if result != nil {
		t.Error("expected nil for unknown FD")
	}

	// IsNetworkFD convenience method
	if !tracker.IsNetworkFD(1000, 5) {
		t.Error("expected IsNetworkFD to return true for tracked FD")
	}
	if tracker.IsNetworkFD(1000, 99) {
		t.Error("expected IsNetworkFD to return false for unknown FD")
	}
}

func TestFDTrackerTrackClose(t *testing.T) {
	tracker := NewFDTracker(DefaultFDTrackerConfig())

	meta := &SocketMeta{
		LocalIP:    net.ParseIP("127.0.0.1"),
		LocalPort:  8080,
		RemoteIP:   net.ParseIP("10.0.0.1"),
		RemotePort: 54321,
		Protocol:   ProtoTCP,
		Family:     afINET,
		Inode:      12345,
		FirstSeen:  time.Now(),
		LastSeen:   time.Now(),
	}

	tracker.storeFD(1000, 5, meta)
	if tracker.Lookup(1000, 5) == nil {
		t.Fatal("expected to find tracked FD before close")
	}

	tracker.TrackClose(1000, 5)
	if tracker.Lookup(1000, 5) != nil {
		t.Error("expected nil after TrackClose")
	}
}

func TestFDTrackerGetSocketsForPID(t *testing.T) {
	tracker := NewFDTracker(DefaultFDTrackerConfig())

	for i := int32(3); i < 8; i++ {
		meta := &SocketMeta{
			LocalIP:    net.ParseIP("127.0.0.1"),
			LocalPort:  uint16(8080 + i),
			RemoteIP:   net.ParseIP("10.0.0.1"),
			RemotePort: uint16(50000 + i),
			Protocol:   ProtoTCP,
			Family:     afINET,
			FirstSeen:  time.Now(),
			LastSeen:   time.Now(),
		}
		tracker.storeFD(1000, i, meta)
	}

	// Add one for a different PID
	tracker.storeFD(2000, 3, &SocketMeta{
		LocalIP:   net.ParseIP("127.0.0.1"),
		LocalPort: 9090,
		FirstSeen: time.Now(),
		LastSeen:  time.Now(),
	})

	sockets := tracker.GetSocketsForPID(1000)
	if len(sockets) != 5 {
		t.Errorf("expected 5 sockets for PID 1000, got %d", len(sockets))
	}

	sockets = tracker.GetSocketsForPID(2000)
	if len(sockets) != 1 {
		t.Errorf("expected 1 socket for PID 2000, got %d", len(sockets))
	}

	sockets = tracker.GetSocketsForPID(9999)
	if len(sockets) != 0 {
		t.Errorf("expected 0 sockets for unknown PID, got %d", len(sockets))
	}
}

func TestFDTrackerStats(t *testing.T) {
	tracker := NewFDTracker(DefaultFDTrackerConfig())

	meta := &SocketMeta{
		LocalIP:   net.ParseIP("127.0.0.1"),
		LocalPort: 8080,
		FirstSeen: time.Now(),
		LastSeen:  time.Now(),
	}

	tracker.storeFD(1000, 5, meta)
	tracker.Lookup(1000, 5)  // hit
	tracker.Lookup(1000, 99) // miss

	stats := tracker.Stats()
	if stats.ActiveEntries != 1 {
		t.Errorf("expected ActiveEntries=1, got %d", stats.ActiveEntries)
	}
	if stats.TotalTracked != 1 {
		t.Errorf("expected TotalTracked=1, got %d", stats.TotalTracked)
	}
	if stats.TotalLookups != 2 {
		t.Errorf("expected TotalLookups=2, got %d", stats.TotalLookups)
	}
	if stats.TotalHits != 1 {
		t.Errorf("expected TotalHits=1, got %d", stats.TotalHits)
	}
	if stats.TotalMisses != 1 {
		t.Errorf("expected TotalMisses=1, got %d", stats.TotalMisses)
	}
	if stats.HitRate != 0.5 {
		t.Errorf("expected HitRate=0.5, got %f", stats.HitRate)
	}
}

func TestFDTrackerEviction(t *testing.T) {
	cfg := DefaultFDTrackerConfig()
	cfg.MaxEntries = 10
	tracker := NewFDTracker(cfg)

	// Fill to capacity + overflow
	for i := int32(0); i < 15; i++ {
		meta := &SocketMeta{
			LocalIP:    net.ParseIP("127.0.0.1"),
			LocalPort:  uint16(8080 + i),
			RemoteIP:   net.ParseIP("10.0.0.1"),
			RemotePort: uint16(50000 + i),
			Protocol:   ProtoTCP,
			Family:     afINET,
			FirstSeen:  time.Now(),
			LastSeen:   time.Now(),
		}
		tracker.storeFD(1000, i, meta)
	}

	stats := tracker.Stats()
	if stats.ActiveEntries > 10 {
		t.Errorf("expected at most 10 active entries after eviction, got %d", stats.ActiveEntries)
	}
	if stats.TotalEvicted == 0 {
		t.Error("expected some evictions")
	}
}

func TestFDTrackerDumpSockets(t *testing.T) {
	tracker := NewFDTracker(DefaultFDTrackerConfig())

	meta := &SocketMeta{
		LocalIP:    net.ParseIP("127.0.0.1"),
		LocalPort:  8080,
		RemoteIP:   net.ParseIP("10.0.0.1"),
		RemotePort: 54321,
		Protocol:   ProtoTCP,
		Family:     afINET,
		Direction:  "outbound",
		Inode:      12345,
		FirstSeen:  time.Now(),
		LastSeen:   time.Now(),
	}
	tracker.storeFD(1000, 5, meta)

	entries := tracker.DumpSockets()
	if len(entries) != 1 {
		t.Fatalf("expected 1 dump entry, got %d", len(entries))
	}
	if entries[0].PID != 1000 {
		t.Errorf("expected PID=1000, got %d", entries[0].PID)
	}
	if entries[0].FD != 5 {
		t.Errorf("expected FD=5, got %d", entries[0].FD)
	}
	if entries[0].Protocol != "tcp" {
		t.Errorf("expected protocol=tcp, got %s", entries[0].Protocol)
	}
}

func TestParseProcNetAddr(t *testing.T) {
	tests := []struct {
		input    string
		wantIP   string
		wantPort uint16
		wantOK   bool
	}{
		{
			input:    "0100007F:0050",
			wantIP:   "127.0.0.1",
			wantPort: 80,
			wantOK:   true,
		},
		{
			input:    "00000000:0000",
			wantIP:   "0.0.0.0",
			wantPort: 0,
			wantOK:   true,
		},
		{
			input:  "invalid",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		ip, port, ok := parseProcNetAddr(tt.input)
		if ok != tt.wantOK {
			t.Errorf("parseProcNetAddr(%q): ok=%v, want %v", tt.input, ok, tt.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if ip.String() != tt.wantIP {
			t.Errorf("parseProcNetAddr(%q): ip=%s, want %s", tt.input, ip.String(), tt.wantIP)
		}
		if port != tt.wantPort {
			t.Errorf("parseProcNetAddr(%q): port=%d, want %d", tt.input, port, tt.wantPort)
		}
	}
}

func TestSocketProtocolString(t *testing.T) {
	tests := []struct {
		proto SocketProtocol
		want  string
	}{
		{ProtoTCP, "tcp"},
		{ProtoUDP, "udp"},
		{ProtoTCP6, "tcp6"},
		{ProtoUDP6, "udp6"},
		{ProtoUnknown, "unknown"},
	}

	for _, tt := range tests {
		if got := tt.proto.String(); got != tt.want {
			t.Errorf("SocketProtocol(%d).String() = %q, want %q", tt.proto, got, tt.want)
		}
	}
}

func TestProtoFromFamily(t *testing.T) {
	if got := protoFromFamily(afINET); got != ProtoTCP {
		t.Errorf("protoFromFamily(AF_INET) = %v, want ProtoTCP", got)
	}
	if got := protoFromFamily(afINET6); got != ProtoTCP6 {
		t.Errorf("protoFromFamily(AF_INET6) = %v, want ProtoTCP6", got)
	}
}

func TestFDTrackerStartStop(t *testing.T) {
	tracker := NewFDTracker(DefaultFDTrackerConfig())
	tracker.Start()
	time.Sleep(50 * time.Millisecond) // Let background goroutine start
	tracker.Stop()
	// Double stop should be safe
	tracker.Stop()
}
