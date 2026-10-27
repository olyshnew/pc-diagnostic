package hardware

import (
	"fmt"
	"github.com/shirou/gopsutil/v3/mem"
	"sync"
	"strings"
)

// DiagnoseMemory выполняет расширенную диагностику памяти с информацией о файле подкачки и предупреждениями
func DiagnoseMemory() DiagnosticResult {
	result := DiagnosticResult{Name: "Память"}
	var sb strings.Builder
	var wg sync.WaitGroup
	wg.Add(2)

	// Каналы для получения результатов
	vmChan := make(chan *mem.VirtualMemoryStat, 1)
	swapChan := make(chan *mem.SwapMemoryStat, 1)
	errChan := make(chan error, 2)

	// Параллельное получение данных о RAM
	go func() {
		defer wg.Done()
		vm, err := mem.VirtualMemory()
		if err != nil {
			errChan <- fmt.Errorf("ошибка получения информации о памяти: %v", err)
			return
		}
		vmChan <- vm
	}()

	// Параллельное получение данных о swap
	go func() {
		defer wg.Done()
		swap, err := mem.SwapMemory()
		if err != nil {
			errChan <- fmt.Errorf("ошибка получения информации о файле подкачки: %v", err)
			return
		}
		swapChan <- swap
	}()

	wg.Wait()
	close(vmChan)
	close(swapChan)
	close(errChan)

	// Обработка ошибок
	if len(errChan) > 0 {
		var errors []string
		for err := range errChan {
			errors = append(errors, err.Error())
		}
		result.Error = fmt.Errorf(strings.Join(errors, "; "))
		return result
	}

	// Получение результатов
	vm := <-vmChan
	var swap *mem.SwapMemoryStat
	select {
	case swap = <-swapChan:
	default:
		swap = nil
	}

	// Конвертируем байты в гигабайты
	totalRAM := float64(vm.Total) / 1024 / 1024 / 1024
	freeRAM := float64(vm.Free) / 1024 / 1024 / 1024
	availableRAM := float64(vm.Available) / 1024 / 1024 / 1024

	var totalSwapGB, usedSwapGB float64
	var swapUsagePercent float64

	if swap != nil {
		totalSwapGB = float64(swap.Total) / 1024 / 1024 / 1024
		usedSwapGB = float64(swap.Used) / 1024 / 1024 / 1024
		swapUsagePercent = swap.UsedPercent
	}

	// Выводим полную информацию о памяти
	sb.WriteString(fmt.Sprintf("  Всего оперативной памяти (RAM): %.2f ГБ\n", totalRAM))
	sb.WriteString(fmt.Sprintf("  Свободно (Free): %.2f ГБ\n", freeRAM))
	sb.WriteString(fmt.Sprintf("  Доступно (Available, включая кэши): %.2f ГБ\n", availableRAM))
	sb.WriteString(fmt.Sprintf("  Используется RAM: %.2f%%\n", vm.UsedPercent))

	// Информация о файле подкачки
	if swap != nil {
		sb.WriteString("\n  Файл подкачки (pagefile):\n")
		sb.WriteString(fmt.Sprintf("    Всего: %.2f ГБ\n", totalSwapGB))
		sb.WriteString(fmt.Sprintf("    Использовано: %.2f ГБ\n", usedSwapGB))
		sb.WriteString(fmt.Sprintf("    Использование: %.2f%%\n", swapUsagePercent))
	} else {
		sb.WriteString("\n  Файл подкачки: не обнаружен или недоступен\n")
	}

	// Предупреждения при критическом использовании
	if vm.UsedPercent > 90 {
		sb.WriteString("\n  ПРЕДУПРЕЖДЕНИЕ: высокая загрузка оперативной памяти (>90%!)\n")
	}
	if swap != nil && swapUsagePercent > 80 {
		sb.WriteString("  ПРЕДУПРЕЖДЕНИЕ: высокое использование файла подкачки (>80%!)\n")
	}

	result.Value = sb.String()
	return result
}
