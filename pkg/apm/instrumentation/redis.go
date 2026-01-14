package instrumentation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"dogwatch/pkg/apm"
)

// RedisHook provides APM instrumentation for Redis clients
// This is designed to work with go-redis but the pattern can be adapted
type RedisHook struct {
	addr     string
	db       int
	spanName string
}

// NewRedisHook creates a new Redis APM hook
func NewRedisHook(addr string, db int) *RedisHook {
	return &RedisHook{
		addr:     addr,
		db:       db,
		spanName: "redis.command",
	}
}

// RedisSpan represents a traced Redis operation
type RedisSpan struct {
	span    *apm.Span
	cmd     string
	start   time.Time
	addr    string
	db      int
}

// StartRedisSpan begins tracing a Redis command
func StartRedisSpan(ctx context.Context, addr string, db int, cmd string, args ...interface{}) (*RedisSpan, context.Context) {
	// Normalize command
	cmdUpper := strings.ToUpper(cmd)
	resource := cmdUpper
	if len(args) > 0 {
		if key, ok := args[0].(string); ok {
			resource = fmt.Sprintf("%s %s", cmdUpper, key)
		}
	}

	span, ctx := apm.StartSpanFromContext(ctx, "redis.command",
		apm.WithSpanType(apm.SpanTypeCache),
		apm.WithResource(resource),
		apm.WithTag(apm.TagSpanKind, apm.SpanKindClient),
	)

	span.SetTag(apm.TagCacheType, "redis")
	span.SetTag("redis.command", cmdUpper)
	span.SetTag("redis.addr", addr)
	span.SetTag("redis.db", fmt.Sprintf("%d", db))
	span.SetTag(apm.TagComponent, "redis")

	// Add key if available
	if len(args) > 0 {
		if key, ok := args[0].(string); ok {
			span.SetTag(apm.TagCacheKey, truncateKey(key))
		}
	}

	return &RedisSpan{
		span:  span,
		cmd:   cmdUpper,
		start: time.Now(),
		addr:  addr,
		db:    db,
	}, ctx
}

// Finish completes the Redis span
func (rs *RedisSpan) Finish(err error) {
	if rs == nil || rs.span == nil {
		return
	}

	duration := time.Since(rs.start)
	rs.span.SetMetric("redis.duration_ms", float64(duration.Milliseconds()))

	if err != nil {
		rs.span.SetError(err)
	}

	rs.span.Finish()

	// Record metrics
	tags := map[string]string{
		"command": rs.cmd,
		"addr":    rs.addr,
		"db":      fmt.Sprintf("%d", rs.db),
	}
	if err != nil {
		tags["error"] = "true"
		apm.RecordCounter("redis.errors", 1, tags)
	}
	apm.RecordHistogram("redis.command.duration_ms", float64(duration.Milliseconds()), tags)
	apm.RecordCounter("redis.command.count", 1, tags)
}

// SetHit marks the operation as a cache hit
func (rs *RedisSpan) SetHit(hit bool) {
	if rs == nil || rs.span == nil {
		return
	}
	if hit {
		rs.span.SetTag(apm.TagCacheHit, "true")
	} else {
		rs.span.SetTag(apm.TagCacheHit, "false")
	}
}

// truncateKey truncates a key for display
func truncateKey(key string) string {
	if len(key) > 100 {
		return key[:97] + "..."
	}
	return key
}

// TracedRedisClient provides a simple traced Redis client interface
// Applications using go-redis should use hooks instead
type TracedRedisClient struct {
	addr string
	db   int
	// In a real implementation, this would wrap an actual Redis client
}

// NewTracedRedisClient creates a new traced Redis client wrapper
func NewTracedRedisClient(addr string, db int) *TracedRedisClient {
	return &TracedRedisClient{addr: addr, db: db}
}

// TraceCommand traces a Redis command execution
// This is a helper for manual instrumentation
func (c *TracedRedisClient) TraceCommand(ctx context.Context, cmd string, args ...interface{}) func(err error) {
	rs, _ := StartRedisSpan(ctx, c.addr, c.db, cmd, args...)
	return func(err error) {
		rs.Finish(err)
	}
}

// Example Redis operations with tracing (for documentation purposes)

// ExampleGet demonstrates tracing a GET operation
func ExampleGet(ctx context.Context, client *TracedRedisClient, key string) {
	finish := client.TraceCommand(ctx, "GET", key)
	// result, err := actualRedisClient.Get(ctx, key).Result()
	var err error // placeholder
	finish(err)
}

// ExampleSet demonstrates tracing a SET operation
func ExampleSet(ctx context.Context, client *TracedRedisClient, key, value string) {
	finish := client.TraceCommand(ctx, "SET", key, value)
	// err := actualRedisClient.Set(ctx, key, value, 0).Err()
	var err error // placeholder
	finish(err)
}

// ExamplePipeline demonstrates tracing a pipeline
func ExamplePipeline(ctx context.Context, client *TracedRedisClient, commands []string) {
	span, ctx := apm.StartSpanFromContext(ctx, "redis.pipeline",
		apm.WithSpanType(apm.SpanTypeCache),
		apm.WithResource(fmt.Sprintf("PIPELINE (%d commands)", len(commands))),
	)
	defer span.Finish()

	span.SetTag("redis.pipeline.size", fmt.Sprintf("%d", len(commands)))
	span.SetTag("redis.addr", client.addr)

	// Execute pipeline...
}

// go-redis Hook interface implementation
// For use with github.com/redis/go-redis/v9

// BeforeProcess is called before each Redis command
func (h *RedisHook) BeforeProcess(ctx context.Context, cmd interface{}) (context.Context, error) {
	// This would extract command info from the go-redis Cmd type
	// cmdStr := cmd.String()
	// rs, ctx := StartRedisSpan(ctx, h.addr, h.db, cmdStr)
	// return context.WithValue(ctx, redisSpanKey{}, rs), nil
	return ctx, nil
}

// AfterProcess is called after each Redis command
func (h *RedisHook) AfterProcess(ctx context.Context, cmd interface{}) error {
	// rs, ok := ctx.Value(redisSpanKey{}).(*RedisSpan)
	// if ok {
	//     rs.Finish(cmd.Err())
	// }
	return nil
}

// BeforeProcessPipeline is called before pipeline execution
func (h *RedisHook) BeforeProcessPipeline(ctx context.Context, cmds []interface{}) (context.Context, error) {
	return ctx, nil
}

// AfterProcessPipeline is called after pipeline execution
func (h *RedisHook) AfterProcessPipeline(ctx context.Context, cmds []interface{}) error {
	return nil
}
