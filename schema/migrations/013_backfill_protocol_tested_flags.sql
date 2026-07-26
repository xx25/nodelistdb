-- Migration 013: rebuild the per-protocol tested flag and overall error
--
-- SetIPv4Result/SetIPv6Result set ProtocolTestResult.Tested and .Error only
-- when the probe SUCCEEDED. Aggregated rows escaped that, because
-- finalizeProtocolResult recomputes both from the per-family fields, but a
-- single-hostname node never goes through the aggregator and those rows are
-- most of the table. So a node that was tested and failed on every address was
-- stored exactly like a node whose protocol was never attempted.
--
-- Affected rows at the time of writing (168,932 total):
--
--   binkp   73,397     telnet  12,579     ifcico  7,954
--   ftp      2,960     vmodem   2,026
--
-- This is not only a counting problem. BuildReachabilityStatsQuery treats
-- "NOT <proto>_tested" as "this protocol has nothing to say about IPv6", so a
-- protocol that was tested and failed over IPv6 was silently forgiven and its
-- row counted as FULLY SUCCESSFUL — 1,156 rows on the /reachability page.
-- The binary now records a failed probe as tested, so without this backfill the
-- 90-day trend would show a step change where the two semantics meet rather
-- than a real change in the network.
--
-- The rule follows finalizeProtocolResult: a protocol was tested if either
-- address family was probed, and the overall error is the first family error
-- when nothing succeeded. It is applied STRICTLY ADDITIVELY, because the
-- per-family columns are not the whole truth: 1,182 ftp and 553 vmodem rows
-- from older code paths (SetFTPResult / SetVModemResult, which always set the
-- flag) carry tested=true and a real error with neither family flag set.
-- Recomputing the flag from the family columns alone would erase those. So this
-- only ever turns a flag ON, and only where a family was actually probed.
--
-- node_test_results is a plain MergeTree, so this is an ALTER UPDATE mutation.
-- The table is small (~175k rows, ~18 MiB), and one mutation covering every
-- column costs a single rewrite. Watch it with:
--   SELECT * FROM system.mutations WHERE table='node_test_results' AND is_done=0

ALTER TABLE node_test_results
    UPDATE
        binkp_tested  = binkp_tested  OR binkp_ipv4_tested  OR binkp_ipv6_tested,
        ifcico_tested = ifcico_tested OR ifcico_ipv4_tested OR ifcico_ipv6_tested,
        telnet_tested = telnet_tested OR telnet_ipv4_tested OR telnet_ipv6_tested,
        ftp_tested    = ftp_tested    OR ftp_ipv4_tested    OR ftp_ipv6_tested,
        vmodem_tested = vmodem_tested OR vmodem_ipv4_tested OR vmodem_ipv6_tested,

        binkp_error  = if(binkp_success  OR binkp_error  != '', binkp_error,
                          if(binkp_ipv4_error  != '', binkp_ipv4_error,  binkp_ipv6_error)),
        ifcico_error = if(ifcico_success OR ifcico_error != '', ifcico_error,
                          if(ifcico_ipv4_error != '', ifcico_ipv4_error, ifcico_ipv6_error)),
        telnet_error = if(telnet_success OR telnet_error != '', telnet_error,
                          if(telnet_ipv4_error != '', telnet_ipv4_error, telnet_ipv6_error)),
        ftp_error    = if(ftp_success    OR ftp_error    != '', ftp_error,
                          if(ftp_ipv4_error    != '', ftp_ipv4_error,    ftp_ipv6_error)),
        vmodem_error = if(vmodem_success OR vmodem_error != '', vmodem_error,
                          if(vmodem_ipv4_error != '', vmodem_ipv4_error, vmodem_ipv6_error))
    WHERE
        (NOT binkp_tested  AND (binkp_ipv4_tested  OR binkp_ipv6_tested))
     OR (NOT ifcico_tested AND (ifcico_ipv4_tested OR ifcico_ipv6_tested))
     OR (NOT telnet_tested AND (telnet_ipv4_tested OR telnet_ipv6_tested))
     OR (NOT ftp_tested    AND (ftp_ipv4_tested    OR ftp_ipv6_tested))
     OR (NOT vmodem_tested AND (vmodem_ipv4_tested OR vmodem_ipv6_tested));
