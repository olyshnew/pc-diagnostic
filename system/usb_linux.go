//go:build linux

package system

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// GetUSBInfo — список USB-устройств (Linux: /sys/bus/usb/devices, резерв lsusb).
func GetUSBInfo() string {
	var b strings.Builder
	entries, _ := filepath.Glob("/sys/bus/usb/devices/*")
	count := 0
	for _, dir := range entries {
		base := filepath.Base(dir)
		if strings.Contains(base, ":") { // ':1.0' — интерфейсы, пропускаем
			continue
		}
		product, vendor, id := sysFile(dir+"/product"), sysFile(dir+"/idVendor"), sysFile(dir+"/idProduct")
		if product == "" && vendor == "" {
			continue
		}
		count++
		fmt.Fprintf(&b, "%d. %s (VID:PID %s:%s)\n", count, orDash(product), orDash(vendor), orDash(id))
	}
	if count == 0 {
		out, err := exec.Command("lsusb").Output()
		if err != nil {
			return "USB-устройства: получить список не удалось (lsusb недоступен)"
		}
		return "USB-устройства (lsusb):\n" + string(out)
	}
	return b.String()
}
