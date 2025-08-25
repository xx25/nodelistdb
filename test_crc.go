package main

import (
	"fmt"
	"github.com/nodelistdb/internal/testing/protocols/emsi"
)

func main() {
	// Test CRC with bforce's known-good packet
	// The data section without ** prefix and without CRC
	dataSection := "EMSI_DAT00F8{EMSI}{2:5001/5001@fidonet}{}{PUA,RMA,RH1}{HYD,ZMO,ZAP,EII}{fe}{binkleyforce}{0.27/linux-gnu}{free software}{IDENT}{[bforce tester][Moscow, Russia][Dmitry Protasoff][Unknown][57600][XW,V34B,IDC,LMD]}{OHFR}{22:00-07:30 22:30-05:30}{TRX#}{[68AA05EE]}"
	
	// Calculate CRC16
	crc := emsi.CalculateCRC16([]byte(dataSection))
	
	fmt.Printf("Data section: %s\n", dataSection[:50])
	fmt.Printf("Data length: %d bytes\n", len(dataSection))
	fmt.Printf("Calculated CRC: %04X\n", crc)
	fmt.Printf("Expected CRC from bforce: 0D66\n")
	
	// Also test with the remote's packet from f115
	remoteDataSection := "EMSI_DAT0129{EMSI}{2:5030/115@fidonet}{}{RH1}{HYD,NRQ,EII}{fe}{binkleyforce}{0.22.7/freebsd5.0}{free software}{IDENT}{[=== EdgeCity ===][Russia, St.Petersburg][Andrey Podkolzin & Dmitriy Yermakov][7-812-311-2286][33600][flags MO,XA,CM,V34,IBN,IFC]}{TRAF}{00000000 00000000}{MOH#}{[00000000]}{TRX#}{[68AA05EE]}"
	remoteCrc := emsi.CalculateCRC16([]byte(remoteDataSection))
	
	fmt.Printf("\nRemote data section from f115: %s\n", remoteDataSection[:50])
	fmt.Printf("Remote data length: %d bytes\n", len(remoteDataSection))
	fmt.Printf("Calculated CRC for remote: %04X\n", remoteCrc)
	fmt.Printf("Expected CRC from remote: 0D66\n")
}