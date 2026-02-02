package probe

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestSymbolTableSort(t *testing.T) {
	table := &SymbolTable{
		symbols: []Symbol{
			{Address: 0x3000, Name: "func_c"},
			{Address: 0x1000, Name: "func_a"},
			{Address: 0x2000, Name: "func_b"},
		},
	}

	table.Sort()

	if !table.sorted {
		t.Error("Table should be marked as sorted")
	}

	if table.symbols[0].Address != 0x1000 {
		t.Errorf("Expected first address to be 0x1000, got 0x%x", table.symbols[0].Address)
	}
	if table.symbols[1].Address != 0x2000 {
		t.Errorf("Expected second address to be 0x2000, got 0x%x", table.symbols[1].Address)
	}
	if table.symbols[2].Address != 0x3000 {
		t.Errorf("Expected third address to be 0x3000, got 0x%x", table.symbols[2].Address)
	}
}

func TestSymbolTableLookup(t *testing.T) {
	table := &SymbolTable{
		symbols: []Symbol{
			{Address: 0x1000, Size: 0x100, Name: "func_a"},
			{Address: 0x2000, Size: 0x200, Name: "func_b"},
			{Address: 0x3000, Size: 0x50, Name: "func_c"},
		},
	}
	table.Sort()

	tests := []struct {
		addr     uint64
		expected string
		found    bool
	}{
		{0x1000, "func_a", true},  // Exact match
		{0x1050, "func_a", true},  // Within func_a
		{0x10FF, "func_a", true},  // End of func_a
		{0x2000, "func_b", true},  // Exact match func_b
		{0x2100, "func_b", true},  // Within func_b
		{0x3000, "func_c", true},  // Exact match func_c
		{0x0500, "", false},       // Before any symbol
		{0x4000, "", false},       // After last symbol (with size check)
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			sym, found := table.Lookup(tt.addr)
			if found != tt.found {
				t.Errorf("Lookup(0x%x) found = %v, want %v", tt.addr, found, tt.found)
				return
			}
			if found && sym.Name != tt.expected {
				t.Errorf("Lookup(0x%x) name = %s, want %s", tt.addr, sym.Name, tt.expected)
			}
		})
	}
}

func TestParseMapLine(t *testing.T) {
	tests := []struct {
		line     string
		wantNil  bool
		pathname string
		perms    string
	}{
		{
			"7f8c9a200000-7f8c9a400000 r-xp 00000000 08:01 12345 /lib/x86_64-linux-gnu/libc.so.6",
			false, "/lib/x86_64-linux-gnu/libc.so.6", "r-xp",
		},
		{
			"7ffd12300000-7ffd12321000 rw-p 00000000 00:00 0 [stack]",
			false, "[stack]", "rw-p",
		},
		{
			"7f8c9a000000-7f8c9a100000 r--p 00000000 08:01 12345",
			false, "", "r--p",
		},
		{
			"invalid line",
			true, "", "",
		},
	}

	for i, tt := range tests {
		name := tt.line
		if len(name) > 20 {
			name = name[:20]
		}
		t.Run(fmt.Sprintf("case_%d_%s", i, name), func(t *testing.T) {
			mapping := parseMapLine(tt.line)
			if tt.wantNil {
				if mapping != nil {
					t.Errorf("Expected nil for invalid line")
				}
				return
			}
			if mapping == nil {
				t.Fatalf("Expected non-nil mapping")
			}
			if mapping.Pathname != tt.pathname {
				t.Errorf("Pathname = %q, want %q", mapping.Pathname, tt.pathname)
			}
			if mapping.Perms != tt.perms {
				t.Errorf("Perms = %q, want %q", mapping.Perms, tt.perms)
			}
		})
	}
}

func TestSymbolResolverKernel(t *testing.T) {
	resolver := NewSymbolResolver()

	// Load kernel symbols
	err := resolver.LoadKernelSymbols()
	if err != nil {
		// May fail if not running as root or kallsyms is restricted
		t.Skipf("Could not load kernel symbols (may need root): %v", err)
	}

	stats := resolver.Stats()
	kernelSyms := stats["kernel_symbols"].(int)
	t.Logf("Loaded %d kernel symbols", kernelSyms)

	if kernelSyms == 0 {
		t.Skip("No kernel symbols loaded (may be restricted)")
	}

	// Try to resolve a common kernel function
	// We don't know the exact address, but if symbols loaded, resolution should work
	// Just verify the resolver doesn't panic
	result := resolver.ResolveKernel(0xffffffff81000000)
	t.Logf("Resolved 0xffffffff81000000 to: %s", result)
}

func TestSymbolResolverUser(t *testing.T) {
	resolver := NewSymbolResolver()

	// Test resolving our own process
	pid := uint32(os.Getpid())

	// Get an address from our own stack
	// We can't easily get a real stack address, but we can test the mapping loading
	result := resolver.ResolveUser(pid, 0x400000)
	t.Logf("Resolved 0x400000 in PID %d to: %s", pid, result)

	// Check that process mappings were cached
	stats := resolver.Stats()
	cachedProcs := stats["cached_processes"].(int)
	if cachedProcs == 0 {
		t.Error("Expected process to be cached after resolution")
	}
}

func TestSymbolResolverELF(t *testing.T) {
	resolver := NewSymbolResolver()

	// Try to load symbols from libc (should exist on most systems)
	libcPaths := []string{
		"/lib/x86_64-linux-gnu/libc.so.6",
		"/lib64/libc.so.6",
		"/usr/lib/libc.so.6",
	}

	var table *SymbolTable
	var foundPath string
	for _, path := range libcPaths {
		if _, err := os.Stat(path); err == nil {
			table = resolver.loadELFSymbols(path)
			if table != nil {
				foundPath = path
				break
			}
		}
	}

	if table == nil {
		t.Skip("Could not find libc to test ELF symbol loading")
	}

	t.Logf("Loaded %d symbols from %s", len(table.symbols), foundPath)

	// Look for a common function
	commonFuncs := []string{"printf", "malloc", "free", "write", "read"}
	foundAny := false
	for _, fname := range commonFuncs {
		for _, sym := range table.symbols {
			if sym.Name == fname {
				t.Logf("Found %s at 0x%x", fname, sym.Address)
				foundAny = true
				break
			}
		}
	}

	if !foundAny {
		t.Log("Warning: Could not find common functions in libc (may be stripped)")
	}
}

func TestGlobalSymbolResolver(t *testing.T) {
	resolver := GetSymbolResolver()
	if resolver == nil {
		t.Fatal("GetSymbolResolver returned nil")
	}

	// Should return the same instance
	resolver2 := GetSymbolResolver()
	if resolver != resolver2 {
		t.Error("GetSymbolResolver should return the same instance")
	}

	stats := resolver.Stats()
	t.Logf("Resolver stats: %+v", stats)
}

func TestSymbolResolverClearCaches(t *testing.T) {
	resolver := NewSymbolResolver()

	// Load some data
	resolver.LoadKernelSymbols()
	resolver.ResolveUser(uint32(os.Getpid()), 0x400000)

	stats := resolver.Stats()
	if stats["cached_processes"].(int) == 0 {
		t.Skip("No processes cached to test clearing")
	}

	// Clear caches
	resolver.ClearAllCaches()

	stats = resolver.Stats()
	if stats["cached_processes"].(int) != 0 {
		t.Error("Process cache should be empty after clear")
	}
	if stats["cached_elf_files"].(int) != 0 {
		t.Error("ELF cache should be empty after clear")
	}
}

func TestResolveKernelFormat(t *testing.T) {
	resolver := NewSymbolResolver()
	err := resolver.LoadKernelSymbols()
	if err != nil {
		t.Skipf("Could not load kernel symbols: %v", err)
	}

	// Test that resolved symbols have proper format
	result := resolver.ResolveKernel(0xffffffff81000000)

	// Should either be a symbol name or a hex address
	if !strings.HasPrefix(result, "0x") {
		// It's a symbol name, should not be empty
		if result == "" {
			t.Error("Resolved symbol should not be empty")
		}
		// Offset format check
		if strings.Contains(result, "+0x") {
			parts := strings.Split(result, "+0x")
			if len(parts) != 2 {
				t.Errorf("Invalid offset format: %s", result)
			}
		}
	}
}
