package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"
	
	"github.com/nodelistdb/internal/testing/protocols"
)

func main() {
	// Enable debug
	os.Setenv("DEBUG_EMSI", "1")
	
	tester := protocols.NewIfcicoTesterWithInfo(
		30*time.Second, 
		"2:5001/5001",
		"Test System",
		"Test Sysop",
		"Test Location",
	)
	
	ctx := context.Background()
	
	fmt.Println("Testing IFCICO connection to 2:5000/111 (byte.nsk.su:60179)")
	result := tester.Test(ctx, "37.192.123.64", 60179, "2:5000/111")
	
	if ifcicoResult, ok := result.(*protocols.IfcicoTestResult); ok {
		fmt.Printf("Success: %v\n", ifcicoResult.Success)
		fmt.Printf("Response Time: %dms\n", ifcicoResult.ResponseMs)
		fmt.Printf("Error: %s\n", ifcicoResult.Error)
		fmt.Printf("System Name: %s\n", ifcicoResult.SystemName)
		fmt.Printf("Mailer Info: %s\n", ifcicoResult.MailerInfo)
		fmt.Printf("Addresses: %v\n", ifcicoResult.Addresses)
		fmt.Printf("Response Type: %s\n", ifcicoResult.ResponseType)
	} else {
		log.Fatal("Failed to get IFCICO result")
	}
}