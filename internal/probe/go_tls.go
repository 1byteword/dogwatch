package probe

import (
	"bufio"
	"debug/elf"
	"debug/gosym"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// GoTLSProbe detects and intercepts Go crypto/tls connections
type GoTLSProbe struct {
	mu            sync.RWMutex
	goBinaries    map[string]*GoBinaryInfo // path -> info
	goVersion     *regexp.Regexp
	symbolCache   map[string]map[string]uint64 // binary -> symbol -> offset
}

// GoBinaryInfo holds information about a Go binary
type GoBinaryInfo struct {
	Path        string
	GoVersion   string
	TLSSymbols  map[string]uint64 // symbol name -> file offset
	IsValid     bool
	BuildInfo   string
}

// GoTLSSymbols defines the symbols we need to hook for Go TLS
var GoTLSSymbols = []string{
	"crypto/tls.(*Conn).Read",
	"crypto/tls.(*Conn).Write",
	"crypto/tls.(*Conn).Handshake",
	"crypto/tls.(*Conn).Close",
}

// NewGoTLSProbe creates a new Go TLS probe
func NewGoTLSProbe() *GoTLSProbe {
	return &GoTLSProbe{
		goBinaries:  make(map[string]*GoBinaryInfo),
		goVersion:   regexp.MustCompile(`go(\d+\.\d+(?:\.\d+)?)`),
		symbolCache: make(map[string]map[string]uint64),
	}
}

// IsGoBinary checks if a file is a Go binary
func (p *GoTLSProbe) IsGoBinary(path string) (bool, string) {
	f, err := elf.Open(path)
	if err != nil {
		return false, ""
	}
	defer f.Close()

	// Check for Go-specific sections
	for _, section := range f.Sections {
		if section.Name == ".go.buildinfo" || section.Name == ".gopclntab" {
			version := p.extractGoVersion(f)
			return true, version
		}
	}

	// Also check for Go symbols
	symbols, err := f.Symbols()
	if err == nil {
		for _, sym := range symbols {
			if strings.HasPrefix(sym.Name, "runtime.") || strings.HasPrefix(sym.Name, "go.") {
				version := p.extractGoVersion(f)
				return true, version
			}
		}
	}

	return false, ""
}

// extractGoVersion tries to extract Go version from binary
func (p *GoTLSProbe) extractGoVersion(f *elf.File) string {
	// Try .go.buildinfo section
	if section := f.Section(".go.buildinfo"); section != nil {
		data, err := section.Data()
		if err == nil {
			// Build info has version string embedded
			if matches := p.goVersion.FindSubmatch(data); len(matches) > 1 {
				return string(matches[1])
			}
		}
	}

	// Try rodata for version string
	if section := f.Section(".rodata"); section != nil {
		data, err := section.Data()
		if err == nil && len(data) > 0 {
			// Look for "go1.XX" pattern
			if matches := p.goVersion.FindSubmatch(data); len(matches) > 1 {
				return string(matches[1])
			}
		}
	}

	return "unknown"
}

// AnalyzeBinary analyzes a Go binary and extracts TLS-related symbols
func (p *GoTLSProbe) AnalyzeBinary(path string) (*GoBinaryInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check cache
	if info, ok := p.goBinaries[path]; ok {
		return info, nil
	}

	f, err := elf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening ELF: %w", err)
	}
	defer f.Close()

	info := &GoBinaryInfo{
		Path:       path,
		TLSSymbols: make(map[string]uint64),
	}

	// Get Go version
	isGo, version := p.IsGoBinary(path)
	if !isGo {
		return nil, fmt.Errorf("not a Go binary: %s", path)
	}
	info.GoVersion = version

	// Get pclntab for symbol lookup
	pclntab := f.Section(".gopclntab")
	if pclntab == nil {
		return nil, fmt.Errorf("no .gopclntab section")
	}

	pclntabData, err := pclntab.Data()
	if err != nil {
		return nil, fmt.Errorf("reading pclntab: %w", err)
	}

	// Get symtab
	symtab := f.Section(".gosymtab")
	var symtabData []byte
	if symtab != nil {
		symtabData, _ = symtab.Data()
	}

	// Parse Go symbol table
	pcln := gosym.NewLineTable(pclntabData, f.Section(".text").Addr)
	table, err := gosym.NewTable(symtabData, pcln)
	if err != nil {
		// Try fallback to regular ELF symbols
		return p.analyzeFallback(path, f, info)
	}

	// Find TLS-related symbols
	for _, sym := range GoTLSSymbols {
		if fn := table.LookupFunc(sym); fn != nil {
			info.TLSSymbols[sym] = fn.Entry
		}
	}

	if len(info.TLSSymbols) == 0 {
		// Fallback to ELF symbols
		return p.analyzeFallback(path, f, info)
	}

	info.IsValid = len(info.TLSSymbols) >= 2
	p.goBinaries[path] = info

	return info, nil
}

// analyzeFallback uses regular ELF symbols when Go symbol table fails
func (p *GoTLSProbe) analyzeFallback(path string, f *elf.File, info *GoBinaryInfo) (*GoBinaryInfo, error) {
	symbols, err := f.Symbols()
	if err != nil {
		return nil, fmt.Errorf("reading symbols: %w", err)
	}

	for _, sym := range symbols {
		for _, target := range GoTLSSymbols {
			// Go symbols might be mangled
			if strings.Contains(sym.Name, strings.ReplaceAll(target, "/", ".")) ||
				strings.Contains(sym.Name, target) {
				info.TLSSymbols[target] = sym.Value
			}
		}
	}

	info.IsValid = len(info.TLSSymbols) >= 2
	p.goBinaries[path] = info

	return info, nil
}

// ScanProcesses scans running processes for Go binaries with TLS
func (p *GoTLSProbe) ScanProcesses() ([]*GoBinaryInfo, error) {
	var results []*GoBinaryInfo

	procDir, err := os.Open("/proc")
	if err != nil {
		return nil, err
	}
	defer procDir.Close()

	entries, err := procDir.Readdirnames(-1)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)

	for _, entry := range entries {
		// Skip non-PID directories
		if entry[0] < '0' || entry[0] > '9' {
			continue
		}

		exePath := filepath.Join("/proc", entry, "exe")
		realPath, err := os.Readlink(exePath)
		if err != nil {
			continue
		}

		// Skip already analyzed
		if seen[realPath] {
			continue
		}
		seen[realPath] = true

		// Check if Go binary
		isGo, _ := p.IsGoBinary(realPath)
		if !isGo {
			continue
		}

		info, err := p.AnalyzeBinary(realPath)
		if err != nil {
			continue
		}

		if info.IsValid {
			results = append(results, info)
		}
	}

	return results, nil
}

// GetHookPoints returns the uprobe hook points for a Go binary
func (p *GoTLSProbe) GetHookPoints(binaryPath string) (map[string]uint64, error) {
	info, err := p.AnalyzeBinary(binaryPath)
	if err != nil {
		return nil, err
	}

	return info.TLSSymbols, nil
}

// FindGoBinariesInDirectory recursively finds Go binaries in a directory
func (p *GoTLSProbe) FindGoBinariesInDirectory(dir string) ([]*GoBinaryInfo, error) {
	var results []*GoBinaryInfo

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if info.IsDir() {
			return nil
		}

		// Check if executable
		if info.Mode()&0111 == 0 {
			return nil
		}

		isGo, _ := p.IsGoBinary(path)
		if !isGo {
			return nil
		}

		binInfo, err := p.AnalyzeBinary(path)
		if err != nil {
			return nil
		}

		if binInfo.IsValid {
			results = append(results, binInfo)
		}

		return nil
	})

	return results, err
}

// GoConnStruct represents the memory layout of a Go tls.Conn
// This varies by Go version, so we use offsets
type GoConnStruct struct {
	// Go 1.20+ offsets (approximate - may vary)
	ConnOffset    uint64 // Offset to underlying net.Conn
	ConfigOffset  uint64 // Offset to *tls.Config
	BufOffset     uint64 // Offset to internal buffer
	HandshakeDone uint64 // Offset to handshakeComplete field
}

// GetConnStructOffsets returns tls.Conn struct offsets for a Go version
func GetConnStructOffsets(version string) GoConnStruct {
	// These are approximate offsets and may need adjustment
	// based on actual Go version analysis

	// Default offsets for Go 1.20+
	offsets := GoConnStruct{
		ConnOffset:    0,   // First field is net.Conn
		ConfigOffset:  16,  // Second pointer field
		BufOffset:     128, // Internal buffer
		HandshakeDone: 200, // Handshake complete flag
	}

	// Adjust for older versions if needed
	if strings.HasPrefix(version, "1.18") || strings.HasPrefix(version, "1.19") {
		offsets.BufOffset = 120
		offsets.HandshakeDone = 192
	}

	return offsets
}

// Stats returns statistics about Go TLS probe
func (p *GoTLSProbe) Stats() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()

	validCount := 0
	versions := make(map[string]int)

	for _, info := range p.goBinaries {
		if info.IsValid {
			validCount++
		}
		versions[info.GoVersion]++
	}

	return map[string]interface{}{
		"analyzed_binaries": len(p.goBinaries),
		"valid_binaries":    validCount,
		"go_versions":       versions,
	}
}

// GetMapsForPID reads /proc/[pid]/maps and finds Go-related mappings
func GetMapsForPID(pid int) ([]MemoryMapping, error) {
	mapsPath := fmt.Sprintf("/proc/%d/maps", pid)
	f, err := os.Open(mapsPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var mappings []MemoryMapping
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		mapping := parseMapLine(scanner.Text())
		if mapping != nil && mapping.Executable {
			mappings = append(mappings, *mapping)
		}
	}

	return mappings, scanner.Err()
}
