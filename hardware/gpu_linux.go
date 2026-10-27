// +build !windows

package hardware

import (
	"fmt"
	"os/exec"
	"strings"
)

// DiagnosticResult структура для хранения результатов диагностики
//type DiagnosticResult struct {
//	Name  string
//	Value string
//	Error error
//}

// DiagnoseGPU получает информацию о видеокарте через lspci
func DiagnoseGPU() DiagnosticResult {
	result := DiagnosticResult{Name: "Видеокарта (Linux)"}

	cmd := exec.Command("lspci", "-v")
	out, err := cmd.Output()
	if err != nil {
		result.Error = fmt.Errorf("ошибка выполнения lspci: %v", err)
		return result
	}

	lines := strings.Split(string(out), "\n")
	var sb strings.Builder
	sb.WriteString("Обнаруженные GPU:\n")

	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), "vga") ||
			strings.Contains(strings.ToLower(line), "3d controller") {
			sb.WriteString(fmt.Sprintf("  %s\n", strings.TrimSpace(line)))
		}
	}

	if sb.Len() == 13 { // "Обнаруженные GPU:\n" — 13 символов
		sb.WriteString("  GPU не обнаружен\n")
	}

	result.Value = sb.String()
	return result
}
