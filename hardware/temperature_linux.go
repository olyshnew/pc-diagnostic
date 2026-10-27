//go:build linux
package hardware

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DiagnoseTemperature читает температуру из sysfs
func DiagnoseTemperature() DiagnosticResult {
	result := DiagnosticResult{Name: "Температура (Linux)"}
	var sb strings.Builder

	sb.WriteString("Температура (Linux):\n")

	basePath := "/sys/class/thermal"

	thermalDirs, err := os.ReadDir(basePath)
	if err != nil {
		result.Error = fmt.Errorf("ошибка чтения thermal‑директории: %v", err)
		return result
	}

	foundSensors := false

	for _, dir := range thermalDirs {
		if !dir.IsDir() || !strings.HasPrefix(dir.Name(), "thermal_zone") {
			continue
	}

		typePath := filepath.Join(basePath, dir.Name(), "type")
		tempPath := filepath.Join(basePath, dir.Name(), "temp")

		typeData, err := os.ReadFile(typePath)
		if err != nil {
			continue
	}

		tempData, err := os.ReadFile(tempPath)
		if err != nil {
			continue
	}

		sensorType := strings.TrimSpace(string(typeData))
		tempStr := strings.TrimSpace(string(tempData))

		tempC, err := strconv.ParseFloat(tempStr, 64)
		if err != nil {
			continue
	}
	tempC /= 1000 // конвертируем из миллиградусов

	sb.WriteString(fmt.Sprintf("  %s: %.1f°C\n", sensorType, tempC))
	foundSensors = true
	}

	if !foundSensors {
		sb.WriteString("  Датчики температуры не обнаружены или недоступны\n")
	}

	result.Value = sb.String()
	return result
}
