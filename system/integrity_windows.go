package system

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"unicode/utf16"
)

func CheckSystemIntegrity() string {
	var result strings.Builder
	result.WriteString("=== ПРОВЕРКА ЦЕЛОСТНОСТИ СИСТЕМНЫХ ФАЙЛОВ ===\n")

	switch runtime.GOOS {
	case "windows":
		result.WriteString("Запуск sfc /scannow...\n(Это может занять 5-15 минут. Требуются права администратора)\n\n")
		cmd := exec.Command("cmd", "/c", "sfc", "/scannow")
		raw, err := cmd.CombinedOutput()
		if err != nil {
			result.WriteString(fmt.Sprintf("  Ошибка выполнения: %v\n", err))
		}
		text := decodeSFCOutput(raw)
		lines := strings.Split(text, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			result.WriteString("  " + line + "\n")
		}
	case "linux":
		result.WriteString(" Linux: аналог 'sfc' зависит от дистрибутива.\n")
		result.WriteString(" Debian/Ubuntu: выполните вручную 'sudo debsums -ca'\n")
		result.WriteString(" RHEL/Fedora: выполните вручную 'rpm -Va'\n")
	default:
		result.WriteString(" Платформа не поддерживается.\n")
	}

	return result.String()
}

func decodeSFCOutput(raw []byte) string {
	if len(raw) < 2 {
		return string(raw)
	}

	start := 0
	if raw[0] == 0xFF && raw[1] == 0xFE {
		start = 2
	}

	var u16 []uint16
	for i := start; i+1 < len(raw); i += 2 {
		u16 = append(u16, uint16(raw[i])|uint16(raw[i+1])<<8)
	}
	decoded := string(utf16.Decode(u16))

	// Нормализуем все переносы
	decoded = strings.ReplaceAll(decoded, "\r\n", "\n")
	decoded = strings.ReplaceAll(decoded, "\r", "\n")

	lines := strings.Split(decoded, "\n")
	var result []string
	var lastProgress string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Все строки прогресса — запоминаем только последнюю
		if strings.HasPrefix(line, "Проверка ") && strings.HasSuffix(line, "завершена.") {
			lastProgress = line
			continue
		}

		// Дедуплицируем остальные строки
		dup := false
		for _, existing := range result {
			if existing == line {
				dup = true
				break
			}
		}
		if !dup {
			result = append(result, line)
		}
	}

	// Вставляем последнюю строку прогресса перед финальным результатом
	if lastProgress != "" {
		insertAt := len(result)
		for i, l := range result {
			if strings.Contains(l, "Защита ресурсов") {
				insertAt = i
				break
			}
		}
		if insertAt <= len(result) {
			result = append(result[:insertAt], append([]string{lastProgress}, result[insertAt:]...)...)
		} else {
			result = append(result, lastProgress)
		}
	}

	return strings.Join(result, "\n")
}
