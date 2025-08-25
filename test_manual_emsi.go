package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"time"
)

func main() {
	conn, err := net.DialTimeout("tcp", "91.151.190.34:60179", 10*time.Second)
	if err != nil {
		log.Fatal("Failed to connect:", err)
	}
	defer conn.Close()
	
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	
	// Send CR to trigger EMSI
	fmt.Println("Sending CR...")
	writer.WriteString("\r")
	writer.Flush()
	
	// Read response
	time.Sleep(1 * time.Second)
	buffer := make([]byte, 1024)
	n, _ := reader.Read(buffer)
	fmt.Printf("Received %d bytes: %q\n", n, buffer[:n])
	
	// Send our EMSI_DAT without domain
	packet := "**EMSI_DAT008B{EMSI}{2:5001/5001}{}{PUA}{ZMO,ZAP}{PID}{NodelistDB}{1.0}{TEST}{IDENT}{[Test System][Test Location][Test Sysop][-Unpublished-][TCP/IP][XA]}7293\r\x11\r"
	fmt.Printf("Sending packet (%d bytes): %q\n", len(packet), packet)
	writer.WriteString(packet)
	writer.Flush()
	
	// Read response
	time.Sleep(3 * time.Second)
	n, _ = reader.Read(buffer)
	fmt.Printf("Received %d bytes: %q\n", n, buffer[:n])
	
	// Try again with simpler packet matching bforce more closely
	packet2 := "**EMSI_DAT0080{EMSI}{2:450/256}{}{PUA}{ZMO,ZAP}{fe}{Test}{1.0}{TEST}{IDENT}{[Test][Test][Test][-Unpublished-][TCP/IP][XA]}BC5A\r\x11\r"
	fmt.Printf("\nTrying simpler packet (%d bytes): %q\n", len(packet2), packet2)
	writer.WriteString(packet2)
	writer.Flush()
	
	time.Sleep(3 * time.Second)
	n, _ = reader.Read(buffer)
	fmt.Printf("Received %d bytes: %q\n", n, buffer[:n])
}