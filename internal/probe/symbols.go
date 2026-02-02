package probe

import (
	"bufio"
	"debug/elf"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// SymbolResolver resolves memory addresses to function names
type SymbolResolver struct {
	mu            sync.RWMutex
	kernelSymbols *SymbolTable
	processCache  map[uint32]*ProcessSymbols // PID -> symbols
	elfCache      map[string]*SymbolTable    // path -> symbols
}

// SymbolTable holds symbols sorted by address for binary search
type SymbolTable struct {
	symbols []Symbol
	sorted  bool
}

// Symbol represents a resolved symbol
type Symbol struct {
	Address uint64
	Size    uint64
	Name    string
	Type    string // "F" for function, "O" for object, etc.
}

// ProcessSymbols holds memory mappings and symbols for a process
type ProcessSymbols struct {
	PID      uint32
	Mappings []MemoryMapping
}

// MemoryMapping represents a mapped region from /proc/[pid]/maps
type MemoryMapping struct {
	StartAddr  uint64
	EndAddr    uint64
	Perms      string
	Offset     uint64
	Device     string
	Inode      uint64
	Pathname   string
	Executable bool
}

// NewSymbolResolver creates a new symbol resolver
func NewSymbolResolver() *SymbolResolver {
	return &SymbolResolver{
		processCache: make(map[uint32]*ProcessSymbols),
		elfCache:     make(map[string]*SymbolTable),
	}
}

// LoadKernelSymbols loads kernel symbols from /proc/kallsyms
func (r *SymbolResolver) LoadKernelSymbols() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, err := os.Open("/proc/kallsyms")
	if err != nil {
		return fmt.Errorf("opening kallsyms: %w", err)
	}
	defer f.Close()

	table := &SymbolTable{
		symbols: make([]Symbol, 0, 100000),
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		addr, err := strconv.ParseUint(fields[0], 16, 64)
		if err != nil || addr == 0 {
			continue
		}

		sym := Symbol{
			Address: addr,
			Name:    fields[2],
			Type:    fields[1],
		}

		// Extract module name if present
		if len(fields) >= 4 {
			sym.Name = fmt.Sprintf("%s [%s]", fields[2], strings.Trim(fields[3], "[]"))
		}

		table.symbols = append(table.symbols, sym)
	}

	table.Sort()
	r.kernelSymbols = table

	return scanner.Err()
}

// Sort sorts the symbol table by address
func (t *SymbolTable) Sort() {
	sort.Slice(t.symbols, func(i, j int) bool {
		return t.symbols[i].Address < t.symbols[j].Address
	})
	t.sorted = true
}

// Lookup finds the symbol containing the given address
func (t *SymbolTable) Lookup(addr uint64) (Symbol, bool) {
	if !t.sorted || len(t.symbols) == 0 {
		return Symbol{}, false
	}

	// Binary search for the largest address <= addr
	idx := sort.Search(len(t.symbols), func(i int) bool {
		return t.symbols[i].Address > addr
	})

	if idx == 0 {
		return Symbol{}, false
	}

	sym := t.symbols[idx-1]

	// If the symbol has a size, check if addr is within bounds
	if sym.Size > 0 && addr >= sym.Address+sym.Size {
		return Symbol{}, false
	}

	return sym, true
}

// ResolveKernel resolves a kernel address to a symbol name
func (r *SymbolResolver) ResolveKernel(addr uint64) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.kernelSymbols == nil {
		return fmt.Sprintf("0x%x", addr)
	}

	sym, found := r.kernelSymbols.Lookup(addr)
	if !found {
		return fmt.Sprintf("0x%x", addr)
	}

	offset := addr - sym.Address
	if offset > 0 {
		return fmt.Sprintf("%s+0x%x", sym.Name, offset)
	}
	return sym.Name
}

// ResolveUser resolves a user-space address for a given PID
func (r *SymbolResolver) ResolveUser(pid uint32, addr uint64) string {
	r.mu.Lock()
	procSyms, ok := r.processCache[pid]
	if !ok {
		// Load process mappings
		procSyms = r.loadProcessMappings(pid)
		if procSyms != nil {
			r.processCache[pid] = procSyms
		}
	}
	r.mu.Unlock()

	if procSyms == nil {
		return fmt.Sprintf("0x%x", addr)
	}

	// Find the mapping containing this address
	for _, mapping := range procSyms.Mappings {
		if addr >= mapping.StartAddr && addr < mapping.EndAddr {
			if mapping.Pathname == "" || mapping.Pathname == "[vdso]" || mapping.Pathname == "[vsyscall]" {
				return fmt.Sprintf("[%s]+0x%x", mapping.Pathname, addr-mapping.StartAddr)
			}

			// Get symbol table for this file
			r.mu.Lock()
			symTable, ok := r.elfCache[mapping.Pathname]
			if !ok {
				symTable = r.loadELFSymbols(mapping.Pathname)
				if symTable != nil {
					r.elfCache[mapping.Pathname] = symTable
				}
			}
			r.mu.Unlock()

			if symTable != nil {
				// Calculate the file offset
				fileOffset := addr - mapping.StartAddr + mapping.Offset
				sym, found := symTable.Lookup(fileOffset)
				if found {
					basename := filepath.Base(mapping.Pathname)
					offset := fileOffset - sym.Address
					if offset > 0 {
						return fmt.Sprintf("%s`%s+0x%x", basename, sym.Name, offset)
					}
					return fmt.Sprintf("%s`%s", basename, sym.Name)
				}
			}

			// Fall back to showing the library and offset
			basename := filepath.Base(mapping.Pathname)
			return fmt.Sprintf("%s+0x%x", basename, addr-mapping.StartAddr)
		}
	}

	return fmt.Sprintf("0x%x", addr)
}

// loadProcessMappings reads /proc/[pid]/maps
func (r *SymbolResolver) loadProcessMappings(pid uint32) *ProcessSymbols {
	mapsPath := fmt.Sprintf("/proc/%d/maps", pid)
	f, err := os.Open(mapsPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	procSyms := &ProcessSymbols{
		PID:      pid,
		Mappings: make([]MemoryMapping, 0),
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		mapping := parseMapLine(scanner.Text())
		if mapping != nil {
			procSyms.Mappings = append(procSyms.Mappings, *mapping)
		}
	}

	return procSyms
}

// parseMapLine parses a line from /proc/[pid]/maps
func parseMapLine(line string) *MemoryMapping {
	// Format: address perms offset dev inode pathname
	// Example: 7f8c9a200000-7f8c9a400000 r-xp 00000000 08:01 12345 /lib/x86_64-linux-gnu/libc.so.6
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return nil
	}

	// Parse address range
	addrParts := strings.Split(fields[0], "-")
	if len(addrParts) != 2 {
		return nil
	}

	startAddr, err := strconv.ParseUint(addrParts[0], 16, 64)
	if err != nil {
		return nil
	}
	endAddr, err := strconv.ParseUint(addrParts[1], 16, 64)
	if err != nil {
		return nil
	}

	// Parse offset
	offset, err := strconv.ParseUint(fields[2], 16, 64)
	if err != nil {
		offset = 0
	}

	// Parse inode
	inode, _ := strconv.ParseUint(fields[4], 10, 64)

	mapping := &MemoryMapping{
		StartAddr:  startAddr,
		EndAddr:    endAddr,
		Perms:      fields[1],
		Offset:     offset,
		Device:     fields[3],
		Inode:      inode,
		Executable: strings.Contains(fields[1], "x"),
	}

	// Get pathname if present
	if len(fields) >= 6 {
		mapping.Pathname = fields[5]
	}

	return mapping
}

// loadELFSymbols loads symbols from an ELF file
func (r *SymbolResolver) loadELFSymbols(path string) *SymbolTable {
	f, err := elf.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	table := &SymbolTable{
		symbols: make([]Symbol, 0),
	}

	// Try to load dynamic symbols first (always present in shared libs)
	dynsyms, err := f.DynamicSymbols()
	if err == nil {
		for _, sym := range dynsyms {
			if sym.Value == 0 || sym.Name == "" {
				continue
			}
			// Only include function symbols
			if elf.ST_TYPE(sym.Info) == elf.STT_FUNC {
				table.symbols = append(table.symbols, Symbol{
					Address: sym.Value,
					Size:    sym.Size,
					Name:    sym.Name,
					Type:    "F",
				})
			}
		}
	}

	// Also try regular symbol table (may not be present in stripped binaries)
	syms, err := f.Symbols()
	if err == nil {
		for _, sym := range syms {
			if sym.Value == 0 || sym.Name == "" {
				continue
			}
			// Only include function symbols
			if elf.ST_TYPE(sym.Info) == elf.STT_FUNC {
				// Check if we already have this symbol
				exists := false
				for _, existing := range table.symbols {
					if existing.Address == sym.Value {
						exists = true
						break
					}
				}
				if !exists {
					table.symbols = append(table.symbols, Symbol{
						Address: sym.Value,
						Size:    sym.Size,
						Name:    sym.Name,
						Type:    "F",
					})
				}
			}
		}
	}

	if len(table.symbols) == 0 {
		return nil
	}

	table.Sort()
	return table
}

// ClearProcessCache clears the cache for a specific PID
func (r *SymbolResolver) ClearProcessCache(pid uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.processCache, pid)
}

// ClearAllCaches clears all cached data
func (r *SymbolResolver) ClearAllCaches() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.processCache = make(map[uint32]*ProcessSymbols)
	r.elfCache = make(map[string]*SymbolTable)
}

// Stats returns statistics about the resolver's cache
func (r *SymbolResolver) Stats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	kernelSymCount := 0
	if r.kernelSymbols != nil {
		kernelSymCount = len(r.kernelSymbols.symbols)
	}

	elfSymCount := 0
	for _, table := range r.elfCache {
		elfSymCount += len(table.symbols)
	}

	return map[string]interface{}{
		"kernel_symbols":    kernelSymCount,
		"cached_processes":  len(r.processCache),
		"cached_elf_files":  len(r.elfCache),
		"cached_elf_symbols": elfSymCount,
	}
}

// Global resolver instance
var (
	globalResolver     *SymbolResolver
	globalResolverOnce sync.Once
)

// GetSymbolResolver returns the global symbol resolver instance
func GetSymbolResolver() *SymbolResolver {
	globalResolverOnce.Do(func() {
		globalResolver = NewSymbolResolver()
		// Pre-load kernel symbols
		if err := globalResolver.LoadKernelSymbols(); err != nil {
			// Log but don't fail - kernel symbol resolution will just not work
			fmt.Printf("Warning: failed to load kernel symbols: %v\n", err)
		}
	})
	return globalResolver
}
