//go:build darwin

package hardware

import (
	"os"
	"os/exec"
	"strings"
)

// DiagnoseTemperature — температура CPU (macOS).
// Прямого публичного датчика, как WMI на Windows, в macOS нет:
// 1) если установлен osx-cpu-temp (brew install osx-cpu-temp) — читаем его;
// 2) при запуске с sudo — powermetrics (SMC);
// 3) иначе — честное сообщение о недоступности.
func DiagnoseTemperature() DiagnosticResult {
	result := DiagnosticResult{Name: "Температура (macOS)"}

	if path, err := exec.LookPath("osx-cpu-temp"); err == nil {
		if out, err := exec.Command(path).Output(); err == nil {
			if s := strings.TrimSpace(string(out)); s != "" {
				result.Value = "Температура (osx-cpu-temp): " + s + "\n"
				return result
			}
		}
	}

	if os.Geteuid() == 0 {
		if out, err := exec.Command("powermetrics", "--samplers", "smc", "-n", "1", "-i", "1").Output(); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				t := strings.TrimSpace(line)
				if strings.Contains(t, "die temperature") {
					result.Value = "Температура (powermetrics): " + t + "\n"
					return result
				}
			}
		}
	}

	result.Value = "Датчики температуры не обнаружены или недоступны.\n" +
		"Варианты: brew install osx-cpu-temp или запуск с sudo (powermetrics)."
	return result
}
