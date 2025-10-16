package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/nodelistdb/internal/config"
	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/logging"
)

func main() {
	// Command line flags
	configFile := flag.String("config", "config.yaml", "Path to configuration file")
	dryRun := flag.Bool("dry-run", false, "Print SQL without executing")
	flag.Parse()

	// Load configuration
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	if err := logging.Initialize(logging.FromStruct(&cfg.Logging)); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	logger := logging.GetLogger()
	logger.Info("Starting flag_statistics backfill")

	// Connect to ClickHouse
	chConfig := &database.ClickHouseConfig{
		Host:         cfg.ClickHouse.Host,
		Port:         cfg.ClickHouse.Port,
		Database:     cfg.ClickHouse.Database,
		Username:     cfg.ClickHouse.Username,
		Password:     cfg.ClickHouse.Password,
		UseSSL:       cfg.ClickHouse.UseSSL,
		MaxOpenConns: 10,
		MaxIdleConns: 5,
		DialTimeout:  10 * time.Second,
	}

	db, err := database.NewClickHouse(chConfig)
	if err != nil {
		logger.Error("Failed to connect to ClickHouse", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	logger.Info("Connected to ClickHouse successfully")

	// SQL to populate flag_statistics from existing nodes table
	backfillSQL := `
	INSERT INTO flag_statistics (
		flag,
		year,
		nodelist_date,
		unique_nodes,
		total_nodes_in_year,
		first_zone,
		first_net,
		first_node,
		first_nodelist_date,
		first_day_number,
		first_system_name,
		first_location,
		first_sysop_name,
		first_phone,
		first_node_type,
		first_region,
		first_max_speed,
		first_is_cm,
		first_is_mo,
		first_has_inet,
		first_raw_line
	)
	WITH
	-- Explode all flags from all sources into a unified list
	all_node_flags AS (
		SELECT
			zone,
			net,
			node,
			nodelist_date,
			day_number,
			toYear(nodelist_date) AS year,
			system_name,
			location,
			sysop_name,
			phone,
			node_type,
			region,
			max_speed,
			is_cm,
			is_mo,
			has_inet,
			raw_line,
			arrayJoin(arrayConcat(
				flags,
				modem_flags,
				extractAll(toString(internet_config), '"([A-Z]{3})"')
			)) AS flag
		FROM nodes
		WHERE length(flags) > 0 OR length(modem_flags) > 0 OR length(extractAll(toString(internet_config), '"([A-Z]{3})"')) > 0
	),
	-- Get first appearance of each flag with complete node information
	flag_first_appearance AS (
		SELECT
			flag,
			argMin((zone, net, node, nodelist_date, day_number, system_name, location, sysop_name, phone, node_type, region, max_speed, is_cm, is_mo, has_inet, raw_line), nodelist_date) AS first_node
		FROM all_node_flags
		GROUP BY flag
	),
	-- Aggregate unique nodes per flag, year, and most recent nodelist_date
	flag_year_stats AS (
		SELECT
			flag,
			year,
			max(nodelist_date) AS nodelist_date,
			uniqExact((zone, net, node)) AS unique_nodes
		FROM all_node_flags
		GROUP BY flag, year
	),
	-- Calculate total unique nodes per year (for percentage calculations)
	-- Count ALL nodes, not just those with flags
	total_nodes_per_year AS (
		SELECT
			toYear(nodelist_date) AS year,
			uniqExact((zone, net, node)) AS total_nodes
		FROM nodes
		GROUP BY year
	)
	SELECT
		s.flag,
		s.year,
		s.nodelist_date,
		s.unique_nodes,
		t.total_nodes AS total_nodes_in_year,
		tupleElement(f.first_node, 1) AS first_zone,
		tupleElement(f.first_node, 2) AS first_net,
		tupleElement(f.first_node, 3) AS first_node,
		tupleElement(f.first_node, 4) AS first_nodelist_date,
		tupleElement(f.first_node, 5) AS first_day_number,
		tupleElement(f.first_node, 6) AS first_system_name,
		tupleElement(f.first_node, 7) AS first_location,
		tupleElement(f.first_node, 8) AS first_sysop_name,
		tupleElement(f.first_node, 9) AS first_phone,
		tupleElement(f.first_node, 10) AS first_node_type,
		tupleElement(f.first_node, 11) AS first_region,
		tupleElement(f.first_node, 12) AS first_max_speed,
		tupleElement(f.first_node, 13) AS first_is_cm,
		tupleElement(f.first_node, 14) AS first_is_mo,
		tupleElement(f.first_node, 15) AS first_has_inet,
		tupleElement(f.first_node, 16) AS first_raw_line
	FROM flag_year_stats s
	LEFT JOIN flag_first_appearance f ON s.flag = f.flag
	LEFT JOIN total_nodes_per_year t ON s.year = t.year
	ORDER BY s.flag, s.year
	`

	if *dryRun {
		fmt.Println("DRY RUN - SQL to be executed:")
		fmt.Println(backfillSQL)
		logger.Info("Dry run completed")
		return
	}

	// Execute backfill
	logger.Info("Starting backfill operation (this may take several minutes)...")
	startTime := time.Now()

	ctx := context.Background()
	if err := db.NativeConn().Exec(ctx, backfillSQL); err != nil {
		logger.Error("Backfill failed", "error", err)
		os.Exit(1)
	}

	duration := time.Since(startTime)
	logger.Info("Backfill completed successfully", "duration", duration)

	// Get statistics
	var totalRows uint64
	countSQL := "SELECT count(*) FROM flag_statistics"
	if err := db.Conn().QueryRow(countSQL).Scan(&totalRows); err != nil {
		logger.Warn("Failed to count rows", "error", err)
	} else {
		logger.Info("Flag statistics populated", "total_rows", totalRows)
	}

	// Show unique flags count
	var uniqueFlags uint64
	uniqueFlagsSQL := "SELECT uniqExact(flag) FROM flag_statistics"
	if err := db.Conn().QueryRow(uniqueFlagsSQL).Scan(&uniqueFlags); err != nil {
		logger.Warn("Failed to count unique flags", "error", err)
	} else {
		logger.Info("Unique flags found", "count", uniqueFlags)
	}

	// Show sample data
	sampleSQL := `
	SELECT flag, year, unique_nodes, first_system_name, first_location
	FROM flag_statistics
	WHERE unique_nodes > 100
	ORDER BY unique_nodes DESC
	LIMIT 10
	`
	rows, err := db.Conn().Query(sampleSQL)
	if err != nil {
		logger.Warn("Failed to query sample data", "error", err)
	} else {
		defer rows.Close()
		fmt.Println("\nTop 10 flags by usage:")
		for rows.Next() {
			var flag, systemName, location string
			var year uint16
			var uniqueNodes uint32
			if err := rows.Scan(&flag, &year, &uniqueNodes, &systemName, &location); err != nil {
				logger.Warn("Failed to scan row", "error", err)
				continue
			}
			fmt.Printf("  %s (%d): %d nodes (first: %s @ %s)\n", flag, year, uniqueNodes, systemName, location)
		}
	}
}
