//go:build darwin

package hardware

import (
	"os/exec"
	"strings"
)

// DiagnoseGPU — информация о видеокарте (macOS: system_profiler SPDisplaysDataType).
func DiagnoseGPU() DiagnosticResult {
	result := DiagnosticResult{Name: "Видеокарта (macOS)"}

	out, err := exec.Command("system_profiler", "SPDisplaysDataType").Output()
	if err != nil {
		result.Value = "Не удалось получить информацию о видеокарте (system_profiler)"
		return result
	}

	var b strings.Builder
	for _, line := range strings.Split(string(out), "\n") {
		t := strings.TrimSpace(line)
		switch {
		case strings.Contains(t, "Chipset Model:"),
			strings.Contains(t, "VRAM"),
			strings.Contains(t, "Metal"),
			strings.Contains(t, "Cores:"),
			strings.Contains(t, "Bus:"),
			strings.Contains(t, "Total Number of Cores"):
			b.WriteString("  " + t + "\n")
		}
	}
	if b.Len() == 0 {
		result.Value = strings.TrimSpace(string(out))
	} else {
		result.Value = b.String()
	}
	return result
}
