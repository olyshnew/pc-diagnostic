// Package scheduler — автоматический сбор метрик по расписанию
// с записью в CSV-журнал.
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"pc-diagnostic/monitor"
	"pc-diagnostic/thresholds"
)

const logFile = "schedule_log.csv"

type scheduleConfig struct {
	Enabled         bool `json:"enabled"`
	IntervalSeconds int  `json:"interval_seconds"`
}

type rootConfig struct {
	Schedule scheduleConfig `json:"schedule"`
}

// RunIfRequested проверяет, запрошен ли фоновый сбор: флагом --schedule
// или секцией "schedule": {"enabled": true} в config.json.
// Если запуск не нужен — сразу возвращает false (обычный запуск программы).
func RunIfRequested() bool {
	// --schedule имеет приоритет: если с ним переданы другие флаги — предупредим.
	

	interval, start := resolveInterval()
	if !start {
		return false
	}
	Run(interval)
	return true
}


// resolveInterval определяет интервал и факт запуска.
// Приоритет: флаг --schedule (со значением или без), затем config.json.
func resolveInterval() (time.Duration, bool) {
	flagSeen := false
	flagVal := 0
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "--schedule" {
			flagSeen = true
			// значение может стоять отдельным аргументом: --schedule 30
			if i+1 < len(os.Args) {
				if d, err := strconv.Atoi(os.Args[i+1]); err == nil && d >= 5 {
					flagVal = d
				}
			}
			break
		}
		if strings.HasPrefix(arg, "--schedule=") {
			flagSeen = true
			if d, err := strconv.Atoi(strings.TrimPrefix(arg, "--schedule=")); err == nil && d >= 5 {
				flagVal = d
			}
			break
		}
	}

	if flagSeen {
	// Предупреждаем, только если --schedule действительно передан
		// вместе с другими флагами.
		for _, a := range os.Args[1:] {
			if strings.HasPrefix(a, "--") && !strings.HasPrefix(a, "--schedule") {
				fmt.Println("[i] Флаг --schedule имеет приоритет: остальные флаги проигнорированы.")
				break
			}
		}
		iv := 60
		if flagVal >= 5 {
			iv = flagVal
		} else if cfg, ok := readConfig(); ok && cfg.IntervalSeconds >= 5 {
			iv = cfg.IntervalSeconds
		}
		return time.Duration(iv) * time.Second, true
	}

	// Без флага — только config.json.
	if cfg, ok := readConfig(); ok && cfg.Enabled {
		iv := cfg.IntervalSeconds
		if iv < 5 {
			iv = 60
		}
		return time.Duration(iv) * time.Second, true
	}
	return 60 * time.Second, false
}

// readConfig читает секцию schedule из config.json (мягко: файл может
// отсутствовать или не содержать секции — это не ошибка).
func readConfig() (scheduleConfig, bool) {
	raw, err := os.ReadFile("config.json")
	if err != nil {
		return scheduleConfig{}, false
	}
	var cfg rootConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return scheduleConfig{}, false
	}
	return cfg.Schedule, true
}

// statsMap снимает метрики через monitor.GetStats и добавляет статусы —
// тот же приём, что в веб-обработчике /api/stats.
func statsMap() map[string]interface{} {
	data, _ := json.Marshal(monitor.GetStats())
	var m map[string]interface{}
	_ = json.Unmarshal(data, &m)

	getF := func(k string) float64 {
		v, _ := m[k].(float64)
		return v
	}
	stCPU := thresholds.ForCPU(getF("cpu_total"))
	stRAM := thresholds.ForRAM(getF("ram_used_pct"))
	stTemp := thresholds.ForTemp(getF("cpu_temp"))
	m["status_cpu"] = string(stCPU)
	m["status_ram"] = string(stRAM)
	m["status_temp"] = string(stTemp)
	m["status_overall"] = string(thresholds.Worst(stCPU, stRAM, stTemp))
	return m
}

func getS(m map[string]interface{}, k string) string {
	v, _ := m[k].(string)
	return v
}

func getF(m map[string]interface{}, k string) float64 {
	v, _ := m[k].(float64)
	return v
}

func csvHeader() string {
	return "time,cpu_total_pct,ram_used_pct,ram_used_gb,ram_total_gb,swap_used_pct,cpu_temp,status_cpu,status_ram,status_temp,status_overall\n"
}

func csvRow(m map[string]interface{}, now time.Time) string {
	return fmt.Sprintf("%s,%.1f,%.1f,%.2f,%.2f,%.1f,%.1f,%s,%s,%s,%s\n",
		now.Format("2006-01-02T15:04:05"),
		getF(m, "cpu_total"),
		getF(m, "ram_used_pct"),
		getF(m, "ram_used_gb"),
		getF(m, "ram_total_gb"),
		getF(m, "swap_used_pct"),
		getF(m, "cpu_temp"),
		getS(m, "status_cpu"),
		getS(m, "status_ram"),
		getS(m, "status_temp"),
		getS(m, "status_overall"),
	)
}

// colorize окрашивает статус для консоли: Normal зелёный,
// Warning жёлтый, Critical красный, Unknown серый.
func colorize(s string) string {
	colors := map[string]string{
		"Normal":   "\033[32m",
		"Warning":  "\033[33m",
		"Critical": "\033[31m",
		"Unknown":  "\033[90m",
	}
	return colors[s] + s + "\033[0m"
}

// Run запускает бесконечный сбор: первый снимок сразу, дальше по тикеру.
// Остановка — Ctrl+C, журнал корректно закрывается.
func Run(interval time.Duration) {
	fmt.Printf("[•] Сбор по расписанию: снимок каждые %v, журнал: %s\n", interval, logFile)
	fmt.Println("    Остановка: Ctrl+C")

	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		if err := os.WriteFile(logFile, []byte(csvHeader()), 0o644); err != nil {
			fmt.Printf("[!] Не удалось создать журнал: %v\n", err)
			return
		}
	}

	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Printf("[!] Не удалось открыть журнал: %v\n", err)
		return
	}
	defer f.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	snapshot := func() {
		now := time.Now()
		m := statsMap()
		if _, err := f.WriteString(csvRow(m, now)); err != nil {
			fmt.Printf("[!] Ошибка записи в журнал: %v\n", err)
			return
		}
		_ = f.Sync()
		fmt.Printf("[%s] CPU %.0f%%  RAM %.0f%%  темп %.1f°C  Статус: %s\n",
			now.Format("15:04:05"),
			getF(m, "cpu_total"),
			getF(m, "ram_used_pct"),
			getF(m, "cpu_temp"),
			colorize(getS(m, "status_overall")),
		)
	}
statsMap()                        // первый прогрев: сенсор отдаёт ноль
time.Sleep(1500 * time.Millisecond)
statsMap()                        // второй прогрев — уже с реальной температурой


	snapshot() // первый снимок сразу, без ожидания тика
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			fmt.Println("\n[✓] Сбор по расписанию остановлен. Журнал: " + logFile)
			return
		case <-t.C:
			snapshot()
		}
	}
}
