// Package apm provides application performance monitoring for Go applications.
// It automatically instruments HTTP handlers, database calls, and external services,
// sending traces and metrics to a dogwatch server.
package apm

import (
	"os"
	"time"
)

// Config holds APM agent configuration
type Config struct {
	// ServiceName is the name of your application (required)
	ServiceName string

	// ServiceVersion is the version of your application
	ServiceVersion string

	// Environment (e.g., production, staging, development)
	Environment string

	// AgentEndpoint is the dogwatch server endpoint (default: http://localhost:9999)
	AgentEndpoint string

	// SampleRate controls trace sampling (0.0-1.0, default: 1.0 = 100%)
	SampleRate float64

	// FlushInterval is how often to send data to the server
	FlushInterval time.Duration

	// MaxSpansPerSecond limits span throughput to prevent overload
	MaxSpansPerSecond int

	// EnableMetrics enables runtime metrics collection
	EnableMetrics bool

	// EnableProfiling enables continuous profiling
	EnableProfiling bool

	// Tags are added to all spans and metrics
	Tags map[string]string

	// Debug enables verbose logging
	Debug bool

	// Disabled completely disables the agent
	Disabled bool
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig(serviceName string) *Config {
	endpoint := os.Getenv("DOGWATCH_AGENT_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:9999"
	}

	env := os.Getenv("DOGWATCH_ENV")
	if env == "" {
		env = "development"
	}

	return &Config{
		ServiceName:       serviceName,
		ServiceVersion:    os.Getenv("DOGWATCH_SERVICE_VERSION"),
		Environment:       env,
		AgentEndpoint:     endpoint,
		SampleRate:        1.0,
		FlushInterval:     10 * time.Second,
		MaxSpansPerSecond: 1000,
		EnableMetrics:     true,
		EnableProfiling:   false,
		Tags:              make(map[string]string),
		Debug:             os.Getenv("DOGWATCH_DEBUG") == "true",
		Disabled:          os.Getenv("DOGWATCH_DISABLED") == "true",
	}
}

// WithVersion sets the service version
func (c *Config) WithVersion(version string) *Config {
	c.ServiceVersion = version
	return c
}

// WithEnvironment sets the environment
func (c *Config) WithEnvironment(env string) *Config {
	c.Environment = env
	return c
}

// WithEndpoint sets the agent endpoint
func (c *Config) WithEndpoint(endpoint string) *Config {
	c.AgentEndpoint = endpoint
	return c
}

// WithSampleRate sets the sampling rate
func (c *Config) WithSampleRate(rate float64) *Config {
	c.SampleRate = rate
	return c
}

// WithTag adds a global tag
func (c *Config) WithTag(key, value string) *Config {
	c.Tags[key] = value
	return c
}

// WithMetrics enables/disables metrics collection
func (c *Config) WithMetrics(enabled bool) *Config {
	c.EnableMetrics = enabled
	return c
}

// WithProfiling enables/disables continuous profiling
func (c *Config) WithProfiling(enabled bool) *Config {
	c.EnableProfiling = enabled
	return c
}
