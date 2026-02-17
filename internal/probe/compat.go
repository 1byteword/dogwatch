package probe

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/features"
)

// KernelVersion represents a parsed kernel version
type KernelVersion struct {
	Major int
	Minor int
	Patch int
}

func (v KernelVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// AtLeast checks if the kernel version is at least the specified version
func (v KernelVersion) AtLeast(major, minor, patch int) bool {
	if v.Major > major {
		return true
	}
	if v.Major < major {
		return false
	}
	if v.Minor > minor {
		return true
	}
	if v.Minor < minor {
		return false
	}
	return v.Patch >= patch
}

// BPFFeatures represents available BPF features
type BPFFeatures struct {
	// Core features
	HasRingBuffer     bool // Requires 5.8+
	HasPerfBuffer     bool // Available since 4.3
	HasBTF            bool // Requires 5.2+ with BTF enabled
	HasCORE           bool // Requires BTF
	HasKprobe         bool // Available since 4.1
	HasTracepoint     bool // Available since 4.7
	HasUprobe         bool // Available since 4.17
	HasPerfEvent      bool // Available since 4.9
	HasStackTrace     bool // Available since 4.4
	HasSpinLock       bool // Available since 5.1
	HasBoundedLoops   bool // Requires 5.3+
	HasGlobalData     bool // Requires 5.2+
	HasBPFTimestamp   bool // Requires 5.1+ (bpf_ktime_get_boot_ns)
	HasBPFGetCurrentCgroupID bool // Requires 4.18+

	// Map types
	HasHashMap        bool
	HasArrayMap       bool
	HasPerfEventArray bool
	HasStackTraceMap  bool
	HasLRUHashMap     bool // Requires 4.10+
	HasRingBufMap     bool // Requires 5.8+

	// Kernel version
	KernelVersion KernelVersion
}

var (
	cachedFeatures *BPFFeatures
	featuresOnce   sync.Once
)

// GetKernelVersion returns the current kernel version
func GetKernelVersion() (KernelVersion, error) {
	var uname syscall.Utsname
	if err := syscall.Uname(&uname); err != nil {
		return KernelVersion{}, fmt.Errorf("uname syscall failed: %w", err)
	}

	// Convert release to string
	var release string
	for _, b := range uname.Release {
		if b == 0 {
			break
		}
		release += string(byte(b))
	}

	return ParseKernelVersion(release)
}

// ParseKernelVersion parses a kernel version string like "5.15.0-generic"
func ParseKernelVersion(release string) (KernelVersion, error) {
	// Strip any suffix after the version numbers
	parts := strings.Split(release, "-")
	versionStr := parts[0]

	vparts := strings.Split(versionStr, ".")
	if len(vparts) < 2 {
		return KernelVersion{}, fmt.Errorf("invalid kernel version: %s", release)
	}

	major, err := strconv.Atoi(vparts[0])
	if err != nil {
		return KernelVersion{}, fmt.Errorf("invalid major version: %s", vparts[0])
	}

	minor, err := strconv.Atoi(vparts[1])
	if err != nil {
		return KernelVersion{}, fmt.Errorf("invalid minor version: %s", vparts[1])
	}

	patch := 0
	if len(vparts) >= 3 {
		// Handle versions like "5.15.0" or "5.15"
		patchStr := vparts[2]
		// Strip any non-numeric suffix
		for i, c := range patchStr {
			if c < '0' || c > '9' {
				patchStr = patchStr[:i]
				break
			}
		}
		if patchStr != "" {
			patch, _ = strconv.Atoi(patchStr)
		}
	}

	return KernelVersion{Major: major, Minor: minor, Patch: patch}, nil
}

// HasBTFSupport checks if the kernel has BTF (BPF Type Format) enabled
func HasBTFSupport() bool {
	// Check for vmlinux BTF
	btfPaths := []string{
		"/sys/kernel/btf/vmlinux",
		"/boot/vmlinux-" + getCurrentKernelRelease(),
	}

	for _, path := range btfPaths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}

	return false
}

func getCurrentKernelRelease() string {
	var uname syscall.Utsname
	if err := syscall.Uname(&uname); err != nil {
		return ""
	}
	var release string
	for _, b := range uname.Release {
		if b == 0 {
			break
		}
		release += string(byte(b))
	}
	return release
}

// DetectBPFFeatures detects available BPF features on the current kernel
func DetectBPFFeatures() *BPFFeatures {
	featuresOnce.Do(func() {
		cachedFeatures = detectFeaturesImpl()
	})
	return cachedFeatures
}

func detectFeaturesImpl() *BPFFeatures {
	f := &BPFFeatures{}

	// Get kernel version
	kv, err := GetKernelVersion()
	if err == nil {
		f.KernelVersion = kv
	}

	// Detect BTF support
	f.HasBTF = HasBTFSupport()
	f.HasCORE = f.HasBTF // CO-RE requires BTF

	// Detect map types using cilium/ebpf features package
	if err := features.HaveMapType(ebpf.Hash); err == nil {
		f.HasHashMap = true
	}
	if err := features.HaveMapType(ebpf.Array); err == nil {
		f.HasArrayMap = true
	}
	if err := features.HaveMapType(ebpf.PerfEventArray); err == nil {
		f.HasPerfBuffer = true
		f.HasPerfEventArray = true
	}
	if err := features.HaveMapType(ebpf.StackTrace); err == nil {
		f.HasStackTrace = true
		f.HasStackTraceMap = true
	}
	if err := features.HaveMapType(ebpf.LRUHash); err == nil {
		f.HasLRUHashMap = true
	}
	if err := features.HaveMapType(ebpf.RingBuf); err == nil {
		f.HasRingBuffer = true
		f.HasRingBufMap = true
	}

	// Detect program types
	if err := features.HaveProgramType(ebpf.Kprobe); err == nil {
		f.HasKprobe = true
	}
	if err := features.HaveProgramType(ebpf.TracePoint); err == nil {
		f.HasTracepoint = true
	}
	if err := features.HaveProgramType(ebpf.PerfEvent); err == nil {
		f.HasPerfEvent = true
	}

	// Uprobe detection - try to verify by checking tracefs availability
	f.HasUprobe = checkUprobeSupport()

	// Detect BPF helpers via version inference
	// bpf_spin_lock requires 5.1+
	f.HasSpinLock = kv.AtLeast(5, 1, 0)
	// Bounded loops require 5.3+
	f.HasBoundedLoops = kv.AtLeast(5, 3, 0)
	// Global data requires 5.2+
	f.HasGlobalData = kv.AtLeast(5, 2, 0)
	// bpf_ktime_get_boot_ns requires 5.1+
	f.HasBPFTimestamp = kv.AtLeast(5, 1, 0)
	// bpf_get_current_cgroup_id requires 4.18+
	f.HasBPFGetCurrentCgroupID = kv.AtLeast(4, 18, 0)

	return f
}

func checkUprobeSupport() bool {
	// Check if tracefs is mounted and uprobe_events exists
	tracefsPaths := []string{
		"/sys/kernel/debug/tracing/uprobe_events",
		"/sys/kernel/tracing/uprobe_events",
	}
	for _, path := range tracefsPaths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

// CheckTracepointExists verifies that a specific tracepoint exists
func CheckTracepointExists(category, name string) bool {
	paths := []string{
		filepath.Join("/sys/kernel/debug/tracing/events", category, name),
		filepath.Join("/sys/kernel/tracing/events", category, name),
	}
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// CheckKprobeTarget verifies that a kernel function can be probed
func CheckKprobeTarget(funcName string) bool {
	// Check /proc/kallsyms for the function
	f, err := os.Open("/proc/kallsyms")
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 3 && fields[2] == funcName {
			return true
		}
	}
	return false
}

// ProbeCapabilities returns a summary of probe capabilities
type ProbeCapabilities struct {
	CanUseTCPConnect  bool
	CanUseHTTPProbe   bool
	CanUseSSLProbe    bool
	CanUseDBProbe     bool
	CanUseProfiler    bool
	CanUseFDTracking  bool // FD-to-socket tracking via /proc
	UseRingBuffer     bool // false = use perf buffer
	MissingFeatures   []string
	Warnings          []string
}

// CheckProbeCapabilities determines what probes can run on this kernel
func CheckProbeCapabilities() *ProbeCapabilities {
	f := DetectBPFFeatures()
	caps := &ProbeCapabilities{
		MissingFeatures: make([]string, 0),
		Warnings:        make([]string, 0),
	}

	// Check kernel version requirements
	if !f.KernelVersion.AtLeast(4, 4, 0) {
		caps.MissingFeatures = append(caps.MissingFeatures, "kernel 4.4+ required")
		return caps
	}

	// TCP connect probe needs kprobe + ring/perf buffer
	if f.HasKprobe && (f.HasRingBuffer || f.HasPerfBuffer) {
		if CheckKprobeTarget("tcp_connect") && CheckKprobeTarget("inet_csk_accept") {
			caps.CanUseTCPConnect = true
		} else {
			caps.Warnings = append(caps.Warnings, "tcp_connect or inet_csk_accept not available for kprobe")
		}
	}

	// HTTP probe needs tracepoint
	if f.HasTracepoint && (f.HasRingBuffer || f.HasPerfBuffer) {
		if CheckTracepointExists("syscalls", "sys_enter_read") {
			caps.CanUseHTTPProbe = true
		} else {
			caps.Warnings = append(caps.Warnings, "syscall tracepoints not available")
		}
	}

	// SSL probe needs uprobe
	if f.HasUprobe && (f.HasRingBuffer || f.HasPerfBuffer) {
		caps.CanUseSSLProbe = true
	}

	// DB probe is similar to HTTP
	caps.CanUseDBProbe = caps.CanUseHTTPProbe

	// FD tracking requires /proc filesystem access (always available on Linux)
	if _, err := os.Stat("/proc/net/tcp"); err == nil {
		caps.CanUseFDTracking = true
	} else {
		caps.Warnings = append(caps.Warnings, "/proc/net/tcp not available, FD tracking disabled")
	}

	// Profiler needs perf event + stack trace
	if f.HasPerfEvent && f.HasStackTrace {
		caps.CanUseProfiler = true
	}

	// Determine ring buffer vs perf buffer
	if f.HasRingBuffer {
		caps.UseRingBuffer = true
	} else if f.HasPerfBuffer {
		caps.UseRingBuffer = false
		caps.Warnings = append(caps.Warnings, "using perf buffer (kernel < 5.8)")
	} else {
		caps.MissingFeatures = append(caps.MissingFeatures, "no ring buffer or perf buffer support")
	}

	// BTF/CO-RE warnings
	if !f.HasBTF {
		caps.Warnings = append(caps.Warnings, "BTF not available, some features may not work")
	}

	return caps
}

// CreateCompatibleProgram creates a minimal BPF program to test loading capability
func CreateCompatibleProgram(progType ebpf.ProgramType) error {
	// Create a minimal BPF program that just returns 0
	prog, err := ebpf.NewProgram(&ebpf.ProgramSpec{
		Type: progType,
		Instructions: asm.Instructions{
			asm.Mov.Imm(asm.R0, 0),
			asm.Return(),
		},
		License: "GPL",
	})
	if err != nil {
		return err
	}
	prog.Close()
	return nil
}

// CompatibilityReport generates a human-readable compatibility report
func CompatibilityReport() string {
	f := DetectBPFFeatures()
	caps := CheckProbeCapabilities()

	var sb strings.Builder
	sb.WriteString("=== dogwatch Kernel Compatibility Report ===\n\n")

	sb.WriteString(fmt.Sprintf("Kernel Version: %s\n", f.KernelVersion))
	sb.WriteString(fmt.Sprintf("BTF Support: %v\n", f.HasBTF))
	sb.WriteString(fmt.Sprintf("CO-RE Support: %v\n", f.HasCORE))
	sb.WriteString("\n")

	sb.WriteString("Buffer Support:\n")
	sb.WriteString(fmt.Sprintf("  Ring Buffer (5.8+): %v\n", f.HasRingBuffer))
	sb.WriteString(fmt.Sprintf("  Perf Buffer (4.3+): %v\n", f.HasPerfBuffer))
	sb.WriteString("\n")

	sb.WriteString("Program Types:\n")
	sb.WriteString(fmt.Sprintf("  Kprobe: %v\n", f.HasKprobe))
	sb.WriteString(fmt.Sprintf("  Tracepoint: %v\n", f.HasTracepoint))
	sb.WriteString(fmt.Sprintf("  Uprobe: %v\n", f.HasUprobe))
	sb.WriteString(fmt.Sprintf("  Perf Event: %v\n", f.HasPerfEvent))
	sb.WriteString("\n")

	sb.WriteString("Map Types:\n")
	sb.WriteString(fmt.Sprintf("  Hash Map: %v\n", f.HasHashMap))
	sb.WriteString(fmt.Sprintf("  Stack Trace: %v\n", f.HasStackTrace))
	sb.WriteString(fmt.Sprintf("  LRU Hash (4.10+): %v\n", f.HasLRUHashMap))
	sb.WriteString(fmt.Sprintf("  Ring Buffer (5.8+): %v\n", f.HasRingBufMap))
	sb.WriteString("\n")

	sb.WriteString("Probe Capabilities:\n")
	sb.WriteString(fmt.Sprintf("  TCP Connect Probe: %v\n", caps.CanUseTCPConnect))
	sb.WriteString(fmt.Sprintf("  HTTP Probe: %v\n", caps.CanUseHTTPProbe))
	sb.WriteString(fmt.Sprintf("  SSL/TLS Probe: %v\n", caps.CanUseSSLProbe))
	sb.WriteString(fmt.Sprintf("  Database Probe: %v\n", caps.CanUseDBProbe))
	sb.WriteString(fmt.Sprintf("  CPU Profiler: %v\n", caps.CanUseProfiler))
	sb.WriteString(fmt.Sprintf("  FD-to-Socket Tracking: %v\n", caps.CanUseFDTracking))
	sb.WriteString(fmt.Sprintf("  Using Ring Buffer: %v\n", caps.UseRingBuffer))
	sb.WriteString("\n")

	if len(caps.MissingFeatures) > 0 {
		sb.WriteString("Missing Features:\n")
		for _, f := range caps.MissingFeatures {
			sb.WriteString(fmt.Sprintf("  - %s\n", f))
		}
		sb.WriteString("\n")
	}

	if len(caps.Warnings) > 0 {
		sb.WriteString("Warnings:\n")
		for _, w := range caps.Warnings {
			sb.WriteString(fmt.Sprintf("  - %s\n", w))
		}
	}

	return sb.String()
}
