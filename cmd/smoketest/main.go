// Command smoketest exercises every storage query against a live ClickHouse
// database and reports failures. Used to validate SQL changes end-to-end
// before/after deployments; it performs reads only.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/storage"
)

var failures int

func check(name string, err error) {
	if err != nil {
		failures++
		fmt.Printf("FAIL %-45s %v\n", name, err)
	} else {
		fmt.Printf("ok   %s\n", name)
	}
}

func main() {
	db, err := database.NewClickHouse(&database.ClickHouseConfig{
		Host: "localhost", Port: 9000, Database: "nodelistdb", Username: "default",
		MaxOpenConns: 4, MaxIdleConns: 2,
		DialTimeout: 10 * time.Second, ReadTimeout: 5 * time.Minute, WriteTimeout: time.Minute,
	})
	if err != nil {
		fmt.Println("connect:", err)
		os.Exit(1)
	}
	defer db.Close()

	s, err := storage.New(db)
	if err != nil {
		fmt.Println("storage:", err)
		os.Exit(1)
	}

	// A diagnostic run has no request behind it and no reason to be cut
	// short, so every read here gets the background context.
	ctx := context.Background()

	// --- Node operations
	latestOnly := true
	zone := 2
	fsx := "fsxnet"
	_, err = s.GetNodes(ctx, database.NodeFilter{Zone: &zone, LatestOnly: &latestOnly, Limit: 3})
	check("GetNodes latest fidonet zone 2", err)
	_, err = s.GetNodes(ctx, database.NodeFilter{Domain: &fsx, LatestOnly: &latestOnly, Limit: 3})
	check("GetNodes latest fsxnet", err)
	z21 := 21
	_, err = s.GetNodes(ctx, database.NodeFilter{Zone: &z21, Domain: &fsx, Limit: 3})
	check("GetNodes historical fsxnet", err)
	_, err = s.GetNodeHistory(ctx, 2, 5001, 100, "")
	check("GetNodeHistory all domains", err)
	_, err = s.GetNodeHistory(ctx, 21, 1, 100, "fsxnet")
	check("GetNodeHistory fsxnet", err)
	_, _, err = s.GetNodeDateRange(ctx, 2, 5001, 100, "fidonet")
	check("GetNodeDateRange", err)
	_, err = s.NodeOps().GetNodeDomains(ctx, 21, 1, 100)
	check("GetNodeDomains", err)
	_, err = s.GetDomains(ctx)
	check("GetDomains", err)
	_, err = s.GetMaxNodelistDate(ctx, "fsxnet")
	check("GetMaxNodelistDate fsxnet", err)
	_, err = s.IsNodelistProcessed(time.Now(), "fsxnet")
	check("IsNodelistProcessed", err)
	_, err = s.NodeOps().CountNodes(ctx, time.Time{}, "fsxnet")
	check("CountNodes fsxnet", err)
	_, err = s.FindConflictingNode(2, 5001, 100, time.Now(), "fidonet")
	check("FindConflictingNode", err)

	// --- Search operations
	_, err = s.SearchNodesBySysop(ctx, "Dmitry", 5, "")
	check("SearchNodesBySysop all", err)
	_, err = s.SearchNodesBySysop(ctx, "Dmitry", 5, "fidonet")
	check("SearchNodesBySysop fidonet", err)
	_, err = s.SearchNodesWithLifetime(ctx, database.NodeFilter{Zone: &z21, Domain: &fsx, Limit: 3})
	check("SearchNodesWithLifetime fsxnet", err)
	_, err = s.GetUniqueSysops(ctx, "", 5, 0)
	check("GetUniqueSysops", err)
	_, err = s.GetNodeChanges(ctx, 21, 1, 100, "fsxnet")
	check("GetNodeChanges fsxnet", err)
	_, err = s.GetPioneersByRegion(ctx, 2, 50, 3, "")
	check("GetPioneersByRegion", err)

	// --- Stats / browse
	latest, err := s.GetLatestStatsDate(ctx, "fidonet")
	check("GetLatestStatsDate", err)
	_, err = s.GetStats(ctx, latest, "fidonet")
	check("GetStats fidonet", err)
	fsxLatest, err := s.GetLatestStatsDate(ctx, "fsxnet")
	check("GetLatestStatsDate fsxnet", err)
	_, err = s.GetStats(ctx, fsxLatest, "fsxnet")
	check("GetStats fsxnet", err)
	_, err = s.GetAvailableDates(ctx, "fsxnet")
	check("GetAvailableDates fsxnet", err)
	_, err = s.GetNearestAvailableDate(ctx, time.Now(), "fsxnet")
	check("GetNearestAvailableDate fsxnet", err)
	_, err = s.GetNodeCountHistory(ctx, "fsxnet")
	check("GetNodeCountHistory fsxnet", err)
	_, err = s.GetBrowseZones(ctx, fsxLatest, "fsxnet")
	check("GetBrowseZones fsxnet", err)
	_, err = s.GetBrowseRegions(ctx, fsxLatest, 21, "fsxnet")
	check("GetBrowseRegions fsxnet", err)
	_, err = s.GetBrowseNets(ctx, fsxLatest, 21, 0, "fsxnet")
	check("GetBrowseNets fsxnet", err)
	_, err = s.GetBrowseNodes(ctx, fsxLatest, 21, 1, "fsxnet")
	check("GetBrowseNodes fsxnet", err)

	// --- Analytics (nodes table)
	_, err = s.GetFlagFirstAppearance(ctx, "CM", "fidonet")
	check("GetFlagFirstAppearance", err)
	_, err = s.GetFlagUsageByYear(ctx, "CM", "fidonet")
	check("GetFlagUsageByYear", err)
	_, err = s.GetNetworkHistory(ctx, 2, 5001, "fidonet")
	check("GetNetworkHistory", err)
	_, err = s.GetPSTNNodes(ctx, 5, 0, "fidonet")
	check("GetPSTNNodes", err)
	_, err = s.GetPSTNCMNodes(ctx, 5)
	check("GetPSTNCMNodes", err)
	_, err = s.GetFileRequestNodes(ctx, 5, "")
	check("GetFileRequestNodes", err)
	_, err = s.GetEmailCapableNodes(ctx, 5, false, "")
	check("GetEmailCapableNodes", err)
	_, err = s.GetEmailFlagTrend(ctx, "")
	check("GetEmailFlagTrend", err)
	_, err = s.GetOnThisDayNodes(ctx, 7, 17, 5, false, "")
	check("GetOnThisDayNodes", err)

	// --- Test results / reachability
	_, err = s.GetNodeTestHistory(ctx, 2, 5001, 100, 7, "")
	check("GetNodeTestHistory all", err)
	_, err = s.GetNodeTestHistory(ctx, 2, 5001, 100, 7, "fidonet")
	check("GetNodeTestHistory fidonet", err)
	_, err = s.GetNodeReachabilityStats(ctx, 2, 5001, 100, 7, "")
	check("GetNodeReachabilityStats", err)
	_, err = s.GetDetailedTestResult(ctx, 2, 5001, 100, "2026-07-16 00:00:00", "")
	check("GetDetailedTestResult", err)
	_, err = s.GetReachabilityTrends(ctx, 90, "")
	check("GetReachabilityTrends", err)
	_, err = s.GetReachabilityTrendsAllTime(ctx, "")
	check("GetReachabilityTrendsAllTime", err)
	_, err = s.SearchNodesByReachability(ctx, true, 5, 7, "")
	check("SearchNodesByReachability", err)

	// --- Protocol / IPv6 analytics
	type f func(context.Context, int, int, bool, string) ([]storage.NodeTestResult, error)
	for name, fn := range map[string]f{
		"GetBinkPEnabledNodes":           s.GetBinkPEnabledNodes,
		"GetIfcicoEnabledNodes":          s.GetIfcicoEnabledNodes,
		"GetTelnetEnabledNodes":          s.GetTelnetEnabledNodes,
		"GetVModemEnabledNodes":          s.GetVModemEnabledNodes,
		"GetFTPEnabledNodes":             s.GetFTPEnabledNodes,
		"GetIPv6EnabledNodes":            s.GetIPv6EnabledNodes,
		"GetIPv6NonWorkingNodes":         s.GetIPv6NonWorkingNodes,
		"GetIPv6AdvertisedIPv4OnlyNodes": s.GetIPv6AdvertisedIPv4OnlyNodes,
		"GetIPv6OnlyNodes":               s.GetIPv6OnlyNodes,
		"GetPureIPv6OnlyNodes":           s.GetPureIPv6OnlyNodes,
		"GetAKAMismatchNodes":            s.GetAKAMismatchNodes,
	} {
		_, err = fn(ctx, 5, 7, false, "")
		check(name, err)
	}
	_, err = s.GetIPv6NodeList(ctx, 5, 7, false, "")
	check("GetIPv6NodeList", err)
	_, err = s.GetIPv6WeeklyNews(ctx, 5, false, "")
	check("GetIPv6WeeklyNews", err)
	_, err = s.GetIPv6IncorrectIPv4CorrectNodes(ctx, 5, 7, false, "")
	check("GetIPv6IncorrectIPv4CorrectNodes", err)
	_, err = s.GetIPv4IncorrectIPv6CorrectNodes(ctx, 5, 7, false, "")
	check("GetIPv4IncorrectIPv6CorrectNodes", err)

	// --- Other networks / geo / software / whois / modem
	_, err = s.GetOtherNetworksSummary(ctx, 7, "")
	check("GetOtherNetworksSummary", err)
	_, err = s.GetNodesInNetwork(ctx, "fsxnet", 5, 7, "")
	check("GetNodesInNetwork", err)
	_, err = s.GetGeoHostingDistribution(ctx, 7, "")
	check("GetGeoHostingDistribution", err)
	_, err = s.GetNodesByCountry(ctx, "US", 7, "")
	check("GetNodesByCountry", err)
	_, err = s.GetBinkPSoftwareDistribution(ctx, 7, "")
	check("GetBinkPSoftwareDistribution", err)
	_, err = s.GetModemAccessibleNodes(ctx, 5, 30, false, "")
	check("GetModemAccessibleNodes", err)
	_, err = s.GetModemNoAnswerNodes(ctx, 5, 30, false, "")
	check("GetModemNoAnswerNodes", err)
	_, err = s.GetAllWhoisResults(ctx, "")
	check("GetAllWhoisResults", err)
	_, err = s.GetNodesByDomain(ctx, "example.com", 7)
	check("GetNodesByDomain", err)

	fmt.Println()
	if failures > 0 {
		fmt.Printf("%d FAILURES\n", failures)
		os.Exit(1)
	}
	fmt.Println("ALL OK")
}
