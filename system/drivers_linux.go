//go:build linux

package system

import (
	"fmt"
	"os/exec"
	"strings"
)

// GetDriversInfo — загруженные модули ядра (Linux-аналог списка драйверов).
func GetDriversInfo() string {
	out, err := exec.Command("lsmod").Output()
	if err != nil {
		return "Загруженные модули ядра: получить список не удалось (lsmod недоступен)"
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var b strings.Builder
	fmt.Fprintf(&b, "Загруженные модули ядра (аналог драйверов): %d шт.\n", len(lines)-1)
	fmt.Fprintln(&b, "Первые 15:")
	for i := 1; i < len(lines) && i <= 15; i++ {
		fields := strings.Fields(lines[i])
		if len(fields) >= 2 {
			fmt.Fprintf(&b, "  %s (%s КБ)\n", fields[0], fields[1])
		}
	}
	return b.String()
}
