//go:build darwin

package system

import (
	"os/exec"
	"strings"
)

// GetAudioInfo — звуковые устройства (macOS: system_profiler SPAudioDataType).
func GetAudioInfo() string {
	out, err := exec.Command("system_profiler", "SPAudioDataType").Output()
	if err != nil {
		return "Звуковые устройства: получить список не удалось (system_profiler недоступен)"
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return "Звуковые устройства: не найдены"
	}
	return "Звуковые устройства (system_profiler):\n" + s
}
