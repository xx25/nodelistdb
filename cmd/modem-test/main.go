// modem-test is a CLI tool for testing modem communication.
// It can test AT commands, dial phone numbers, and perform EMSI handshakes.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nodelistdb/internal/modemd/modem"
	"github.com/nodelistdb/internal/testing/protocols/emsi"
)

func main() {
	// Command line flags
	device := flag.String("device", "/dev/ttyACM0", "Serial port device")
	baudRate := flag.Int("baud", 115200, "Baud rate")
	dialNumber := flag.String("dial", "", "Phone number to dial (e.g., 900)")
	dialTimeout := flag.Duration("dial-timeout", 200*time.Second, "Dial timeout")
	carrierTimeout := flag.Duration("carrier-timeout", 5*time.Second, "Carrier detect timeout")
	emsiTimeout := flag.Duration("emsi-timeout", 30*time.Second, "EMSI handshake timeout")
	doEmsi := flag.Bool("emsi", false, "Perform EMSI handshake after connect")
	interactive := flag.Bool("interactive", false, "Interactive AT command mode")
	initString := flag.String("init", "ATZ", "Modem init string")
	hangupMethod := flag.String("hangup", "dtr", "Hangup method: dtr or escape")
	localAddr := flag.String("addr", "2:5001/5001", "Our FidoNet address for EMSI")
	systemName := flag.String("system", "NodelistDB Modem Tester", "Our system name for EMSI")
	debug := flag.Bool("debug", false, "Enable debug output")

	flag.Parse()

	fmt.Println("=== NodelistDB Modem Test Tool ===")
	fmt.Printf("Device: %s\n", *device)
	fmt.Printf("Baud rate: %d\n", *baudRate)

	// Create modem configuration
	cfg := modem.Config{
		Device:           *device,
		BaudRate:         *baudRate,
		InitString:       *initString,
		DialPrefix:       modem.ATDT,
		HangupMethod:     *hangupMethod,
		ATCommandTimeout: 5 * time.Second,
		ReadTimeout:      1 * time.Second,
	}

	// Create modem
	m, err := modem.New(cfg)
	if err != nil {
		fmt.Printf("ERROR: Failed to create modem: %v\n", err)
		os.Exit(1)
	}

	// Setup signal handler for cleanup
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt, cleaning up...")
		if m.InDataMode() {
			fmt.Println("Hanging up...")
			if err := m.Hangup(); err != nil {
				fmt.Printf("Hangup error: %v\n", err)
			}
		}
		m.Close()
		os.Exit(0)
	}()

	// Open modem
	fmt.Printf("\nOpening %s...\n", *device)
	if err := m.Open(); err != nil {
		fmt.Printf("ERROR: Failed to open modem: %v\n", err)
		os.Exit(1)
	}
	defer m.Close()

	fmt.Println("Modem opened and initialized successfully")

	// Get modem info
	fmt.Println("\n--- Modem Info ---")
	info, err := m.GetInfo()
	if err != nil {
		fmt.Printf("Failed to get modem info: %v\n", err)
	} else {
		if info.Manufacturer != "" {
			fmt.Printf("Manufacturer: %s\n", info.Manufacturer)
		}
		if info.Model != "" {
			fmt.Printf("Model: %s\n", info.Model)
		}
		if info.Firmware != "" {
			fmt.Printf("Firmware: %s\n", info.Firmware)
		}
		fmt.Printf("Raw response:\n%s\n", info.RawResponse)
	}

	// Get modem status
	status, err := m.GetStatus()
	if err != nil {
		fmt.Printf("Failed to get modem status: %v\n", err)
	} else {
		fmt.Printf("\n--- Modem Status ---\n")
		fmt.Printf("DCD (Carrier): %v\n", status.DCD)
		fmt.Printf("DSR (Ready):   %v\n", status.DSR)
		fmt.Printf("CTS (Clear):   %v\n", status.CTS)
		fmt.Printf("RI (Ring):     %v\n", status.RI)
	}

	// Interactive mode
	if *interactive {
		runInteractive(m)
		return
	}

	// Dial if number provided
	if *dialNumber != "" {
		runDial(m, *dialNumber, *dialTimeout, *carrierTimeout, *doEmsi, *emsiTimeout, *localAddr, *systemName, *debug)
	}

	fmt.Println("\nTest complete")
}

func runInteractive(m *modem.Modem) {
	fmt.Println("\n=== Interactive AT Command Mode ===")
	fmt.Println("Enter AT commands (type 'quit' to exit)")

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		input, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		cmd := strings.TrimSpace(input)
		if cmd == "" {
			continue
		}
		if strings.ToLower(cmd) == "quit" || strings.ToLower(cmd) == "exit" {
			break
		}

		response, err := m.SendAT(cmd, 10*time.Second)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		} else {
			fmt.Printf("%s\n", response)
		}
	}
}

func runDial(m *modem.Modem, number string, dialTimeout, carrierTimeout time.Duration, doEmsi bool, emsiTimeout time.Duration, localAddr, systemName string, debug bool) {
	fmt.Printf("\n=== Dialing %s ===\n", number)
	fmt.Printf("Dial timeout: %v\n", dialTimeout)
	fmt.Printf("Carrier timeout: %v\n", carrierTimeout)

	startTime := time.Now()

	result, err := m.Dial(number, dialTimeout, carrierTimeout)
	if err != nil {
		fmt.Printf("ERROR: Dial failed: %v\n", err)
		if result != nil {
			fmt.Printf("Dial time: %v\n", result.DialTime)
			fmt.Printf("Error: %s\n", result.Error)
		}
		return
	}

	fmt.Printf("\n--- Dial Result ---\n")
	fmt.Printf("Success: %v\n", result.Success)
	fmt.Printf("Dial time: %v\n", result.DialTime)

	if result.Success {
		fmt.Printf("Connect speed: %d\n", result.ConnectSpeed)
		if result.Protocol != "" {
			fmt.Printf("Protocol: %s\n", result.Protocol)
		}
		fmt.Printf("Ring count: %d\n", result.RingCount)
		fmt.Printf("Carrier time: %v\n", result.CarrierTime)

		// Get modem status after connect
		status, err := m.GetStatus()
		if err == nil {
			fmt.Printf("\n--- Post-Connect Status ---\n")
			fmt.Printf("DCD (Carrier): %v\n", status.DCD)
		}

		if doEmsi {
			runEmsiHandshake(m, emsiTimeout, localAddr, systemName, debug)
		} else {
			// Just stay connected for a moment to verify
			fmt.Println("\nConnected! Waiting 3 seconds before hangup...")
			time.Sleep(3 * time.Second)
		}

		// Hangup
		fmt.Println("\nHanging up...")
		if err := m.Hangup(); err != nil {
			fmt.Printf("Hangup error: %v\n", err)
			fmt.Println("Attempting modem reset...")
			if err := m.Reset(); err != nil {
				fmt.Printf("Reset error: %v\n", err)
			}
		} else {
			fmt.Println("Hangup successful")
		}
	} else {
		fmt.Printf("Error: %s\n", result.Error)
	}

	fmt.Printf("\nTotal time: %v\n", time.Since(startTime))
}

func runEmsiHandshake(m *modem.Modem, timeout time.Duration, localAddr, systemName string, debug bool) {
	fmt.Printf("\n=== EMSI Handshake ===\n")
	fmt.Printf("Timeout: %v\n", timeout)
	fmt.Printf("Our address: %s\n", localAddr)
	fmt.Printf("Our system: %s\n", systemName)

	// Get net.Conn wrapper
	conn := m.GetConn()

	// Create EMSI session
	session := emsi.NewSessionWithInfo(
		conn,
		localAddr,
		systemName,
		"Test Operator",
		"Test Location",
	)
	session.SetTimeout(timeout)
	session.SetDebug(debug)

	// Perform handshake
	fmt.Println("\nStarting EMSI handshake...")
	startTime := time.Now()

	if err := session.Handshake(); err != nil {
		fmt.Printf("ERROR: EMSI handshake failed: %v\n", err)
		fmt.Printf("Handshake time: %v\n", time.Since(startTime))
		return
	}

	fmt.Printf("Handshake time: %v\n", time.Since(startTime))

	// Get remote info
	info := session.GetRemoteInfo()
	if info != nil {
		fmt.Println("\n--- Remote System Info ---")
		fmt.Printf("System name: %s\n", info.SystemName)
		fmt.Printf("Location: %s\n", info.Location)
		fmt.Printf("Sysop: %s\n", info.Sysop)
		fmt.Printf("Mailer: %s %s\n", info.MailerName, info.MailerVersion)
		fmt.Printf("Addresses: %v\n", info.Addresses)

		// Validate address
		if len(info.Addresses) > 0 {
			fmt.Printf("\nAddress validation:\n")
			for _, addr := range info.Addresses {
				fmt.Printf("  %s\n", addr)
			}
		}
	} else {
		fmt.Println("No remote info received (handshake may have been partial)")
	}
}
