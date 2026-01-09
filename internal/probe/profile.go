package probe

import (
	"encoding/binary"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"golang.org/x/sys/unix"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall" -target amd64 profile ../../bpf/profile.c -- -I../../bpf

const (
	maxStackDepth = 32
	sampleFreqHz  = 49 // Sample frequency in Hz (49 to avoid lockstep with other timers)
)

// StackKey matches the BPF struct
type StackKey struct {
	PID           uint32
	TGID          uint32
	KernelStackID int32
	UserStackID   int32
	Comm          [16]byte
}

// ProfileProbe handles CPU profiling via perf events
type ProfileProbe struct {
	objs    *profileObjects
	links   []link.Link
	mu      sync.RWMutex
	running bool
}

// StackSample represents a sampled stack with count
type StackSample struct {
	Comm        string   `json:"comm"`
	PID         uint32   `json:"pid"`
	Count       uint64   `json:"count"`
	KernelStack []uint64 `json:"kernel_stack,omitempty"`
	UserStack   []uint64 `json:"user_stack,omitempty"`
}

// FlameNode represents a node in the flame graph
type FlameNode struct {
	Name     string       `json:"name"`
	Value    uint64       `json:"value"`
	Children []*FlameNode `json:"children,omitempty"`
}

// NewProfileProbe creates a new CPU profiler
func NewProfileProbe() (*ProfileProbe, error) {
	objs := &profileObjects{}
	if err := loadProfileObjects(objs, nil); err != nil {
		return nil, fmt.Errorf("loading profile objects: %w", err)
	}

	p := &ProfileProbe{
		objs: objs,
	}

	// Attach to each CPU
	numCPU := runtime.NumCPU()
	for cpu := 0; cpu < numCPU; cpu++ {
		l, err := attachPerfEvent(cpu, objs.ProfileCpu)
		if err != nil {
			p.Close()
			return nil, fmt.Errorf("attaching to CPU %d: %w", cpu, err)
		}
		p.links = append(p.links, l)
	}

	p.running = true
	return p, nil
}

// attachPerfEvent attaches the BPF program to a CPU's perf event
func attachPerfEvent(cpu int, prog *ebpf.Program) (link.Link, error) {
	// Create perf event for CPU sampling
	attr := unix.PerfEventAttr{
		Type:   unix.PERF_TYPE_SOFTWARE,
		Config: unix.PERF_COUNT_SW_CPU_CLOCK,
		Size:   uint32(unsafe.Sizeof(unix.PerfEventAttr{})),
		Sample: uint64(sampleFreqHz),
		Bits:   unix.PerfBitFreq,
	}

	fd, err := unix.PerfEventOpen(&attr, -1, cpu, -1, unix.PERF_FLAG_FD_CLOEXEC)
	if err != nil {
		return nil, fmt.Errorf("perf_event_open: %w", err)
	}

	// Attach BPF program to perf event
	l, err := link.AttachRawLink(link.RawLinkOptions{
		Target:  fd,
		Program: prog,
		Attach:  ebpf.AttachPerfEvent,
	})
	if err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("attach raw link: %w", err)
	}

	return l, nil
}

// GetSamples returns the current stack samples
func (p *ProfileProbe) GetSamples() ([]StackSample, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.running {
		return nil, fmt.Errorf("profiler not running")
	}

	var samples []StackSample

	var key StackKey
	var count uint64

	iter := p.objs.StackCounts.Iterate()
	for iter.Next(&key, &count) {
		sample := StackSample{
			Comm:  strings.TrimRight(string(key.Comm[:]), "\x00"),
			PID:   key.PID,
			Count: count,
		}

		// Read kernel stack
		if key.KernelStackID >= 0 {
			stack, err := p.readStack(key.KernelStackID)
			if err == nil {
				sample.KernelStack = stack
			}
		}

		// Read user stack
		if key.UserStackID >= 0 {
			stack, err := p.readStack(key.UserStackID)
			if err == nil {
				sample.UserStack = stack
			}
		}

		samples = append(samples, sample)
	}

	// Sort by count descending
	sort.Slice(samples, func(i, j int) bool {
		return samples[i].Count > samples[j].Count
	})

	return samples, nil
}

// readStack reads a stack trace from the stack_traces map
func (p *ProfileProbe) readStack(stackID int32) ([]uint64, error) {
	stackBuf := make([]byte, maxStackDepth*8)
	err := p.objs.StackTraces.Lookup(uint32(stackID), &stackBuf)
	if err != nil {
		return nil, err
	}

	var stack []uint64
	for i := 0; i < maxStackDepth; i++ {
		addr := binary.LittleEndian.Uint64(stackBuf[i*8:])
		if addr == 0 {
			break
		}
		stack = append(stack, addr)
	}

	return stack, nil
}

// GetFlameGraph returns data formatted for flame graph visualization
func (p *ProfileProbe) GetFlameGraph() (interface{}, error) {
	samples, err := p.GetSamples()
	if err != nil {
		return nil, err
	}

	// Build flame graph from samples
	root := &FlameNode{
		Name:     "root",
		Value:    0,
		Children: make([]*FlameNode, 0),
	}

	for _, sample := range samples {
		// Build stack path: comm -> kernel stack (reversed) -> user stack (reversed)
		var path []string
		path = append(path, sample.Comm)

		// Add kernel stack frames (reversed, leaf to root)
		for i := len(sample.KernelStack) - 1; i >= 0; i-- {
			path = append(path, fmt.Sprintf("0x%x", sample.KernelStack[i]))
		}

		// Add user stack frames (reversed)
		for i := len(sample.UserStack) - 1; i >= 0; i-- {
			path = append(path, fmt.Sprintf("0x%x", sample.UserStack[i]))
		}

		// Add to flame graph
		addToFlameGraph(root, path, sample.Count)
	}

	// Calculate total value
	var total uint64
	for _, child := range root.Children {
		total += child.Value
	}
	root.Value = total

	return root, nil
}

func addToFlameGraph(node *FlameNode, path []string, count uint64) {
	if len(path) == 0 {
		return
	}

	name := path[0]
	var child *FlameNode

	for _, c := range node.Children {
		if c.Name == name {
			child = c
			break
		}
	}

	if child == nil {
		child = &FlameNode{
			Name:     name,
			Value:    0,
			Children: make([]*FlameNode, 0),
		}
		node.Children = append(node.Children, child)
	}

	child.Value += count

	if len(path) > 1 {
		addToFlameGraph(child, path[1:], count)
	}
}

// ClearSamples clears all collected samples
func (p *ProfileProbe) ClearSamples() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Delete all entries from stack_counts
	var key StackKey
	var count uint64
	iter := p.objs.StackCounts.Iterate()
	var keysToDelete []StackKey

	for iter.Next(&key, &count) {
		keysToDelete = append(keysToDelete, key)
	}

	for _, k := range keysToDelete {
		p.objs.StackCounts.Delete(k)
	}

	return nil
}

// Close cleans up the profiler
func (p *ProfileProbe) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.running = false

	for _, l := range p.links {
		l.Close()
	}

	if p.objs != nil {
		p.objs.Close()
	}

	return nil
}

// resolveSymbol tries to resolve an address to a symbol name
// This is a placeholder - real symbol resolution requires reading /proc/kallsyms
// and /proc/[pid]/maps
func resolveSymbol(addr uint64) string {
	// For now, just return the hex address
	// TODO: Implement proper symbol resolution
	return fmt.Sprintf("0x%x", addr)
}

// LoadKallsyms loads kernel symbols from /proc/kallsyms
func LoadKallsyms() (map[uint64]string, error) {
	symbols := make(map[uint64]string)

	data, err := os.ReadFile("/proc/kallsyms")
	if err != nil {
		return nil, err
	}

	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		addr, err := parseHex(fields[0])
		if err != nil {
			continue
		}

		symbols[addr] = fields[2]
	}

	return symbols, nil
}

func parseHex(s string) (uint64, error) {
	var val uint64
	_, err := fmt.Sscanf(s, "%x", &val)
	return val, err
}
