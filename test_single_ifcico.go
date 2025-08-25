package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"
	
	"github.com/nodelistdb/internal/testing/protocols"
)

func main() {
	var (
		address  = flag.String("address", "2:5030/115", "FidoNet address to test")
		hostname = flag.String("hostname", "", "Hostname or IP to test")
		port     = flag.Int("port", 60179, "Port to test")
		timeout  = flag.Duration("timeout", 30*time.Second, "Connection timeout")
		debug    = flag.Bool("debug", true, "Enable debug output")
	)
	flag.Parse()
	
	// Set debug environment variable
	if *debug {
		os.Setenv("DEBUG_EMSI", "1")
		log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	}
	
	log.Printf("========================================")
	log.Printf("IFCICO Connection Test")
	log.Printf("========================================")
	log.Printf("Target Address: %s", *address)
	log.Printf("Target Host: %s:%d", *hostname, *port)
	log.Printf("Timeout: %v", *timeout)
	log.Printf("Debug Mode: %v", *debug)
	log.Printf("========================================")
	
	// Create tester
	tester := protocols.NewIfcicoTesterWithInfo(
		*timeout,
		"2:5001/5001",
		"NodelistDB Test",
		"Test Operator", 
		"Test Location",
	)
	
	if *debug {
		tester.SetDebug(true)
	}
	
	// Create context
	ctx, cancel := context.WithTimeout(context.Background(), *timeout + 5*time.Second)
	defer cancel()
	
	// Perform test
	log.Printf("Starting test...")
	startTime := time.Now()
	
	result := tester.Test(ctx, *hostname, *port, *address)
	
	duration := time.Since(startTime)
	log.Printf("========================================")
	log.Printf("Test completed in %v", duration)
	log.Printf("========================================")
	
	// Display results
	if ifcicoResult, ok := result.(*protocols.IfcicoTestResult); ok {
		log.Printf("Success: %v", ifcicoResult.Success)
		log.Printf("Response Time: %d ms", ifcicoResult.ResponseMs)
		
		if ifcicoResult.Success {
			log.Printf("System Name: %s", ifcicoResult.SystemName)
			log.Printf("Mailer Info: %s", ifcicoResult.MailerInfo)
			log.Printf("Addresses: %v", ifcicoResult.Addresses)
			log.Printf("Response Type: %s", ifcicoResult.ResponseType)
		} else {
			log.Printf("Error: %s", ifcicoResult.Error)
		}
	} else {
		log.Printf("ERROR: Unexpected result type")
	}
	
	log.Printf("========================================")
}