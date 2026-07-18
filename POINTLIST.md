# Pointlist (FTS-5002) Support

NodelistDB imports FidoNet pointlists alongside nodelists. Points live in their
own `points` table — they have their own axes (overlapping publication sources,
per-source cadence) and would otherwise pollute every node-level statistic.

The full peer-reviewed design lives at `~/.claude/plans/pointlist-support.md`;
this document describes what is implemented and how to operate it.

## Status

- **Phase 1 (ingest) — implemented**: schema, boss-family parser
  (boss / poss / pvt formats), import gate, corrected-file replay, parser CLI
  mode, bulk import script, weekly sync hook.
- **Phase 2 (read surfaces) — implemented**: snapshot readers (§ Snapshot
  semantics below), API endpoints, web pages (point page, node-page Points
  section, 4-D search, browse counts, stats tile).
- **Phase 3 (completeness) — implemented** except diffs: combined/V7 `point`
  format, `fakenet` format, inline nodelist `Point` lines +
  `-extract-points` backfill, pointlist file distribution
  (`/pointlists`, `/download/pointlist/...`). Diffs remain out of scope
  (full weeklies cover the corpus).

## Design summary

- **Separate `points` table**, not a `point` column on `nodes`: adding `point`
  to `nodes` would require recreate+swap of a huge production table and every
  node stat would need `point = 0` filtering. Sort key is
  `(domain, zone, net, node, point, pointlist_date, conflict_sequence)` —
  `domain` IS in the sort key (unlike `nodes`, this table was born after
  multi-network support).
- **Store all sources; resolve overlap at query time.** Pointlist sources
  overlap (Z2PNT aggregates the regional lists) and publish at *different day
  numbers* in the same week. Every file's rows are stored verbatim, stamped
  `list_source` (r24, z2, ...) + `source_priority` (0 net-level, 10 regional,
  20 zone rollup). "Current" readers (Phase 2) pick the latest issue per source
  within a 60-day staleness window, then `LIMIT 1 BY (domain,zone,net,node,point)`
  ordered by `source_priority ASC, pointlist_date DESC`.
- **Import gate = `pointlist_files` table** keyed
  `(domain, list_source, pointlist_date)`. The gate row is registered only
  after all point chunks inserted successfully, so a crash mid-import never
  looks like a completed file. Before importing an un-gated file, partial
  remnants for its key are deleted — retries are idempotent.
- **Multi-network ready**: every row carries `domain` (stamped from the
  parser's `-network` flag, default fidonet). Pointlists exist only for
  fidonet today, but nothing assumes that.

## Formats

| Format | Marker | Handling |
|---|---|---|
| boss (§2.1) | `Boss,2:240/2188` + point lines with empty field 1 | imported |
| pvt variant | `Pvt` in field 1 under `Boss,` | imported (same code path) |
| poss (§2.3) | `Point` in field 1 under `Boss,` | imported (same code path) |
| point (§2.2, combined/V7) | full nodelist context + `Point,` lines | imported (context lines set the boss, only `Point,` rows are stored; no Zone line → `-default-zone`) |
| fakenet (§2.4) | `Host,<fakenet>,...` | imported (real boss from address-shaped sysname — POINTS24 — or `UBOSS:230/26` flag — DK-POINT; unresolvable Host blocks skipped + counted) |
| inline nodelist points | `Point,` lines inside a nodelist | imported during nodelist import (list_source `nodelist`, priority 0); `-extract-points` backfills archived files |
| fidouser (§2.5) | `Surname, First ... 2:283/708.74` | skipped by design (redundant with r28 boss series) |
| diffs (`*_D.*`, `*DIFF*`) | NODEDIFF format | never imported as full lists (`DerivePointlistSource` rejects them) |

The keyword vocabulary is format-relative: an empty first field is a POINT
line in the boss family but a NODE (context) line in combined/fakenet files —
which is why the three families get three code paths. The fake/combined
series (POINTS24 → r24, DK-POINT → r23, R##PNT_P → r##) share the region's
`list_source`, so the import gate automatically gap-fills: whichever flavour
of a date is imported first wins and the others are skipped.

Boss lines may carry full nodelist-style extras
(`Boss,2:280/1049,The_Coast,...,INA:thecoastbbs.nl,IBN,CM` — Z2PNT does this);
the extras are tolerated and ignored — the boss itself is a node and lives in
the `nodes` table. Boss addresses with a 4D `.0` suffix (`Boss,2:465/101.0`,
seen in r46 1997) are accepted. Point lines use the full FTS-5000 keyword
vocabulary: empty field, `Point`, `Pvt`, `Down`, `Hold` (case-insensitive) are
all imported as points.

Point flags are parsed with the same machinery as node flags (`CM`, `MO`,
`IBN:host`, `INA:` → `is_cm` / `is_mo` / `has_inet` / `internet_config`).

## Dates and sources

- The issue date comes from the **3-digit day in the filename** plus `-year`
  (compressed weeklies truncate the day: `R24PNT.Z05` holds `R24PNT.005`;
  extract first and feed the inner file). Header `Day number` is the fallback;
  the filename wins on mismatch because some series (r46) ship stale headers.
  Non-Friday dates are logged as warnings. A day number that does not exist in
  the given year (day 366 of a leap year archived under the *next* year's
  directory — real case: `P28-LIST.366` of 2004 in `r28/2005/`) resolves to
  the previous year with a warning.
- `list_source` derives from the **filename family, never the directory** —
  the corpus contains strays (a `R50PNT.293` sits inside `R56/BOSS/`). Known
  families: `R##PNT*` → `r##`, `Z2PNT` → `z2` (priority 20), `PNT46REG` → r46,
  `P28-LIST` → r28, `R45POINT` → r45, `POINT_48` → r48, `POINTR34`/`PTLSTR34`
  → r34. Unknown families are quarantined unless `-list-source` is given.
- Charsets: `-charset cp437` (default) for Western Europe, `cp866` for the
  cyrillic regions (r45, r46, r50); `raw_line` is stored decoded to UTF-8.
- **Same-date variant archives**: the corpus occasionally holds two archives
  whose inner files carry the same series + day (`R24PNT.Z73` official release
  vs `R24PNT0.Z73` pre-release convert; `PNT46R_F.Z07` even smuggles a
  2009-era list under a 2020 inner name). The first one imported wins the
  gate; the other is skipped. C-locale sort order (`LC_ALL=C`, what servers
  use) processes `NAME.Z##` before `NAME0.Z##`/`NAME_F.Z##` variants, which
  prefers the official file. Use `-reimport` with the other file to override
  a wrong pick.

## Importing

```bash
# One file
./bin/parser -config config.yaml -pointlist -year 2024 -charset cp866 \
    -path pnt46reg.012

# Corrected-file replay (deletes that one (domain, source, date) and reimports)
./bin/parser -config config.yaml -pointlist -year 2024 -reimport -path R24PNT.005

# Bulk historical corpus (regional boss series first, then fake/V7
# gap-fillers — shared list_source, the gate skips boss-covered dates —
# z2 last; flock-guarded; extracts .Z##/.L## archives; per-region charset
# map; quarantine report)
bash scripts/import_pointlists.sh -d /path/to/fidohist-pntlist
bash scripts/import_pointlists.sh -n -r '^r24$'   # dry-run one region
```

Pointlist CLI flags: `-pointlist` (mode), `-list-source`, `-source-priority`,
`-format` (auto/boss/poss/pvt/point/fakenet), `-year`, `-default-zone` (2),
`-charset`, `-reimport`, `-force`, `-shrink-check` (fail/warn). `-network`
selects the domain as for nodelists.

```bash
# Backfill inline nodelist Point lines from archived nodelists
# (bypasses the nodelist gate; idempotent via the (domain,'nodelist',date)
# pointlist gate — every scanned file gets a gate row, even with 0 points)
./bin/parser -config config.yaml -extract-points -recursive -path ~/nodelists
```

During normal (sequential) nodelist import, inline `Point,` lines are
extracted automatically and gated the same way; `-concurrent` imports skip
extraction — run `-extract-points` afterwards.

Sanity thresholds (truncation guard): a file yielding 0 points is refused
without `-force`. A file below 50% of the **nearest-dated** already-imported
issue of the same series is refused by default (`-shrink-check=fail`) — right
for the weekly sync, where a truncated download is the realistic risk — or
imported with a warning under `-shrink-check=warn`, which the bulk script
uses because historical series legitimately collapse (r20 went 71 → 8 points
in one week of 2017 when dead entries were purged). (Nearest, not "previous":
bulk import cannot walk compressed weeklies chronologically — the `.Z##`
suffix is day-mod-100.)

Files whose series cannot be derived and no `-list-source` was given are
**quarantined**: counted separately, marked with a greppable `QUARANTINED:`
line, collected into the bulk script's quarantine report, and never imported
silently. The bulk script exits non-zero when any file *failed* (quarantines
alone do not fail the run).

## Ongoing weekly sync

`sync_nodelists.sh` has an `EXTRA_POINTLISTS` array mirroring `EXTRA_NETWORKS`:

```bash
# "name|source|url_or_dir|filename_regex|charset[|network]"
EXTRA_POINTLISTS=("z2|http|http://ambrosia60.goip.de/bbsfiles/pointlist/|z2pnt\.z[0-9]{2}|cp437")
```

Idempotency comes from the `pointlist_files` gate — every matching remote file
is fed to the parser each run and already-imported issues are skipped.
Archived copies land at `<LOCAL_POINTLIST_DIR>/<network>/<source>/<year>/NAME.DDD.gz`.

## Snapshot semantics (read surfaces)

Because sources overlap and publish on different day numbers, every
"current/as-of" reader resolves a **snapshot**: per `(domain, list_source)`
the latest imported issue at or before the as-of date within a **60-day
staleness window** (dead series — r22 ended 2014 — must not resurrect points
into current views), then overlapping rows for the same 4-D address collapse
to the most authoritative source (`source_priority ASC`: net-level <
regional < zone rollup; `pointlist_date DESC` on ties). Implemented as one
query shape in `internal/storage/query_builder_points.go`
(`pointSnapshotInnerSQL`, gate-table driven). A nil as-of anchors at the
domain's newest imported issue, so views stay populated if the sync lags.
History readers skip the snapshot and label every row with its source.

## API

- `GET /api/points?zone=&net=&node=&point=&sysop_name=&list_source=&latest_only=&domain=...` — search
  (`latest_only=true` = snapshot; else full history, newest first).
- `GET /api/nodes/{zone}/{net}/{node}/points[?date=]` — snapshot points under a boss.
- `GET /api/points/{zone}/{net}/{node}/{point}` — current snapshot entry
  (`X-Available-Domains` header when the address exists in several networks).
- `GET /api/points/{zone}/{net}/{node}/{point}/history` — all sources, labeled.
- `GET /api/pointlists/dates[?source=]`, `GET /api/pointlists/sources` — gate-table metadata.

Domain resolution consults the **points table itself** (`resolvePointDomain`):
resolving via the boss node could 404 a point that exists only in a network
the node-level heuristic does not prefer.

## Web

- **Point page** `/points/{zone}/{net}/{node}/{point}`: latest entry + full
  labeled history + raw lines.
- **Node page**: a "Points" section (snapshot under the boss) when non-empty.
- **Search**: a 4-D address (`2:5001/100.7`) routes to the point page instead
  of silently dropping the point suffix. Every search also runs against the
  points table, shown (when non-empty) as a
  separately-labeled Points result section driven by the same form criteria
  (mirroring the node search's branch precedence: sysop mode > full address >
  individual fields), lifetime-aggregated per 4-D address with an
  Active/Historical badge. History-shaped point queries only see fully
  imported issues (gate-table predicate), like the snapshot path.
  Because this query now fires on every search, it is defended on four
  fronts: text predicates are written as `lowerUTF8(col) LIKE
  lowerUTF8(pattern)` (NOT `ILIKE`, which skip indexes never accelerate)
  against ngrambf_v1 indexes over the same expressions (migration 010); the
  lifetime SQL is two-phase (an inner DISTINCT picks the first LIMIT
  identities in primary-key order, so a bare-zone search does not GROUP BY
  millions of rows — 13.5s → 1.0s measured); text terms under 3 bytes skip
  the points search entirely (`pointTextSearchMinLen` — below the trigram
  floor they would full-scan, and dropping the term instead would show
  points that ignore it); and the query is cancelled with the request
  context if the client disconnects.
- **Browse net page**: per-boss point-count column (explicit `?date=` =
  snapshot as of that date; the current view anchors at the newest imported
  pointlist so a lagging pointlist feed does not blank the column).
- **Stats**: pointlist tile (total points, bosses, per-zone) — same
  anchoring rule as browse; the tile itself states its as-of date.
- **Downloads**: `/pointlists` index + `/pointlists/{network}/{source}/{year}`
  listings + `/download/pointlist/{network}/{source}/{year}/{file}` (gzip
  decompressed on the fly), scanning
  `<nodelist_root>/pointlists/<network>/<source>/<year>/` (`POINTLIST_PATH`
  env overrides the root) — the layout `sync_nodelists.sh` archives into.
  Pointlist URLs are included in `/download/urls.txt`. FTP distribution =
  config mount only (add the pointlist directory to `ftp.mounts`).

## Deployment

Run `schema/migrations/009_pointlists.sql` on production ClickHouse before (or
after — the migration is purely additive, `CREATE TABLE IF NOT EXISTS` only)
deploying binaries. `CreateSchema()` also creates both tables automatically on
parser startup. No existing table or query is touched.

## Explicit non-goals

- Testdaemon testing of points (some carry IBN/INA hosts — future idea).
- FidoUser import; fakenet header mapping-table parsing.
- Feeding pointlist flags into `flag_statistics`.
- Any change to `nodes` schema, node stats, sysop pages, or testdaemon.
- NODEDIFF application (full weeklies cover the corpus).
