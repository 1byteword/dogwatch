//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_endian.h>

#define AF_INET 2

char LICENSE[] SEC("license") = "GPL";

// Event structure sent to userspace
// Note: Field order matters for alignment - 8-byte fields first
struct event {
    __u64 ts_ns;
    __u32 pid;
    __u32 uid;
    __u32 saddr;
    __u32 daddr;
    __u16 sport;
    __u16 dport;
    char comm[16];
};

// Ring buffer to send events to userspace
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} events SEC(".maps");

// Track sockets being connected (for getting source port later)
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 4096);
    __type(key, __u64);   // pid_tgid
    __type(value, struct sock *);
} sockets SEC(".maps");

SEC("kprobe/tcp_connect")
int kprobe_tcp_connect(struct pt_regs *ctx)
{
    struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);
    __u64 pid_tgid = bpf_get_current_pid_tgid();

    // Store socket for later retrieval
    bpf_map_update_elem(&sockets, &pid_tgid, &sk, BPF_ANY);

    return 0;
}

SEC("kretprobe/tcp_connect")
int kretprobe_tcp_connect(struct pt_regs *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    struct sock **skp = bpf_map_lookup_elem(&sockets, &pid_tgid);
    if (!skp) {
        return 0;
    }

    struct sock *sk = *skp;
    bpf_map_delete_elem(&sockets, &pid_tgid);

    // Read socket info
    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) {
        return 0;
    }

    e->pid = pid_tgid >> 32;
    e->uid = bpf_get_current_uid_gid() & 0xFFFFFFFF;
    e->ts_ns = bpf_ktime_get_ns();

    // Read addresses from socket
    BPF_CORE_READ_INTO(&e->saddr, sk, __sk_common.skc_rcv_saddr);
    BPF_CORE_READ_INTO(&e->daddr, sk, __sk_common.skc_daddr);
    BPF_CORE_READ_INTO(&e->sport, sk, __sk_common.skc_num);
    __u16 dport;
    BPF_CORE_READ_INTO(&dport, sk, __sk_common.skc_dport);
    e->dport = bpf_ntohs(dport);

    bpf_get_current_comm(&e->comm, sizeof(e->comm));

    bpf_ringbuf_submit(e, 0);

    return 0;
}

// Also track TCP accepts for incoming connections
SEC("kretprobe/inet_csk_accept")
int kretprobe_inet_csk_accept(struct pt_regs *ctx)
{
    struct sock *sk = (struct sock *)PT_REGS_RC(ctx);
    if (!sk) {
        return 0;
    }

    __u16 family;
    BPF_CORE_READ_INTO(&family, sk, __sk_common.skc_family);

    // Only handle IPv4 for now
    if (family != AF_INET) {
        return 0;
    }

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) {
        return 0;
    }

    __u64 pid_tgid = bpf_get_current_pid_tgid();
    e->pid = pid_tgid >> 32;
    e->uid = bpf_get_current_uid_gid() & 0xFFFFFFFF;
    e->ts_ns = bpf_ktime_get_ns();

    // For accepts, local is "dest" and remote is "source" (incoming)
    BPF_CORE_READ_INTO(&e->daddr, sk, __sk_common.skc_rcv_saddr);
    BPF_CORE_READ_INTO(&e->saddr, sk, __sk_common.skc_daddr);
    __u16 sport;
    BPF_CORE_READ_INTO(&sport, sk, __sk_common.skc_dport);
    e->sport = bpf_ntohs(sport);
    BPF_CORE_READ_INTO(&e->dport, sk, __sk_common.skc_num);

    bpf_get_current_comm(&e->comm, sizeof(e->comm));

    bpf_ringbuf_submit(e, 0);

    return 0;
}
