//go:build darwin

package system

import (
	"os/exec"
	"strings"
)

// GetDiskInfo — расширенная информация о дисках (macOS: diskutil list + df).
func GetDiskInfo() string {
	var b strings.Builder
	if out, err := exec.Command("diskutil", "list").Output(); err == nil {
		b.WriteString("Диски (diskutil list):\n")
		b.WriteString(strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("df", "-h").Output(); err == nil {
		b.WriteString("\n\nСмонтированные файловые системы (df):\n")
		b.WriteString(strings.TrimSpace(string(out)))
	}
	return b.String()
}
