//go:build ignore

/*
 * SSL/HTTPS Interception Probe
 * ============================
 *
 * STATUS: WORKING - using tracefs-based uprobe attachment (SSLProbeV2)
 *
 * This BPF program intercepts plaintext HTTPS traffic by attaching uprobes to
 * OpenSSL's SSL_read/SSL_write functions. These functions handle data AFTER
 * decryption (SSL_read) and BEFORE encryption (SSL_write), giving us access
 * to plaintext HTTP data within HTTPS connections.
 *
 * IMPLEMENTATION:
 * - Uses tracefs-based uprobe registration (/sys/kernel/debug/tracing/uprobe_events)
 * - Attaches BPF programs via perf_event_open with PERF_TYPE_TRACEPOINT
 * - Attaches to all CPUs for system-wide tracing
 * - Supports both SSL_write/SSL_read and SSL_write_ex/SSL_read_ex (OpenSSL 3.x)
 *
 * NOTE: The cilium/ebpf link-based uprobe attachment does NOT work for this use case.
 * The tracefs + perf_event_open approach (SSLProbeV2 in ssl_v2.go) is required.
 *
 * HTTP/1.1 traffic is captured with method/path/status extraction.
 * HTTP/2 traffic appears as binary frames (not human-readable HTTP text).
 */

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>

#define MAX_PAYLOAD_SIZE 256

char LICENSE[] SEC("license") = "GPL";

// Event types
#define EVENT_TYPE_REQUEST  1
#define EVENT_TYPE_RESPONSE 2

// SSL event sent to userspace
struct ssl_event {
    __u64 ts_ns;
    __u32 pid;
    __u32 tid;
    __u32 uid;
    __u32 len;
    __u8  event_type;
    __u8  _pad[3];
    char  comm[16];
    char  payload[MAX_PAYLOAD_SIZE];
};

// Ring buffer for events
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 512 * 1024);
} ssl_events SEC(".maps");

// Track SSL_read calls to get buffer on return
struct ssl_read_args {
    __u64 buf;
    __u32 len;
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 10240);
    __type(key, __u64);  // pid_tgid
    __type(value, struct ssl_read_args);
} ssl_read_args_map SEC(".maps");

// Check if data looks like HTTP request
static __always_inline int is_http_request(const char *buf) {
    char b0, b1, b2, b3;
    bpf_probe_read_user(&b0, 1, buf);
    bpf_probe_read_user(&b1, 1, buf + 1);
    bpf_probe_read_user(&b2, 1, buf + 2);
    bpf_probe_read_user(&b3, 1, buf + 3);

    if (b0 == 'G' && b1 == 'E' && b2 == 'T' && b3 == ' ') return 1;
    if (b0 == 'P' && b1 == 'O' && b2 == 'S' && b3 == 'T') return 1;
    if (b0 == 'P' && b1 == 'U' && b2 == 'T' && b3 == ' ') return 1;
    if (b0 == 'D' && b1 == 'E' && b2 == 'L' && b3 == 'E') return 1;
    if (b0 == 'P' && b1 == 'A' && b2 == 'T' && b3 == 'C') return 1;
    if (b0 == 'H' && b1 == 'E' && b2 == 'A' && b3 == 'D') return 1;
    if (b0 == 'O' && b1 == 'P' && b2 == 'T' && b3 == 'I') return 1;

    return 0;
}

// Check if data looks like HTTP response
static __always_inline int is_http_response(const char *buf) {
    char b0, b1, b2, b3;
    bpf_probe_read_user(&b0, 1, buf);
    bpf_probe_read_user(&b1, 1, buf + 1);
    bpf_probe_read_user(&b2, 1, buf + 2);
    bpf_probe_read_user(&b3, 1, buf + 3);

    if (b0 == 'H' && b1 == 'T' && b2 == 'T' && b3 == 'P') return 1;

    return 0;
}

// Emit SSL event
static __always_inline void emit_ssl_event(const char *buf, __u32 len, __u8 event_type) {
    if (len < 4) return;

    struct ssl_event *e = bpf_ringbuf_reserve(&ssl_events, sizeof(*e), 0);
    if (!e) return;

    __u64 pid_tgid = bpf_get_current_pid_tgid();
    e->ts_ns = bpf_ktime_get_ns();
    e->pid = pid_tgid >> 32;
    e->tid = (__u32)pid_tgid;
    e->uid = bpf_get_current_uid_gid() & 0xFFFFFFFF;
    e->len = len;
    e->event_type = event_type;

    bpf_get_current_comm(&e->comm, sizeof(e->comm));

    __u32 read_size = len > MAX_PAYLOAD_SIZE ? MAX_PAYLOAD_SIZE : len;
    bpf_probe_read_user(&e->payload, read_size, buf);

    bpf_ringbuf_submit(e, 0);
}

// SSL_write(SSL *ssl, const void *buf, int num)
// Captures outgoing plaintext before encryption
SEC("uprobe/SSL_write")
int uprobe_ssl_write(struct pt_regs *ctx) {
    const char *buf = (const char *)PT_REGS_PARM2(ctx);
    int num = (int)PT_REGS_PARM3(ctx);

    if (num <= 0 || num > 65535) return 0;

    if (is_http_request(buf)) {
        emit_ssl_event(buf, num, EVENT_TYPE_REQUEST);
    } else if (is_http_response(buf)) {
        emit_ssl_event(buf, num, EVENT_TYPE_RESPONSE);
    }

    return 0;
}

// SSL_write_ex(SSL *ssl, const void *buf, size_t num, size_t *written)
// OpenSSL 3.x extended write function
SEC("uprobe/SSL_write_ex")
int uprobe_ssl_write_ex(struct pt_regs *ctx) {
    const char *buf = (const char *)PT_REGS_PARM2(ctx);
    size_t num = (size_t)PT_REGS_PARM3(ctx);

    if (num == 0 || num > 65535) return 0;

    if (is_http_request(buf)) {
        emit_ssl_event(buf, (__u32)num, EVENT_TYPE_REQUEST);
    } else if (is_http_response(buf)) {
        emit_ssl_event(buf, (__u32)num, EVENT_TYPE_RESPONSE);
    }

    return 0;
}

// SSL_read entry - save buffer pointer for later
// SSL_read(SSL *ssl, void *buf, int num)
SEC("uprobe/SSL_read")
int uprobe_ssl_read_entry(struct pt_regs *ctx) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();

    struct ssl_read_args args = {
        .buf = (__u64)PT_REGS_PARM2(ctx),
        .len = (int)PT_REGS_PARM3(ctx),
    };

    bpf_map_update_elem(&ssl_read_args_map, &pid_tgid, &args, BPF_ANY);
    return 0;
}

// SSL_read return - capture decrypted data
SEC("uretprobe/SSL_read")
int uretprobe_ssl_read(struct pt_regs *ctx) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();

    struct ssl_read_args *args = bpf_map_lookup_elem(&ssl_read_args_map, &pid_tgid);
    if (!args) return 0;

    int ret = PT_REGS_RC(ctx);
    bpf_map_delete_elem(&ssl_read_args_map, &pid_tgid);

    if (ret <= 0) return 0;

    const char *buf = (const char *)args->buf;
    __u32 len = ret;

    if (is_http_request(buf)) {
        emit_ssl_event(buf, len, EVENT_TYPE_REQUEST);
    } else if (is_http_response(buf)) {
        emit_ssl_event(buf, len, EVENT_TYPE_RESPONSE);
    }

    return 0;
}

// SSL_read_ex entry - OpenSSL 3.x extended read
// SSL_read_ex(SSL *ssl, void *buf, size_t num, size_t *readbytes)
SEC("uprobe/SSL_read_ex")
int uprobe_ssl_read_ex_entry(struct pt_regs *ctx) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();

    struct ssl_read_args args = {
        .buf = (__u64)PT_REGS_PARM2(ctx),
        .len = (size_t)PT_REGS_PARM3(ctx),
    };

    bpf_map_update_elem(&ssl_read_args_map, &pid_tgid, &args, BPF_ANY);
    return 0;
}

// SSL_read_ex return - capture decrypted data
SEC("uretprobe/SSL_read_ex")
int uretprobe_ssl_read_ex(struct pt_regs *ctx) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();

    struct ssl_read_args *args = bpf_map_lookup_elem(&ssl_read_args_map, &pid_tgid);
    if (!args) return 0;

    int ret = PT_REGS_RC(ctx);
    bpf_map_delete_elem(&ssl_read_args_map, &pid_tgid);

    // SSL_read_ex returns 1 on success, 0 on failure
    if (ret != 1) return 0;

    const char *buf = (const char *)args->buf;
    __u32 len = args->len;  // For _ex, use the buffer size since actual read is in readbytes

    if (is_http_request(buf)) {
        emit_ssl_event(buf, len > 256 ? 256 : len, EVENT_TYPE_REQUEST);
    } else if (is_http_response(buf)) {
        emit_ssl_event(buf, len > 256 ? 256 : len, EVENT_TYPE_RESPONSE);
    }

    return 0;
}
