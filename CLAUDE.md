# CLAUDE.md

## What is dogwatch

eBPF-powered observability platform. Single binary alternative to Datadog/Grafana/New Relic. Zero-config distributed tracing - drop it on a server, see everything in 60 seconds.

## Philosophy

- **Simplicity over features** - Single binary, no external dependencies
- **Zero-config over flexibility** - eBPF auto-discovers everything
- **Self-hosted over SaaS** - Your data stays yours
- **Don't over-engineer** - No abstractions "for later", no premature optimization

## Architecture

```
Single Go binary containing:
├── eBPF probes (kernel-level tracing)
├── HTTP API + Web UI
├── SQLite storage (no Postgres/Redis required)
├── Prometheus scraping
├── Alerting engine
└── All features built-in
```

No external dependencies. No Docker required. Just `./dogwatch` and go.

## Current Priorities (in order)

Core features complete. See "Do NOT Build Yet" for what's deferred.

## Recently Completed

- ✅ Full PromQL support (Grafana-compatible, `/api/v1/query`, 60+ functions)
- ✅ Profile → Trace linking (click flamegraph hotspot → see related traces)
- ✅ Migration tools (`dogwatch migrate` CLI + Web wizard for Datadog/Grafana/Prometheus)
- ✅ Script engine & library (`dogwatch run mysql/slow_queries`)
- ✅ MySQL/PostgreSQL/Redis eBPF parsing
- ✅ OTLP receiver (gRPC + HTTP)
- ✅ Backup/restore with scheduler
- ✅ Cost Intelligence dashboard
- ✅ Native histogram support (accurate p99/p99.9)
- ✅ Security headers & CSRF protection
- ✅ Recording rules
- ✅ Service catalog with auto-discovery

## Do NOT Build Yet

- HA/clustering (single binary is fine for now)
- Wasm plugins (cool but not MVP)
- LLM/AI features (add after core is solid)
- Terraform provider (nice-to-have)
- Custom ML models (use LLM APIs if needed)
- Multi-tenancy (enterprise feature, later)
- Pluggable storage backends (SQLite works for now)

## Key Directories

| Path | Purpose |
|------|---------|
| `cmd/dogwatch/main.go` | Entry point, wires everything |
| `internal/probe/` | eBPF probes for tracing |
| `internal/metrics/` | Metrics collection/storage |
| `internal/logs/` | Log ingestion/storage |
| `internal/trace/` | Distributed tracing |
| `internal/storage/` | SQLite persistence |
| `internal/web/` | HTTP handlers + UI |
| `internal/rbac/` | Auth, sessions, API keys |
| `internal/alerting/` | Alert rules and evaluation |
| `internal/catalog/` | Service catalog |
| `internal/synthetics/` | Synthetic monitoring |
| `internal/correlation/` | Cross-signal correlation |
| `internal/scripts/` | Script engine for instant analysis |
| `internal/migration/` | Dashboard/alert migration from Datadog/Grafana/Prometheus |
| `internal/promql/` | PromQL parser, engine, Prometheus API |
| `internal/profile/` | CPU profiling and profile-trace linking |

## Code Style

- Standard Go conventions
- Error handling: return errors, don't panic
- Logging: `log.Printf` for now, structured later
- No frameworks - stdlib where possible
- Comments only where logic isn't obvious

## Security Notes

- Passwords: bcrypt cost 12
- Sessions: JWT with HMAC-SHA256
- API keys: `dw_` prefix, hashed storage
- RBAC: Owner > Admin > Editor > Viewer
- CSRF: Token-based protection enabled
- Headers: CSP, X-Frame-Options, X-Content-Type-Options configured

## Testing

```bash
go test ./...           # Unit tests
go build ./cmd/dogwatch # Build binary
./dogwatch              # Run locally on :9999
```

## For Full Context

Read `VISION.md` - 10,000+ lines covering:
- Product strategy and 7 killer features
- Competitive analysis (Datadog, Splunk, Wavefront, etc.)
- Features to steal from competitors
- 18 modern pain points to solve
- Pricing model
- GTM strategy
- Technical roadmap

## Common Tasks

**Add a new API endpoint:**
1. Add handler in `internal/web/`
2. Register route in `cmd/dogwatch/main.go`

**Add a new metric type:**
1. Define in `internal/metrics/`
2. Add storage in `internal/storage/`

**Fix security issues:**
See VISION.md "Security Audit" section for known gaps.

## What Makes This Different

1. **Zero-config** - eBPF sees all traffic without SDK instrumentation
2. **Cost transparency** - Shows "this would cost $X on Datadog"
3. **Single binary** - `curl | sh` and done
4. **Self-hosted** - No data leaves your infrastructure
