package main

import (
	"context"
	"fmt"
	"time"
	
	"github.com/nodelistdb/internal/testing/protocols"
)

func main() {
	// Create tester without debug
	tester := protocols.NewIfcicoTesterWithInfo(
		30*time.Second,
		"2:5001/5001",
		"NodelistDB Test",
		"Test Operator", 
		"Test Location",
	)
	
	// Test with and without debug
	fmt.Println("=========================================")
	fmt.Println("Test 1: Debug OFF (default)")
	fmt.Println("=========================================")
	
	ctx := context.Background()
	result := tester.Test(ctx, "24.62.212.226", 60179, "1:1/19")
	
	if ifcicoResult, ok := result.(*protocols.IfcicoTestResult); ok {
		fmt.Printf("Success: %v\n", ifcicoResult.Success)
		if ifcicoResult.Success {
			fmt.Printf("System: %s\n", ifcicoResult.SystemName)
		} else {
			fmt.Printf("Error: %s\n", ifcicoResult.Error)
		}
	}
	
	fmt.Println("\n=========================================")
	fmt.Println("Test 2: Debug ON (via SetDebug)")
	fmt.Println("=========================================")
	
	// Enable debug mode
	tester.SetDebug(true)
	
	result = tester.Test(ctx, "24.62.212.226", 60179, "1:1/19")
	
	if ifcicoResult, ok := result.(*protocols.IfcicoTestResult); ok {
		fmt.Printf("Success: %v\n", ifcicoResult.Success)
		if ifcicoResult.Success {
			fmt.Printf("System: %s\n", ifcicoResult.SystemName)
		} else {
			fmt.Printf("Error: %s\n", ifcicoResult.Error)
		}
	}
	
	// Also test that it won't log if we turn debug off again
	fmt.Println("\n=========================================")
	fmt.Println("Test 3: Debug OFF again")
	fmt.Println("=========================================")
	
	tester.SetDebug(false)
	result = tester.Test(ctx, "24.62.212.226", 60179, "1:1/19")
	
	if ifcicoResult, ok := result.(*protocols.IfcicoTestResult); ok {
		fmt.Printf("Success: %v\n", ifcicoResult.Success)
		if ifcicoResult.Success {
			fmt.Printf("System: %s\n", ifcicoResult.SystemName)
		} else {
			fmt.Printf("Error: %s\n", ifcicoResult.Error)
		}
	}
}