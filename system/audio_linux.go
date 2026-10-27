//go:build linux

package system

import (
	"os"
	"os/exec"
	"strings"
)

// GetAudioInfo — звуковые устройства (Linux: /proc/asound/cards, резерв aplay -l).
func GetAudioInfo() string {
	data, err := os.ReadFile("/proc/asound/cards")
	if err == nil && strings.TrimSpace(string(data)) != "" {
		return "Звуковые устройства (ALSA):\n" + string(data)
	}
	out, err2 := exec.Command("aplay", "-l").Output()
	if err2 == nil && strings.TrimSpace(string(out)) != "" {
		return "Звуковые устройства (aplay):\n" + string(out)
	}
	return "Звуковые устройства: не найдены (ALSA не обнаружена)"
}
