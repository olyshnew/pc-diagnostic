//go:build windows
// +build windows

package system

import (
	"encoding/csv"
	"fmt"
	"os/exec"
	"strings"
)

// GetDriversInfo получает информацию об установленных драйверах
func GetDriversInfo() string {
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		`[Console]::OutputEncoding=[System.Text.Encoding]::UTF8; `+
			`Get-CimInstance Win32_SystemDriver | `+
			`Select-Object DisplayName,Name,PathName,Status | `+
			`Sort-Object Name | `+
			`ConvertTo-Csv -NoTypeInformation`)

	output, err := cmd.Output()
	if err != nil {
		return "Установленные драйверы: ошибка получения (PowerShell)"
	}

	reader := csv.NewReader(strings.NewReader(strings.TrimSpace(string(output))))
	records, err := reader.ReadAll()
	if err != nil || len(records) < 2 {
		return "Установленные драйверы: не найдены"
	}

	var sb strings.Builder
	sb.WriteString("Установленные драйверы (WMI):\n")
	for _, row := range records[1:] {
		displayName := "Неизвестно"
		name := "Неизвестно"
		pathName := "Неизвестно"
		status := "Неизвестно"

		if len(row) > 0 && row[0] != "" {
			displayName = row[0]
		}
		if len(row) > 1 && row[1] != "" {
			name = row[1]
		}
		if len(row) > 2 && row[2] != "" {
			pathName = row[2]
		}
		if len(row) > 3 && row[3] != "" {
			status = row[3]
		}

		sb.WriteString(fmt.Sprintf("  %s (%s, Путь: %s, Статус: %s)\n",
			displayName, name, pathName, status))
	}

	return sb.String()
}
