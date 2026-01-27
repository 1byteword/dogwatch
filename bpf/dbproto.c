//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_endian.h>

#define MAX_PAYLOAD_SIZE 512

char LICENSE[] SEC("license") = "GPL";

// Database types
#define DB_TYPE_UNKNOWN   0
#define DB_TYPE_MYSQL     1
#define DB_TYPE_POSTGRES  2
#define DB_TYPE_REDIS     3

// Event types
#define EVENT_TYPE_QUERY    1
#define EVENT_TYPE_RESPONSE 2

// Database event sent to userspace
struct db_event {
    __u64 ts_ns;
    __u32 pid;
    __u32 tid;
    __u32 uid;
    __u32 payload_size;
    __u8  db_type;
    __u8  event_type;
    __u8  _pad[2];
    char  comm[16];
    char  payload[MAX_PAYLOAD_SIZE];
};

// Ring buffer for events
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1024 * 1024);
} db_events SEC(".maps");

// Track buffer pointers for read syscalls
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 10240);
    __type(key, __u64);    // pid_tgid
    __type(value, __u64);  // buffer pointer
} read_buffers SEC(".maps");

// MySQL packet detection
// Format: 3-byte length (LE) + 1-byte seq + payload
// COM_QUERY = 0x03, COM_STMT_PREPARE = 0x16, COM_STMT_EXECUTE = 0x17
static __always_inline int is_mysql_query(const char *buf, __u32 size) {
    if (size < 5) return 0;

    char b0, b1, b2, b3, b4;
    bpf_probe_read_user(&b0, 1, buf);
    bpf_probe_read_user(&b1, 1, buf + 1);
    bpf_probe_read_user(&b2, 1, buf + 2);
    bpf_probe_read_user(&b3, 1, buf + 3);
    bpf_probe_read_user(&b4, 1, buf + 4);

    // Check if length field is reasonable (payload length = total - 4)
    __u32 pkt_len = (__u32)((__u8)b0) | ((__u32)((__u8)b1) << 8) | ((__u32)((__u8)b2) << 16);
    if (pkt_len < 1 || pkt_len > 16777215) return 0;

    // Check command byte (after 4-byte header)
    // COM_QUERY=0x03, COM_STMT_PREPARE=0x16, COM_STMT_EXECUTE=0x17
    // COM_INIT_DB=0x02, COM_FIELD_LIST=0x04
    if (b4 == 0x03 || b4 == 0x16 || b4 == 0x17 || b4 == 0x02) {
        return 1;
    }

    return 0;
}

// MySQL response detection (OK, ERR, or result set)
static __always_inline int is_mysql_response(const char *buf, __u32 size) {
    if (size < 5) return 0;

    char b0, b1, b2, b3, b4;
    bpf_probe_read_user(&b0, 1, buf);
    bpf_probe_read_user(&b1, 1, buf + 1);
    bpf_probe_read_user(&b2, 1, buf + 2);
    bpf_probe_read_user(&b3, 1, buf + 3);
    bpf_probe_read_user(&b4, 1, buf + 4);

    // Check length is reasonable
    __u32 pkt_len = (__u32)((__u8)b0) | ((__u32)((__u8)b1) << 8) | ((__u32)((__u8)b2) << 16);
    if (pkt_len < 1 || pkt_len > 16777215) return 0;

    // OK packet: 0x00, EOF: 0xfe, ERR: 0xff
    if (b4 == 0x00 || b4 == 0xfe || b4 == 0xff) {
        return 1;
    }

    // Could be result set (field count as first byte)
    // Field count is typically 1-250 for result sets
    if ((__u8)b4 >= 1 && (__u8)b4 <= 250) {
        return 1;
    }

    return 0;
}

// PostgreSQL query detection
// Simple Query: 'Q' + 4-byte length + query string
// Parse: 'P' + 4-byte length + statement + query
// Execute: 'E' + 4-byte length + portal + max_rows
static __always_inline int is_postgres_query(const char *buf, __u32 size) {
    if (size < 5) return 0;

    char b0;
    bpf_probe_read_user(&b0, 1, buf);

    // Query message types from frontend
    // Q = Simple Query, P = Parse, E = Execute, B = Bind
    if (b0 == 'Q' || b0 == 'P' || b0 == 'E' || b0 == 'B') {
        // Verify length is reasonable
        char len_bytes[4];
        bpf_probe_read_user(len_bytes, 4, buf + 1);
        __u32 msg_len = bpf_ntohl(*(__u32 *)len_bytes);
        if (msg_len >= 4 && msg_len < 1000000) {
            return 1;
        }
    }

    return 0;
}

// PostgreSQL response detection
// T = RowDescription, D = DataRow, C = CommandComplete
// E = ErrorResponse, Z = ReadyForQuery
static __always_inline int is_postgres_response(const char *buf, __u32 size) {
    if (size < 5) return 0;

    char b0;
    bpf_probe_read_user(&b0, 1, buf);

    // Response message types from backend
    if (b0 == 'T' || b0 == 'D' || b0 == 'C' || b0 == 'E' || b0 == 'Z' ||
        b0 == '1' || b0 == '2' || b0 == 'n' || b0 == 's') {
        // Verify length is reasonable
        char len_bytes[4];
        bpf_probe_read_user(len_bytes, 4, buf + 1);
        __u32 msg_len = bpf_ntohl(*(__u32 *)len_bytes);
        if (msg_len >= 4 && msg_len < 1000000) {
            return 1;
        }
    }

    return 0;
}

// Redis RESP protocol detection
// Commands: *<count>\r\n$<len>\r\n<arg>\r\n...
// Inline commands: PING\r\n, QUIT\r\n
static __always_inline int is_redis_command(const char *buf, __u32 size) {
    if (size < 4) return 0;

    char b0, b1;
    bpf_probe_read_user(&b0, 1, buf);
    bpf_probe_read_user(&b1, 1, buf + 1);

    // RESP array (most commands): *<number>
    if (b0 == '*' && b1 >= '0' && b1 <= '9') {
        return 1;
    }

    // Inline commands (PING, QUIT, etc.)
    char b2, b3;
    bpf_probe_read_user(&b2, 1, buf + 2);
    bpf_probe_read_user(&b3, 1, buf + 3);

    // PING
    if (b0 == 'P' && b1 == 'I' && b2 == 'N' && b3 == 'G') return 1;
    // QUIT
    if (b0 == 'Q' && b1 == 'U' && b2 == 'I' && b3 == 'T') return 1;
    // INFO
    if (b0 == 'I' && b1 == 'N' && b2 == 'F' && b3 == 'O') return 1;
    // AUTH
    if (b0 == 'A' && b1 == 'U' && b2 == 'T' && b3 == 'H') return 1;

    return 0;
}

// Redis response detection
// +OK, -ERR, :123, $<len>, *<count>
static __always_inline int is_redis_response(const char *buf, __u32 size) {
    if (size < 3) return 0;

    char b0;
    bpf_probe_read_user(&b0, 1, buf);

    // Simple string: +
    // Error: -
    // Integer: :
    // Bulk string: $
    // Array: *
    if (b0 == '+' || b0 == '-' || b0 == ':' || b0 == '$' || b0 == '*') {
        return 1;
    }

    return 0;
}

// Detect database type and event type
static __always_inline void detect_db_protocol(const char *buf, __u32 size, __u8 *db_type, __u8 *event_type) {
    *db_type = DB_TYPE_UNKNOWN;
    *event_type = 0;

    // Try MySQL first (most common)
    if (is_mysql_query(buf, size)) {
        *db_type = DB_TYPE_MYSQL;
        *event_type = EVENT_TYPE_QUERY;
        return;
    }
    if (is_mysql_response(buf, size)) {
        *db_type = DB_TYPE_MYSQL;
        *event_type = EVENT_TYPE_RESPONSE;
        return;
    }

    // Try PostgreSQL
    if (is_postgres_query(buf, size)) {
        *db_type = DB_TYPE_POSTGRES;
        *event_type = EVENT_TYPE_QUERY;
        return;
    }
    if (is_postgres_response(buf, size)) {
        *db_type = DB_TYPE_POSTGRES;
        *event_type = EVENT_TYPE_RESPONSE;
        return;
    }

    // Try Redis
    if (is_redis_command(buf, size)) {
        *db_type = DB_TYPE_REDIS;
        *event_type = EVENT_TYPE_QUERY;
        return;
    }
    if (is_redis_response(buf, size)) {
        *db_type = DB_TYPE_REDIS;
        *event_type = EVENT_TYPE_RESPONSE;
        return;
    }
}

// Emit a database event
static __always_inline void emit_db_event(char *buf, size_t count, __u8 db_type, __u8 event_type) {
    struct db_event *e = bpf_ringbuf_reserve(&db_events, sizeof(*e), 0);
    if (!e) {
        return;
    }

    __u64 pid_tgid = bpf_get_current_pid_tgid();
    e->ts_ns = bpf_ktime_get_ns();
    e->pid = pid_tgid >> 32;
    e->tid = (__u32)pid_tgid;
    e->uid = bpf_get_current_uid_gid() & 0xFFFFFFFF;
    e->payload_size = count > MAX_PAYLOAD_SIZE ? MAX_PAYLOAD_SIZE : count;
    e->db_type = db_type;
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

// Capture writes that look like database protocol
SEC("tracepoint/syscalls/sys_enter_write")
int trace_db_write_entry(struct trace_event_raw_sys_enter *ctx)
{
    char *buf = (char *)ctx->args[1];
    size_t count = (size_t)ctx->args[2];

    if (count < 4 || count > 65535) {
        return 0;
    }

    __u8 db_type, event_type;
    detect_db_protocol(buf, count, &db_type, &event_type);

    if (db_type != DB_TYPE_UNKNOWN) {
        emit_db_event(buf, count, db_type, event_type);
    }

    return 0;
}

// Store buffer pointer on read entry
SEC("tracepoint/syscalls/sys_enter_read")
int trace_db_read_entry(struct trace_event_raw_sys_enter *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 buf_ptr = (__u64)ctx->args[1];

    bpf_map_update_elem(&read_buffers, &pid_tgid, &buf_ptr, BPF_ANY);
    return 0;
}

// Check read result for database data
SEC("tracepoint/syscalls/sys_exit_read")
int trace_db_read_exit(struct trace_event_raw_sys_exit *ctx)
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

    __u8 db_type, event_type;
    detect_db_protocol(buf, count, &db_type, &event_type);

    if (db_type != DB_TYPE_UNKNOWN) {
        emit_db_event(buf, count, db_type, event_type);
    }

    return 0;
}

// Capture sendto for database protocol
SEC("tracepoint/syscalls/sys_enter_sendto")
int trace_db_sendto_entry(struct trace_event_raw_sys_enter *ctx)
{
    char *buf = (char *)ctx->args[1];
    size_t count = (size_t)ctx->args[2];

    if (count < 4 || count > 65535) {
        return 0;
    }

    __u8 db_type, event_type;
    detect_db_protocol(buf, count, &db_type, &event_type);

    if (db_type != DB_TYPE_UNKNOWN) {
        emit_db_event(buf, count, db_type, event_type);
    }

    return 0;
}

// Store buffer for recvfrom
SEC("tracepoint/syscalls/sys_enter_recvfrom")
int trace_db_recvfrom_entry(struct trace_event_raw_sys_enter *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 buf_ptr = (__u64)ctx->args[1];

    bpf_map_update_elem(&read_buffers, &pid_tgid, &buf_ptr, BPF_ANY);
    return 0;
}

// Check recvfrom result for database data
SEC("tracepoint/syscalls/sys_exit_recvfrom")
int trace_db_recvfrom_exit(struct trace_event_raw_sys_exit *ctx)
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

    __u8 db_type, event_type;
    detect_db_protocol(buf, count, &db_type, &event_type);

    if (db_type != DB_TYPE_UNKNOWN) {
        emit_db_event(buf, count, db_type, event_type);
    }

    return 0;
}
