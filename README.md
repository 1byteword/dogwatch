# dogwatch

eBPF-powered observability for Linux. Zero-config, single binary, instant insights.

```
┌─────────────────────────────────────────────────────────────┐
│  dogwatch - see what your services are doing in real-time  │
└─────────────────────────────────────────────────────────────┘
```

## What it does

- **TCP Connection Tracking** - See every outbound connection with process name, PID, destination
- **HTTP Tracing** - Auto-discover endpoints, request counts, error rates
- **Latency Metrics** - P50, P99, average response times per endpoint
- **Web Dashboard** - Real-time UI at `localhost:9999`
- **Zero Config** - Just run it. No agents to configure, no code changes.

## Quick Install

```bash
curl -fsSL https://raw.githubusercontent.com/1byteword/dogwatch/main/install.sh | sudo bash
```

Or with wget:
```bash
wget -qO- https://raw.githubusercontent.com/1byteword/dogwatch/main/install.sh | sudo bash
```

## Usage

```bash
# Start dogwatch (requires root for eBPF)
sudo dogwatch

# Open http://localhost:9999 in your browser
```

**CLI Flags:**
```
-port 9999    Web UI port (default: 9999)
-i 5          Stats refresh interval in seconds (default: 5)
-v            Verbose mode - show individual events in terminal
-no-web       Disable web UI, CLI only
```

## What you'll see

**Web Dashboard:**
- Total connections, requests, errors
- HTTP endpoints with method, path, request count, error rate, p50/p99 latency
- TCP connections showing process → remote:port

**CLI Output:**
```
╔══════════════════════════════════════════════════════════════════════╗
║                         ENDPOINT STATS                               ║
╠══════════════════════════════════════════════════════════════════════╣
║ METHOD  PATH                            COUNT    ERR      P50      P99 ║
╟──────────────────────────────────────────────────────────────────────╢
║ GET     /api/users                        142      0      12ms     89ms ║
║ POST    /api/orders                        38      2      45ms    230ms ║
╠══════════════════════════════════════════════════════════════════════╣
║                        CONNECTION MAP                                ║
╠══════════════════════════════════════════════════════════════════════╣
║ SOURCE           DESTINATION                  COUNT                   ║
╟──────────────────────────────────────────────────────────────────────╢
║ node             postgres:5432                   89                   ║
║ python           redis:6379                      234                  ║
╚══════════════════════════════════════════════════════════════════════╝
```

## Requirements

- Linux kernel 5.8+ (for BPF ring buffers)
- Root access (for eBPF)
- x86_64 architecture

Check your kernel version:
```bash
uname -r
```

## Building from Source

```bash
git clone https://github.com/1byteword/dogwatch.git
cd dogwatch
go build ./cmd/dogwatch
sudo ./dogwatch
```

To regenerate BPF bindings (requires clang, libbpf):
```bash
go generate ./internal/probe/...
go build ./cmd/dogwatch
```

## How it works

dogwatch uses eBPF (extended Berkeley Packet Filter) to hook into kernel functions:

- **kprobe/tcp_connect** - Captures outbound TCP connections
- **kretprobe/inet_csk_accept** - Captures inbound connections
- **tracepoint/syscalls/sys_enter_write** - Intercepts HTTP request data
- **tracepoint/syscalls/sys_exit_read** - Intercepts HTTP response data

All tracing happens in-kernel with minimal overhead. No packet capture, no tcpdump, no iptables.

## Limitations

- **Plain HTTP only** - HTTPS traffic is encrypted and not visible (SSL interception is WIP)
- **Linux only** - eBPF is a Linux kernel feature
- **x86_64 only** - ARM support coming soon

## License

MIT

## Contributing

PRs welcome! See [issues](https://github.com/1byteword/dogwatch/issues) for ideas.
