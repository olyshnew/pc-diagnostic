//go:build windows

package hardware

import (
	"encoding/csv"
	"fmt"
	"os/exec"
	"strings"
)

func DiagnoseGPU() DiagnosticResult {
	result := DiagnosticResult{Name: "Видеокарта"}

	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		`Get-CimInstance Win32_VideoController | Select-Object Name,AdapterRAM,DriverVersion | ConvertTo-Csv -NoTypeInformation`)
	output, err := cmd.Output()
	if err != nil {
		result.Error = fmt.Errorf("PowerShell: %v", err)
		return result
	}

	reader := csv.NewReader(strings.NewReader(strings.TrimSpace(string(output))))
	records, err := reader.ReadAll()
	if err != nil || len(records) < 2 {
		result.Value = "Видеокарта: не обнаружена\n"
		return result
	}

	var sb strings.Builder
	for i, row := range records[1:] { // пропускаем заголовок
		name := "Неизвестно"
		ram := "—"
		driver := "—"
		if len(row) > 0 && row[0] != "" {
			name = row[0]
		}
		if len(row) > 1 && row[1] != "" {
			//ramMB, _ := parseFloatSafe(row[1])
			//ram = fmt.Sprintf("%.0f МБ", ramMB)
			ramBytes, _ := parseFloatSafe(row[1])
ramMB := ramBytes / 1024 / 1024
ram = fmt.Sprintf("%.0f МБ", ramMB)

		}
		if len(row) > 2 && row[2] != "" {
			driver = row[2]
		}
		sb.WriteString(fmt.Sprintf("  Видеокарта %d:\n", i+1))
		sb.WriteString(fmt.Sprintf("    Название: %s\n", name))
		sb.WriteString(fmt.Sprintf("    Видеопамять: %s\n", ram))
		sb.WriteString(fmt.Sprintf("    Версия драйвера: %s\n", driver))
	}

	result.Value = sb.String()
	return result
}

func parseFloatSafe(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}
