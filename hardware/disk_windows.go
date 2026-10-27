// +build windows
package hardware

import (
	"fmt"
	"github.com/shirou/gopsutil/v3/disk"
	"strings"
)

// DiagnoseDisk получает информацию о дисках в системе
func DiagnoseDisk() DiagnosticResult {
	result := DiagnosticResult{Name: "Диски"}
	var sb strings.Builder

	//sb.WriteString("Диски:\n")

	partitions, err := disk.Partitions(true)
	if err != nil {
		result.Error = fmt.Errorf("ошибка получения информации о разделах: %v", err)
		return result
	}

	for _, partition := range partitions {
		usage, err := disk.Usage(partition.Mountpoint)
		if err != nil {
			sb.WriteString(fmt.Sprintf("  Раздел %s: ошибка получения информации об использовании: %v\n",
				partition.Device, err))
		continue
	}

	totalGB := float64(usage.Total) / 1024 / 1024 / 1024
	freeGB := float64(usage.Free) / 1024 / 1024 / 1024
	usedGB := float64(usage.Used) / 1024 / 1024 / 1024

	// Ручной расчёт процентов — в gopsutil нет полей FreePercent/UsedPercent
	freePercent := (float64(usage.Free) / float64(usage.Total)) * 100
	usedPercent := (float64(usage.Used) / float64(usage.Total)) * 100

	sb.WriteString(fmt.Sprintf("  Устройство: %s\n", partition.Device))
	sb.WriteString(fmt.Sprintf("    Точка монтирования: %s\n", partition.Mountpoint))
	sb.WriteString(fmt.Sprintf("    Тип файловой системы: %s\n", partition.Fstype))
	sb.WriteString(fmt.Sprintf("    Всего: %.2f ГБ\n", totalGB))
	sb.WriteString(fmt.Sprintf("    Свободно: %.2f ГБ (%.2f%%)\n", freeGB, freePercent))
	sb.WriteString(fmt.Sprintf("    Использовано: %.2f ГБ (%.2f%%)\n", usedGB, usedPercent))

	// Предупреждение при низком свободном месте
	if freePercent < 10 {
		sb.WriteString("    ПРЕДУПРЕЖДЕНИЕ: мало свободного места (<10%)!\n")
	}

	sb.WriteString("\n")
	}

	result.Value = sb.String()
	return result
}
