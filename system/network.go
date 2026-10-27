package system

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// DiagnosticResult структура для хранения результатов диагностики
type DiagnosticResult struct {
	Name  string
	Value string
	Error error
}

// isWindows проверяет, работает ли система на Windows
func isWindows() bool {
	return runtime.GOOS == "windows"
}

// getDNSInfo получает информацию о DNS‑серверах
func getDNSInfo() (string, error) {
	if isWindows() {
		cmd := exec.Command("ipconfig", "/all")
		output, err := cmd.Output()
		if err != nil {
			return "", err
	}

		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "DNS Servers") || strings.Contains(line, "DNS‑серверы") {
				parts := strings.Split(line, ":")
				if len(parts) > 1 {
					dns := strings.TrimSpace(parts[1])
					return dns, nil
			}
		}
	}
	} else {
		// Linux: читаем /etc/resolv.conf
		data, err := os.ReadFile("/etc/resolv.conf")
		if err != nil {
			return "", err
	}

		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "nameserver") {
				parts := strings.Fields(line)
				if len(parts) > 1 {
					return parts[1], nil
			}
		}
	}
	}
	return "не найден", nil
}

// DiagnoseNetwork выполняет диагностику сетевых настроек
func DiagnoseNetwork() DiagnosticResult {
	result := DiagnosticResult{Name: "Сетевые настройки"}
	var sb strings.Builder

	dns, err := getDNSInfo()
	if err != nil {
		sb.WriteString(fmt.Sprintf("  DNS: ошибка получения — %v\n", err))
	} else {
		sb.WriteString(fmt.Sprintf("  DNS: %s\n", dns))
	}

	result.Value = sb.String()
	return result
}
