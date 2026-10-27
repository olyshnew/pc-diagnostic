package utils

import (
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
)

// Ping выполняет пинг до хоста и возвращает среднее время отклика
func Ping(host string, count int) (string, error) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("ping", host, "-n", strconv.Itoa(count))
	case "darwin", "linux":
		cmd = exec.Command("ping", host, "-c", strconv.Itoa(count))
	default:
		return "", fmt.Errorf("unsupported platform")
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Если пакеты потеряны, ping вернет ошибку выхода, но нам нужен текст
	}

	// Парсинг для Linux/macOS
	reUnix := regexp.MustCompile(`rtt [\w/]* = [\d.]+/([\d.]+)/[\d.]+/[\d.]+`)
	matchesUnix := reUnix.FindStringSubmatch(string(output))
	if len(matchesUnix) > 1 {
		return "Успех. Среднее время: " + matchesUnix[1] + " мс", nil
	}

	// Парсинг для Windows — поддерживает и русский, и английский вывод:
	//   Русский:    "Среднее = 26мс"  или  "Среднее = 26 мс"
	//   Английский: "Average = 26ms"  или  "Average = 26 ms"
	reWin := regexp.MustCompile(`(?:Среднее|Average)\s*=\s*(\d+)\s*м?ms`)
	matchesWin := reWin.FindStringSubmatch(string(output))
	if len(matchesWin) > 1 {
		return "Успех. Среднее время: " + matchesWin[1] + " мс", nil
	}

	// Если ничего не нашли, выводим сырой результат
	return "Не удалось распарсить результат:\n" + string(output), nil
}
