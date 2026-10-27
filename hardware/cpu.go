package hardware

import (
	"fmt"
	"runtime"
)

// GetCPUInfo получает информацию о процессоре
func GetCPUInfo() DiagnosticResult {
	result := DiagnosticResult{
		Name: "Процессор",
	}

	result.Value = fmt.Sprintf(
		"Архитектура: %s\nКоличество ядер: %d\nОС: %s",
		runtime.GOARCH,
		runtime.NumCPU(),
		runtime.GOOS,
	)

	return result
}
