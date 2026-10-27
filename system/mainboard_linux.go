//go:build linux

package system

import (
	"fmt"
	"os"
	"strings"
)

// sysFile — чтение sysfs-файла без ошибок: пустая строка при недоступности.
func sysFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(data))
	if s == "Unknown" || s == "Not Specified" || s == "Default string" || s == "To Be Filled By O.E.M." {
		return ""
	}
	return s
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// GetMainboardInfo — материнская плата и BIOS/UEFI (Linux, /sys/class/dmi/id).
func GetMainboardInfo() string {
	var b strings.Builder
	vendor := sysFile("/sys/class/dmi/id/board_vendor")
	name := sysFile("/sys/class/dmi/id/board_name")
	fmt.Fprintln(&b, "Материнская плата:")
	if vendor != "" || name != "" {
		fmt.Fprintf(&b, "  Производитель: %s\n", orDash(vendor))
		fmt.Fprintf(&b, "  Модель: %s\n", orDash(name))
		if v := sysFile("/sys/class/dmi/id/board_version"); v != "" {
			fmt.Fprintf(&b, "  Версия: %s\n", v)
		}
	} else {
		fmt.Fprintln(&b, "  Не удалось получить информацию (/sys/class/dmi/id недоступен)")
	}
	fmt.Fprintln(&b, "BIOS/UEFI:")
	bv, bn := sysFile("/sys/class/dmi/id/bios_vendor"), sysFile("/sys/class/dmi/id/bios_version")
	if bv != "" || bn != "" {
		fmt.Fprintf(&b, "  Производитель: %s\n", orDash(bv))
		fmt.Fprintf(&b, "  Версия: %s\n", orDash(bn))
		if d := sysFile("/sys/class/dmi/id/bios_date"); d != "" {
			fmt.Fprintf(&b, "  Дата: %s\n", d)
		}
	} else {
		fmt.Fprintln(&b, "  Не удалось получить информацию")
	}
	return b.String()
}
