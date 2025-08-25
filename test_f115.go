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
	
	fmt.Println("Testing IFCICO connection to f115.spb.ru (91.151.190.34:60179)")
	result := tester.Test(ctx, "91.151.190.34", 60179, "2:5030/1557")
	
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