package hardware

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// getDNSServers получает список DNS‑серверов
func getDNSServers() ([]string, error) {
	var servers []string

	if runtime.GOOS == "windows" {
		cmd := exec.Command("powershell", "-Command",
			"(Get-DnsClientServerAddress -AddressFamily IPv4).ServerAddresses")
		output, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("ошибка выполнения PowerShell команды: %v", err)
	}

		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				servers = append(servers, line)
		}
	}
	} else {
		// Linux: читаем /etc/resolv.conf
		data, err := os.ReadFile("/etc/resolv.conf")
		if err != nil {
			return nil, err
	}

		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "nameserver") {
				parts := strings.Fields(line)
				if len(parts) > 1 {
					servers = append(servers, parts[1])
			}
		}
	}
	}

	if len(servers) == 0 {
		return nil, fmt.Errorf("DNS‑серверы не найдены")
	}
	return servers, nil
}

// DiagnoseNetwork получает информацию о сетевых настройках
func DiagnoseNetwork() DiagnosticResult {
	result := DiagnosticResult{Name: "Сеть"}
	var sb strings.Builder

	dnsServers, err := getDNSServers()
	if err != nil {
		sb.WriteString(fmt.Sprintf("  DNS‑серверы: ошибка получения — %v\n", err))
	} else {
		sb.WriteString("  DNS‑серверы:\n")
		for _, server := range dnsServers {
			sb.WriteString(fmt.Sprintf("    %s\n", server))
		}
	}

	result.Value = sb.String()
	return result
}
