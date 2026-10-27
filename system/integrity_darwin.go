//go:build darwin

package system

import (
	"os/exec"
	"strings"
)

// CheckSystemIntegrity — проверка целостности системных компонентов (macOS).
// Прямого аналога sfc /scannow нет: системные файлы защищает SIP.
func CheckSystemIntegrity() string {
	var b strings.Builder
	b.WriteString("Проверка целостности системы (macOS):\n")

	out, err := exec.Command("csrutil", "status").Output()
	if err == nil {
		status := strings.TrimSpace(string(out))
		b.WriteString("  SIP (System Integrity Protection): " + status + "\n")
		if strings.Contains(status, "disabled") {
			b.WriteString("  ВНИМАНИЕ: SIP отключён — системные файлы не защищены\n")
		}
	} else {
		b.WriteString("  SIP: статус получить не удалось\n")
	}

	b.WriteString("  Примечание: аналога sfc /scannow в macOS нет — целостность\n")
	b.WriteString("  системных файлов защищает SIP; восстановление тома выполняется\n")
	b.WriteString("  через Первую помощь (First Aid) в Дисковой утилите.\n")
	return b.String()
}
