//go:build darwin

package system

import (
	"os/exec"
	"strings"
)

// GetDriversInfo — загруженные расширения ядра (macOS: аналог драйверов).
func GetDriversInfo() string {
	if out, err := exec.Command("kextstat").Output(); err == nil && strings.TrimSpace(string(out)) != "" {
		return "Загруженные расширения ядра (kextstat):\n" + strings.TrimSpace(string(out))
	}
	out, err := exec.Command("systemextensionsctl", "list").Output()
	if err != nil {
		return "Драйверы: получить список не удалось (kextstat/systemextensionsctl)"
	}
	return "Системные расширения (systemextensionsctl):\n" + strings.TrimSpace(string(out))
}
