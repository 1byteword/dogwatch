package pii

import (
	"encoding/json"
	"sync"
)

// TypeConfig holds configuration for a specific PII type
type TypeConfig struct {
	Enabled  bool              `json:"enabled"`
	Strategy RedactionStrategy `json:"strategy"`
}

// CustomPattern defines a custom PII detection pattern
type CustomPattern struct {
	Name     string            `json:"name"`
	Pattern  string            `json:"pattern"`
	Enabled  bool              `json:"enabled"`
	Strategy RedactionStrategy `json:"strategy"`
}

// Config holds PII detection and redaction configuration
type Config struct {
	mu sync.RWMutex

	// Per-type configuration
	TypeConfigs map[PIIType]TypeConfig `json:"type_configs"`

	// Custom patterns
	CustomPatterns []CustomPattern `json:"custom_patterns"`

	// Allowlist - values that should not be redacted
	Allowlist []string `json:"allowlist"`

	// Denylist - values that should always be detected
	Denylist []string `json:"denylist"`

	// Default strategy when type-specific not defined
	DefaultStrategy RedactionStrategy `json:"default_strategy"`

	// Global enable/disable
	Enabled bool `json:"enabled"`

	// Whether to include IP addresses (often needed for debugging)
	IncludeIPAddresses bool `json:"include_ip_addresses"`
}

// DefaultConfig returns a default PII configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:            true,
		DefaultStrategy:    StrategyMask,
		IncludeIPAddresses: false,
		TypeConfigs: map[PIIType]TypeConfig{
			TypeEmail: {
				Enabled:  true,
				Strategy: StrategyMask,
			},
			TypePhone: {
				Enabled:  true,
				Strategy: StrategyMask,
			},
			TypeSSN: {
				Enabled:  true,
				Strategy: StrategyMask,
			},
			TypeCreditCard: {
				Enabled:  true,
				Strategy: StrategyMask,
			},
			TypeIPAddress: {
				Enabled:  false, // Disabled by default
				Strategy: StrategyMask,
			},
			TypeAWSKey: {
				Enabled:  true,
				Strategy: StrategyFormat,
			},
			TypeGitHubToken: {
				Enabled:  true,
				Strategy: StrategyFormat,
			},
			TypeStripeKey: {
				Enabled:  true,
				Strategy: StrategyFormat,
			},
			TypeGenericKey: {
				Enabled:  true,
				Strategy: StrategyMask,
			},
			TypeJWT: {
				Enabled:  true,
				Strategy: StrategyFormat,
			},
			TypePassword: {
				Enabled:  true,
				Strategy: StrategyMask,
			},
		},
		CustomPatterns: []CustomPattern{},
		Allowlist:      []string{},
		Denylist:       []string{},
	}
}

// IsTypeEnabled checks if a PII type is enabled
func (c *Config) IsTypeEnabled(piiType PIIType) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.Enabled {
		return false
	}

	// Special handling for IP addresses
	if piiType == TypeIPAddress && !c.IncludeIPAddresses {
		return false
	}

	if cfg, ok := c.TypeConfigs[piiType]; ok {
		return cfg.Enabled
	}

	// Default to enabled for known types
	return true
}

// GetStrategy returns the redaction strategy for a PII type
func (c *Config) GetStrategy(piiType PIIType) RedactionStrategy {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if cfg, ok := c.TypeConfigs[piiType]; ok {
		return cfg.Strategy
	}

	return c.DefaultStrategy
}

// SetTypeConfig updates configuration for a PII type
func (c *Config) SetTypeConfig(piiType PIIType, enabled bool, strategy RedactionStrategy) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.TypeConfigs == nil {
		c.TypeConfigs = make(map[PIIType]TypeConfig)
	}

	c.TypeConfigs[piiType] = TypeConfig{
		Enabled:  enabled,
		Strategy: strategy,
	}
}

// AddCustomPattern adds a custom detection pattern
func (c *Config) AddCustomPattern(name, pattern string, strategy RedactionStrategy) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.CustomPatterns = append(c.CustomPatterns, CustomPattern{
		Name:     name,
		Pattern:  pattern,
		Enabled:  true,
		Strategy: strategy,
	})
}

// RemoveCustomPattern removes a custom pattern by name
func (c *Config) RemoveCustomPattern(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, cp := range c.CustomPatterns {
		if cp.Name == name {
			c.CustomPatterns = append(c.CustomPatterns[:i], c.CustomPatterns[i+1:]...)
			return true
		}
	}
	return false
}

// AddToAllowlist adds a value to the allowlist
func (c *Config) AddToAllowlist(value string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check for duplicates
	for _, v := range c.Allowlist {
		if v == value {
			return
		}
	}

	c.Allowlist = append(c.Allowlist, value)
}

// RemoveFromAllowlist removes a value from the allowlist
func (c *Config) RemoveFromAllowlist(value string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, v := range c.Allowlist {
		if v == value {
			c.Allowlist = append(c.Allowlist[:i], c.Allowlist[i+1:]...)
			return true
		}
	}
	return false
}

// IsAllowed checks if a value is in the allowlist
func (c *Config) IsAllowed(value string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, v := range c.Allowlist {
		if v == value {
			return true
		}
	}
	return false
}

// AddToDenylist adds a value to the denylist
func (c *Config) AddToDenylist(value string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, v := range c.Denylist {
		if v == value {
			return
		}
	}

	c.Denylist = append(c.Denylist, value)
}

// RemoveFromDenylist removes a value from the denylist
func (c *Config) RemoveFromDenylist(value string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, v := range c.Denylist {
		if v == value {
			c.Denylist = append(c.Denylist[:i], c.Denylist[i+1:]...)
			return true
		}
	}
	return false
}

// IsDenied checks if a value is in the denylist
func (c *Config) IsDenied(value string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, v := range c.Denylist {
		if v == value {
			return true
		}
	}
	return false
}

// Clone creates a deep copy of the config
func (c *Config) Clone() *Config {
	c.mu.RLock()
	defer c.mu.RUnlock()

	clone := &Config{
		Enabled:            c.Enabled,
		DefaultStrategy:    c.DefaultStrategy,
		IncludeIPAddresses: c.IncludeIPAddresses,
		TypeConfigs:        make(map[PIIType]TypeConfig),
		CustomPatterns:     make([]CustomPattern, len(c.CustomPatterns)),
		Allowlist:          make([]string, len(c.Allowlist)),
		Denylist:           make([]string, len(c.Denylist)),
	}

	for k, v := range c.TypeConfigs {
		clone.TypeConfigs[k] = v
	}
	copy(clone.CustomPatterns, c.CustomPatterns)
	copy(clone.Allowlist, c.Allowlist)
	copy(clone.Denylist, c.Denylist)

	return clone
}

// ToJSON serializes config to JSON
func (c *Config) ToJSON() ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return json.Marshal(c)
}

// FromJSON deserializes config from JSON
func ConfigFromJSON(data []byte) (*Config, error) {
	config := DefaultConfig()
	if err := json.Unmarshal(data, config); err != nil {
		return nil, err
	}
	return config, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Validate custom patterns are valid regex
	for _, cp := range c.CustomPatterns {
		if cp.Pattern == "" {
			continue
		}
		// Pattern validation is done at compile time in detector
	}

	return nil
}
