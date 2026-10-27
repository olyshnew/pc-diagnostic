//go:build linux

package system

import (
	"fmt"
	"os/exec"
	"strings"
)

// CheckIntegrity — проверка целостности системных файлов (Linux).
// Аналог sfc /scannow: debsums (Debian/Ubuntu) или rpm -Va (RPM-системы).
func CheckSystemIntegrity() string {
	if path, err := exec.LookPath("debsums"); err == nil {
		out, err := exec.Command(path, "-s").CombinedOutput()
		if err != nil && len(out) == 0 {
			return fmt.Sprintf("Проверка целостности (debsums): ошибка запуска: %v", err)
		}
		text := strings.TrimSpace(string(out))
		if text == "" {
			return "Проверка целостности (debsums): все проверенные файлы соответствуют пакетам, ошибок нет"
		}
		lines := strings.Split(text, "\n")
		if len(lines) > 20 {
			return fmt.Sprintf("Проверка целостности (debsums): обнаружены отличия, первые 20 строк:\n%s\n(показано 20 из %d)", strings.Join(lines[:20], "\n"), len(lines))
		}
		return "Проверка целостности (debsums):\n" + text
	}
	if path, err := exec.LookPath("rpm"); err == nil {
		out, _ := exec.Command(path, "-Va").CombinedOutput()
		text := strings.TrimSpace(string(out))
		if text == "" {
			return "Проверка целостности (rpm -Va): отклонений не обнаружено"
		}
		return "Проверка целостности (rpm -Va), отличия:\n" + text
	}
	return "Проверка целостности: инструмент не найден (установите debsums — Debian/Ubuntu; на RPM-системах доступна rpm -Va)"
}
