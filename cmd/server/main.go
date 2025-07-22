package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"nodelistdb/internal/api"
	"nodelistdb/internal/database"
	"nodelistdb/internal/storage"
	"nodelistdb/internal/web"
)

func main() {
	// Command line flags
	var (
		dbPath = flag.String("db", "./nodelist.duckdb", "Path to DuckDB database file")
		port   = flag.String("port", "8080", "HTTP server port")
		host   = flag.String("host", "localhost", "HTTP server host")
	)
	flag.Parse()

	fmt.Println("FidoNet Nodelist Server (DuckDB)")
	fmt.Println("===============================")
	fmt.Printf("Database: %s\n", *dbPath)
	fmt.Printf("Server: http://%s:%s\n", *host, *port)
	fmt.Println()

	// Initialize database
	log.Println("Initializing DuckDB database...")
	db, err := database.New(*dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Get DuckDB version
	if version, err := db.GetVersion(); err == nil {
		fmt.Printf("DuckDB version: %s\n", version)
	}

	// Create schema
	if err := db.CreateSchema(); err != nil {
		log.Fatalf("Failed to create schema: %v", err)
	}

	log.Println("Database initialized successfully")

	// Initialize storage layer
	storage, err := storage.New(db)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer storage.Close()

	// Initialize API and Web servers
	apiServer := api.New(storage)
	webServer := web.New(storage)

	// Set up HTTP routes
	mux := http.NewServeMux()
	
	// API routes
	apiServer.SetupRoutes(mux)
	
	// Web routes
	webServer.SetupRoutes(mux)

	// Server configuration
	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%s", *host, *port),
		Handler: mux,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Server starting on http://%s:%s", *host, *port)
		log.Println("Available endpoints:")
		log.Println("  Web Interface:")
		log.Printf("    http://%s:%s/        - Home page\n", *host, *port)
		log.Printf("    http://%s:%s/search  - Search nodes\n", *host, *port)
		log.Printf("    http://%s:%s/stats   - Statistics\n", *host, *port)
		log.Println("  REST API:")
		log.Printf("    http://%s:%s/api/health          - API health check\n", *host, *port)
		log.Printf("    http://%s:%s/api/nodes           - Search nodes\n", *host, *port)
		log.Printf("    http://%s:%s/api/nodes/1/234/56  - Get specific node\n", *host, *port)
		log.Printf("    http://%s:%s/api/stats           - Network statistics\n", *host, *port)
		log.Println()
		
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Server shutting down...")
	if err := server.Close(); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}