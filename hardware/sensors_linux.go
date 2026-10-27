//go:build linux

package hardware

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// GetSensorsInfo — вентиляторы и напряжения (Linux: /sys/class/hwmon, lm-sensors).
// Имя функции сверить с sensors_windows.go после переименования.
func GetSensorsInfo() string {
	dirs, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	var b strings.Builder
	fans, volts := 0, 0
	for _, dir := range dirs {
		chip := hwmonFile(dir, "name")
		fanFiles, _ := filepath.Glob(dir + "/fan*_input")
		for _, f := range fanFiles {
			if v := hwmonFile(dir, filepath.Base(f)); v != "" {
				if rpm, err := strconv.Atoi(v); err == nil && rpm > 0 {
					label := hwmonFile(dir, strings.TrimSuffix(filepath.Base(f), "_input")+"_label")
					fmt.Fprintf(&b, "Вентилятор: %s (%s) — %d об/мин\n", orDashStr(label), chip, rpm)
					fans++
				}
			}
		}
		vFiles, _ := filepath.Glob(dir + "/in*_input")
		for _, f := range vFiles {
			if v := hwmonFile(dir, filepath.Base(f)); v != "" {
				if mv, err := strconv.ParseFloat(v, 64); err == nil && mv > 0 {
					label := hwmonFile(dir, strings.TrimSuffix(filepath.Base(f), "_input")+"_label")
					fmt.Fprintf(&b, "Напряжение: %s (%s) — %.2f В\n", orDashStr(label), chip, mv/1e6)
					volts++
				}
			}
		}
	}
	if fans == 0 && volts == 0 {
		return "Вентиляторы и напряжения: данные hwmon недоступны.\nУстановите lm-sensors и выполните 'sudo sensors-detect' — данные появятся в /sys/class/hwmon."
	}
	return b.String()
}

// hwmonFile — чтение одного файла hwmon: пустая строка при недоступности.
func hwmonFile(dir, name string) string {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func orDashStr(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
