//go:build darwin

package system

import (
	"os/exec"
	"strings"
)

// GetMainboardInfo — информация о системе (macOS: system_profiler SPHardwareDataType).
func GetMainboardInfo() string {
	out, err := exec.Command("system_profiler", "SPHardwareDataType").Output()
	if err != nil {
		return "Материнская плата: не удалось получить информацию (system_profiler)"
	}
	var b strings.Builder
	b.WriteString("Информация о системе (system_profiler):\n")
	for _, line := range strings.Split(string(out), "\n") {
		t := strings.TrimSpace(line)
		switch {
		case strings.Contains(t, "Model Name:"),
			strings.Contains(t, "Model Identifier:"),
			strings.Contains(t, "Chip:"),
			strings.Contains(t, "Processor Name:"),
			strings.Contains(t, "Memory:"),
			strings.Contains(t, "Serial Number"):
			b.WriteString("  " + t + "\n")
		}
	}
	return b.String()
}
