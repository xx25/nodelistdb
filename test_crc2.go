package main

import (
	"fmt"
	"github.com/nodelistdb/internal/testing/protocols/emsi"
)

func main() {
	// Test our actual packet
	ourDataSection := "EMSI_DAT0099{EMSI}{2:5001/5001}{}{8N1,PUA}{NCP}{PID}{NodelistDB}{1.0}{TEST}{IDENT}{[NodelistDB Test Daemon][Test Location][Test Operator][-Unpublished-][TCP/IP][XA]}"
	
	// Calculate CRC16
	crc := emsi.CalculateCRC16([]byte(ourDataSection))
	
	fmt.Printf("Our data section: %s\n", ourDataSection[:50])
	fmt.Printf("Data length: %d bytes\n", len(ourDataSection))
	fmt.Printf("Calculated CRC: %04X\n", crc)
	fmt.Printf("CRC in packet: C5CF\n")
	
	// Check if it matches
	if fmt.Sprintf("%04X", crc) == "C5CF" {
		fmt.Println("CRC matches!")
	} else {
		fmt.Println("CRC mismatch!")
	}
	
	// Also create the same packet using CreateEMSI_DAT function
	data := &emsi.EMSIData{
		SystemName:    "NodelistDB Test Daemon",
		Location:      "Test Location",
		Sysop:         "Test Operator",
		Phone:         "-Unpublished-",
		Speed:         "TCP/IP",
		Flags:         "XA",
		MailerName:    "NodelistDB",
		MailerVersion: "1.0",
		MailerSerial:  "TEST",
		Addresses:     []string{"2:5001/5001"},
		Protocols:     []string{}, // Will use NCP
		Password:      "",
	}
	
	packet := emsi.CreateEMSI_DAT(data)
	fmt.Printf("\nGenerated packet: %s\n", packet)
}