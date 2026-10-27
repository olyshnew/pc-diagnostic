package utils

import (
	"fmt"
	"strings"
)

// CheckError проверяет ошибки и возвращает сообщение об ошибке, если она есть
// Если ошибка отсутствует, возвращает пустую строку
func CheckError(err error, message string) string {
	if err != nil {
		return fmt.Sprintf("Ошибка: %s - %v", message, err)
	}
	return ""
}

// Print выводит строку без перевода строки и без цвета.
// Используется для подсказок ввода в стиле «Номер архива: » —
// курсор остаётся на той же строке.
func Print(s string) {
	fmt.Print(s)
}

// PrintSeparator возвращает строку с разделителем
func PrintSeparator() string {
	return "\n" + strings.Repeat("-", 30)
}

// FormatBytes форматирует байты в читаемый вид (КБ, МБ, ГБ, ТБ)
func FormatBytes(bytes int64) string {
	const unit = 1024
	units := []string{"B", "KB", "MB", "GB", "TB"}

	if bytes < unit {
		return fmt.Sprintf("%d %s", bytes, units[0])
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
		if exp >= len(units)-1 { // защита от выхода за границы массива
			break
		}
	}

	// Ограничиваем максимальную единицу измерения (TB)
	if exp+1 >= len(units) {
		exp = len(units) - 2
	}

	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp+1])
}
