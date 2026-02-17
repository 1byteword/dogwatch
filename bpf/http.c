//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_endian.h>

#define AF_INET 2
#define MAX_PAYLOAD_SIZE 256

char LICENSE[] SEC("license") = "GPL";

// HTTP event types
#define EVENT_TYPE_REQUEST  1
#define EVENT_TYPE_RESPONSE 2

// HTTP event sent to userspace
struct http_event {
    __u64 ts_ns;
    __u64 sock_cookie;
    __u32 pid;
    __u32 tid;
    __u32 uid;
    __u32 saddr;
    __u32 daddr;
    __u16 sport;
    __u16 dport;
    __u32 payload_size;
    __u8  event_type;
    __u8  _pad[3];
    char  comm[16];
    char  payload[MAX_PAYLOAD_SIZE];
};

// Ring buffer for events
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 512 * 1024);
} http_events SEC(".maps");

// Track buffer pointers for read syscalls
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 10240);
    __type(key, __u64);    // pid_tgid
    __type(value, __u64);  // buffer pointer as u64
} read_buffers SEC(".maps");

// Check if data looks like HTTP request
static __always_inline int is_http_request(const char *buf, __u32 size) {
    if (size < 4) return 0;

    char b0, b1, b2, b3;
    bpf_probe_read_user(&b0, 1, buf);
    bpf_probe_read_user(&b1, 1, buf + 1);
    bpf_probe_read_user(&b2, 1, buf + 2);
    bpf_probe_read_user(&b3, 1, buf + 3);

    // GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS
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
static __always_inline int is_http_response(const char *buf, __u32 size) {
    if (size < 4) return 0;

    char b0, b1, b2, b3;
    bpf_probe_read_user(&b0, 1, buf);
    bpf_probe_read_user(&b1, 1, buf + 1);
    bpf_probe_read_user(&b2, 1, buf + 2);
    bpf_probe_read_user(&b3, 1, buf + 3);

    // HTTP/1.x response
    if (b0 == 'H' && b1 == 'T' && b2 == 'T' && b3 == 'P') return 1;

    return 0;
}

// Emit an HTTP event
static __always_inline void emit_http_event(char *buf, size_t count, __u8 event_type) {
    struct http_event *e = bpf_ringbuf_reserve(&http_events, sizeof(*e), 0);
    if (!e) {
        return;
    }

    __u64 pid_tgid = bpf_get_current_pid_tgid();
    e->ts_ns = bpf_ktime_get_ns();
    e->pid = pid_tgid >> 32;
    e->tid = (__u32)pid_tgid;
    e->uid = bpf_get_current_uid_gid() & 0xFFFFFFFF;
    e->sock_cookie = 0;
    e->saddr = 0;
    e->daddr = 0;
    e->sport = 0;
    e->dport = 0;
    e->payload_size = count > MAX_PAYLOAD_SIZE ? MAX_PAYLOAD_SIZE : count;
    e->event_type = event_type;

    bpf_get_current_comm(&e->comm, sizeof(e->comm));

    // Read payload safely
    __u32 read_size = e->payload_size;
    if (read_size > MAX_PAYLOAD_SIZE) {
        read_size = MAX_PAYLOAD_SIZE;
    }
    bpf_probe_read_user(&e->payload, read_size, buf);

    bpf_ringbuf_submit(e, 0);
}

// Capture writes that look like HTTP
SEC("tracepoint/syscalls/sys_enter_write")
int trace_write_entry(struct trace_event_raw_sys_enter *ctx)
{
    char *buf = (char *)ctx->args[1];
    size_t count = (size_t)ctx->args[2];

    if (count < 4 || count > 65535) {
        return 0;
    }

    if (is_http_request(buf, count)) {
        emit_http_event(buf, count, EVENT_TYPE_REQUEST);
    } else if (is_http_response(buf, count)) {
        emit_http_event(buf, count, EVENT_TYPE_RESPONSE);
    }

    return 0;
}

// Store buffer pointer on read entry
SEC("tracepoint/syscalls/sys_enter_read")
int trace_read_entry(struct trace_event_raw_sys_enter *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 buf_ptr = (__u64)ctx->args[1];

    bpf_map_update_elem(&read_buffers, &pid_tgid, &buf_ptr, BPF_ANY);
    return 0;
}

// Check read result for HTTP data
SEC("tracepoint/syscalls/sys_exit_read")
int trace_read_exit(struct trace_event_raw_sys_exit *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 *buf_ptr = bpf_map_lookup_elem(&read_buffers, &pid_tgid);
    if (!buf_ptr) {
        return 0;
    }

    char *buf = (char *)*buf_ptr;
    bpf_map_delete_elem(&read_buffers, &pid_tgid);

    long ret = ctx->ret;
    if (ret <= 0) {
        return 0;
    }

    size_t count = (size_t)ret;
    if (count < 4 || count > 65535) {
        return 0;
    }

    if (is_http_request(buf, count)) {
        emit_http_event(buf, count, EVENT_TYPE_REQUEST);
    } else if (is_http_response(buf, count)) {
        emit_http_event(buf, count, EVENT_TYPE_RESPONSE);
    }

    return 0;
}

// Also capture sendto for HTTP
SEC("tracepoint/syscalls/sys_enter_sendto")
int trace_sendto_entry(struct trace_event_raw_sys_enter *ctx)
{
    char *buf = (char *)ctx->args[1];
    size_t count = (size_t)ctx->args[2];

    if (count < 4 || count > 65535) {
        return 0;
    }

    if (is_http_request(buf, count)) {
        emit_http_event(buf, count, EVENT_TYPE_REQUEST);
    } else if (is_http_response(buf, count)) {
        emit_http_event(buf, count, EVENT_TYPE_RESPONSE);
    }

    return 0;
}

// Store buffer for recvfrom
SEC("tracepoint/syscalls/sys_enter_recvfrom")
int trace_recvfrom_entry(struct trace_event_raw_sys_enter *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 buf_ptr = (__u64)ctx->args[1];

    bpf_map_update_elem(&read_buffers, &pid_tgid, &buf_ptr, BPF_ANY);
    return 0;
}

// Check recvfrom result
SEC("tracepoint/syscalls/sys_exit_recvfrom")
int trace_recvfrom_exit(struct trace_event_raw_sys_exit *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 *buf_ptr = bpf_map_lookup_elem(&read_buffers, &pid_tgid);
    if (!buf_ptr) {
        return 0;
    }

    char *buf = (char *)*buf_ptr;
    bpf_map_delete_elem(&read_buffers, &pid_tgid);

    long ret = ctx->ret;
    if (ret <= 0) {
        return 0;
    }

    size_t count = (size_t)ret;
    if (count < 4 || count > 65535) {
        return 0;
    }

    if (is_http_request(buf, count)) {
        emit_http_event(buf, count, EVENT_TYPE_REQUEST);
    } else if (is_http_response(buf, count)) {
        emit_http_event(buf, count, EVENT_TYPE_RESPONSE);
    }

    return 0;
}

// Helper: read the first iov_base and iov_len from an iovec array pointer.
// struct iovec { void *iov_base; size_t iov_len; }
// Returns 0 on success, -1 on failure.
static __always_inline int read_first_iov(const void *iov_array, char **base_out, size_t *len_out)
{
    // Read iov_base (first field of struct iovec)
    void *iov_base;
    if (bpf_probe_read_user(&iov_base, sizeof(iov_base), iov_array) < 0)
        return -1;
    // Read iov_len (second field, offset = sizeof(void*))
    size_t iov_len;
    if (bpf_probe_read_user(&iov_len, sizeof(iov_len), (const char *)iov_array + sizeof(void *)) < 0)
        return -1;
    *base_out = (char *)iov_base;
    *len_out = iov_len;
    return 0;
}

// Capture sendmsg for HTTP
// sendmsg(int fd, const struct msghdr *msg, int flags)
// struct msghdr { ..., struct iovec *msg_iov, size_t msg_iovlen, ... }
// On x86_64: msg_iov is at offset 8 in msghdr (after msg_name + msg_namelen)
SEC("tracepoint/syscalls/sys_enter_sendmsg")
int trace_sendmsg_entry(struct trace_event_raw_sys_enter *ctx)
{
    // args[1] = pointer to struct msghdr (user_msghdr)
    const void *msg_ptr = (const void *)ctx->args[1];
    if (!msg_ptr)
        return 0;

    // Read msg_iov pointer from msghdr
    // struct user_msghdr: void *msg_name (8), int msg_namelen (4), pad (4), struct iovec *msg_iov (8)
    // Offset of msg_iov = 16 on 64-bit
    struct iovec *iov_ptr;
    if (bpf_probe_read_user(&iov_ptr, sizeof(iov_ptr), (const char *)msg_ptr + 16) < 0)
        return 0;

    char *buf;
    size_t count;
    if (read_first_iov(iov_ptr, &buf, &count) < 0)
        return 0;

    if (count < 4 || count > 65535)
        return 0;

    if (is_http_request(buf, count)) {
        emit_http_event(buf, count, EVENT_TYPE_REQUEST);
    } else if (is_http_response(buf, count)) {
        emit_http_event(buf, count, EVENT_TYPE_RESPONSE);
    }

    return 0;
}

// Store buffer for recvmsg (read the first iov from msghdr on entry)
SEC("tracepoint/syscalls/sys_enter_recvmsg")
int trace_recvmsg_entry(struct trace_event_raw_sys_enter *ctx)
{
    const void *msg_ptr = (const void *)ctx->args[1];
    if (!msg_ptr)
        return 0;

    // Read msg_iov pointer from msghdr (offset 16 on 64-bit)
    struct iovec *iov_ptr;
    if (bpf_probe_read_user(&iov_ptr, sizeof(iov_ptr), (const char *)msg_ptr + 16) < 0)
        return 0;

    // Read the first iov_base
    void *iov_base;
    if (bpf_probe_read_user(&iov_base, sizeof(iov_base), iov_ptr) < 0)
        return 0;

    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 buf_ptr = (__u64)iov_base;
    bpf_map_update_elem(&read_buffers, &pid_tgid, &buf_ptr, BPF_ANY);

    return 0;
}

// Check recvmsg result for HTTP data
SEC("tracepoint/syscalls/sys_exit_recvmsg")
int trace_recvmsg_exit(struct trace_event_raw_sys_exit *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 *buf_ptr = bpf_map_lookup_elem(&read_buffers, &pid_tgid);
    if (!buf_ptr)
        return 0;

    char *buf = (char *)*buf_ptr;
    bpf_map_delete_elem(&read_buffers, &pid_tgid);

    long ret = ctx->ret;
    if (ret <= 0)
        return 0;

    size_t count = (size_t)ret;
    if (count < 4 || count > 65535)
        return 0;

    if (is_http_request(buf, count)) {
        emit_http_event(buf, count, EVENT_TYPE_REQUEST);
    } else if (is_http_response(buf, count)) {
        emit_http_event(buf, count, EVENT_TYPE_RESPONSE);
    }

    return 0;
}

// Capture writev for HTTP (scatter/gather write)
// writev(int fd, const struct iovec *iov, int iovcnt)
SEC("tracepoint/syscalls/sys_enter_writev")
int trace_writev_entry(struct trace_event_raw_sys_enter *ctx)
{
    const void *iov_array = (const void *)ctx->args[1];
    // iovcnt = ctx->args[2], we only look at the first iov entry

    char *buf;
    size_t count;
    if (read_first_iov(iov_array, &buf, &count) < 0)
        return 0;

    if (count < 4 || count > 65535)
        return 0;

    if (is_http_request(buf, count)) {
        emit_http_event(buf, count, EVENT_TYPE_REQUEST);
    } else if (is_http_response(buf, count)) {
        emit_http_event(buf, count, EVENT_TYPE_RESPONSE);
    }

    return 0;
}

// Store buffer for readv (scatter/gather read)
// readv(int fd, const struct iovec *iov, int iovcnt)
SEC("tracepoint/syscalls/sys_enter_readv")
int trace_readv_entry(struct trace_event_raw_sys_enter *ctx)
{
    const void *iov_array = (const void *)ctx->args[1];

    // Read the first iov_base
    void *iov_base;
    if (bpf_probe_read_user(&iov_base, sizeof(iov_base), iov_array) < 0)
        return 0;

    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 buf_ptr = (__u64)iov_base;
    bpf_map_update_elem(&read_buffers, &pid_tgid, &buf_ptr, BPF_ANY);

    return 0;
}

// Check readv result for HTTP data
SEC("tracepoint/syscalls/sys_exit_readv")
int trace_readv_exit(struct trace_event_raw_sys_exit *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 *buf_ptr = bpf_map_lookup_elem(&read_buffers, &pid_tgid);
    if (!buf_ptr)
        return 0;

    char *buf = (char *)*buf_ptr;
    bpf_map_delete_elem(&read_buffers, &pid_tgid);

    long ret = ctx->ret;
    if (ret <= 0)
        return 0;

    size_t count = (size_t)ret;
    if (count < 4 || count > 65535)
        return 0;

    if (is_http_request(buf, count)) {
        emit_http_event(buf, count, EVENT_TYPE_REQUEST);
    } else if (is_http_response(buf, count)) {
        emit_http_event(buf, count, EVENT_TYPE_RESPONSE);
    }

    return 0;
}
