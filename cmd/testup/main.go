package main

import (
	"fmt"

	"github.com/cilium/ebpf/link"
)

func main() {
	path := "/lib/x86_64-linux-gnu/libssl.so.3"
	ex, err := link.OpenExecutable(path)
	if err != nil {
		fmt.Printf("OpenExecutable error: %v\n", err)
		return
	}

	fmt.Printf("Opened: %s\n", path)

	syms := []string{"SSL_write", "SSL_read", "SSL_write_ex", "SSL_read_ex"}
	for _, s := range syms {
		offset, err := ex.Address(s)
		if err != nil {
			fmt.Printf("Symbol %s: ERROR - %v\n", s, err)
		} else {
			fmt.Printf("Symbol %s: offset 0x%x\n", s, offset)
		}
	}
}
