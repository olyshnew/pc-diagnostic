package monitor

import (
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// Stats — структура для передачи метрик в JSON
type Stats struct {
	Timestamp   string    `json:"timestamp"`
	CPUTotal    float64   `json:"cpu_total"`
	CPUCores    []float64 `json:"cpu_cores"`
	RAMUsedPct  float64   `json:"ram_used_pct"`
	RAMUsedGB   float64   `json:"ram_used_gb"`
	RAMTotalGB  float64   `json:"ram_total_gb"`
	SwapUsedPct float64   `json:"swap_used_pct"`
	SwapUsedGB  float64   `json:"swap_used_gb"`
	SwapTotalGB float64   `json:"swap_total_gb"`
	CPUTemp     float64   `json:"cpu_temp"`
}

// Кэш температуры — обновляется в фоне каждые 5 секунд
var (
	cachedTemp     float64
	tempMu         sync.Mutex
	tempInitialized bool
)

func init() {
	// Фоновое обновление температуры
	go func() {
		for {
			t := getCPUTempWindows()
			tempMu.Lock()
			cachedTemp = t
			tempInitialized = true
			tempMu.Unlock()
			time.Sleep(5 * time.Second)
		}
	}()
}

// GetStats собирает текущие метрики системы
func GetStats() Stats {
	stats := Stats{
		Timestamp: time.Now().Format("15:04:05"),
	}

	vm, _ := mem.VirtualMemory()
	if vm != nil {
		stats.RAMUsedPct = vm.UsedPercent
		stats.RAMUsedGB = float64(vm.Used) / 1024 / 1024 / 1024
		stats.RAMTotalGB = float64(vm.Total) / 1024 / 1024 / 1024
	}

	swap, _ := mem.SwapMemory()
	if swap != nil {
		stats.SwapUsedPct = swap.UsedPercent
		stats.SwapUsedGB = float64(swap.Used) / 1024 / 1024 / 1024
		stats.SwapTotalGB = float64(swap.Total) / 1024 / 1024 / 1024
	}

	cpuCores, _ := cpu.Percent(0, true)
	stats.CPUCores = cpuCores

	cpuTotal, _ := cpu.Percent(0, false)
	if len(cpuTotal) > 0 {
		stats.CPUTotal = cpuTotal[0]
	}

	// Берём температуру из кэша — мгновенно, без задержки
	tempMu.Lock()
	stats.CPUTemp = cachedTemp
	tempMu.Unlock()

	return stats
}
