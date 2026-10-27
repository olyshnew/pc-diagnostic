//go:build linux

package system

import (
	"os/exec"
	"strings"
)

// GetDiskInfo — расширенная информация о дисках (Linux: lsblk + df).
// Имя функции сверить с disk_windows.go после переименования.
func GetDiskInfo() string {
	out, err := exec.Command("lsblk", "-o", "NAME,SIZE,TYPE,FSTYPE,MOUNTPOINT,MODEL").Output()
	if err != nil {
		return "Диски: lsblk недоступен"
	}
	var b strings.Builder
	b.WriteString("Диски (lsblk):\n")
	b.WriteString(strings.TrimSpace(string(out)))
	if dfOut, err := exec.Command("df", "-h", "--output=source,fstype,size,used,avail,pcent,target").Output(); err == nil {
		b.WriteString("\nСмонтированные файловые системы (df):\n")
		b.WriteString(strings.TrimSpace(string(dfOut)))
	}
	return b.String()
}
