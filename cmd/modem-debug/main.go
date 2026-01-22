// Simple modem debug tool to see raw dial responses
package main

import (
	"fmt"
	"time"

	"github.com/mfkenney/go-serial/v2"
)

func main() {
	fmt.Println("Opening /dev/ttyACM0...")
	port, err := serial.Open("/dev/ttyACM0",
		serial.WithBaudrate(115200),
		serial.WithDataBits(8),
		serial.WithParity(serial.NoParity),
		serial.WithStopBits(serial.OneStopBit),
		serial.WithReadTimeout(2000),
	)
	if err != nil {
		fmt.Printf("Open error: %v\n", err)
		return
	}
	defer port.Close()

	_ = port.SetDTR(true)
	time.Sleep(100 * time.Millisecond)
	_ = port.ResetInputBuffer()
	_ = port.ResetOutputBuffer()

	// Init modem
	fmt.Println("Sending ATZ...")
	_, _ = port.Write([]byte("ATZ\r"))
	time.Sleep(500 * time.Millisecond)
	readAll(port)

	// Dial
	fmt.Println("\nDialing 900...")
	_, _ = port.Write([]byte("ATDT900\r"))

	// Read for 2 minutes
	start := time.Now()
	for time.Since(start) < 2*time.Minute {
		buf := make([]byte, 256)
		n, _ := port.Read(buf)
		if n > 0 {
			fmt.Printf("[%v] %d bytes: %q\n", time.Since(start).Round(time.Millisecond), n, string(buf[:n]))
		}
	}

	// Hangup
	fmt.Println("\nHanging up...")
	_ = port.SetDTR(false)
	time.Sleep(500 * time.Millisecond)
	_ = port.SetDTR(true)
}

func readAll(port *serial.Port) {
	for {
		buf := make([]byte, 256)
		n, _ := port.Read(buf)
		if n > 0 {
			fmt.Printf("Read: %q\n", string(buf[:n]))
		} else {
			break
		}
	}
}
