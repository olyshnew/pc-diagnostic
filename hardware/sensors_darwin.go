//go:build darwin

package hardware

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// GetSensorsInfo — вентиляторы и напряжения (macOS).
// Данные SMC доступны через powermetrics только с правами root —
// это аналог требования «администратор» для LibreHardwareMonitor на Windows.
func GetSensorsInfo() string {
	if os.Geteuid() != 0 {
		return "Вентиляторы и напряжения: для чтения SMC-датчиков запустите с sudo\n(аналог прав администратора на Windows)"
	}

	out, err := exec.Command("powermetrics", "--samplers", "smc", "-n", "1", "-i", "1").Output()
	if err != nil {
		return "Вентиляторы и напряжения: не удалось получить данные powermetrics: " + err.Error()
	}

	var b strings.Builder
	b.WriteString("Датчики (powermetrics/SMC):\n")
	fans, temp := 0, ""
	for _, line := range strings.Split(string(out), "\n") {
		t := strings.TrimSpace(line)
		if strings.Contains(t, "Fan") && strings.Contains(t, "rpm") {
			fmt.Fprintf(&b, "  %s\n", t)
			fans++
		}
		if strings.Contains(t, "die temperature") {
			temp = t
		}
	}
	if temp != "" {
		fmt.Fprintf(&b, "  %s\n", temp)
	}
	if fans == 0 && temp == "" {
		return "Вентиляторы и напряжения: SMC не отдал данных на этом устройстве"
	}
	return b.String()
}
