package hardware

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// GetSensorsInfo собирает скорости вентиляторов и напряжения на компонентах.
// Источник — WMI-интерфейс LibreHardwareMonitor (резерв: OpenHardwareMonitor).
//
// Условия работы: LibreHardwareMonitor запущен от имени администратора,
// в нём включено Options → Enable WMI, и он остаётся открытым
// на время диагностики.
func GetSensorsInfo() string {
	if runtime.GOOS != "windows" {
		return "Вентиляторы и напряжения: доступно только на Windows"
	}

	fans, volts, ns := querySensorsOnce()

	if ns == "" {
		return "Вентиляторы и напряжения: поставщик данных не найден.\n" +
			"Стандартный WMI Windows не отдаёт обороты вентиляторов и напряжения.\n" +
			"Установите LibreHardwareMonitor (github.com/LibreHardwareMonitor/LibreHardwareMonitor),\n" +
			"запустите его от имени администратора и включите Options → Enable WMI.\n" +
			"После этого LibreHardwareMonitor должен оставаться запущенным на время диагностики."
	}

	var result strings.Builder
	result.WriteString("Вентиляторы и напряжения:\n")
	result.WriteString(fmt.Sprintf("Источник данных: WMI, пространство имён %s\n\n", ns))

	if len(fans) == 0 {
		result.WriteString("Вентиляторы: датчики не обнаружены " +
			"(проверьте, что LibreHardwareMonitor запущен от администратора)\n")
	} else {
		result.WriteString("Вентиляторы (об/мин):\n")
		for _, r := range fans {
			mark := ""
			if r.value == 0 {
				mark = "  ← 0: разъём не используется или вентилятор не вращается"
			}
			result.WriteString(fmt.Sprintf("  %-30s %6.0f%s\n", r.name, r.value, mark))
		}
	}

	result.WriteString("\n")

	if len(volts) == 0 {
		result.WriteString("Напряжения: датчики не обнаружены\n")
	} else {
		result.WriteString("Напряжения:\n")
		for _, r := range volts {
			result.WriteString(fmt.Sprintf("  %-30s %8.3f В\n", r.name, r.value))
		}
	}

	return result.String()
}

type lhmSensor struct {
	Name       string  `json:"Name"`
	SensorType string  `json:"SensorType"`
	Value      float64 `json:"Value"`
}

type sensorReading struct {
	name  string
	value float64
}

// querySensorsOnce опрашивает оба известных пространства имён
// и возвращает прочитанные сенсоры и имя сработавшего пространства.
func querySensorsOnce() (fans []sensorReading, volts []sensorReading, namespace string) {
	namespaces := []string{"root/LibreHardwareMonitor", "root/OpenHardwareMonitor"}

	for _, ns := range namespaces {
		out, err := runSensorsPS(ns)
		if err != nil {
			continue // пространства имён нет — пробуем следующее
		}

		trimmed := strings.TrimSpace(out)
		if trimmed == "" || trimmed == "[]" {
			continue // пространство есть, но подходящих сенсоров нет
		}

		var raw []lhmSensor
		if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
			continue
		}

		for _, s := range raw {
			switch s.SensorType {
			case "Fan":
				fans = append(fans, sensorReading{name: s.Name, value: s.Value})
			case "Voltage":
				// нулевые каналы напряжения — почти всегда неиспользуемые
				// разъёмы материнской платы, не захламляем отчёт
				if s.Value != 0 {
					volts = append(volts, sensorReading{name: s.Name, value: s.Value})
				}
			}
		}
		return fans, volts, ns
	}

	return nil, nil, ""
}

// runSensorsPS делает один WMI-запрос через PowerShell.
// $ErrorActionPreference='Stop' превращает отсутствие пространства имён
// в ошибку, которую querySensorsOnce перехватывает и переключается на резерв.
func runSensorsPS(namespace string) (string, error) {
	ps := `$ErrorActionPreference = 'Stop'; ` +
		`$s = Get-CimInstance -Namespace ` + namespace + ` -ClassName Sensor | ` +
		`Where-Object { $_.SensorType -in 'Fan','Voltage' -and $null -ne $_.Value }; ` +
		`@($s | Select-Object Name, SensorType, Value) | ConvertTo-Json -Compress`

	full := `[Console]::OutputEncoding=[Text.Encoding]::UTF8; ` + ps
	out, err := exec.Command("powershell", "-NoProfile", "-Command", full).Output()
	return string(out), err
}
