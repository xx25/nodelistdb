# Multi-Network (FTN Domain) Support — Implementation Plan

## Context

NodelistDB currently assumes FidoNet-only. Other FTN networks exist (fsxNet — zone 21, nodelist `FSXNET.191`; also tqwNet, araknet, retronet, etc.). The sibling project fidomail is already multi-network ("domain-first": the network name is an explicit string, never inferred from zone — zone numbers are reused across FTN networks, so zone can never discriminate).

A physical node can hold AKAs in several networks at once. The test daemon already receives the remote's **full AKA list** during BinkP (`M_ADR`) and IFCICO (EMSI_DAT) handshakes and stores it in `binkp_addresses`/`ifcico_addresses`. So one successful test can cover the same host's entries in other networks — no retesting needed.

**Design decisions (confirmed):**
1. Store **derived test-result rows** for AKA-matched entries in other networks (marked via a new `derived_from_address` column), so those nodes get visible history/reachability without being retested.
2. Skip retesting **only when the other entry's hostname set overlaps** the tested one; entries advertising different hostnames get tested independently (a dead per-network hostname must not look operational).
3. **Full web UI support**: network selector in search/browse, per-network stats, domain shown on node/reachability pages, per-network nodelist downloads/FTP.

**Two hard collisions today:**
- Import gate `IsNodelistProcessed` = `SELECT COUNT(*) FROM nodes WHERE nodelist_date = ?` (`internal/storage/query_builder_dates.go:10`) — once any network's nodelist for a date is imported, all other networks' nodelists for that same date are silently skipped as "already processed".
- "Latest nodelist" everywhere = global `MAX(nodelist_date)` — fsxNet is weekly, FidoNet daily, so fsxNet would never be "latest" and would vanish from stats/browse/daemon node selection.

*This plan was peer-reviewed (blind two-AI debate, findings verified against the code); all confirmed findings are folded in.*

## Design principles

- New dimension `domain` — lowercase network name (`fidonet`, `fsxnet`, …) — added to `nodes`, `node_test_results`, `flag_statistics`. Existing prod rows default to `'fidonet'`. Terminology matches fidomail's `FTNAddress.Domain`.
- **No zone→domain mapping** (fidomail's deliberate choice): a row's domain comes from which network's nodelist it was parsed from; announced AKAs are matched by 3D key (`zone:net/node`) across all domains, constrained by an `@domain` suffix only when the AKA carries one.
- Per-network configuration (name + nodelist filename pattern), modeled on fidomail's `NetworkConfig{Network, Dir, FilenamePattern}`.
- `ORDER BY` unchanged for `nodes` / `node_test_results` (ClickHouse `MODIFY ORDER BY` cannot append a defaulted column; MergeTree doesn't enforce uniqueness anyway — dedup is app-level; prod `PARTITION BY zone` keeps fsxNet (zone 21) in its own partitions for pruning). **Exception:** `flag_statistics` is ReplacingMergeTree — `domain` MUST be added to its ORDER BY or background merges silently collapse two networks' rows into one (see Phase 1).
- Project convention (lesson from migration 004): new columns are appended at the **end** of INSERT column lists to avoid position mismatches.

## Phase 1 — Schema (all THREE DDL sources) & core model

DDL is defined in three places; all must change identically:
- `schema/clickhouse_schema.sql` (static reference)
- `internal/database/clickhouse.go` `CreateSchema()` (~line 189 — used by `make init-db`, fresh installs)
- `internal/testing/storage/clickhouse_schema.go` `initSchema()` (~line 12 — used by testdaemon / test containers)

1. **`schema/migrations/008_multi_network.sql`** (005 is already used twice; latest applied is 007):
   - `ALTER TABLE nodes ADD COLUMN domain LowCardinality(String) DEFAULT 'fidonet'`
   - `ALTER TABLE node_test_results ADD COLUMN domain LowCardinality(String) DEFAULT 'fidonet', ADD COLUMN derived_from_address String DEFAULT ''`
   - `flag_statistics`: **recreate + swap** (same pattern as `migrations/001_repartition_nodes_by_zone.sql`) with `ORDER BY (flag, year, nodelist_date, domain)` — a bare ADD COLUMN would let ReplacingMergeTree merges keep only one domain's row per (flag, year, date).
   - `ALTER ADD COLUMN ... DEFAULT` is metadata-only on MergeTree — safe on prod-size tables; old binaries keep working (explicit insert column lists + column defaults).
2. Update all three DDL sources with the new columns (and the new `flag_statistics` sort key). Also fix pre-existing drift: `clickhouse_schema.sql` says `PARTITION BY toYYYYMM(nodelist_date)` for `nodes` while prod is `PARTITION BY zone` (migration 001) — align the static file while touching it.
3. `pstn_dead_nodes` (migration 007, ReplacingMergeTree ORDER BY zone,net,node without domain): explicitly **N/A for now** — fsxNet entries are IP-only; PSTN modem testing stays fidonet-scoped. Revisit if a PSTN-bearing network is ever added.
4. **`internal/database/models.go`**: add `Node.Domain string`; `NodeFilter.Domain *string`; `ComputeFtsId()` appends `@domain` **only when domain != 'fidonet'** — `"z:n/n@date#seq"` stays valid for all existing rows, so no backfill and no mixed-format ambiguity (`-rebuild-fts` only rebuilds indexes; it does not rewrite column values). fts_id parsing handles the optional suffix.

## Phase 2 — Config & parser

5. **`internal/config/config.go`** — new section:
   ```yaml
   networks:
     - name: fidonet          # default entry, injected if section absent
       nodelist_pattern: '(?i)^nodelist\.(\d{3})$'   # .gz handled by existing trim
     - name: fsxnet
       nodelist_pattern: '(?i)^fsxnet\.(\d{3})$'
   ```
   `[]NetworkConfig{Name, NodelistPattern, Path(optional)}`; validation: lowercase unique names, compilable regex.
6. **`cmd/parser/main.go`**: add `-network <name>` flag (default `fidonet`); `isNodelistFile` (line ~388) uses the selected network's compiled pattern instead of the hardcoded `nodelist` prefix; stamp `Node.Domain` on every parsed node; thread domain through `internal/concurrent/multi_processor.go` (interfaces at lines 33/41).
7. **`internal/parser/parser_date.go`**: add a generic fallback filename pattern `\.(\d{3})$` for day-number extraction (header parsing already handles fsxNet's header — verified against `FSXNET.191`).
8. **Import gate**: `IsNodelistProcessed(domain, date)` → `WHERE domain = ? AND nodelist_date = ?`; thread the new parameter through `query_builder_dates.go:9`, `node_operations.go:208`, `storage.go:126`, `cached_storage_stats.go:182`, `concurrent/multi_processor.go`, `cmd/parser/main.go:247`.
9. **Conflict handling per domain**: add `AND domain = ?` to `ConflictCheckSQL`/`MarkConflictSQL` (`query_builder_nodes.go:534,541`) — otherwise the same 3D address appearing in two networks on the same date is falsely marked `has_conflict`.

## Phase 3 — Storage / query layer

10. **Insert paths**: append `domain` at the end of column lists + bindings in `query_builder_nodes.go` (~150, ~233, ~260), `result_parser.go` scan targets, **and the modem direct-submit path `internal/storage/modem_result_operations.go:119`** (a third `node_test_results` writer — PSTN results stay `'fidonet'` explicitly, but the column must be in the list once inserts name it).
11. **Per-domain "latest"** — replace every global `MAX(nodelist_date)`:
    - Domain-scoped queries: `(SELECT MAX(nodelist_date) FROM nodes WHERE domain = ?)`.
    - Cross-domain queries: `JOIN (SELECT domain, MAX(nodelist_date) AS latest FROM nodes GROUP BY domain) l ON n.domain = l.domain AND n.nodelist_date = l.latest` (ClickHouse has no correlated subqueries).
    - Verified sites: `query_builder_dates.go:15,30`; `query_builder_nodes.go:428,552,584,625,653`; **`BuildNodesQuery` LatestOnly branch `query_builder_nodes.go:270-292`** (inner `GROUP BY zone,net,node` must become `GROUP BY domain,zone,net,node` and the IN-tuple must include domain — otherwise a domain-filtered "latest" query can return zero rows for a 3D address existing in both domains with different dates); `analytics_operations.go:543,632,738`; `query_builder_analytics.go:131`; `internal/testing/storage/clickhouse_nodes.go:26,56,82,102`.
12. **Other date helpers** in `query_builder_dates.go` (`Available`, `Exists`, `NearestBefore/After`, `ConsecutiveCheck`, `NextDate`): add domain scoping — otherwise fidonet's dense daily dates leak into fsxNet's weekly date-picker/browse-by-date views.
13. **`UpdateFlagStatistics`** (`analytics_operations.go:222-370`): make fully domain-aware — the `flag_first_appearance` and `total_nodes_per_year` CTEs currently scan all of `nodes` with no domain predicate; group and insert per domain (writes the new `flag_statistics.domain` column).
14. **Node-identity queries**: domain predicate/param in `NodeHistorySQL` (:513), `NodeDateRangeSQL` (:526), first/last-date (:528), sysop window functions `PARTITION BY zone, net, node` → `PARTITION BY domain, zone, net, node` (:559, :591), and `buildWhereConditions` (:452) honoring `NodeFilter.Domain`.
15. **Cached storage** (`cached_storage*.go`): update pass-through signatures; cache keys gain a domain component wherever the underlying query became domain-scoped.
16. **Stats**: `GetStats`/`NetworkStats` gain a domain dimension (zone distribution etc. computed within a domain).

## Phase 4 — API & Web (full UI scope)

17. **API**:
    - New `GET /api/networks`: list domains with latest nodelist date + node count.
    - `?domain=` filter on `/api/nodes`, `/api/nodes/pstn`, `/api/stats`, sysop endpoints.
    - `/api/nodes/{zone}/{net}/{node}` (+ history/changes/timeline): optional `?domain=`; when omitted — if the 3D address exists in exactly one domain use it, else prefer `fidonet` and include `available_domains` in the response. Existing URLs keep working. Document in Swagger.
18. **Web** (`internal/web/`):
    - `/browse`: top level lists networks (name, node count, latest date); zone/region/net handlers accept `?domain=` (default `fidonet` → old URLs unchanged).
    - `/search`: domain dropdown. Node history & reachability pages: domain badge; derived results labeled "via AKA <address>".
    - `/stats`: per-network sections or selector.
    - Replace data-describing hardcoded "FidoNet" strings (`handlers_analytics.go`, `handlers.go:158`, …); site branding stays.
19. **Nodelist file distribution**:
    - Layout: fidonet stays at `<root>/<year>/` (backward compat); new networks at `<root>/<network>/<year>/`.
    - `internal/api/handlers_nodelist.go:98`: per-network filename pattern instead of the hardcoded `nodelist.` prefix; download routes gain a network path segment for non-fidonet.
    - **`internal/web/nodelist_download.go:56-83` `scanNodelistDirectory`**: currently only accepts 4-digit year dirs directly under root — make it network-aware so `<root>/fsxnet/` isn't silently skipped.
    - FTP: one mount per network (existing `mounts` config already supports this — config-only, no code change).

## Phase 5 — Testdaemon: multi-network + cross-network AKA dedup (core requirement)

20. **Node source** (`internal/testing/storage/clickhouse_nodes.go`): select `domain`; per-domain latest date (JOIN pattern from step 11); `internal/testing/models/node.go` gains `Domain`; `GetNodeTestHistory`/`GetNodesByZone` gain domain params.
21. **Scheduler identity**: `nodeKey` = `"zone:net/node@domain"` in `scheduler_utils.go:11` **and** in `ResetNodeSchedule` (`scheduler_utils.go:60`), which independently rebuilds the key today — route both through the single `nodeKey()` helper, or schedule resets for non-fidonet nodes silently no-op.
22. **AKA equivalence index** (new, e.g. `internal/testing/daemon/aka_equiv.go`): in-memory map linking schedule entries across domains that belong to one physical host.
    - Seeded at startup from (a) recent `binkp_addresses`/`ifcico_addresses` arrays in test history and (b) identical-hostname-set matching across domains in the current nodelists (covers cold start with no history).
    - Updated after every direct test from the announced AKA union.
    - **Cycle-level guard** (fixes a cold-start race): `runTestCycle` (`daemon_testing.go:31-118`) snapshots all due nodes up front and dispatches concurrently, so two AKA-linked entries both due (guaranteed on cold start) would both direct-test before either's derivation runs. After the snapshot, collapse each equivalence group to one representative (prefer the stalest); deferred members are covered by derivation or the next cycle.
23. **AKA union — collect from per-hostname results, not the aggregated object**: `CreateAggregatedResult` (`test_aggregator.go`) keeps only the **first** successful protocol result per protocol, so later hostnames' announced AKAs are dropped. The dedup step must union `Addresses` across **all** per-hostname protocol results of the test (plus the aggregated one) before matching.
24. **Derivation flow** (new `internal/testing/daemon/aka_dedup.go`), triggered after each successful direct test:
    - Parse each announced AKA as `zone:net/node[.point][@domain]` (skip points; keep `@domain` — regex precedent in `internal/storage/test_other_networks.go:84`; note `binkp/session.go:387` normalization strips domains, so parse from the raw arrays).
    - For each AKA ≠ tested address: find schedule entries in **other** domains matching the 3D key; if the AKA carried `@domain`, constrain to that domain (fidomail's matching rule).
    - **Hostname-overlap gate**: other entry's `InternetHostnames` ∩ tested entry's hostnames ≠ ∅ (case-insensitive). No overlap → leave it on its own schedule.
    - **Derived result**: deep-copy the aggregated result; override `Zone/Net/Node/Domain/Address` to the other entry's identity; recompute `AddressValidated` (+ `AddressValidatedIPv4/IPv6`) by membership of the other address in the announced per-version AKA lists (same semantics as existing `ValidateAddress`); set `DerivedFromAddress` = tested node's address; store via the normal batch path.
    - Update the other entry's schedule: `LastTestTime = now`, `NextTestTime = now + TestInterval`, `TestReason = "aka_derived"`.
    - **No chains**: derived results never trigger further derivation.
    - Restart-safe for free: `InitializeSchedules` seeds `LastTestTime` from `GetNodeTestHistory`, which includes derived rows stored under the other entry's identity.
    - Steady-state alternation: A's test derives B and pushes B out one interval; when B later comes due first, its direct test derives A — one direct test per host per interval, and each network's hostnames still get exercised over time.
25. **Models/persistence**: `internal/testing/models/test_result.go` gains `Domain`, `DerivedFromAddress`; `internal/testing/storage/clickhouse_tests.go` appends both columns at the **end** of the insert list (`flushBatch`, `resultToValues`).
26. **CLI paths**: `-test-node`/`-test-limit` accept an optional `@domain` suffix (default fidonet).

## Phase 6 — Analytics adjustments

27. Test-result analytics (`test_operations.go`, `test_aka_mismatch.go`, protocol/IPv6 analytics): carry `domain` in per-node grouping/joins; reachability pages filter by domain and label derived rows.
    - **Counting convention (state explicitly in code/docs)**: analytics count per FTN node identity, not per physical host — derived rows are intentionally counted in their own domain, consistent with the existing CLAUDE.md guidance on `is_aggregated`/multi-hostname counting.
28. **Other Networks report** (`test_other_networks.go`): annotate each detected network with whether its nodelist is now imported (join against `SELECT DISTINCT domain FROM nodes`) — turns the report into a coverage view.
29. `timeavail` ZMH defaults: unchanged (zone-2 fallback for unknown zones is acceptable; fsxNet is IP-only). Note only.

## Phase 7 — Sync script & rollout

30. **`sync_nodelists.sh`**: generalize to an array of sources `{network, source (ftp/http), remote filename regex, local subdir}`; decompress `.zNN`/`.zip` archive variants; invoke parser with `-network <name>` per source. Fidonet flow byte-for-byte unchanged. (Rebase on the uncommitted whitespace/remote-sync edits already in the working tree.)
31. **Rollout order**:
    1. Run migration 008 on prod ClickHouse (10.121.17.211) — includes the `flag_statistics` recreate+swap; old binaries unaffected (defaults + explicit column lists).
    2. Deploy parser + server + testdaemon (`bash deploy.sh`).
    3. Import fsxNet: `./bin/parser -config config.yaml -network fsxnet -path .../FSXNET.191`.
    4. Verify (below). No FTS backfill needed — fidonet keeps the old fts_id format by design.

## Verification

- `make test` + new unit tests: parser stamps domain; per-domain `IsNodelistProcessed`; conflict check domain-scoped; AKA parsing (with/without `@domain`, points); hostname-overlap gate; AKA union across per-hostname results; derived-result construction (identity overridden, `address_validated` recomputed, `derived_from_address` set); equivalence-group collapse within a test cycle; `ResetNodeSchedule` with domain key.
- Local end-to-end: `make init-db` (exercises the updated `CreateSchema()`); import `test_nodelists/` as fidonet + `FSXNET.191` as fsxnet; both dates coexist (import gate no longer collides); browse/search/API with and without `?domain=`; `/api/networks` lists both; date-pickers show only the domain's own dates.
- Daemon: `./bin/testdaemon -config config.yaml -once -test-limit "2:5001/100"` (project rule: test only 2:5001/100) — normal path unaffected; AKA-dedup via integration tests with fake announced-AKA fixtures (including the cold-start both-due scenario).
- Prod after import: zone-21 nodes appear under fsxnet, fsxNet latest date correct, flag stats per-domain, no fidonet regressions in stats/analytics.

## Risks / notes

- `domain` is not in `nodes`/`node_test_results` sort keys on prod → domain-filtered scans rely on zone partitioning; acceptable (fsxNet = partition 21). Fresh installs keep identical sort keys to avoid drift.
- The same 3D address in two domains is now legal — every "node key" string (`zone:net/node`) used in maps/URLs must include domain or stay domain-scoped (scheduler key, fts_id, API disambiguation are covered above; watch for others during implementation).
- Brand-new AKA pairs with no test history and disjoint hostnames still direct-test twice once (that's how equivalence is first learned) — by design.
- API consumers not passing `?domain=` get fidonet-preferred behavior — documented in Swagger.
