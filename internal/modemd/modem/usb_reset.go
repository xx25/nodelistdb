package modem

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GetUSBDeviceID finds the USB vendor:product IDs for a serial device.
// Maps /dev/ttyACM0 -> 0ace:1611 by following sysfs symlinks.
// Returns empty strings if device is not USB-based.
func GetUSBDeviceID(serialDevice string) (vendor, product string, err error) {
	devName := filepath.Base(serialDevice)

	// Follow symlink: /sys/class/tty/ttyACM0/device -> ../../../1-9:1.0
	devicePath := filepath.Join("/sys/class/tty", devName, "device")
	resolved, err := filepath.EvalSymlinks(devicePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve device path: %w", err)
	}

	// Walk up the directory tree to find USB device with idVendor
	for dir := resolved; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		vendorFile := filepath.Join(dir, "idVendor")
		vendorData, err := os.ReadFile(vendorFile)
		if err != nil {
			continue // Not a USB device directory, keep walking up
		}

		vendor = strings.TrimSpace(string(vendorData))

		productFile := filepath.Join(dir, "idProduct")
		productData, err := os.ReadFile(productFile)
		if err != nil {
			return "", "", fmt.Errorf("found idVendor but not idProduct in %s", dir)
		}
		product = strings.TrimSpace(string(productData))

		return vendor, product, nil
	}

	return "", "", fmt.Errorf("USB device IDs not found for %s", serialDevice)
}

// ResetUSBDevice resets a USB device using the usbreset command.
// Requires sudo permissions for usbreset.
func ResetUSBDevice(vendor, product string) error {
	deviceID := fmt.Sprintf("%s:%s", vendor, product)

	cmd := exec.Command("sudo", "usbreset", deviceID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("usbreset %s failed: %v: %s", deviceID, err, strings.TrimSpace(string(output)))
	}

	// Wait for device to re-enumerate
	time.Sleep(3 * time.Second)
	return nil
}

// WaitForDevice waits for a serial device to appear after USB reset.
// Returns nil if device appears within timeout, error otherwise.
func WaitForDevice(device string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	pollInterval := 200 * time.Millisecond

	for time.Now().Before(deadline) {
		if _, err := os.Stat(device); err == nil {
			// Device exists, wait a bit more for it to stabilize
			time.Sleep(500 * time.Millisecond)
			return nil
		}
		time.Sleep(pollInterval)
	}

	return fmt.Errorf("device %s did not appear within %v", device, timeout)
}
