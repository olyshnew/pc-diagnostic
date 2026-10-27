// +build windows
package system

import (
	"os"        // <-- Добавлено
	"os/exec"
	"runtime"
	"strings"
)

// +build windows

func getAutostartWindows() ([]string, error) {
	var paths = []string{
		`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`,
		`HKCU\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`,
	}
	var items []string
	for _, path := range paths {
		cmd := exec.Command("reg", "query", path)
		out, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "REG_SZ") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					items = append(items, parts[0])
				}
			}
		}
	}
	return items, nil
}

// +build linux

func getAutostartLinux() ([]string, error) {
	var items []string
	systemdPaths := []string{"/etc/systemd/system/", "/lib/systemd/user/"}
	userPath := ""

	if home, err := os.UserHomeDir(); err == nil { // Теперь os определен
		userPath = home + "/.config/systemd/user/"
	}

	// Системные сервисы
	for _, p := range systemdPaths {
		out, _ := exec.Command("ls", p).Output()
		files := strings.Fields(string(out))
		for _, f := range files {
			if strings.HasSuffix(f, ".service") {
				items = append(items, "Service: "+f)
			}
		}
	}
	// Пользовательские сервисы
	if userPath != "" {
		if out, err := exec.Command("ls", userPath).Output(); err == nil {
			files := strings.Fields(string(out))
			for _, f := range files {
				if strings.HasSuffix(f, ".service") {
					items = append(items, "User Service: "+f)
				}
			}
		}
	}
	return items, nil
}

// GetAutostartInfo получает информацию об автозагрузке
func GetAutostartInfo() string {
	var result strings.Builder
	result.WriteString("=== ПРОГРАММЫ В АВТОЗАГРУЗКЕ ===\n")

	switch runtime.GOOS {
	case "windows":
		items, err := getAutostartWindows()
		if err != nil {
			return "Автозагрузка: ошибка получения информации (" + err.Error() + ")"
		}
		for _, i := range items {
			result.WriteString(" " + i + "\n")
		}
	case "linux":
		items, err := getAutostartLinux()
		if err != nil {
			return "Автозагрузка: ошибка получения информации (" + err.Error() + ")"
		}
		for _, i := range items {
			result.WriteString(" " + i + "\n")
		}
	default:
		result.WriteString(" Автозагрузка: платформа не поддерживается\n")
	}

	return result.String()
}
