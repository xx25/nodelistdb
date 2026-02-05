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

	// RX Phase Behavior
	SkipFirstRXReq bool `yaml:"skip_first_rx_req"` // true (FSC-0056: caller waits on first RX try)

	// Protocol Features
	Protocols []string `yaml:"protocols"` // ["ZMO", "ZAP"]

	// EMSI-II Features (FSC-0088.001)
	EnableEMSI2        bool     `yaml:"enable_emsi2"`         // Advertise EII flag
	EnableDFB          bool     `yaml:"enable_dfb"`           // Advertise DFB (fall-back) flag
	CallerPrefsMode    bool     `yaml:"caller_prefs_mode"`    // true = EMSI-II caller-prefs (default)
	FileRequestCapable bool     `yaml:"file_request_capable"` // Advertise FRQ flag
	NoFileRequests     bool     `yaml:"no_file_requests"`     // Advertise NRQ flag
	RequireFNC         bool     `yaml:"require_fnc"`          // Request 8.3 filename conversion

	// Link Codes (expanded from single PUA)
	LinkCode       string   `yaml:"link_code"`       // PUA, PUP, NPU, HAT (default: PUA)
	LinkQualifiers []string `yaml:"link_qualifiers"` // PMO, NFE, NXP, NRQ, HNM, HXT, HFE, HRQ

	// Per-AKA Flags (for multi-address systems, FSC-0088.001)
	PerAKAFlags map[int]*PerAKAConfig `yaml:"per_aka_flags"` // Key is address index (0-based)

	// Deprecated flags control (EMSI-II: don't send ARC, XMA)
	SuppressDeprecated bool `yaml:"suppress_deprecated"` // Don't send ARC, XMA in EMSI-II
}

// PerAKAConfig holds link flags for a specific AKA index (FSC-0088.001)
// Flags use XXn format in EMSI packets (e.g., PU2, HA3)
type PerAKAConfig struct {
	// Pickup flags
	Pickup       *bool `yaml:"pickup,omitempty"`        // PUn
	PickupMail   *bool `yaml:"pickup_mail,omitempty"`   // PMn (mail only)
	NoFiles      *bool `yaml:"no_files,omitempty"`      // NFn (no TICs/attaches)
	NoCompressed *bool `yaml:"no_compressed,omitempty"` // NXn (no compressed mail)
	NoRequests   *bool `yaml:"no_requests,omitempty"`   // NRn (no file requests)

	// Hold flags
	Hold         *bool `yaml:"hold,omitempty"`          // HAn
	HoldNonMail  *bool `yaml:"hold_non_mail,omitempty"` // HNn (hold except mail)
	HoldCompress *bool `yaml:"hold_compress,omitempty"` // HXn (hold compressed)
	HoldFiles    *bool `yaml:"hold_files,omitempty"`    // HFn (hold TICs/attaches)
	HoldRequests *bool `yaml:"hold_requests,omitempty"` // HRn (hold requests)
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

		// RX phase: caller should not send REQ on first try per FSC-0056.001
		// (answering system sends REQ; caller waits then sends NAK on retry)
		SkipFirstRXReq: true,

		// Minimum protocol set
		Protocols: []string{"ZMO", "ZAP"},

		// EMSI-II defaults (conservative - EMSI-I compatible)
		EnableEMSI2:        false,  // Don't advertise EII by default
		EnableDFB:          false,  // Don't advertise DFB by default
		CallerPrefsMode:    true,   // EMSI-II requires caller-prefs
		FileRequestCapable: false,  // Don't advertise FRQ by default
		NoFileRequests:     false,  // Don't send NRQ unless explicitly disabled
		RequireFNC:         false,  // Don't request filename conversion
		LinkCode:           "PUA",  // Default: Pickup All
		LinkQualifiers:     nil,    // No qualifiers by default
		PerAKAFlags:        nil,    // No per-AKA flags by default
		SuppressDeprecated: false,  // Send ARC, XMA for EMSI-I compat
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

	// RX Phase Behavior
	SkipFirstRXReq *bool `yaml:"skip_first_rx_req,omitempty"`

	// Protocol Features - pointer allows explicit empty list for NCP
	Protocols *[]string `yaml:"protocols,omitempty"`

	// EMSI-II Overrides (FSC-0088.001)
	EnableEMSI2        *bool     `yaml:"enable_emsi2,omitempty"`
	EnableDFB          *bool     `yaml:"enable_dfb,omitempty"`
	CallerPrefsMode    *bool     `yaml:"caller_prefs_mode,omitempty"`
	FileRequestCapable *bool     `yaml:"file_request_capable,omitempty"`
	NoFileRequests     *bool     `yaml:"no_file_requests,omitempty"`
	RequireFNC         *bool     `yaml:"require_fnc,omitempty"`
	LinkCode           *string   `yaml:"link_code,omitempty"`
	LinkQualifiers     *[]string `yaml:"link_qualifiers,omitempty"`
	PerAKAFlags        map[int]*PerAKAConfig `yaml:"per_aka_flags,omitempty"`
	SuppressDeprecated *bool     `yaml:"suppress_deprecated,omitempty"`
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
// Merges non-zero non-bool fields from provided config, then applies default-false
// bool fields that are set to true. This properly handles partial YAML configs.
//
// Note: To override default-true bools (like SendINQTwice) to false, use NodeOverride
// which supports pointer bools for proper set/unset distinction.
func NewConfigManagerWithConfig(global *Config) *ConfigManager {
	merged := DefaultConfig()
	if global != nil {
		// Merge non-bool fields (safe for partial configs)
		merged.MergeFrom(global)
		// Apply default-false bools that are explicitly set to true
		// This avoids zeroing default-true bools like SendINQTwice
		merged.applyDefaultFalseBoolsFrom(global)
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
	// Deep copy LinkQualifiers
	if c.LinkQualifiers != nil {
		cp.LinkQualifiers = make([]string, len(c.LinkQualifiers))
		copy(cp.LinkQualifiers, c.LinkQualifiers)
	}
	// Deep copy PerAKAFlags
	if c.PerAKAFlags != nil {
		cp.PerAKAFlags = make(map[int]*PerAKAConfig)
		for k, v := range c.PerAKAFlags {
			if v != nil {
				cp.PerAKAFlags[k] = v.Copy()
			}
		}
	}
	return &cp
}

// Copy creates a copy of PerAKAConfig
func (p *PerAKAConfig) Copy() *PerAKAConfig {
	if p == nil {
		return nil
	}
	cp := *p
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
	// Deep copy LinkQualifiers slice if present
	if n.LinkQualifiers != nil {
		qualifiers := make([]string, len(*n.LinkQualifiers))
		copy(qualifiers, *n.LinkQualifiers)
		cp.LinkQualifiers = &qualifiers
	}
	// Deep copy PerAKAFlags map if present
	if n.PerAKAFlags != nil {
		cp.PerAKAFlags = make(map[int]*PerAKAConfig)
		for k, v := range n.PerAKAFlags {
			if v != nil {
				cp.PerAKAFlags[k] = v.Copy()
			}
		}
	}
	return &cp
}

// MergeFrom merges non-zero values from another config
// Used to apply partial YAML configs over defaults
// NOTE: Bool fields are NOT merged - use MergeFromComplete for full config merging
func (c *Config) MergeFrom(other *Config) {
	if other == nil {
		return
	}
	c.mergeNonBoolFields(other)
}

// MergeFromComplete merges ALL fields including bools from another config.
// Non-bool fields use non-zero merge semantics; bool fields are copied directly.
// WARNING: This will zero default-true bools if other has them as false.
// For partial YAML configs, use MergeFrom + applyDefaultFalseBoolsFrom instead.
func (c *Config) MergeFromComplete(other *Config) {
	if other == nil {
		return
	}
	c.mergeNonBoolFields(other)

	// Copy all bool fields - caller indicates they want complete merge
	c.PreventiveINQ = other.PreventiveINQ
	c.SendINQTwice = other.SendINQTwice
	c.SendREQTwice = other.SendREQTwice
	c.SendACKTwice = other.SendACKTwice
	c.SendNAKOnRetry = other.SendNAKOnRetry
	c.AcceptFDLenWithCR = other.AcceptFDLenWithCR
	c.EnableEMSI2 = other.EnableEMSI2
	c.EnableDFB = other.EnableDFB
	c.CallerPrefsMode = other.CallerPrefsMode
	c.FileRequestCapable = other.FileRequestCapable
	c.NoFileRequests = other.NoFileRequests
	c.RequireFNC = other.RequireFNC
	c.SuppressDeprecated = other.SuppressDeprecated
	c.SkipFirstRXReq = other.SkipFirstRXReq
}

// applyDefaultFalseBoolsFrom applies bool fields that default to false when set to true.
// Since we can't distinguish "not set" from "explicitly false" with non-pointer bools,
// we only apply true values. This fixes the bug where enable_emsi2: true was ignored
// while preserving default-true bools like SendINQTwice in partial configs.
//
// Note: To set bools to false (override default-true values), use NodeOverride which
// uses pointer bools and can properly distinguish set vs unset.
func (c *Config) applyDefaultFalseBoolsFrom(other *Config) {
	if other == nil {
		return
	}
	// Only apply true values - false could mean "not set" in YAML
	// All these bools default to false, so true means explicitly enabled
	if other.PreventiveINQ {
		c.PreventiveINQ = true
	}
	if other.EnableEMSI2 {
		c.EnableEMSI2 = true
	}
	if other.EnableDFB {
		c.EnableDFB = true
	}
	if other.FileRequestCapable {
		c.FileRequestCapable = true
	}
	if other.NoFileRequests {
		c.NoFileRequests = true
	}
	if other.RequireFNC {
		c.RequireFNC = true
	}
	if other.SuppressDeprecated {
		c.SuppressDeprecated = true
	}
	if other.SkipFirstRXReq {
		c.SkipFirstRXReq = true
	}
}

// mergeNonBoolFields merges non-zero non-bool values from another config
func (c *Config) mergeNonBoolFields(other *Config) {
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

	// EMSI-II non-bool fields
	if other.LinkCode != "" {
		c.LinkCode = other.LinkCode
	}
	if other.LinkQualifiers != nil {
		c.LinkQualifiers = make([]string, len(other.LinkQualifiers))
		copy(c.LinkQualifiers, other.LinkQualifiers)
	}
	if other.PerAKAFlags != nil {
		c.PerAKAFlags = make(map[int]*PerAKAConfig)
		for k, v := range other.PerAKAFlags {
			if v != nil {
				c.PerAKAFlags[k] = v.Copy()
			}
		}
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
	// RX Phase
	if override.SkipFirstRXReq != nil {
		c.SkipFirstRXReq = *override.SkipFirstRXReq
	}
	// Protocols - pointer allows explicit empty list for NCP
	if override.Protocols != nil {
		c.Protocols = make([]string, len(*override.Protocols))
		copy(c.Protocols, *override.Protocols)
	}

	// EMSI-II Overrides (FSC-0088.001)
	if override.EnableEMSI2 != nil {
		c.EnableEMSI2 = *override.EnableEMSI2
	}
	if override.EnableDFB != nil {
		c.EnableDFB = *override.EnableDFB
	}
	if override.CallerPrefsMode != nil {
		c.CallerPrefsMode = *override.CallerPrefsMode
	}
	if override.FileRequestCapable != nil {
		c.FileRequestCapable = *override.FileRequestCapable
	}
	if override.NoFileRequests != nil {
		c.NoFileRequests = *override.NoFileRequests
	}
	if override.RequireFNC != nil {
		c.RequireFNC = *override.RequireFNC
	}
	if override.LinkCode != nil {
		c.LinkCode = *override.LinkCode
	}
	if override.LinkQualifiers != nil {
		c.LinkQualifiers = make([]string, len(*override.LinkQualifiers))
		copy(c.LinkQualifiers, *override.LinkQualifiers)
	}
	if override.PerAKAFlags != nil {
		c.PerAKAFlags = make(map[int]*PerAKAConfig)
		for k, v := range override.PerAKAFlags {
			if v != nil {
				c.PerAKAFlags[k] = v.Copy()
			}
		}
	}
	if override.SuppressDeprecated != nil {
		c.SuppressDeprecated = *override.SuppressDeprecated
	}
}

// normalizeConfigAddress converts "2:5020/2021.0" to "2:5020/2021"
// Named differently from normalizeAddress in session.go to avoid conflict
func normalizeConfigAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	addr = strings.TrimSuffix(addr, ".0")
	return addr
}
