// Package history — просмотр журнала мониторинга schedule_log.csv
// в виде сводки за весь период наблюдения.
package history

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

type row struct {
	t      time.Time
	cpu    float64
	ram    float64
	temp   float64
	swap   float64
	status string // status_overall
}

const logFile = "schedule_log.csv"

// SummaryText возвращает сводку по журналу строкой. Журнал может
// отсутствовать или быть пустым — это нормальный случай, не ошибка.
// Используется и консолью, и веб-интерфейсом.
func SummaryText() string {
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		return "[i] Журнал ещё не создан. Запустите сбор: pc-diagnostic.exe --schedule=60\n"
	}

	f, err := os.Open(logFile)
	if err != nil {
		return fmt.Sprintf("[!] Не удалось открыть журнал: %v\n", err)
	}
	defer f.Close()

	r := csv.NewReader(f)

	var rows []row
	for i := 0; ; i++ {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Sprintf("[!] Ошибка чтения журнала: %v\n", err)
		}
		if i == 0 {
			continue // заголовок
		}
		// Формат: time,cpu_total_pct,ram_used_pct,ram_used_gb,ram_total_gb,
		//         swap_used_pct,cpu_temp,status_cpu,status_ram,status_temp,status_overall
		if len(rec) < 11 {
			continue // неполная строка — пропускаем
		}
		rr := row{status: strings.TrimSpace(rec[10])}
		if rr.t, err = time.Parse("2006-01-02T15:04:05", strings.TrimSpace(rec[0])); err != nil {
			continue
		}
		rr.cpu = atof(rec[1])
		rr.ram = atof(rec[2])
		rr.swap = atof(rec[5])
		rr.temp = atof(rec[6])
		rows = append(rows, rr)
	}

	if len(rows) == 0 {
		return "[i] Журнал пуст (нет ни одной записи).\n"
	}

	// Накопители.
	var cpuSum, ramSum, tempSum float64
	var cpuMax, ramMax, tempMax float64
	warnings, criticals := 0, 0
	for _, rr := range rows {
		cpuSum += rr.cpu
		ramSum += rr.ram
		tempSum += rr.temp
		if rr.cpu > cpuMax {
			cpuMax = rr.cpu
		}
		if rr.ram > ramMax {
			ramMax = rr.ram
		}
		if rr.temp > tempMax {
			tempMax = rr.temp
		}
		switch rr.status {
		case "Warning":
			warnings++
		case "Critical":
			criticals++
		}
	}
	n := float64(len(rows))
	first, last := rows[0].t, rows[len(rows)-1].t
	span := last.Sub(first)
	if span < 0 {
		span = 0
	}

	var b strings.Builder
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "========================================")
	fmt.Fprintln(&b, "    ИСТОРИЯ МОНИТОРИНГА (schedule_log.csv)")
	fmt.Fprintln(&b, "========================================")
	fmt.Fprintf(&b, "Период наблюдения:  %s — %s\n", first.Format("02.01.2006 15:04:05"), last.Format("15:04:05"))
	fmt.Fprintf(&b, "Длительность:       %v\n", span.Round(time.Second))
	fmt.Fprintf(&b, "Снимков:            %d\n", len(rows))
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "CPU:   среднее %.1f%%, максимум %.1f%%\n", cpuSum/n, cpuMax)
	fmt.Fprintf(&b, "RAM:   среднее %.1f%%, максимум %.1f%%\n", ramSum/n, ramMax)
	fmt.Fprintf(&b, "Температура CPU: среднее %.1f°C, максимум %.1f°C\n", tempSum/n, tempMax)
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "Статусы: Critical %d, Warning %d, Normal %d\n",
		criticals, warnings, len(rows)-warnings-criticals)

	// Вывод-оценка по счётчикам.
	switch {
	case criticals > 0:
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "[!] В журнале есть критические записи — просмотрите график и нагрузки за это время.")
	case warnings > 0:
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "[i] Были предупреждения — система временами нагружалась.")
	default:
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "[✓] За время наблюдения серьёзных проблем не зафиксировано.")
	}
	return b.String()
}

// Print печатает сводку по журналу в консоль.
func Print() {
	fmt.Print(SummaryText())
}

func atof(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}
