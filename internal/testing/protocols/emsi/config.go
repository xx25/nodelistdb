package emsi

import (
	"strings"
	"sync"
	"time"
)

// Config holds EMSI handshake configuration
// Defaults are FSC-0056.001 compliant
type Config struct {
	// Timeouts (FSC-0056.001: T1=20s step, T2=60s master)
	MasterTimeout     time.Duration `yaml:"master_timeout"`     // T2: 60s
	StepTimeout       time.Duration `yaml:"step_timeout"`       // T1: 20s
	FirstStepTimeout  time.Duration `yaml:"first_step_timeout"` // First try: 10s
	CharacterTimeout  time.Duration `yaml:"character_timeout"`  // Char read: 5s

	// Retry Settings (FSC-0056.001: 6 retries)
	MaxRetries int           `yaml:"max_retries"` // 6
	RetryDelay time.Duration `yaml:"retry_delay"` // 1s

	// Initial Handshake Behavior
	InitialStrategy   string        `yaml:"initial_strategy"`    // "wait" (FSC-0056 default)
	InitialCRInterval time.Duration `yaml:"initial_cr_interval"` // 500ms (if send_cr)
	InitialCRTimeout  time.Duration `yaml:"initial_cr_timeout"`  // 5s (if send_cr)

	// EMSI_INQ Behavior
	PreventiveINQ        bool          `yaml:"preventive_inq"`         // false (standard)
	PreventiveINQTimeout time.Duration `yaml:"preventive_inq_timeout"` // 5s
	SendINQTwice         bool          `yaml:"send_inq_twice"`         // true (FSC-0056)
	INQInterval          time.Duration `yaml:"inq_interval"`           // 200ms

	// EMSI_REQ/ACK Behavior
	SendREQTwice   bool `yaml:"send_req_twice"`    // true
	SendACKTwice   bool `yaml:"send_ack_twice"`    // true (de facto standard)
	SendNAKOnRetry bool `yaml:"send_nak_on_retry"` // true

	// Compatibility
	AcceptFDLenWithCR bool `yaml:"accept_fd_len_with_cr"` // true (FrontDoor bug)

	// Protocol Features
	Protocols []string `yaml:"protocols"` // ["ZMO", "ZAP"]
}

// DefaultConfig returns FSC-0056.001 compliant defaults
// Note: Some values are practical optimizations or de facto standards,
// documented inline where they deviate from strict FSC-0056.001.
func DefaultConfig() *Config {
	return &Config{
		// FSC-0056.001 standard timeouts
		MasterTimeout:    60 * time.Second, // T2 (FSC-0056.001)
		StepTimeout:      20 * time.Second, // T1 (FSC-0056.001)
		FirstStepTimeout: 10 * time.Second, // Practical optimization (not in spec)
		CharacterTimeout: 5 * time.Second,  // Practical default (not in spec)

		// FSC-0056.001 standard retries
		MaxRetries: 6,                  // FSC-0056.001
		RetryDelay: 1 * time.Second,    // Practical default

		// Initial handshake: wait for remote EMSI_INQ (FSC-0056.001 calling side)
		// Alternative strategies available via config for problematic nodes
		InitialStrategy:   "wait",
		InitialCRInterval: 500 * time.Millisecond, // Used with "send_cr" strategy
		InitialCRTimeout:  5 * time.Second,        // Used with "send_cr" strategy

		// EMSI_INQ behavior
		PreventiveINQ:        false,                  // Not in FSC-0056 (configurable)
		PreventiveINQTimeout: 5 * time.Second,
		SendINQTwice:         true,                   // FSC-0056.001 recommends
		INQInterval:          200 * time.Millisecond,

		// Packet behavior
		SendREQTwice:   true, // FSC-0056.001
		SendACKTwice:   true, // De facto standard (all mailers do this)
		SendNAKOnRetry: true, // FSC-0056.001

		// Compatibility workarounds (enabled by default)
		AcceptFDLenWithCR: true, // FrontDoor bug workaround

		// Minimum protocol set
		Protocols: []string{"ZMO", "ZAP"},
	}
}

// NodeOverride holds per-node configuration overrides
// Only non-nil fields override the global config
// Loaded from daemon YAML configuration
type NodeOverride struct {
	// Timeouts
	MasterTimeout    *time.Duration `yaml:"master_timeout,omitempty"`
	StepTimeout      *time.Duration `yaml:"step_timeout,omitempty"`
	FirstStepTimeout *time.Duration `yaml:"first_step_timeout,omitempty"`
	CharacterTimeout *time.Duration `yaml:"character_timeout,omitempty"`

	// Retry Settings
	MaxRetries *int           `yaml:"max_retries,omitempty"`
	RetryDelay *time.Duration `yaml:"retry_delay,omitempty"`

	// Initial Handshake
	InitialStrategy   *string        `yaml:"initial_strategy,omitempty"`
	InitialCRInterval *time.Duration `yaml:"initial_cr_interval,omitempty"`
	InitialCRTimeout  *time.Duration `yaml:"initial_cr_timeout,omitempty"`

	// EMSI_INQ Behavior
	PreventiveINQ        *bool          `yaml:"preventive_inq,omitempty"`
	PreventiveINQTimeout *time.Duration `yaml:"preventive_inq_timeout,omitempty"`
	SendINQTwice         *bool          `yaml:"send_inq_twice,omitempty"`
	INQInterval          *time.Duration `yaml:"inq_interval,omitempty"`

	// EMSI_REQ/ACK Behavior
	SendREQTwice   *bool `yaml:"send_req_twice,omitempty"`
	SendACKTwice   *bool `yaml:"send_ack_twice,omitempty"`
	SendNAKOnRetry *bool `yaml:"send_nak_on_retry,omitempty"`

	// Compatibility
	AcceptFDLenWithCR *bool `yaml:"accept_fd_len_with_cr,omitempty"`

	// Protocol Features - pointer allows explicit empty list for NCP
	Protocols *[]string `yaml:"protocols,omitempty"`
}

// ConfigManager manages global config and per-node overrides
// Thread-safe for concurrent access
type ConfigManager struct {
	mu        sync.RWMutex
	global    *Config
	overrides map[string]*NodeOverride // key: normalized FidoNet address
}

// NewConfigManager creates a config manager with FSC-0056.001 defaults
func NewConfigManager() *ConfigManager {
	return &ConfigManager{
		global:    DefaultConfig(),
		overrides: make(map[string]*NodeOverride),
	}
}

// NewConfigManagerWithConfig creates a config manager with custom global config
// Merges provided config with defaults to handle partial YAML configs
func NewConfigManagerWithConfig(global *Config) *ConfigManager {
	merged := DefaultConfig()
	if global != nil {
		merged.MergeFrom(global)
	}
	return &ConfigManager{
		global:    merged,
		overrides: make(map[string]*NodeOverride),
	}
}

// GetConfigForNode returns effective config for a specific node
// Returns a copy, safe for concurrent use
func (m *ConfigManager) GetConfigForNode(addr string) *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := m.global.Copy()
	if override, ok := m.overrides[normalizeConfigAddress(addr)]; ok {
		cfg.ApplyOverride(override)
	}
	return cfg
}

// SetOverride sets per-node override
// The override is defensively copied to prevent shared mutation.
func (m *ConfigManager) SetOverride(addr string, override *NodeOverride) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if override != nil {
		override = override.Copy()
	}
	m.overrides[normalizeConfigAddress(addr)] = override
}

// LoadOverrides loads per-node overrides from a map (typically from YAML)
// Overrides are defensively copied to prevent shared mutation.
func (m *ConfigManager) LoadOverrides(overrides map[string]*NodeOverride) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for addr, override := range overrides {
		if override != nil {
			override = override.Copy()
		}
		m.overrides[normalizeConfigAddress(addr)] = override
	}
}

// Copy creates a deep copy of config
func (c *Config) Copy() *Config {
	cp := *c
	cp.Protocols = make([]string, len(c.Protocols))
	copy(cp.Protocols, c.Protocols)
	return &cp
}

// Copy creates a copy of NodeOverride
// Since all fields are pointers to immutable primitives, a shallow copy is sufficient.
func (n *NodeOverride) Copy() *NodeOverride {
	if n == nil {
		return nil
	}
	cp := *n
	// Deep copy Protocols slice if present
	if n.Protocols != nil {
		protocols := make([]string, len(*n.Protocols))
		copy(protocols, *n.Protocols)
		cp.Protocols = &protocols
	}
	return &cp
}

// MergeFrom merges non-zero values from another config
// Used to apply partial YAML configs over defaults
func (c *Config) MergeFrom(other *Config) {
	if other == nil {
		return
	}
	if other.MasterTimeout != 0 {
		c.MasterTimeout = other.MasterTimeout
	}
	if other.StepTimeout != 0 {
		c.StepTimeout = other.StepTimeout
	}
	if other.FirstStepTimeout != 0 {
		c.FirstStepTimeout = other.FirstStepTimeout
	}
	if other.CharacterTimeout != 0 {
		c.CharacterTimeout = other.CharacterTimeout
	}
	if other.MaxRetries != 0 {
		c.MaxRetries = other.MaxRetries
	}
	if other.RetryDelay != 0 {
		c.RetryDelay = other.RetryDelay
	}
	if other.InitialStrategy != "" {
		c.InitialStrategy = other.InitialStrategy
	}
	if other.InitialCRInterval != 0 {
		c.InitialCRInterval = other.InitialCRInterval
	}
	if other.InitialCRTimeout != 0 {
		c.InitialCRTimeout = other.InitialCRTimeout
	}
	// Bool fields: DO NOT copy in MergeFrom because we can't distinguish
	// "not set" from "explicitly set to false". Bools can only be changed
	// via NodeOverride which uses pointers.
	// If you need to change bool defaults globally, use NewConfigManagerWithConfig
	// with a fully populated Config, or use NodeOverride for specific nodes.

	if other.PreventiveINQTimeout != 0 {
		c.PreventiveINQTimeout = other.PreventiveINQTimeout
	}
	if other.INQInterval != 0 {
		c.INQInterval = other.INQInterval
	}
	// Protocols: check for non-nil (not length) to allow empty slice for NCP
	if other.Protocols != nil {
		c.Protocols = make([]string, len(other.Protocols))
		copy(c.Protocols, other.Protocols)
	}
}

// ApplyOverride applies non-nil override fields to config
func (c *Config) ApplyOverride(override *NodeOverride) {
	if override == nil {
		return
	}
	// Timeouts
	if override.MasterTimeout != nil {
		c.MasterTimeout = *override.MasterTimeout
	}
	if override.StepTimeout != nil {
		c.StepTimeout = *override.StepTimeout
	}
	if override.FirstStepTimeout != nil {
		c.FirstStepTimeout = *override.FirstStepTimeout
	}
	if override.CharacterTimeout != nil {
		c.CharacterTimeout = *override.CharacterTimeout
	}
	// Retry Settings
	if override.MaxRetries != nil {
		c.MaxRetries = *override.MaxRetries
	}
	if override.RetryDelay != nil {
		c.RetryDelay = *override.RetryDelay
	}
	// Initial Handshake
	if override.InitialStrategy != nil {
		c.InitialStrategy = *override.InitialStrategy
	}
	if override.InitialCRInterval != nil {
		c.InitialCRInterval = *override.InitialCRInterval
	}
	if override.InitialCRTimeout != nil {
		c.InitialCRTimeout = *override.InitialCRTimeout
	}
	// EMSI_INQ Behavior
	if override.PreventiveINQ != nil {
		c.PreventiveINQ = *override.PreventiveINQ
	}
	if override.PreventiveINQTimeout != nil {
		c.PreventiveINQTimeout = *override.PreventiveINQTimeout
	}
	if override.SendINQTwice != nil {
		c.SendINQTwice = *override.SendINQTwice
	}
	if override.INQInterval != nil {
		c.INQInterval = *override.INQInterval
	}
	// EMSI_REQ/ACK Behavior
	if override.SendREQTwice != nil {
		c.SendREQTwice = *override.SendREQTwice
	}
	if override.SendACKTwice != nil {
		c.SendACKTwice = *override.SendACKTwice
	}
	if override.SendNAKOnRetry != nil {
		c.SendNAKOnRetry = *override.SendNAKOnRetry
	}
	// Compatibility
	if override.AcceptFDLenWithCR != nil {
		c.AcceptFDLenWithCR = *override.AcceptFDLenWithCR
	}
	// Protocols - pointer allows explicit empty list for NCP
	if override.Protocols != nil {
		c.Protocols = make([]string, len(*override.Protocols))
		copy(c.Protocols, *override.Protocols)
	}
}

// normalizeConfigAddress converts "2:5020/2021.0" to "2:5020/2021"
// Named differently from normalizeAddress in session.go to avoid conflict
func normalizeConfigAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	addr = strings.TrimSuffix(addr, ".0")
	return addr
}
