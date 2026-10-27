package monitor

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/guptarohit/asciigraph"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"golang.org/x/term"

	"pc-diagnostic/utils"
	"pc-diagnostic/thresholds"
)

// StartMonitoring запускает цикл опроса датчиков
func StartMonitoring() {
	utils.Println(utils.Info("Режим МОНИТОРИНГА реального времени."))
	utils.Println(utils.Dim("Нажмите Enter для выхода и возврата в главное меню."))
	utils.Println("")

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		utils.Println(utils.ErrorText(fmt.Sprintf("Не удалось перевести терминал в сырой режим: %v", err)))
		return
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			printDashboard()

			if isEnterPressed() {
				utils.Println("")
				utils.Println(utils.Warning("Остановка мониторинга..."))
				return
			}
		}
	}
}

// printDashboard собирает метрики и отрисовывает их
func printDashboard() {
	clearScreen()

	// --- ПАМЯТЬ ---
	vm, _ := mem.VirtualMemory()
	swap, _ := mem.SwapMemory()

	utils.Println(utils.Header("=== ИСПОЛЬЗОВАНИЕ ПАМЯТИ ==="))
	ramData := []float64{vm.UsedPercent}
	fmt.Println(asciigraph.Plot(ramData,
		asciigraph.Caption("RAM Usage (%)"),
		asciigraph.Height(8),
		asciigraph.SeriesColors(asciigraph.Red),
	))
	utils.Printf("Использовано: %s | Всего: %.1f GB",
		utils.ColorByPercent(vm.UsedPercent),
		float64(vm.Total)/1024/1024/1024)
	utils.Println("")

	if swap.Total > 0 {
		swapData := []float64{swap.UsedPercent}
		fmt.Println(asciigraph.Plot(swapData,
			asciigraph.Caption("Swap Usage (%)"),
			asciigraph.Height(4),
			asciigraph.SeriesColors(asciigraph.Yellow),
		))
		utils.Printf("Файл подкачки: %s", utils.ColorByPercent(swap.UsedPercent))
		utils.Println("")
	}

	// --- ПРОЦЕССОР ---
	cpuPercents, err := cpu.Percent(0, true)
	if err == nil && len(cpuPercents) > 0 {
		utils.Println(utils.Header("=== ЗАГРУЗКА CPU ПО ЯДРАМ ==="))
		for i, p := range cpuPercents {
			graph := asciigraph.Plot([]float64{p},
				asciigraph.Height(5),
				asciigraph.Width(20),
				asciigraph.Caption(fmt.Sprintf("CPU %d", i+1)),
			)
			utils.Printf("%s %s", graph, utils.ColorByPercent(p))
		}
		utils.Println("")
	}

	totalCpu, _ := cpu.Percent(0, false)
	if len(totalCpu) > 0 {
		utils.Printf("Общая загрузка CPU: %s", utils.ColorByPercent(totalCpu[0]))
		utils.Println("")
	}

	// --- ТЕМПЕРАТУРА ---
	temp := getCPUTempWindows()
	if temp > 0 {
		utils.Printf("Температура CPU: %s", utils.ColorByTemp(temp))
		utils.Println("")
	} else {
		utils.Println(utils.Dim("Температура CPU: недоступна (требуются права админа/драйверы)"))
		utils.Println("")
	}

	// --- СТАТУС СИСТЕМЫ ---
	stRAM := thresholds.ForRAM(vm.UsedPercent)

	stCPU := thresholds.StatusUnknown
	if len(totalCpu) > 0 {
		stCPU = thresholds.ForCPU(totalCpu[0])
	}

	stTemp := thresholds.ForTemp(temp)
	overall := thresholds.Worst(stCPU, stRAM, stTemp)

	utils.Println(utils.Header("=== СОСТОЯНИЕ СИСТЕМЫ ==="))
	utils.Printf("CPU: %s | RAM: %s | Температура: %s | Общий статус: %s",
		statusColored(stCPU), statusColored(stRAM), statusColored(stTemp), statusColored(overall))
	utils.Println("")


	// Подсказка внизу
	utils.Println(utils.Header(strings.Repeat("=", 40)))
	utils.Println(utils.Bold("[Enter]") + utils.Dim(" - Выход в меню"))
}

// clearScreen очищает экран консоли
func clearScreen() {
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		_ = cmd.Run()
	default:
		fmt.Print("\033[H\033[2J")
	}
}

// isEnterPressed проверяет, была ли нажата клавиша Enter (неблокирующее чтение)
// В raw-режиме Enter = \r (0x0D) на Windows и Linux
func isEnterPressed() bool {
	buffer := make([]byte, 1)

	os.Stdin.SetReadDeadline(time.Now())
	n, err := os.Stdin.Read(buffer)

	if n > 0 && err == nil {
		// \r = 0x0D — Enter в raw-режиме
		return buffer[0] == '\r' || buffer[0] == '\n'
	}
	return false
}

func getCPUTempWindows() float64 {
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		`Get-WmiObject MSAcpi_ThermalZoneTemperature -Namespace root/wmi | Select-Object -ExpandProperty CurrentTemperature`)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	outStr := strings.TrimSpace(string(out))
	if outStr == "" {
		return 0
	}
	// Значение в десятых долях Кельвина
	var tempK float64
	if _, err := fmt.Sscanf(outStr, "%f", &tempK); err != nil {
		return 0
	}
	return tempK/10 - 273.15
}
// statusColored возвращает статус, окрашенный ANSI-цветом
// (зелёный/жёлтый/красный/серый — как ColorByPercent и ColorByTemp).
func statusColored(s thresholds.Status) string {
	colors := map[thresholds.Status]string{
		thresholds.StatusNormal:   "\033[32m",
		thresholds.StatusWarning:  "\033[33m",
		thresholds.StatusCritical: "\033[31m",
		thresholds.StatusUnknown:  "\033[90m",
	}
	return colors[s] + string(s) + "\033[0m"
}

