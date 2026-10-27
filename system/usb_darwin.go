//go:build darwin

package system

import (
	"os/exec"
	"strings"
)

// GetUSBInfo — список USB-устройств (macOS: system_profiler SPUSBDataType).
func GetUSBInfo() string {
	out, err := exec.Command("system_profiler", "SPUSBDataType").Output()
	if err != nil {
		return "USB-устройства: получить список не удалось (system_profiler недоступен)"
	}
	var b strings.Builder
	b.WriteString("USB-устройства (system_profiler):\n")
	for _, line := range strings.Split(string(out), "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		switch {
		case strings.Contains(t, "Product ID:"),
			strings.Contains(t, "Vendor ID:"),
			strings.Contains(t, "Manufacturer:"),
			strings.Contains(t, "Serial Number"):
			b.WriteString("  " + t + "\n")
		case !strings.Contains(t, ":"): // заголовок устройства (например, USB31Bus)
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}
