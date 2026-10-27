package system

import (
	"fmt"
	"io"
	"net/http"
	"time"
	"strings"

	"pc-diagnostic/utils"
)

const testFileURL = "https://speedtest.tele2.net/100MB.zip"

func TestInternetSpeed() string {
	var result strings.Builder
	result.WriteString("=== ТЕСТ СКОРОСТИ ИНТЕРНЕТА ===\n")

	start := time.Now()
	resp, err := http.Get(testFileURL)
	if err != nil {
		// Запасной сервер
		resp, err = http.Get("https://cachefly.cachefly.net/100mb.test")
		if err != nil {
			return result.String() + " Ошибка соединения: " + err.Error() + "\n"
		}
	}
	defer resp.Body.Close()

	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		return result.String() + " Ошибка при чтении потока: " + err.Error() + "\n"
	}

	duration := time.Since(start).Seconds()
	sizeMB := float64(100)
	speedMbps := (sizeMB / duration) * 8

	result.WriteString(fmt.Sprintf(" Скачан тестовый файл %.0f МБ за %.2f сек.\n", sizeMB, duration))
	result.WriteString(fmt.Sprintf(" Средняя скорость: %.2f Мбит/с\n", speedMbps))

	if pong, err := utils.Ping("77.88.8.8", 4); err == nil {
		result.WriteString(" Пинг Yandex DNS: " + pong + "\n")
	}
	if pong, err := utils.Ping("8.8.8.8", 4); err == nil {
		result.WriteString(" Пинг Google DNS: " + pong + "\n")
	}

	return result.String()
}
