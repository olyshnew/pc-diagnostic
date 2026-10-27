// +build windows

package hardware

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func DiagnoseTemperature() DiagnosticResult {
	result := DiagnosticResult{
		Name: "Температура компонентов",
	}

	// WMI через PowerShell — тот же метод, что в monitor/collector.go
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		`Get-WmiObject MSAcpi_ThermalZoneTemperature -Namespace root/wmi | Select-Object -ExpandProperty CurrentTemperature`)
	out, err := cmd.Output()
	if err != nil {
		result.Value = "Не удалось получить доступ к датчикам температуры.\nПопробуйте запустить от имени Администратора."
		return result
	}

	outStr := strings.TrimSpace(string(out))
	if outStr == "" {
		result.Value = "Датчики температуры не обнаружены.\nПримечание: метод MSAcpi_ThermalZoneTemperature недоступен на этой системе."
		return result
	}

	// WMI может вернуть несколько значений — построчно
	lines := strings.Split(outStr, "\n")
	var sb strings.Builder
	found := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Значение в десятых долях Кельвина
		tempK, err := strconv.ParseFloat(line, 64)
		if err != nil {
			continue
		}
		tempC := tempK/10 - 273.15
		sb.WriteString(fmt.Sprintf("Температура (Thermal Zone): %.1f°C\n", tempC))
		found = true
	}

	if !found {
		sb.WriteString("Датчики CPU не распознаны этим методом.\n")
	}

	result.Value = sb.String()
	return result
}
