.PHONY: all generate build clean

CLANG ?= clang
GO ?= go

all: generate build

# Generate Go bindings from BPF C code
generate:
	cd internal/probe && $(GO) generate ./...

# Build the dogwatch binary
build:
	CGO_ENABLED=0 $(GO) build -o dogwatch ./cmd/dogwatch

# Clean build artifacts
clean:
	rm -f dogwatch
	rm -f internal/probe/bpf_*.go
	rm -f internal/probe/bpf_*.o

# Install dependencies
deps:
	$(GO) get github.com/cilium/ebpf@latest
	$(GO) get github.com/cilium/ebpf/cmd/bpf2go@latest
	$(GO) mod tidy

# Run dogwatch (requires root)
run: build
	sudo ./dogwatch
