package main

import (
	"debug/elf"
	"fmt"
)

func main() {
	path := "/lib/x86_64-linux-gnu/libssl.so.3"
	f, err := elf.Open(path)
	if err != nil {
		fmt.Printf("Open error: %v\n", err)
		return
	}
	defer f.Close()

	fmt.Printf("Opened: %s\n", path)

	syms, err := f.DynamicSymbols()
	if err != nil {
		fmt.Printf("DynamicSymbols error: %v\n", err)
		return
	}

	targets := []string{"SSL_write", "SSL_read", "SSL_write_ex", "SSL_read_ex"}
	for _, target := range targets {
		found := false
		for _, sym := range syms {
			if sym.Name == target {
				fmt.Printf("Symbol %s: offset 0x%x, size %d\n", target, sym.Value, sym.Size)
				found = true
				break
			}
		}
		if !found {
			fmt.Printf("Symbol %s: NOT FOUND\n", target)
		}
	}
}
