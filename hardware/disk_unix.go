// +build linux darwin

package hardware

import (
	"fmt"
	"os"
	"syscall"
	"strings"
)

// DiagnosticResult структура для хранения результатов диагностики
//type DiagnosticResult struct {
//	Name  string
//	Value string
//	Error error
//}

// DiagnoseDisk выполняет диагностику дисков для Unix‑систем
func DiagnoseDisk() DiagnosticResult {
	result := DiagnosticResult{Name: "Диски (Unix-системы)"}
	dir := "/" // Проверяем корневой раздел вместо текущей директории

	var stat syscall.Statfs_t
	err := syscall.Statfs(dir, &stat)
	if err != nil {
		result.Error = fmt.Errorf("ошибка получения информации о диске: %v", err)
		return result
	}

	// Расчёт объёмов в байтах
	total := uint64(stat.Blocks) * uint64(stat.Bsize)
	free := uint64(stat.Bavail) * uint64(stat.Bsize)
	used := total - free

	// Форматирование в гигабайтах с округлением до 2 знаков
	gb := 1024.0 * 1024.0 * 1024.0

	var resultBuilder strings.Builder
	resultBuilder.WriteString(fmt.Sprintf("Общее пространство: %.2f ГБ\n", float64(total)/gb))
	resultBuilder.WriteString(fmt.Sprintf("Свободное место: %.2f ГБ\n", float64(free)/gb))
	resultBuilder.WriteString(fmt.Sprintf("Используемое место: %.2f ГБ", float64(used)/gb))

	// Дополнительная информация о директории
	info, err := os.Stat(dir)
	if err == nil {
		resultBuilder.WriteString(fmt.Sprintf(
			"\n\nТекущая директория: %s\nРазмер: %d байт\nВремя модификации: %s",
			dir,
			info.Size(),
			info.ModTime().Format("2006-01-02 15:04:05"),
		))
	} else {
		resultBuilder.WriteString(fmt.Sprintf("\n\nНе удалось получить информацию о директории %s: %v", dir, err))
	}

	result.Value = resultBuilder.String()
	return result
}
