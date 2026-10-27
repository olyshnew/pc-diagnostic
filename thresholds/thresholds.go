// Package thresholds реализует систему статусов Normal/Warning/Critical/Unknown
// с пороговыми значениями из config.json.
package thresholds
import (
	"encoding/json"
	"os"
	"sync"
)

// Status — статус состояния компонента.
type Status string

const (
	StatusNormal   Status = "Normal"
	StatusWarning  Status = "Warning"
	StatusCritical Status = "Critical"
	StatusUnknown  Status = "Unknown"
)

type limit struct {
	Warning  float64 `json:"warning"`
	Critical float64 `json:"critical"`
}

type thresholdConfig struct {
	CPU  limit `json:"cpu"`
	RAM  limit `json:"ram"`
	Temp limit `json:"temp"`
}

var (
	once   sync.Once
	loaded thresholdConfig
)

// load читает config.json один раз за время работы программы.
// Отсутствующие или некорректные значения заменяются значениями по умолчанию —
// программа не должна падать из-за опечатки в конфиге.
func load() thresholdConfig {
	once.Do(func() {
		loaded = thresholdConfig{
			CPU:  limit{Warning: 70, Critical: 90},
			RAM:  limit{Warning: 80, Critical: 95},
			Temp: limit{Warning: 70, Critical: 85},
		}

		data, err := os.ReadFile("config.json")
		if err != nil {
			return // файла нет — работаем на дефолтах
		}

		var wrapper struct {
			Thresholds *thresholdConfig `json:"thresholds"`
		}
		if json.Unmarshal(data, &wrapper) != nil || wrapper.Thresholds == nil {
			return // секция отсутствует или битая — дефолты
		}

		t := wrapper.Thresholds
		if t.CPU.Warning > 0 {
			loaded.CPU.Warning = t.CPU.Warning
		}
		if t.CPU.Critical > 0 {
			loaded.CPU.Critical = t.CPU.Critical
		}
		if t.RAM.Warning > 0 {
			loaded.RAM.Warning = t.RAM.Warning
		}
		if t.RAM.Critical > 0 {
			loaded.RAM.Critical = t.RAM.Critical
		}
		if t.Temp.Warning > 0 {
			loaded.Temp.Warning = t.Temp.Warning
		}
		if t.Temp.Critical > 0 {
			loaded.Temp.Critical = t.Temp.Critical
		}
	})
	return loaded
}

// eval возвращает статус по значению и порогам.
func eval(v float64, l limit) Status {
	if v >= l.Critical {
		return StatusCritical
	}
	if v >= l.Warning {
		return StatusWarning
	}
	return StatusNormal
}

// ForCPU оценивает загрузку процессора (0–100%).
func ForCPU(pct float64) Status { return eval(pct, load().CPU) }

// ForRAM оценивает занятость оперативной памяти (0–100%).
func ForRAM(pct float64) Status { return eval(pct, load().RAM) }

// ForTemp оценивает температуру CPU. Значение 0 и ниже трактуется
// как «датчик недоступен» — статус Unknown.
func ForTemp(temp float64) Status {
	if temp <= 0 {
		return StatusUnknown
	}
	return eval(temp, load().Temp)
}

// Worst сводит частные статусы в общий: критичное важнее предупреждения,
// предупреждение важнее нормы. Unknown не понижает общий статус, если
// хотя бы по одному компоненту есть данные.
func Worst(list ...Status) Status {
	hasWarning, hasUnknown, hasNormal := false, false, false
	for _, s := range list {
		switch s {
		case StatusCritical:
			return StatusCritical
		case StatusWarning:
			hasWarning = true
		case StatusUnknown:
			hasUnknown = true
		case StatusNormal:
			hasNormal = true
		}
	}
	switch {
	case hasWarning:
		return StatusWarning
	case hasNormal:
		return StatusNormal
	case hasUnknown:
		return StatusUnknown
	default:
		return StatusUnknown
	}
}
