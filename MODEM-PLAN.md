# Distributed Modem Testing System - Architecture Plan

## Overview

This document describes the architecture for implementing modem-based testing of FidoNet nodelist entries with phone numbers. The system uses remote modem daemons located in different geographic regions to test nodes, with results reported back to the central server via API.

## Key Requirements

1. **Same Protocol Results**: EMSI/IFCICO results should be identical whether via IP or modem (SystemName, MailerInfo, Addresses, etc.)
2. **Multiple Remote Servers**: Different physical locations with modems testing different countries
3. **Server-Assigned Queues**: Server assigns nodes to daemons based on prefix configuration in config.yaml
4. **Config-Based Management**: All daemon configuration (prefixes, priorities, API keys) in config.yaml, not database
5. **Daemon-Side Modem Selection**: Daemons decide which local modem to use based on node's protocol flags
6. **Simple Assignment**: Each node assigned to exactly one daemon (requires single-threaded assignment - see ClickHouse Limitations)
7. **Decoupled Architecture**: Remote servers communicate via API, no direct database access

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Main Server (VM)                                │
│                                                                              │
│  ┌──────────────────┐    ┌──────────────────┐    ┌──────────────────────┐   │
│  │   config.yaml    │    │   API Endpoints   │    │     ClickHouse       │   │
│  │                  │    │                   │    │                      │   │
│  │ modem_api:       │───▶│ /api/modem/*      │◀──▶│ modem_test_queue     │   │
│  │   callers:       │    │                   │    │ modem_caller_status  │   │
│  │   - modem-eu-01  │    │                   │    │ node_test_results    │   │
│  │   - modem-ru-01  │    │                   │    │                      │   │
│  │   - modem-us-01  │    │                   │    │                      │   │
│  └──────────────────┘    └──────────────────┘    └──────────────────────┘   │
│           │                      ▲                         │                 │
│           │ Assignment           │ HTTPS                   │ Runtime         │
│           │ (from config)        │                         │ Status          │
│           ▼                      │                         ▼                 │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │  Node Assignment Logic (reads prefixes from config.yaml):             │   │
│  │  +49... → modem-eu-01 (exclude +7,+86,+1)                            │   │
│  │  +7...  → modem-ru-01 (include +7)                                   │   │
│  │  +1...  → modem-us-01 (include +1)                                   │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
└──────────────────────────────────┼──────────────────────────────────────────┘
                                   │
        ┌──────────────────────────┼──────────────────────────┐
        │                          │                          │
        ▼                          ▼                          ▼
┌───────────────────┐      ┌───────────────────┐      ┌───────────────────┐
│  Modem Daemon EU  │      │  Modem Daemon RU  │      │  Modem Daemon US  │
│                   │      │                   │      │                   │
│  Config:          │      │  Config:          │      │  Config:          │
│  caller_id        │      │  caller_id        │      │  caller_id        │
│  api_key          │      │  api_key          │      │  api_key          │
│  (prefix routing  │      │  (prefix routing  │      │  (prefix routing  │
│   on server side) │      │   on server side) │      │   on server side) │
│                   │      │                   │      │                   │
│  Local Modems:    │      │  Local Modems:    │      │  Local Modems:    │
│  ┌─────────────┐  │      │  ┌─────────────┐  │      │  ┌─────────────┐  │
│  │zyxel, usr   │  │      │  │generic-v34  │  │      │  │usr-courier  │  │
│  └─────────────┘  │      │  └─────────────┘  │      │  └─────────────┘  │
│        │          │      │        │          │      │        │          │
│        ▼          │      │        ▼          │      │        ▼          │
│  Daemon selects   │      │  Daemon selects   │      │  Daemon selects   │
│  best modem for   │      │  best modem for   │      │  best modem for   │
│  node's flags     │      │  node's flags     │      │  node's flags     │
│  (V34, ZYX, etc.) │      │  (V34, ZYX, etc.) │      │  (V34, ZYX, etc.) │
│                   │      │                   │      │                   │
│  ┌─────────────┐  │      │  ┌─────────────┐  │      │  ┌─────────────┐  │
│  │ EMSI Session│  │      │  │ EMSI Session│  │      │  │ EMSI Session│  │
│  │ (reused!)   │  │      │  │ (reused!)   │  │      │  │ (reused!)   │  │
│  └─────────────┘  │      │  └─────────────┘  │      │  └─────────────┘  │
└───────────────────┘      └───────────────────┘      └───────────────────┘
```

## Why Server-Assigned Queues (Not Shared Queue with Claiming)

| Aspect | Shared Queue + Claiming | Server-Assigned Queues |
|--------|------------------------|------------------------|
| **Complexity** | Badger cache, atomic claims, TTL | Simple assignment on entry |
| **Race Conditions** | Complex handling needed | None - deterministic |
| **Rollback Logic** | Required for partial failures | Not needed |
| **Cache/DB Sync** | Reconciliation task needed | Single source of truth |
| **Code Size** | ~500 extra lines | Minimal |

## Why Config-Based Daemon Management (Not Database)

| Aspect | Database Storage | Config File |
|--------|-----------------|-------------|
| **Single Source of Truth** | Config + DB tables | Config only |
| **Deployment** | Requires API calls or DB access | Edit config, restart |
| **Version Control** | Can't easily track changes | Git-trackable |
| **Consistency** | Daemon might register with wrong prefixes | Server defines routing |
| **Simplicity** | Registration endpoint, DB migrations | Just config parsing |
| **Admin Visibility** | Need admin UI or DB queries | Read config file |

## Why Daemon-Side Modem Selection (Not Server-Side)

| Aspect | Server-Side Selection | Daemon-Side Selection |
|--------|----------------------|----------------------|
| **Server Knowledge** | Must know all modem protocols | Only knows phone prefixes |
| **Configuration** | Server config per modem type | Daemon config only |
| **Flexibility** | Server update to add modem | Daemon update only |
| **Simplicity** | Protocol matching in API | Server just passes flags |

## Data Model

### 1. Modem Callers Configuration (config.yaml)

All daemon configuration is stored in the server's config.yaml file. No database table needed for configuration - only runtime status tracking.

```yaml
# config.yaml
modem_api:
  enabled: true

  # Daemon definitions (all configuration here, not in database)
  callers:
    - caller_id: "modem-eu-01"
      name: "Europe Modem Server"
      api_key_hash: "sha256:abc123..."
      location: "Frankfurt, Germany"
      priority: 10                    # Higher = preferred for overlapping prefixes
      prefix_mode: exclude            # "include", "exclude", or "all"
      prefixes:                       # Prefixes to include/exclude
        - "+7"                        # Russia - different daemon
        - "+86"                       # China - blocked
        - "+1"                        # USA - different daemon

    - caller_id: "modem-ru-01"
      name: "Russia Modem Server"
      api_key_hash: "sha256:def456..."
      location: "Moscow, Russia"
      priority: 10
      prefix_mode: include
      prefixes:
        - "+7"

    - caller_id: "modem-us-01"
      name: "US Modem Server"
      api_key_hash: "sha256:ghi789..."
      location: "New York, USA"
      priority: 10
      prefix_mode: include
      prefixes:
        - "+1"

    - caller_id: "modem-fallback"
      name: "Fallback Server"
      api_key_hash: "sha256:jkl012..."
      location: "Amsterdam, Netherlands"
      priority: 1                     # Lower priority - only if no other matches
      prefix_mode: all                # Handles any prefix
      prefixes: []

  # Queue management
  orphan_check_interval: 5m
  offline_threshold: 10m
  stale_in_progress_threshold: 1h    # Reclaim nodes stuck in_progress longer than this

  # Rate limits
  rate_limits:
    requests_per_second: 10
    burst_size: 20
    max_requests_per_minute: 600

  # Request limits
  max_batch_size: 100
  max_body_size_mb: 1
```

### 2. Table: `modem_caller_status` (Runtime Status Only)

Minimal table for tracking daemon heartbeats and runtime statistics. No configuration stored here.

```sql
CREATE TABLE modem_caller_status (
    caller_id String,                    -- References config.yaml caller_id
    last_heartbeat DateTime,
    status Enum8('active'=1, 'inactive'=2, 'maintenance'=3),

    -- Runtime stats (updated by heartbeat)
    modems_available UInt8 DEFAULT 0,
    modems_in_use UInt8 DEFAULT 0,
    tests_completed UInt32 DEFAULT 0,
    tests_failed UInt32 DEFAULT 0,
    last_test_time DateTime DEFAULT toDateTime(0),

    updated_at DateTime
) ENGINE = MergeTree()
ORDER BY caller_id
```

**Status Updates**: Use mutations for heartbeat updates:
```sql
ALTER TABLE modem_caller_status
UPDATE last_heartbeat = now(), status = 'active', modems_available = ?, ...
WHERE caller_id = ?
```

For first heartbeat (new daemon), insert a row. Subsequent heartbeats use UPDATE.

### 3. Table: `modem_test_queue` (Pre-Assigned Queue)

Nodes are assigned to specific daemons on queue entry. No claiming needed.

```sql
CREATE TABLE modem_test_queue (
    -- Node identification (stable identity - PRIMARY KEY for deduplication)
    zone UInt16,
    net UInt16,
    node UInt16,
    conflict_sequence UInt8 DEFAULT 0,   -- Matches nodelist duplicate handling
    phone String,
    phone_normalized String,             -- "+74951234567" for prefix matching
    modem_flags Array(String),           -- [V34, V32B, ZYX] - passed to daemon for local selection
    flags Array(String),                 -- General node flags [CM, XA, TAN] for time availability

    -- Time availability (denormalized from flags for query efficiency)
    is_cm Bool DEFAULT false,            -- Has CM flag (24/7 available)
    time_flags Array(String),            -- T-flags like [T, A, N] parsed from flags

    -- Pre-assignment (replaces claiming)
    assigned_to String,                  -- caller_id of assigned daemon
    assigned_at DateTime,

    -- Scheduling
    priority UInt8,
    retry_count UInt8,
    next_attempt_after DateTime,         -- For retry backoff

    -- Status
    status Enum8('pending'=1, 'in_progress'=2, 'completed'=3, 'failed'=4),
    in_progress_since DateTime DEFAULT toDateTime(0),  -- For stale detection
    last_tested_at DateTime DEFAULT toDateTime(0),
    last_error String DEFAULT '',

    -- Metadata
    created_at DateTime,
    updated_at DateTime
) ENGINE = MergeTree()
ORDER BY (zone, net, node, conflict_sequence);

-- Secondary index for daemon queries (WHERE assigned_to = ? AND status = ?)
ALTER TABLE modem_test_queue ADD INDEX idx_assigned_status (assigned_to, status) TYPE set(10) GRANULARITY 4;
```

**ORDER BY vs Query Pattern**: ORDER BY uses node identity for deduplication checks, but main queries filter by `assigned_to` and `status`. The secondary index helps ClickHouse skip irrelevant granules.

**Why MergeTree (not ReplacingMergeTree)**:
- ReplacingMergeTree deduplication is asynchronous (happens during background merges)
- MergeTree with application-level checks is more predictable
- Simpler to reason about for small-scale deployment (2-3 daemons)

**ClickHouse Limitations** (acceptable for small scale):
- `ALTER UPDATE` is asynchronous (returns before completion), but completes in ms-seconds, well before next daemon poll (30s+)
- No UNIQUE constraints, so duplicate prevention relies on single-threaded assignment
- For 2-3 daemons with 30s polling, these are non-issues in practice

**Duplicate Prevention**: Application-level check before insert, executed in single goroutine (no parallel assignment workers).

**Status Updates**: Use ClickHouse mutations:
```sql
-- Mark as in_progress (with timestamp for stale detection)
ALTER TABLE modem_test_queue
UPDATE status = 'in_progress', in_progress_since = now(), updated_at = now()
WHERE zone = ? AND net = ? AND node = ? AND conflict_sequence = ?

-- Mark as completed/failed (clears in_progress_since)
ALTER TABLE modem_test_queue
UPDATE status = 'completed', in_progress_since = toDateTime(0), last_tested_at = now(), updated_at = now()
WHERE zone = ? AND net = ? AND node = ? AND conflict_sequence = ?
```

### 4. Extend `node_test_results` Table (Modem Columns)

These columns mirror the IFCICO structure since the same EMSI protocol is used:

```sql
-- EMSI results (same structure as ifcico)
modem_tested Bool DEFAULT false,
modem_success Bool DEFAULT false,
modem_response_ms UInt32 DEFAULT 0,
modem_system_name String DEFAULT '',
modem_mailer_info String DEFAULT '',
modem_addresses Array(String) DEFAULT [],
modem_address_valid Bool DEFAULT false,
modem_response_type String DEFAULT '',
modem_software_source String DEFAULT '',
modem_error String DEFAULT '',

-- Modem-specific fields
modem_connect_speed UInt32 DEFAULT 0,     -- Actual bps (33600, etc.)
modem_protocol String DEFAULT '',          -- V.34, V.92, etc.
modem_caller_id String DEFAULT '',         -- Which daemon tested this
modem_phone_dialed String DEFAULT '',      -- Phone number used
modem_ring_count UInt8 DEFAULT 0,
modem_carrier_time_ms UInt32 DEFAULT 0,

-- Daemon-reported modem info (daemon decides which modem to use)
modem_used String DEFAULT '',              -- Which modem was used (daemon's local ID)
modem_match_reason String DEFAULT '',      -- Why this modem was selected
modem_expected_speed UInt32 DEFAULT 0,     -- Expected speed based on matching
modem_actual_speed UInt32 DEFAULT 0        -- Actual negotiated speed
```

## Node Assignment Logic

When a node with a phone number is added to the modem test queue, the server assigns it to the appropriate daemon based on prefix matching. Daemon configuration is read from config.yaml.

### Config Loading

```go
// internal/config/modem.go

type ModemAPIConfig struct {
    Enabled                    bool                `yaml:"enabled"`
    Callers                    []ModemCallerConfig `yaml:"callers"`
    OrphanCheckInterval        time.Duration       `yaml:"orphan_check_interval"`
    OfflineThreshold           time.Duration       `yaml:"offline_threshold"`
    StaleInProgressThreshold   time.Duration       `yaml:"stale_in_progress_threshold"`
    RateLimits                 RateLimitConfig     `yaml:"rate_limits"`
    MaxBatchSize               int                 `yaml:"max_batch_size"`
    MaxBodySizeMB              int                 `yaml:"max_body_size_mb"`
}

type ModemCallerConfig struct {
    CallerID   string   `yaml:"caller_id"`
    Name       string   `yaml:"name"`
    APIKeyHash string   `yaml:"api_key_hash"`
    Location   string   `yaml:"location"`
    Priority   int      `yaml:"priority"`
    PrefixMode string   `yaml:"prefix_mode"`  // "include", "exclude", "all"
    Prefixes   []string `yaml:"prefixes"`
}

// GetActiveCallers returns callers from config that have recent heartbeats
func (c *ModemAPIConfig) GetActiveCallers(statusStore *Storage) []ModemCallerConfig {
    var active []ModemCallerConfig

    for _, caller := range c.Callers {
        status, err := statusStore.GetCallerStatus(caller.CallerID)
        if err != nil || status.LastHeartbeat.Before(time.Now().Add(-c.OfflineThreshold)) {
            continue // Skip offline daemons
        }
        active = append(active, caller)
    }

    // Sort by priority (highest first)
    sort.Slice(active, func(i, j int) bool {
        return active[i].Priority > active[j].Priority
    })

    return active
}
```

### Assignment Algorithm

```go
// internal/storage/modem_assignment.go

// ModemAssigner handles node-to-daemon assignment using config
type ModemAssigner struct {
    config  *config.ModemAPIConfig
    storage *Storage
}

// AssignModemTestNode assigns a node to the appropriate daemon based on prefix
func (a *ModemAssigner) AssignModemTestNode(node *ModemQueueEntry) error {
    // Check if node already exists in queue (duplicate prevention)
    exists, err := a.storage.NodeExistsInQueue(node.Zone, node.Net, node.Node, node.ConflictSequence)
    if err != nil {
        return fmt.Errorf("failed to check queue: %w", err)
    }
    if exists {
        return nil // Already queued, nothing to do
    }

    // Get callers from config (sorted by priority)
    callers := a.config.Callers

    // Sort by priority descending
    sort.Slice(callers, func(i, j int) bool {
        return callers[i].Priority > callers[j].Priority
    })

    phoneNorm := normalizePhone(node.Phone)

    for _, caller := range callers {
        if matchesPrefix(caller, phoneNorm) {
            // Check if daemon is active (has recent heartbeat)
            if !a.isDaemonActive(caller.CallerID) {
                continue // Skip inactive daemons
            }

            node.AssignedTo = caller.CallerID
            node.AssignedAt = time.Now()
            return a.storage.InsertModemQueueEntry(node)
        }
    }

    // No daemon can handle this prefix - log and skip
    return fmt.Errorf("no daemon available for prefix: %s", extractPrefix(phoneNorm))
}

// NodeExistsInQueue checks if a node is already in the queue
func (s *Storage) NodeExistsInQueue(zone, net, node uint16, cs uint8) (bool, error) {
    var count uint64
    err := s.db.QueryRow(`
        SELECT count() FROM modem_test_queue
        WHERE zone = ? AND net = ? AND node = ? AND conflict_sequence = ?
    `, zone, net, node, cs).Scan(&count)
    return count > 0, err
}

func (a *ModemAssigner) isDaemonActive(callerID string) bool {
    status, err := a.storage.GetCallerStatus(callerID)
    if err != nil {
        return false
    }
    return status.LastHeartbeat.After(time.Now().Add(-a.config.OfflineThreshold))
}

// matchesPrefix checks if a caller config can handle a phone number
func matchesPrefix(caller config.ModemCallerConfig, phoneNorm string) bool {
    switch caller.PrefixMode {
    case "all":
        return true

    case "include":
        for _, prefix := range caller.Prefixes {
            if strings.HasPrefix(phoneNorm, normalizePrefix(prefix)) {
                return true
            }
        }
        return false

    case "exclude":
        for _, prefix := range caller.Prefixes {
            if strings.HasPrefix(phoneNorm, normalizePrefix(prefix)) {
                return false
            }
        }
        return true
    }
    return false
}

// ReassignNode updates an existing queue entry to a new daemon.
// Unlike AssignModemTestNode, this updates existing rows rather than inserting.
func (a *ModemAssigner) ReassignNode(node *ModemQueueEntry) error {
    phoneNorm := normalizePhone(node.Phone)

    // Get callers from config (sorted by priority)
    callers := a.config.Callers
    sort.Slice(callers, func(i, j int) bool {
        return callers[i].Priority > callers[j].Priority
    })

    for _, caller := range callers {
        if matchesPrefix(caller, phoneNorm) {
            if !a.isDaemonActive(caller.CallerID) {
                continue
            }

            // Update existing row with new assignment
            return a.storage.UpdateNodeAssignment(
                node.Zone, node.Net, node.Node, node.ConflictSequence,
                caller.CallerID,
            )
        }
    }

    return fmt.Errorf("no daemon available for prefix: %s", extractPrefix(phoneNorm))
}

// UpdateNodeAssignment updates the assigned daemon for an existing queue entry
func (s *Storage) UpdateNodeAssignment(zone, net, node uint16, cs uint8, callerID string) error {
    return s.execMutation(`
        ALTER TABLE modem_test_queue
        UPDATE assigned_to = ?, assigned_at = now(), updated_at = now()
        WHERE zone = ? AND net = ? AND node = ? AND conflict_sequence = ?
    `, callerID, zone, net, node, cs)
}

// normalizePrefix removes formatting from prefix
func normalizePrefix(prefix string) string {
    // "+7" -> "+7", "+49" -> "+49", "+33-75" -> "+3375"
    return strings.ReplaceAll(strings.ReplaceAll(prefix, "-", ""), " ", "")
}
```

### API Key Validation

```go
// internal/api/modem_auth.go

// ValidateAPIKey checks API key against config and returns caller_id
func (h *ModemHandler) ValidateAPIKey(apiKey string) (string, error) {
    keyHash := "sha256:" + sha256Hash(apiKey)

    for _, caller := range h.config.Callers {
        if caller.APIKeyHash == keyHash {
            return caller.CallerID, nil
        }
    }

    return "", fmt.Errorf("invalid API key")
}
```

### Queue Population

**Batch assignment** (scheduled job or on-demand):

```go
// AssignUnassignedNodes populates queue for all configured daemons
func (a *ModemAssigner) AssignUnassignedNodes() error {
    nodes, _ := a.storage.GetNodesWithPhonesNotInQueue()

    for _, node := range nodes {
        if err := a.AssignModemTestNode(node); err != nil {
            // Log but continue - some nodes may not have matching daemons
            logging.Warn("failed to assign node", "node", node.Address, "error", err)
        }
    }

    return nil
}

```

## API Endpoints

All endpoints require Bearer token authentication. The API key is validated against config.yaml - no registration needed.

### 1. Get Assigned Nodes

Daemon retrieves only nodes assigned to it. No claiming needed.

```
GET /api/modem/nodes
Authorization: Bearer <api-key>

Query Parameters:
  limit=50                        # Max nodes to return
  only_callable=true              # Filter by time availability (default: true)

Response: 200 OK
{
    "nodes": [
        {
            "zone": 2,
            "net": 5020,
            "node": 100,
            "conflict_sequence": 0,
            "phone": "+49-555-1234567",
            "phone_normalized": "+495551234567",
            "address": "2:5020/100",
            "modem_flags": ["V34", "V32B", "ZYX"],
            "flags": ["CM", "XA"],
            "priority": 5,
            "retry_count": 0,
            "is_callable_now": true,
            "next_call_window": null
        },
        {
            "zone": 2,
            "net": 5020,
            "node": 101,
            "conflict_sequence": 0,
            "phone": "+49-555-7654321",
            "phone_normalized": "+495557654321",
            "address": "2:5020/101",
            "modem_flags": ["V32B", "MNP"],
            "flags": ["TAN"],
            "priority": 3,
            "retry_count": 1,
            "is_callable_now": false,
            "next_call_window": {
                "start_utc": "2026-01-10T20:00:00Z",
                "end_utc": "2026-01-11T08:00:00Z",
                "source": "T-flag"
            }
        }
    ],
    "remaining": 472
}
```

**Server-side query** (filters ICM-only and non-callable nodes):

```go
func (s *Storage) GetAssignedModemNodes(callerID string, limit int, onlyCallable bool) ([]ModemNode, error) {
    query := `
        SELECT zone, net, node, conflict_sequence, phone, phone_normalized,
               modem_flags, flags, is_cm, time_flags, priority, retry_count
        FROM modem_test_queue
        WHERE assigned_to = ?
          AND status = 'pending'
          AND next_attempt_after <= now()
          AND NOT (has(flags, 'ICM') AND NOT is_cm)  -- Exclude ICM-only nodes (no modem)
        ORDER BY priority DESC, next_attempt_after ASC
        LIMIT ?
    `
    nodes, err := s.queryModemNodes(query, callerID, limit)
    if err != nil {
        return nil, err
    }

    // Compute time availability for each node (server-side)
    var result []ModemNode
    for _, node := range nodes {
        node.IsCallableNow = timeavail.IsCallableNow(node.IsCM, node.TimeFlags, time.Now().UTC())
        if !node.IsCallableNow {
            node.NextCallWindow = timeavail.GetNextWindow(node.TimeFlags, time.Now().UTC())
        }

        // Filter non-callable if requested (default: true)
        if onlyCallable && !node.IsCallableNow {
            continue
        }
        result = append(result, node)
    }

    return result, nil
}
```

**Key point**: Server does ALL filtering. Daemon receives only nodes it should actually test, so marking in_progress is safe.

### 2. Mark Nodes In Progress

When daemon starts testing nodes, it marks them as in_progress.

```
POST /api/modem/in-progress
Authorization: Bearer <api-key>

Request:
{
    "nodes": [
        {"zone": 2, "net": 5020, "node": 100, "conflict_sequence": 0},
        {"zone": 2, "net": 5020, "node": 101, "conflict_sequence": 0}
    ]
}

Response: 200 OK
{
    "marked": 2
}
```

### 3. Submit Results

```
POST /api/modem/results
Authorization: Bearer <api-key>

Request:
{
    "results": [
        {
            "zone": 2,
            "net": 5020,
            "node": 100,
            "conflict_sequence": 0,
            "test_time": "2026-01-10T14:30:00Z",

            // EMSI results (same as IFCICO)
            "success": true,
            "response_ms": 45000,
            "system_name": "BBS System Name",
            "mailer_info": "BinkD 1.0",
            "addresses": ["2:5020/100"],
            "address_valid": true,
            "response_type": "DAT",
            "software_source": "emsi_dat",

            // Modem-specific (daemon-reported)
            "connect_speed": 33600,
            "modem_protocol": "V.34",
            "phone_dialed": "+49-555-1234567",
            "ring_count": 3,
            "carrier_time_ms": 8500,
            "modem_used": "zyxel-1",
            "match_reason": "Proprietary match: ZYX",
            "error": ""
        },
        {
            "zone": 2,
            "net": 5020,
            "node": 101,
            "conflict_sequence": 0,
            "test_time": "2026-01-10T14:32:00Z",
            "success": false,
            "error": "NO CARRIER after 60s",
            "response_ms": 60000
        }
    ]
}

Response: 200 OK
{
    "accepted": 2,
    "stored": 2
}
```

**Result Processing:**
1. Store test results in `node_test_results` table
2. Update queue status to `completed` or `failed` based on result
3. For failed nodes, increment `retry_count` and set `next_attempt_after`

### 4. Heartbeat

```
POST /api/modem/heartbeat
Authorization: Bearer <api-key>

Request:
{
    "status": "active",
    "modems_available": 3,
    "modems_in_use": 1,
    "tests_completed": 45,
    "tests_failed": 3,
    "last_test_time": "2026-01-10T14:32:00Z"
}

Response: 200 OK
{
    "ack": true,
    "assigned_nodes": 480
}
```

### 5. Release Nodes (Optional - for graceful shutdown or reassignment)

```
POST /api/modem/release
Authorization: Bearer <api-key>

Request:
{
    "nodes": [
        {"zone": 2, "net": 5020, "node": 100, "conflict_sequence": 0}
    ],
    "reason": "modem_failure"
}

Response: 200 OK
{
    "released": 1,
    "reassigned_to": "modem-eu-02"
}
```

## Modem Daemon Design

The modem daemon is a separate binary that runs on remote servers with modem hardware. It handles all modem-related logic locally.

### Structure

```go
// cmd/modem-daemon/main.go

type ModemDaemon struct {
    // Communication
    apiClient   *APIClient     // HTTP client to main server
    callerID    string
    apiKey      string

    // Hardware (local)
    modemPool   *ModemPool     // Manages physical modems
    modemMatcher *ModemMatcher // Selects best modem for node (LOCAL logic)

    // Config
    config      Config
}
```

### Main Loop (Pull Model)

```go
func (d *ModemDaemon) Run(ctx context.Context) {
    // 1. Start heartbeat goroutine (also validates API key on first call)
    go d.heartbeatLoop(ctx)

    // 2. Main loop: fetch assigned nodes, test, report
    for {
        select {
        case <-ctx.Done():
            return
        default:
            // Get nodes assigned to us (already filtered by server)
            nodes := d.apiClient.GetAssignedNodes(d.config.BatchSize)

            if len(nodes) == 0 {
                time.Sleep(30 * time.Second)
                continue
            }

            // Mark as in_progress
            d.apiClient.MarkInProgress(nodes)

            // Test nodes (daemon selects modem locally)
            results := d.testNodes(nodes)

            // Submit results
            d.apiClient.SubmitResults(results)
        }
    }
}
```

### Local Modem Selection (Daemon-Side)

The daemon decides which modem to use based on the node's `modem_flags`. Server never needs to know about modem protocols.

```go
// internal/modem/matcher.go (in modem-daemon)

type ModemCapabilities struct {
    ID             string
    DevicePath     string
    SpeedProtocols []string  // [V34, V32B, V32, V22, V21]
    ErrorProtocols []string  // [V42B, V42, MNP]
    Proprietary    []string  // [ZYX, HST, PEP]
    MaxSpeed       int
    IsDigital      bool      // For V.90S/X2S server-side
}

// Protocol speed rankings (higher = faster)
var speedRanking = map[string]int{
    "V90S": 56000, "V90C": 56000,
    "X2S":  56000, "X2C":  56000,
    "V34":  33600,
    "VFC":  28800,
    "V32T": 21600,
    "Z19":  19200,
    "H16":  16800,
    "V32B": 14400, "H14": 14400, "HST": 14400,
    "V32":  9600,  "H96": 9600,  "V29": 9600,
    "V22":  1200,
    "V21":  300,
}

// Proprietary protocol pairs (our modem -> node modem)
var proprietaryPairs = map[string]string{
    "ZYX":  "ZYX",   // Zyxel <-> Zyxel
    "HST":  "HST",   // USR <-> USR
    "PEP":  "PEP",   // Telebit <-> Telebit
    "X2S":  "X2C",   // Our X2S -> Node's X2C
    "V90S": "V90C",  // Our V.90S -> Node's V.90C
}

func (m *ModemMatcher) SelectBestModem(nodeFlags []string) (*ModemCapabilities, MatchResult) {
    var bestModem *ModemCapabilities
    var bestScore int
    var bestReason string

    for _, modem := range m.modems {
        score, reason := m.calculateMatchScore(modem, nodeFlags)
        if score > bestScore {
            bestScore = score
            bestModem = modem
            bestReason = reason
        }
    }

    return bestModem, MatchResult{
        Score:          bestScore,
        Reason:         bestReason,
        ExpectedSpeed:  m.estimateSpeed(bestModem, nodeFlags),
    }
}

func (m *ModemMatcher) calculateMatchScore(modem *ModemCapabilities, nodeFlags []string) (int, string) {
    // Priority 1: Proprietary match (same vendor = best performance)
    for _, prop := range modem.Proprietary {
        expectedNodeFlag, ok := proprietaryPairs[prop]
        if ok && contains(nodeFlags, expectedNodeFlag) {
            return 10000 + speedRanking[prop], fmt.Sprintf("Proprietary match: %s", prop)
        }
    }

    // Priority 2: Highest speed standard protocol
    bestSpeed := 0
    bestProto := ""
    for _, proto := range modem.SpeedProtocols {
        if contains(nodeFlags, proto) && speedRanking[proto] > bestSpeed {
            bestSpeed = speedRanking[proto]
            bestProto = proto
        }
    }

    if bestSpeed > 0 {
        return bestSpeed, fmt.Sprintf("Standard protocol: %s (%d bps)", bestProto, bestSpeed)
    }

    // Fallback: any modem can try
    return 1, "Fallback (no matching protocol)"
}
```

### Modem Selection Examples

**Example 1: Node with Zyxel modem**
```
Node flags: V32B, V42B, ZYX, CM
Our modems: [zyxel-1, usr-courier, generic-v34]

Result: Select zyxel-1
Reason: Proprietary match: ZYX (Zyxel <-> Zyxel = faster than V.34)
Expected speed: ~19200 bps (ZYX mode)
```

**Example 2: Node with V.90C (56K client)**
```
Node flags: V90C, V34, V32B, V42B
Our modems: [zyxel-1, usr-courier (digital), generic-v34]

Result: Select usr-courier
Reason: Proprietary match: V90S->V90C (56K)
Expected speed: 56000 bps downstream
```

**Example 3: Node with standard V.34**
```
Node flags: V34, V32B, V42B
Our modems: [zyxel-1, usr-courier, generic-v34]

Result: Select any (all support V.34)
Reason: Standard protocol: V34 (33600 bps)
Expected speed: 33600 bps
Tiebreaker: Prefer modem with V42B, then by availability
```

**Example 4: Node with USR HST**
```
Node flags: HST, H16, V32B, MNP
Our modems: [zyxel-1, usr-courier, generic-v34]

Result: Select usr-courier
Reason: Proprietary match: HST (16800 bps HST mode)
Expected speed: 16800 bps
```

### Node Testing (Reuses EMSI Protocol)

```go
func (d *ModemDaemon) testNode(node *NodeJob) *TestResult {
    // 1. Select best modem for this node's flags (LOCAL decision)
    modem, matchResult := d.modemMatcher.SelectBestModem(node.ModemFlags)

    if modem == nil {
        return d.errorResult(node, "no compatible modem available")
    }

    // 2. Acquire the specific modem
    if !d.modemPool.TryAcquire(modem.ID, 30*time.Second) {
        fallback := d.modemPool.AcquireAny()
        if fallback == nil {
            return d.errorResult(node, "all modems busy")
        }
        modem = fallback
    }
    defer d.modemPool.Release(modem.ID)

    // 3. Dial phone number
    conn, dialInfo := modem.Dial(node.Phone, d.config.DialTimeout)
    if dialInfo.Error != nil {
        return &TestResult{
            Node:    node,
            Success: false,
            Error:   dialInfo.Error.Error(),
        }
    }
    defer conn.Hangup()

    // 4. REUSE existing EMSI code from internal/testing/protocols/emsi/
    session := emsi.NewSession(conn, d.config.OurAddress)
    session.SetTimeout(d.config.EMSITimeout)

    if err := session.Handshake(); err != nil {
        return &TestResult{
            Node:         node,
            Success:      false,
            Error:        err.Error(),
            ConnectSpeed: dialInfo.Speed,
            Protocol:     dialInfo.Protocol,
        }
    }

    // 5. Extract results (same as IfcicoTester)
    info := session.GetRemoteInfo()
    return &TestResult{
        Node:          node,
        Success:       true,
        SystemName:    info.SystemName,
        MailerInfo:    fmt.Sprintf("%s %s", info.MailerName, info.MailerVersion),
        Addresses:     info.Addresses,
        AddressValid:  session.ValidateAddress(node.Address),
        ConnectSpeed:  dialInfo.Speed,
        Protocol:      dialInfo.Protocol,
        RingCount:     dialInfo.Rings,
        CarrierTimeMs: dialInfo.CarrierTime,
        ModemUsed:     modem.ID,
        MatchReason:   matchResult.Reason,
    }
}
```

### Key Insight: EMSI Protocol Reuse

The existing `emsi.Session` works over any `net.Conn`-compatible connection:

```go
// Current usage (IP):
conn, _ := net.Dial("tcp", "host:port")
session := emsi.NewSession(conn, ourAddress)

// Modem usage (same interface!):
modemConn := modem.Dial("+49-555-1234567")  // Returns net.Conn-compatible
session := emsi.NewSession(modemConn, ourAddress)
```

The modem connection just needs to implement `net.Conn` interface (Read, Write, Close, deadlines).

## Time Availability Support

The codebase already has a comprehensive time availability system in `internal/testing/timeavail/` that the modem daemon must use.

### Existing Implementation

| Component | File | Purpose |
|-----------|------|---------|
| `NodeAvailability` | `types.go` | Holds parsed time windows |
| `IsCallableNow()` | `types.go` | Checks if node callable at given time |
| `ParseAvailability()` | `parser.go` | Parses flags into time windows |
| `TIMSTable` | `tims_table.go` | 26 T-flag letters (A-Z) |
| `ZMHDefaults` | `zmh_defaults.go` | Per-zone mail hours |

### Supported Flags

| Flag | Meaning | Example |
|------|---------|---------|
| `CM` | Continuous Mail - 24/7 PSTN availability | Always callable |
| `ICM` | Internet Continuous Mail - 24/7 IP only | Skip for modem testing |
| `ZMH` | Zone Mail Hour | Zone 2: 02:00-03:00 UTC |
| `Txx` | TIMS time flags | `TAN` = 00:00-06:00 + 20:00-08:00 |
| `#nn` | Specific UTC hour | `#02` = 02:00-03:00 UTC |

### Integration

The API includes time availability data for each node, computed server-side:

```json
{
    "nodes": [
        {
            "zone": 2,
            "net": 5020,
            "node": 100,
            "phone": "+49-555-1234567",
            "flags": ["CM", "V34"],
            "is_cm": true,
            "is_callable_now": true,
            "next_call_window": null
        },
        {
            "zone": 2,
            "net": 5020,
            "node": 101,
            "phone": "+49-555-7654321",
            "flags": ["TAN", "V32B"],
            "is_cm": false,
            "is_callable_now": false,
            "next_call_window": {
                "start_utc": "2026-01-10T20:00:00Z",
                "end_utc": "2026-01-11T08:00:00Z",
                "source": "T-flag"
            }
        }
    ]
}
```

**Daemon Logic:**
```go
func (d *ModemDaemon) testNodes(nodes []*NodeJob) []*TestResult {
    var results []*TestResult

    // Server already filtered out ICM-only and non-callable nodes
    // Daemon can test all received nodes without additional filtering
    for _, node := range nodes {
        result := d.testNode(node)
        results = append(results, result)
    }

    return results
}
```

## Queue Management

### Daemon Offline Handling

When a daemon goes offline (missed heartbeats), its assigned nodes should be reassigned. The reassignment logic uses daemon configuration from config.yaml.

```go
// Background job running every orphan_check_interval
func (a *ModemAssigner) ReassignOrphanedNodes() error {
    // Check each configured daemon's status
    for _, caller := range a.config.Callers {
        status, err := a.storage.GetCallerStatus(caller.CallerID)

        // If no status or heartbeat too old, daemon is offline
        isOffline := err != nil ||
            status.LastHeartbeat.Before(time.Now().Add(-a.config.OfflineThreshold))

        if !isOffline {
            continue
        }

        // Get BOTH pending AND in_progress nodes from offline daemon
        nodes, _ := a.storage.GetNodesForOfflineCaller(caller.CallerID)

        for _, node := range nodes {
            // Reset status to pending first
            a.storage.ResetNodeStatus(node.Zone, node.Net, node.Node, node.ConflictSequence)

            // Reassign to another daemon (updates existing row, doesn't insert)
            if err := a.ReassignNode(&node); err != nil {
                logging.Warn("failed to reassign node",
                    "node", fmt.Sprintf("%d:%d/%d", node.Zone, node.Net, node.Node),
                    "error", err)
            }
        }

        // Update status to inactive
        a.storage.UpdateCallerStatus(caller.CallerID, "inactive")
    }

    return nil
}

// GetNodesForOfflineCaller returns both pending and in_progress nodes
func (s *Storage) GetNodesForOfflineCaller(callerID string) ([]ModemQueueEntry, error) {
    return s.queryModemQueue(`
        SELECT * FROM modem_test_queue
        WHERE assigned_to = ?
          AND status IN ('pending', 'in_progress')
    `, callerID)
}

// ResetNodeStatus resets a node back to pending status without clearing assignment.
// Use ReassignNode to change the assigned daemon.
func (s *Storage) ResetNodeStatus(zone, net, node uint16, cs uint8) error {
    return s.execMutation(`
        ALTER TABLE modem_test_queue
        UPDATE status = 'pending', in_progress_since = toDateTime(0), updated_at = now()
        WHERE zone = ? AND net = ? AND node = ? AND conflict_sequence = ?
    `, zone, net, node, cs)
}
```

### Stale In-Progress Detection

Even for active daemons, nodes can get stuck in `in_progress` (daemon crash mid-test, network issue, etc.). A separate job reclaims stale nodes.

```go
// Background job to reclaim stale in_progress nodes
func (a *ModemAssigner) ReclaimStaleNodes() error {
    // Uses config.StaleInProgressThreshold (default: 1h)
    staleNodes, _ := a.storage.GetStaleInProgressNodes(a.config.StaleInProgressThreshold)

    for _, node := range staleNodes {
        logging.Warn("reclaiming stale node",
            "node", fmt.Sprintf("%d:%d/%d", node.Zone, node.Net, node.Node),
            "assigned_to", node.AssignedTo,
            "in_progress_since", node.InProgressSince)

        // Increment retry count and reset to pending
        a.storage.RequeueStaleNode(node.Zone, node.Net, node.Node, node.ConflictSequence)
    }

    return nil
}

func (s *Storage) GetStaleInProgressNodes(threshold time.Duration) ([]ModemQueueEntry, error) {
    return s.queryModemQueue(`
        SELECT * FROM modem_test_queue
        WHERE status = 'in_progress'
          AND in_progress_since < now() - INTERVAL ? SECOND
    `, int(threshold.Seconds()))
}

func (s *Storage) RequeueStaleNode(zone, net, node uint16, cs uint8) error {
    return s.execMutation(`
        ALTER TABLE modem_test_queue
        UPDATE status = 'pending',
               in_progress_since = toDateTime(0),
               retry_count = retry_count + 1,
               next_attempt_after = now() + INTERVAL 5 MINUTE,
               last_error = 'stale: reclaimed after timeout',
               updated_at = now()
        WHERE zone = ? AND net = ? AND node = ? AND conflict_sequence = ?
    `, zone, net, node, cs)
}
```

### Orphaned Node Recovery

Nodes can become orphaned (assigned_to = '') if something goes wrong during reassignment or if a daemon is removed from config. A separate job recovers these nodes.

```go
// Background job to recover orphaned nodes (nodes with empty assigned_to)
func (a *ModemAssigner) RecoverOrphanedNodes() error {
    orphanedNodes, err := a.storage.GetOrphanedNodes()
    if err != nil {
        return fmt.Errorf("failed to get orphaned nodes: %w", err)
    }

    for _, node := range orphanedNodes {
        logging.Info("recovering orphaned node",
            "node", fmt.Sprintf("%d:%d/%d", node.Zone, node.Net, node.Node),
            "old_status", node.Status)

        // Reset status to pending first (orphaned in_progress nodes need this)
        a.storage.ResetNodeStatus(node.Zone, node.Net, node.Node, node.ConflictSequence)

        // Then reassign to an active daemon
        if err := a.ReassignNode(&node); err != nil {
            logging.Warn("failed to recover orphaned node",
                "node", fmt.Sprintf("%d:%d/%d", node.Zone, node.Net, node.Node),
                "error", err)
        }
    }

    return nil
}

// GetOrphanedNodes returns nodes in queue with empty assigned_to
func (s *Storage) GetOrphanedNodes() ([]ModemQueueEntry, error) {
    return s.queryModemQueue(`
        SELECT * FROM modem_test_queue
        WHERE assigned_to = ''
          AND status IN ('pending', 'in_progress')
    `)
}
```

**Note**: This job should run alongside `ReassignOrphanedNodes` and `ReclaimStaleNodes` as part of the periodic queue maintenance.

### New Daemon Coming Online

When a daemon sends its first heartbeat, the server can populate its queue. This happens automatically via the assignment logic which checks for active daemons.

```go
// Called after successful heartbeat from a daemon
func (a *ModemAssigner) OnDaemonHeartbeat(callerID string) error {
    // Update status
    a.storage.UpdateCallerStatus(callerID, "active")

    // Check if this daemon has any assigned nodes
    count, _ := a.storage.CountPendingNodesForCaller(callerID)
    if count > 0 {
        return nil // Already has work
    }

    // Find nodes matching this daemon's prefixes from config
    caller := a.config.GetCaller(callerID)
    if caller == nil {
        return fmt.Errorf("unknown caller_id: %s", callerID)
    }

    // Assign unqueued nodes that match this daemon's prefixes
    return a.AssignMatchingNodes(callerID)
}
```

## Security Considerations

### Authentication

1. **API Key Authentication**: Bearer tokens per daemon, hashed in config.yaml
2. **No Direct DB Access**: Remote daemons never connect to ClickHouse
3. **HTTPS Required**: All API communication over TLS
4. **Config-Based Validation**: API keys validated against config file, not database

### DoS Protection

```go
type RateLimitConfig struct {
    RequestsPerSecond    float64 `yaml:"requests_per_second"`     // 10
    BurstSize            int     `yaml:"burst_size"`              // 20
    MaxRequestsPerMinute int     `yaml:"max_requests_per_minute"` // 600
}

const (
    MaxResultsBodySize   = 1 * 1024 * 1024  // 1 MB (batch results)
    MaxHeartbeatBodySize = 1 * 1024         // 1 KB
    MaxBatchSize         = 100              // Max nodes per request
)
```

### Caller Verification

```go
func (h *ModemHandler) verifyCallerID(r *http.Request) (string, error) {
    apiKey := extractAPIKey(r)
    // Use ValidateAPIKey for consistent hash comparison
    return h.ValidateAPIKey(apiKey)
}
```

## Configuration

### Server Configuration (config.yaml)

See **Data Model > Modem Callers Configuration** section above for the complete server configuration including daemon definitions with prefix routing.

### Modem Daemon Configuration (modem-daemon.yaml)

The daemon configuration is minimal - it only needs credentials and local hardware settings. All prefix routing is configured server-side in config.yaml.

```yaml
# Credentials (must match server's config.yaml)
caller_id: "modem-eu-01"
api_url: "https://nodelistdb.example.com/api/modem"
api_key: "modem-eu-01-secret-key"  # Hashed version in server config

# EMSI session identity
our_address: "2:5001/100"
system_name: "NodelistDB Modem Tester"
sysop: "Test Operator"
location: "Frankfurt, Germany"

# Local modem hardware (daemon decides which to use based on node's flags)
modems:
  - id: "zyxel-1"
    device: "/dev/ttyUSB0"
    init_string: "ATZ"
    capabilities:
      speed_protocols: ["V34", "V32B", "V32", "V22", "V21"]
      error_protocols: ["V42B", "V42", "MNP"]
      proprietary: ["ZYX", "Z19"]
      max_speed: 33600

  - id: "usr-courier"
    device: "/dev/ttyUSB1"
    init_string: "ATZ"
    capabilities:
      speed_protocols: ["V90S", "V34", "V32B", "V32", "V22", "V21"]
      error_protocols: ["V42B", "V42", "MNP"]
      proprietary: ["X2S", "HST", "H16", "H14"]
      max_speed: 56000
      is_digital: true

timeouts:
  dial: 60s
  carrier: 30s
  emsi: 30s

polling:
  interval: 30s
  batch_size: 50

heartbeat:
  interval: 60s
```

## Implementation Phases

### Phase 1: Core Infrastructure
- [ ] Add modem-related columns to `node_test_results` table
- [ ] Create `modem_caller_status` table (runtime status only)
- [ ] Create `modem_test_queue` table with assignment
- [ ] Database migrations
- [ ] Add `ModemAPIConfig` to config loading

### Phase 2: Assignment Logic
- [ ] Implement prefix matching algorithm (reads from config)
- [ ] Implement node assignment on queue entry
- [ ] Handle "no daemon for prefix" case (logging)
- [ ] Queue population job

### Phase 3: API Endpoints
- [ ] Implement `GET /api/modem/nodes` (assigned nodes only)
- [ ] Implement `POST /api/modem/in-progress`
- [ ] Implement `POST /api/modem/results`
- [ ] Implement `POST /api/modem/heartbeat`
- [ ] Implement `POST /api/modem/release`
- [ ] API authentication (Bearer token validated against config)

### Phase 4: Queue Management
- [ ] Daemon offline detection (heartbeat timeout vs config threshold)
- [ ] Node reassignment for offline daemons (both pending AND in_progress)
- [ ] Stale in_progress detection and reclamation
- [ ] Orphaned node recovery (nodes with empty assigned_to)
- [ ] Retry scheduling with backoff
- [ ] Periodic maintenance job orchestration (run all recovery jobs together)

### Phase 5: Modem Daemon
- [ ] Create `cmd/modem-daemon/` binary
- [ ] API client implementation
- [ ] Local modem pool abstraction (hardware access)
- [ ] Local modem capability matching
- [ ] Wrap modem connection as `net.Conn`
- [ ] Configuration file support
- [ ] Logging and metrics

### Phase 6: Integration & UI
- [ ] Web interface for modem test results
- [ ] Analytics for modem connectivity rates
- [ ] Admin UI for modem daemon status (reads config + runtime status)
- [ ] Grafana dashboards (optional)

## Monitoring & Observability

### Metrics to Track

- Queue size per daemon
- Test success/failure rates per daemon
- Average response times
- Nodes pending (unassigned)
- Orphaned nodes count (assigned_to = '')
- Stale in_progress nodes count
- Daemon heartbeat status
- Reassignment counts
- Recovery job execution counts

### Alerts

- Daemon heartbeat missing > 10 minutes
- High test failure rate (> 30%)
- Queue growing faster than processing
- Nodes with no matching daemon (prefix coverage gap)
- Orphaned nodes detected (assigned_to = '') - indicates recovery job failure

## Client Implementation Guide

This section describes how to implement a modem testing client (daemon) that communicates with the NodelistDB server API.

### Overview

The modem daemon is a standalone application that:
1. Authenticates with the server using an API key
2. Fetches assigned nodes to test
3. Performs modem calls and EMSI handshakes
4. Reports results back to the server
5. Sends periodic heartbeats

### Authentication

All API requests require Bearer token authentication:

```
Authorization: Bearer <your-api-key>
```

The API key must be registered in the server's `config.yaml`. Contact the server administrator to obtain your `caller_id` and API key.

### Main Loop

```
┌─────────────────────────────────────────────────────────────┐
│                      DAEMON MAIN LOOP                        │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  1. Start heartbeat goroutine (runs every 60s)              │
│     └─► POST /api/modem/heartbeat                           │
│                                                              │
│  2. Main testing loop:                                       │
│     ┌──────────────────────────────────────────────────┐    │
│     │ a) GET /api/modem/nodes?limit=50                 │    │
│     │    └─► Receive list of assigned nodes            │    │
│     │                                                  │    │
│     │ b) If no nodes: sleep 30s, goto (a)              │    │
│     │                                                  │    │
│     │ c) POST /api/modem/in-progress                   │    │
│     │    └─► Mark nodes as being tested                │    │
│     │                                                  │    │
│     │ d) For each node:                                │    │
│     │    - Select best modem based on node's flags     │    │
│     │    - Dial phone number                           │    │
│     │    - Perform EMSI handshake                      │    │
│     │    - Collect results                             │    │
│     │                                                  │    │
│     │ e) POST /api/modem/results                       │    │
│     │    └─► Submit test results                       │    │
│     │                                                  │    │
│     │ f) Goto (a)                                      │    │
│     └──────────────────────────────────────────────────┘    │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### API Endpoints

#### 1. Get Assigned Nodes

Fetch nodes assigned to your daemon for testing.

```
GET /api/modem/nodes?limit=50&only_callable=true
Authorization: Bearer <api-key>
```

**Query Parameters:**
- `limit` (optional): Maximum nodes to return (default: 50, max: 100)
- `only_callable` (optional): Filter by time availability (default: true)

**Response:**
```json
{
  "nodes": [
    {
      "zone": 2,
      "net": 5020,
      "node": 100,
      "conflict_sequence": 0,
      "phone": "+7-495-123-4567",
      "phone_normalized": "+74951234567",
      "address": "2:5020/100",
      "modem_flags": ["V34", "V32B", "ZYX"],
      "flags": ["CM", "XA", "IBN"],
      "priority": 70,
      "retry_count": 0,
      "is_callable_now": true,
      "next_call_window": null
    }
  ],
  "remaining": 125
}
```

**Important Fields:**
- `modem_flags`: Use these to select the best modem (V34, V32B, HST, ZYX, etc.)
- `is_callable_now`: Server-computed time availability
- `next_call_window`: When node becomes callable (if not now)

#### 2. Mark Nodes In Progress

Mark nodes as being actively tested (prevents timeout reclamation).

```
POST /api/modem/in-progress
Authorization: Bearer <api-key>
Content-Type: application/json

{
  "nodes": [
    {"zone": 2, "net": 5020, "node": 100, "conflict_sequence": 0},
    {"zone": 2, "net": 5020, "node": 101, "conflict_sequence": 0}
  ]
}
```

**Response:**
```json
{
  "marked": 2
}
```

#### 3. Submit Results

Submit test results after completing modem calls.

```
POST /api/modem/results
Authorization: Bearer <api-key>
Content-Type: application/json

{
  "results": [
    {
      "zone": 2,
      "net": 5020,
      "node": 100,
      "conflict_sequence": 0,
      "test_time": "2026-01-15T14:30:00Z",
      "success": true,
      "response_ms": 45000,
      "system_name": "My BBS System",
      "mailer_info": "BinkD 1.1",
      "addresses": ["2:5020/100", "2:5020/100.1"],
      "address_valid": true,
      "response_type": "EMSI_DAT",
      "software_source": "emsi_dat",
      "connect_speed": 33600,
      "modem_protocol": "V.34",
      "phone_dialed": "+7-495-123-4567",
      "ring_count": 3,
      "carrier_time_ms": 8500,
      "modem_used": "zyxel-1",
      "match_reason": "Proprietary match: ZYX"
    },
    {
      "zone": 2,
      "net": 5020,
      "node": 101,
      "conflict_sequence": 0,
      "test_time": "2026-01-15T14:32:00Z",
      "success": false,
      "response_ms": 60000,
      "error": "NO CARRIER after 60s"
    }
  ]
}
```

**Response:**
```json
{
  "accepted": 2,
  "stored": 2
}
```

**Result Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| zone, net, node, conflict_sequence | int | Yes | Node identifier |
| test_time | string | Yes | RFC3339 timestamp |
| success | bool | Yes | Whether EMSI handshake succeeded |
| response_ms | uint32 | Yes | Total time from dial to result |
| error | string | On failure | Error description |
| system_name | string | On success | Remote system name from EMSI |
| mailer_info | string | On success | Mailer software info |
| addresses | []string | On success | FidoNet addresses from EMSI |
| address_valid | bool | On success | Whether expected address was in response |
| response_type | string | On success | EMSI response type (EMSI_DAT, etc.) |
| connect_speed | uint32 | Optional | Modem connect speed (bps) |
| modem_protocol | string | Optional | Negotiated protocol (V.34, V.32bis) |
| phone_dialed | string | Optional | Actual phone number dialed |
| ring_count | uint8 | Optional | Rings before answer |
| carrier_time_ms | uint32 | Optional | Time to carrier detect |
| modem_used | string | Optional | Local modem identifier |
| match_reason | string | Optional | Why this modem was selected |

#### 4. Heartbeat

Send periodic heartbeats to indicate daemon is alive.

```
POST /api/modem/heartbeat
Authorization: Bearer <api-key>
Content-Type: application/json

{
  "status": "active",
  "modems_available": 3,
  "modems_in_use": 1,
  "tests_completed": 45,
  "tests_failed": 3,
  "last_test_time": "2026-01-15T14:32:00Z"
}
```

**Status Values:**
- `active`: Daemon is running and testing
- `inactive`: Daemon is paused
- `maintenance`: Daemon is in maintenance mode

**Response:**
```json
{
  "ack": true,
  "assigned_nodes": 125
}
```

**Important:** Send heartbeats every 60 seconds. If no heartbeat for 10 minutes, the server considers the daemon offline and reassigns its nodes.

#### 5. Release Nodes (Optional)

Release nodes back to queue (e.g., on graceful shutdown or modem failure).

```
POST /api/modem/release
Authorization: Bearer <api-key>
Content-Type: application/json

{
  "nodes": [
    {"zone": 2, "net": 5020, "node": 100, "conflict_sequence": 0}
  ],
  "reason": "modem_failure"
}
```

**Response:**
```json
{
  "released": 1
}
```

### Modem Selection Algorithm

Select the best local modem based on node's `modem_flags`:

```
Priority 1: Proprietary protocol match (same vendor = best performance)
  - ZYX <-> ZYX (Zyxel)
  - HST <-> HST (USR)
  - V90S -> V90C (56K)

Priority 2: Highest common speed protocol
  - V34 (33600 bps)
  - V32B (14400 bps)
  - V32 (9600 bps)

Priority 3: Fallback to any available modem
```

**Example:**
```
Node flags: ["V32B", "V42B", "ZYX", "CM"]
Your modems: [zyxel-1, usr-courier, generic-v34]

Result: Select zyxel-1
Reason: Proprietary match ZYX (Zyxel <-> Zyxel)
```

### Error Handling

| HTTP Status | Meaning | Action |
|-------------|---------|--------|
| 200 | Success | Process response |
| 400 | Bad request | Fix request format |
| 401 | Unauthorized | Check API key |
| 429 | Rate limited | Back off and retry |
| 500 | Server error | Retry with backoff |

### Example Python Client

```python
import requests
import time
from datetime import datetime

class ModemDaemon:
    def __init__(self, api_url, api_key):
        self.api_url = api_url
        self.headers = {"Authorization": f"Bearer {api_key}"}

    def get_nodes(self, limit=50):
        resp = requests.get(
            f"{self.api_url}/nodes",
            params={"limit": limit, "only_callable": "true"},
            headers=self.headers
        )
        resp.raise_for_status()
        return resp.json()["nodes"]

    def mark_in_progress(self, nodes):
        resp = requests.post(
            f"{self.api_url}/in-progress",
            json={"nodes": [
                {"zone": n["zone"], "net": n["net"],
                 "node": n["node"], "conflict_sequence": n["conflict_sequence"]}
                for n in nodes
            ]},
            headers=self.headers
        )
        resp.raise_for_status()
        return resp.json()["marked"]

    def submit_results(self, results):
        resp = requests.post(
            f"{self.api_url}/results",
            json={"results": results},
            headers=self.headers
        )
        resp.raise_for_status()
        return resp.json()

    def heartbeat(self, status="active", modems_available=1, modems_in_use=0):
        resp = requests.post(
            f"{self.api_url}/heartbeat",
            json={
                "status": status,
                "modems_available": modems_available,
                "modems_in_use": modems_in_use,
                "tests_completed": self.tests_completed,
                "tests_failed": self.tests_failed
            },
            headers=self.headers
        )
        resp.raise_for_status()
        return resp.json()

    def test_node(self, node):
        """Override this with actual modem testing logic"""
        # 1. Select best modem based on node["modem_flags"]
        # 2. Dial node["phone"]
        # 3. Perform EMSI handshake
        # 4. Return result dict
        pass

    def run(self):
        while True:
            nodes = self.get_nodes()
            if not nodes:
                time.sleep(30)
                continue

            self.mark_in_progress(nodes)

            results = []
            for node in nodes:
                result = self.test_node(node)
                results.append(result)

            self.submit_results(results)

# Usage
daemon = ModemDaemon(
    api_url="https://nodelistdb.example.com/api/modem",
    api_key="your-api-key-here"
)
daemon.run()
```

### Daemon Configuration Example

```yaml
# modem-daemon.yaml
api:
  url: "https://nodelistdb.example.com/api/modem"
  key: "your-api-key-here"

identity:
  address: "2:5001/100"
  system_name: "NodelistDB Modem Tester"
  sysop: "Test Operator"
  location: "Moscow, Russia"

modems:
  - id: "zyxel-1"
    device: "/dev/ttyUSB0"
    init: "ATZ"
    protocols: ["V34", "V32B", "ZYX"]
    max_speed: 33600

  - id: "usr-courier"
    device: "/dev/ttyUSB1"
    init: "ATZ"
    protocols: ["V90S", "V34", "HST"]
    max_speed: 56000
    is_digital: true

timeouts:
  dial: 60s
  carrier: 30s
  emsi: 30s

polling:
  interval: 30s
  batch_size: 50

heartbeat:
  interval: 60s
```
