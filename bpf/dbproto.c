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
    if (size < 6) return 0;  // Need at least header + cmd + 1 byte

    char header[6];
    bpf_probe_read_user(header, 6, buf);

    // Parse 3-byte length (little-endian)
    __u32 pkt_len = (__u32)((__u8)header[0]) |
                    ((__u32)((__u8)header[1]) << 8) |
                    ((__u32)((__u8)header[2]) << 16);

    // Packet length must be reasonable AND match buffer size
    // pkt_len is payload length, total packet = pkt_len + 4
    if (pkt_len < 1 || pkt_len > 16777215) return 0;

    // Verify packet length is consistent with actual data size
    // Allow some tolerance for partial reads, but total should be close
    __u32 expected_total = pkt_len + 4;
    if (expected_total > size + 100) return 0;  // Way too big
    if (size > expected_total + 1000) return 0; // Data way bigger than packet claims

    // Sequence number - for new commands it resets to 0, but we allow any value
    // since persistent connections may have various states
    // Just reject clearly invalid values
    __u8 seq_num = (__u8)header[3];
    (void)seq_num;  // Currently unused, kept for future validation

    // Command byte
    __u8 cmd = (__u8)header[4];

    // COM_QUERY (0x03) - basic validation, detailed check in userspace
    if (cmd == 0x03) {
        if (pkt_len < 2) return 0;  // Need some query text
        // MySQL 8+ may have extra bytes before query (query attributes)
        // Check first few bytes for printable SQL or skip leading nulls
        char query_bytes[4];
        bpf_probe_read_user(query_bytes, 4, buf + 5);

        // Skip leading control characters (MySQL 8 query attributes)
        for (int i = 0; i < 4; i++) {
            char c = query_bytes[i];
            // Found printable char - accept
            if (c >= 32 && c <= 126) return 1;
            if (c == '\t' || c == '\n' || c == '\r') return 1;
            // Control char 0-31 (except tab/newline) - skip and continue
        }
        // All 4 bytes were control chars - likely not SQL
        return 0;
    }

    // COM_STMT_PREPARE (0x16)
    if (cmd == 0x16) {
        if (pkt_len < 2) return 0;
        return 1;
    }

    // COM_STMT_EXECUTE (0x17) - has specific binary format
    // First 4 bytes after cmd are statement ID (uint32)
    if (cmd == 0x17 && pkt_len >= 5) {
        return 1;
    }

    // COM_INIT_DB (0x02) - database name should be printable
    if (cmd == 0x02 && pkt_len >= 2) {
        char db_char;
        bpf_probe_read_user(&db_char, 1, buf + 5);
        if ((db_char >= 'a' && db_char <= 'z') ||
            (db_char >= 'A' && db_char <= 'Z') ||
            db_char == '_') {
            return 1;
        }
        return 0;
    }

    return 0;
}

// MySQL response detection (OK, ERR, or result set)
static __always_inline int is_mysql_response(const char *buf, __u32 size) {
    if (size < 5) return 0;

    char header[12];
    __u32 read_len = size < 12 ? size : 12;
    bpf_probe_read_user(header, read_len, buf);

    // Parse 3-byte length (little-endian)
    __u32 pkt_len = (__u32)((__u8)header[0]) |
                    ((__u32)((__u8)header[1]) << 8) |
                    ((__u32)((__u8)header[2]) << 16);

    // Packet length must be reasonable
    if (pkt_len < 1 || pkt_len > 16777215) return 0;

    // Verify packet length is consistent with actual data size
    __u32 expected_total = pkt_len + 4;
    if (expected_total > size + 100) return 0;
    if (size > expected_total + 1000) return 0;

    // Sequence number for responses is typically 1+ (after query's 0)
    __u8 seq_num = (__u8)header[3];
    // Could be any value in a conversation, but very high is suspicious
    if (seq_num > 100) return 0;

    __u8 status = (__u8)header[4];

    // OK packet (0x00) - has specific structure
    // Byte 5+ are affected_rows (length-encoded), last_insert_id, status_flags, warnings
    if (status == 0x00) {
        // OK packet should be at least 7 bytes total (header + min OK body)
        if (pkt_len >= 3) return 1;
        return 0;
    }

    // ERR packet (0xff) - has error code and message
    if (status == 0xff) {
        // ERR packet: 0xff + 2-byte error code + '#' + 5-char state + message
        if (pkt_len >= 3 && size >= 9) {
            // Error code is 2 bytes after 0xff
            // Optionally followed by '#' marker
            // Only match if error code is in valid MySQL range (1000-4999 common)
            __u16 err_code = (__u16)((__u8)header[5]) | ((__u16)((__u8)header[6]) << 8);
            if (err_code >= 1000 && err_code < 10000) return 1;
            // Also accept errors without code validation if '#' marker present
            if (size >= 8 && header[7] == '#') return 1;
        }
        return 0;
    }

    // EOF packet (0xfe) - MySQL 4.1+ has warnings and status
    if (status == 0xfe) {
        // EOF is exactly 5 bytes payload in MySQL 4.1+: 0xfe + 2 warnings + 2 status
        if (pkt_len == 5 || pkt_len == 1) return 1;
        return 0;
    }

    // Result set - first packet has field count (1-252, but typically small)
    // This is the trickiest case - could be confused with other protocols
    // Only accept if field count is reasonable (1-50 columns typical)
    if (status >= 1 && status <= 50) {
        // For result sets, the packet length should be small (just the count)
        if (pkt_len == 1 || (pkt_len <= 9 && pkt_len >= 1)) {
            return 1;
        }
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
// +OK\r\n, -ERR message\r\n, :123\r\n, $<len>\r\n, *<count>\r\n
static __always_inline int is_redis_response(const char *buf, __u32 size) {
    if (size < 4) return 0;

    char header[4];
    bpf_probe_read_user(header, 4, buf);

    char b0 = header[0];
    char b1 = header[1];

    // Simple string: +
    if (b0 == '+') return 1;

    // Error: - but NOT ----BEGIN (certificate)
    if (b0 == '-') {
        if (b1 == '-') return 0;  // Likely certificate ----
        if (b1 >= 'A' && b1 <= 'Z') return 1;  // Error type like ERR, WRONGTYPE
        return 0;
    }

    // Integer: :123
    if (b0 == ':') {
        if ((b1 >= '0' && b1 <= '9') || b1 == '-') return 1;
        return 0;
    }

    // Bulk string: $6
    if (b0 == '$') {
        if ((b1 >= '0' && b1 <= '9') || b1 == '-') return 1;
        return 0;
    }

    // Array: *2
    if (b0 == '*') {
        if ((b1 >= '0' && b1 <= '9') || b1 == '-') return 1;
        return 0;
    }

    return 0;
}

// Detect database type and event type
static __always_inline void detect_db_protocol(const char *buf, __u32 size, __u8 *db_type, __u8 *event_type) {
    *db_type = DB_TYPE_UNKNOWN;
    *event_type = 0;

    // Try Redis FIRST - it has very distinctive RESP markers (*, +, -, $, :)
    // and is less likely to false-positive on other protocols
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

    // Try PostgreSQL - has clear message type markers (Q, P, E, B, T, D, C, etc.)
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

    // Try MySQL last - binary protocol is harder to distinguish
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
}

// Emit a database event
static __always_inline void emit_db_event(char *buf, __u32 count, __u8 db_type, __u8 event_type) {
    struct db_event *e = bpf_ringbuf_reserve(&db_events, sizeof(*e), 0);
    if (!e) {
        return;
    }

    __u64 pid_tgid = bpf_get_current_pid_tgid();
    e->ts_ns = bpf_ktime_get_ns();
    e->pid = pid_tgid >> 32;
    e->tid = (__u32)pid_tgid;
    e->uid = bpf_get_current_uid_gid() & 0xFFFFFFFF;
    e->db_type = db_type;
    e->event_type = event_type;

    bpf_get_current_comm(&e->comm, sizeof(e->comm));

    // Read payload safely - explicit bounds for BPF verifier
    __u32 read_size = count > MAX_PAYLOAD_SIZE ? MAX_PAYLOAD_SIZE : count;
    read_size &= (MAX_PAYLOAD_SIZE - 1);  // Verifier-friendly bound: 0-511
    e->payload_size = read_size;

    bpf_probe_read_user(&e->payload, read_size, buf);

    bpf_ringbuf_submit(e, 0);
}

// Capture writes that look like database protocol
SEC("tracepoint/syscalls/sys_enter_write")
int trace_db_write_entry(struct trace_event_raw_sys_enter *ctx)
{
    char *buf = (char *)ctx->args[1];
    __u64 raw_count = (__u64)ctx->args[2];

    // Explicit bound for BPF verifier
    if (raw_count < 4 || raw_count > 65535) {
        return 0;
    }
    __u32 count = raw_count & 0xFFFF;

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

    // Explicit bound for BPF verifier
    __u32 count = (__u32)ret & 0xFFFF;  // Max 65535
    if (count < 4) {
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
    __u64 raw_count = (__u64)ctx->args[2];

    // Explicit bound for BPF verifier
    if (raw_count < 4 || raw_count > 65535) {
        return 0;
    }
    __u32 count = raw_count & 0xFFFF;

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

    // Explicit bound for BPF verifier
    __u32 count = (__u32)ret & 0xFFFF;  // Max 65535
    if (count < 4) {
        return 0;
    }

    __u8 db_type, event_type;
    detect_db_protocol(buf, count, &db_type, &event_type);

    if (db_type != DB_TYPE_UNKNOWN) {
        emit_db_event(buf, count, db_type, event_type);
    }

    return 0;
}

// Helper: read the first iov_base and iov_len from an iovec array pointer.
// struct iovec { void *iov_base; size_t iov_len; }
// Returns 0 on success, -1 on failure.
static __always_inline int read_first_iov(const void *iov_array, char **base_out, __u32 *len_out)
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
    *len_out = (__u32)(iov_len & 0xFFFF);  // Bound for BPF verifier
    return 0;
}

// Capture sendmsg for database protocol
// sendmsg(int fd, const struct msghdr *msg, int flags)
SEC("tracepoint/syscalls/sys_enter_sendmsg")
int trace_db_sendmsg_entry(struct trace_event_raw_sys_enter *ctx)
{
    const void *msg_ptr = (const void *)ctx->args[1];
    if (!msg_ptr)
        return 0;

    // Read msg_iov pointer from msghdr (offset 16 on 64-bit)
    struct iovec *iov_ptr;
    if (bpf_probe_read_user(&iov_ptr, sizeof(iov_ptr), (const char *)msg_ptr + 16) < 0)
        return 0;

    char *buf;
    __u32 count;
    if (read_first_iov(iov_ptr, &buf, &count) < 0)
        return 0;

    if (count < 4)
        return 0;

    __u8 db_type, event_type;
    detect_db_protocol(buf, count, &db_type, &event_type);

    if (db_type != DB_TYPE_UNKNOWN) {
        emit_db_event(buf, count, db_type, event_type);
    }

    return 0;
}

// Store buffer for recvmsg (read the first iov from msghdr on entry)
SEC("tracepoint/syscalls/sys_enter_recvmsg")
int trace_db_recvmsg_entry(struct trace_event_raw_sys_enter *ctx)
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

// Check recvmsg result for database data
SEC("tracepoint/syscalls/sys_exit_recvmsg")
int trace_db_recvmsg_exit(struct trace_event_raw_sys_exit *ctx)
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

    __u32 count = (__u32)ret & 0xFFFF;
    if (count < 4)
        return 0;

    __u8 db_type, event_type;
    detect_db_protocol(buf, count, &db_type, &event_type);

    if (db_type != DB_TYPE_UNKNOWN) {
        emit_db_event(buf, count, db_type, event_type);
    }

    return 0;
}

// Capture writev for database protocol (scatter/gather write)
// writev(int fd, const struct iovec *iov, int iovcnt)
SEC("tracepoint/syscalls/sys_enter_writev")
int trace_db_writev_entry(struct trace_event_raw_sys_enter *ctx)
{
    const void *iov_array = (const void *)ctx->args[1];

    char *buf;
    __u32 count;
    if (read_first_iov(iov_array, &buf, &count) < 0)
        return 0;

    if (count < 4)
        return 0;

    __u8 db_type, event_type;
    detect_db_protocol(buf, count, &db_type, &event_type);

    if (db_type != DB_TYPE_UNKNOWN) {
        emit_db_event(buf, count, db_type, event_type);
    }

    return 0;
}

// Store buffer for readv (scatter/gather read)
// readv(int fd, const struct iovec *iov, int iovcnt)
SEC("tracepoint/syscalls/sys_enter_readv")
int trace_db_readv_entry(struct trace_event_raw_sys_enter *ctx)
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

// Check readv result for database data
SEC("tracepoint/syscalls/sys_exit_readv")
int trace_db_readv_exit(struct trace_event_raw_sys_exit *ctx)
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

    __u32 count = (__u32)ret & 0xFFFF;
    if (count < 4)
        return 0;

    __u8 db_type, event_type;
    detect_db_protocol(buf, count, &db_type, &event_type);

    if (db_type != DB_TYPE_UNKNOWN) {
        emit_db_event(buf, count, db_type, event_type);
    }

    return 0;
}
