package probe

import (
	"debug/elf"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Constants for perf_event_open
const (
	PERF_TYPE_TRACEPOINT = 2
	PERF_TYPE_UPROBE     = 7
)

// UprobeAttachmentInfo holds info about an attached uprobe
type UprobeAttachmentInfo struct {
	Name     string
	Symbol   string
	Offset   uint64
	IsReturn bool
	PerfFD   int
}

// SSLSymbolOffsets holds SSL function offsets
type SSLSymbolOffsets struct {
	SSLWrite     uint64
	SSLWriteEx   uint64
	SSLRead      uint64
	SSLReadEx    uint64
}

// GetSSLSymbolOffsets reads symbol offsets from the OpenSSL library
func GetSSLSymbolOffsets(libPath string) (*SSLSymbolOffsets, error) {
	f, err := elf.Open(libPath)
	if err != nil {
		return nil, fmt.Errorf("open ELF: %w", err)
	}
	defer f.Close()

	symbols, err := f.DynamicSymbols()
	if err != nil {
		return nil, fmt.Errorf("read symbols: %w", err)
	}

	offsets := &SSLSymbolOffsets{}
	for _, sym := range symbols {
		switch sym.Name {
		case "SSL_write":
			offsets.SSLWrite = sym.Value
		case "SSL_write_ex":
			offsets.SSLWriteEx = sym.Value
		case "SSL_read":
			offsets.SSLRead = sym.Value
		case "SSL_read_ex":
			offsets.SSLReadEx = sym.Value
		}
	}

	return offsets, nil
}

// RegisterTraceFSProbe registers a uprobe via tracefs and returns the event ID
func RegisterTraceFSProbe(name, libPath string, offset uint64, isReturn bool) (int, error) {
	// Remove any existing probe with this name
	eventsPath := "/sys/kernel/debug/tracing/uprobe_events"
	f, err := os.OpenFile(eventsPath, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return 0, fmt.Errorf("open uprobe_events: %w", err)
	}

	// Try to remove existing probe (ignore error)
	f.WriteString(fmt.Sprintf("-:uprobes/%s\n", name))
	f.Close()

	// Re-open and write new probe
	f, err = os.OpenFile(eventsPath, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return 0, fmt.Errorf("open uprobe_events: %w", err)
	}
	defer f.Close()

	probeType := "p"
	if isReturn {
		probeType = "r"
	}

	def := fmt.Sprintf("%s:uprobes/%s %s:0x%x\n", probeType, name, libPath, offset)
	if _, err := f.WriteString(def); err != nil {
		return 0, fmt.Errorf("write probe definition: %w", err)
	}

	// Read the event ID
	idPath := fmt.Sprintf("/sys/kernel/debug/tracing/events/uprobes/%s/id", name)
	idBytes, err := os.ReadFile(idPath)
	if err != nil {
		return 0, fmt.Errorf("read event id: %w", err)
	}

	var eventID int
	fmt.Sscanf(strings.TrimSpace(string(idBytes)), "%d", &eventID)

	return eventID, nil
}

// AttachBPFToTracepoint attaches a BPF program to a tracepoint/uprobe via perf_event_open
// Returns slice of perf FDs (one per CPU) for proper cleanup
func AttachBPFToTracepoint(eventID int, progFD int) (int, error) {
	// For system-wide tracing (pid=-1), we need to attach to each CPU
	// We return a "master" FD and track all CPUs internally
	return attachBPFToTracepointAllCPUs(eventID, progFD)
}

// attachBPFToTracepointAllCPUs attaches to all CPUs for system-wide tracing
func attachBPFToTracepointAllCPUs(eventID int, progFD int) (int, error) {
	// Get number of CPUs
	numCPUs := getNumCPUs()
	if numCPUs <= 0 {
		numCPUs = 1
	}

	var firstFD int
	for cpu := 0; cpu < numCPUs; cpu++ {
		attr := unix.PerfEventAttr{
			Type:   PERF_TYPE_TRACEPOINT,
			Config: uint64(eventID),
			Size:   uint32(unsafe.Sizeof(unix.PerfEventAttr{})),
		}

		fd, err := unix.PerfEventOpen(&attr, -1, cpu, -1, unix.PERF_FLAG_FD_CLOEXEC)
		if err != nil {
			// If first CPU fails, return error
			if cpu == 0 {
				return 0, fmt.Errorf("perf_event_open cpu %d: %w", cpu, err)
			}
			// Otherwise just warn and continue
			continue
		}

		// Enable the event
		if err := unix.IoctlSetInt(fd, unix.PERF_EVENT_IOC_ENABLE, 0); err != nil {
			syscall.Close(fd)
			if cpu == 0 {
				return 0, fmt.Errorf("enable perf event cpu %d: %w", cpu, err)
			}
			continue
		}

		// Attach BPF program
		if err := unix.IoctlSetInt(fd, unix.PERF_EVENT_IOC_SET_BPF, progFD); err != nil {
			syscall.Close(fd)
			if cpu == 0 {
				return 0, fmt.Errorf("attach BPF cpu %d: %w", cpu, err)
			}
			continue
		}

		// Track first FD for return (caller will close)
		// Note: This is a simplification - ideally we'd return all FDs
		// For now we keep additional FDs open (they'll be closed on process exit)
		if cpu == 0 {
			firstFD = fd
		} else {
			// Store additional FDs for cleanup (kept globally for simplicity)
			additionalPerfFDs = append(additionalPerfFDs, fd)
		}
	}

	return firstFD, nil
}

// additionalPerfFDs stores extra perf FDs for multi-CPU attachment
var additionalPerfFDs []int

// CleanupAdditionalPerfFDs closes all additional perf FDs
func CleanupAdditionalPerfFDs() {
	for _, fd := range additionalPerfFDs {
		syscall.Close(fd)
	}
	additionalPerfFDs = nil
}

// getNumCPUs returns the number of online CPUs
func getNumCPUs() int {
	data, err := os.ReadFile("/sys/devices/system/cpu/online")
	if err != nil {
		return 1
	}
	// Parse format like "0-3" or "0,2-3"
	s := strings.TrimSpace(string(data))
	if strings.Contains(s, "-") {
		parts := strings.Split(s, "-")
		if len(parts) == 2 {
			var max int
			fmt.Sscanf(parts[1], "%d", &max)
			return max + 1
		}
	}
	return 1
}

// CleanupTraceFSProbe removes a tracefs uprobe
func CleanupTraceFSProbe(name string) error {
	eventsPath := "/sys/kernel/debug/tracing/uprobe_events"
	f, err := os.OpenFile(eventsPath, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(fmt.Sprintf("-:uprobes/%s\n", name))
	return err
}

// ResolveLibraryPath finds the actual library path, handling symlinks correctly
func ResolveLibraryPath(name string) (string, error) {
	// For uprobes, we need the path that processes actually load
	// This means NOT resolving symlinks, as the inode matters

	// Check common paths in order
	paths := []string{
		"/lib/x86_64-linux-gnu/" + name,
		"/lib64/" + name,
		"/usr/lib/x86_64-linux-gnu/" + name,
		"/usr/lib64/" + name,
		"/usr/lib/" + name,
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			// Return the path as-is, don't resolve symlinks
			return p, nil
		}
	}

	// Try to find via ldconfig cache
	return "", fmt.Errorf("library %s not found", name)
}

// GetLibraryBaseAddress gets the base address where a library is loaded in a process
func GetLibraryBaseAddress(pid int, libName string) (uint64, error) {
	mapsPath := fmt.Sprintf("/proc/%d/maps", pid)
	data, err := os.ReadFile(mapsPath)
	if err != nil {
		return 0, err
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, libName) && strings.Contains(line, "r-x") {
			// Found the executable segment
			parts := strings.Fields(line)
			if len(parts) > 0 {
				addrRange := strings.Split(parts[0], "-")
				if len(addrRange) > 0 {
					var addr uint64
					fmt.Sscanf(addrRange[0], "%x", &addr)
					return addr, nil
				}
			}
		}
	}

	return 0, fmt.Errorf("library %s not found in process %d", libName, pid)
}

// GetLibraryInode gets the inode of a library file
func GetLibraryInode(path string) (uint64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("failed to get stat info")
	}

	return stat.Ino, nil
}

// FindLibraryByInode finds the library path that matches a given inode
func FindLibraryByInode(targetInode uint64) (string, error) {
	searchPaths := []string{
		"/lib/x86_64-linux-gnu",
		"/lib64",
		"/usr/lib/x86_64-linux-gnu",
		"/usr/lib64",
		"/usr/lib",
	}

	for _, dir := range searchPaths {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !strings.Contains(entry.Name(), "libssl") {
				continue
			}

			path := filepath.Join(dir, entry.Name())
			info, err := os.Stat(path)
			if err != nil {
				continue
			}

			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok {
				continue
			}

			if stat.Ino == targetInode {
				return path, nil
			}
		}
	}

	return "", fmt.Errorf("library with inode %d not found", targetInode)
}
