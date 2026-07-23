-- Migration 011: recover INA hostnames dropped by the parser
--
-- Until this release the parser stored INA in a map[string]string, so a
-- nodelist line carrying the flag more than once
-- (",113,...,IBN,INA:horris.now.im,INA:horris.privatedns.org,...") kept only
-- the LAST value. The testdaemon builds its hostname list from
-- internet_config, so those nodes were only ever tested on one of their
-- addresses, and the node page could not show the others either.
--
-- 42 nodes are affected across 53,546 rows (2013-01-16 onwards, zones 1 and 2).
-- New imports are already correct once the new parser binary is deployed; this
-- backfill repairs the rows already stored, and also keeps the affected nodes
-- from showing a one-off spurious "inet_INA changed" entry in their history on
-- the first nodelist imported after the deploy.
--
-- The rewrite only reshapes defaults.INA - every other key is untouched. The
-- values come from raw_line, which stores the original nodelist line verbatim,
-- in line order with exact repeats dropped (matching the parser's own dedup).
-- The scalar form "INA":"host" occurs exactly once in each affected document
-- (protocol details are objects, never a bare string), so replaceRegexpOne
-- cannot hit anything else.
--
-- Verified read-only over all 53,546 matching rows before running:
--   * every row has a scalar "INA":"..." to replace (no silent no-ops)
--   * every rewritten document is valid JSON
--   * the resulting array length equals the number of INA values in raw_line
--   * no INA value contains a quote or space
--
-- IEM (the default email) went through the same overwriting map and so had the
-- same latent bug, but no row in either nodes or points has ever carried the
-- flag twice, so there is nothing to repair and this migration is INA-only:
--   SELECT count() FROM nodelistdb.nodes  WHERE length(extractAll(raw_line, 'IEM:')) > 1;  -- 0
--   SELECT count() FROM nodelistdb.points WHERE length(extractAll(raw_line, 'IEM:')) > 1;  -- 0
--
-- The points table has 9 rows with repeated INA. Point history collapses
-- unchanged periods by comparing configs semantically (not byte-wise), so those
-- rows self-correct on the next pointlist import and need no backfill.
--
-- Optional and re-runnable: the WHERE clause requires the scalar form, so a
-- second run matches nothing. Old binaries keep working - they read
-- defaults.INA as a string and simply ignore rows where it is now a list;
-- deploy the new binaries first if that matters.
--
-- This is a mutation over a 31.5M row table (partitioned by zone, so only the
-- zone 1 and 2 partitions are rewritten). Watch it with:
--   SELECT * FROM system.mutations WHERE table = 'nodes' AND NOT is_done;

ALTER TABLE nodelistdb.nodes
UPDATE internet_config = CAST(
    replaceRegexpOne(
        toString(internet_config),
        '"INA":"[^"]*"',
        concat('"INA":["', arrayStringConcat(arrayDistinct(extractAll(raw_line, 'INA:([^,\r]+)')), '","'), '"]')
    ) AS JSON)
WHERE length(arrayDistinct(extractAll(raw_line, 'INA:([^,\r]+)'))) > 1
  AND match(toString(internet_config), '"INA":"[^"]*"');

-- Expected afterwards: 0 rows.
--   SELECT count() FROM nodelistdb.nodes
--   WHERE length(arrayDistinct(extractAll(raw_line, 'INA:([^,\r]+)'))) > 1
--     AND match(toString(internet_config), '"INA":"[^"]*"');
