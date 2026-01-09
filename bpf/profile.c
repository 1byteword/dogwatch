//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_core_read.h>

char LICENSE[] SEC("license") = "GPL";

#define MAX_STACK_DEPTH 32
#define MAX_ENTRIES 10240

// Stack trace storage
struct {
    __uint(type, BPF_MAP_TYPE_STACK_TRACE);
    __uint(key_size, sizeof(u32));
    __uint(value_size, MAX_STACK_DEPTH * sizeof(u64));
    __uint(max_entries, MAX_ENTRIES);
} stack_traces SEC(".maps");

// Key for aggregation: pid + stack_id
struct stack_key {
    u32 pid;
    u32 tgid;
    s32 kernel_stack_id;
    s32 user_stack_id;
    char comm[16];
};

// Count per stack
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, MAX_ENTRIES);
    __type(key, struct stack_key);
    __type(value, u64);
} stack_counts SEC(".maps");

SEC("perf_event")
int profile_cpu(struct bpf_perf_event_data *ctx)
{
    u64 id = bpf_get_current_pid_tgid();
    u32 pid = id >> 32;
    u32 tgid = id;

    // Skip idle process
    if (pid == 0)
        return 0;

    struct stack_key key = {};
    key.pid = pid;
    key.tgid = tgid;

    bpf_get_current_comm(&key.comm, sizeof(key.comm));

    // Get kernel stack trace
    key.kernel_stack_id = bpf_get_stackid(ctx, &stack_traces, 0);

    // Get user stack trace
    key.user_stack_id = bpf_get_stackid(ctx, &stack_traces, BPF_F_USER_STACK);

    // Increment count for this stack
    u64 *count = bpf_map_lookup_elem(&stack_counts, &key);
    if (count) {
        __sync_fetch_and_add(count, 1);
    } else {
        u64 one = 1;
        bpf_map_update_elem(&stack_counts, &key, &one, BPF_ANY);
    }

    return 0;
}
