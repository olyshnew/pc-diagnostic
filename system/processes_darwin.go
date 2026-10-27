//go:build darwin

package system

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/shirou/gopsutil/v3/process"
)

type procRowDarwin struct {
	name string
	pid  int32
	cpu  float64
	mem  float64
}

// GetProcessesInfo — топ-10 процессов по накопленному CPU и по RAM (gopsutil).
func GetProcessesInfo() string {
	procs, err := process.Processes()
	if err != nil {
		return fmt.Sprintf("Процессы: ошибка получения списка: %v", err)
	}
	var rows []procRowDarwin
	for _, p := range procs {
		name, _ := p.Name()
		if name == "" {
			continue
		}
		t, err1 := p.Times()
		mi, err2 := p.MemoryInfo()
		if err1 != nil || err2 != nil {
			continue
		}
		rows = append(rows, procRowDarwin{name, p.Pid, t.Total(), float64(mi.RSS) / 1024 / 1024})
	}

	byCPU := append([]procRowDarwin(nil), rows...)
	sort.Slice(byCPU, func(i, j int) bool { return byCPU[i].cpu > byCPU[j].cpu })
	byRAM := append([]procRowDarwin(nil), rows...)
	sort.Slice(byRAM, func(i, j int) bool { return byRAM[i].mem > byRAM[j].mem })

	var b strings.Builder
	b.WriteString("Топ-10 процессов по времени CPU:\n")
	for i := 0; i < len(byCPU) && i < 10; i++ {
		fmt.Fprintf(&b, "  %d. %-20s PID %-7d CPU %.1f с\n", i+1, byCPU[i].name, byCPU[i].pid, byCPU[i].cpu)
	}
	b.WriteString("\nТоп-10 процессов по RAM:\n")
	for i := 0; i < len(byRAM) && i < 10; i++ {
		fmt.Fprintf(&b, "  %d. %-20s PID %-7d RAM %.0f МБ\n", i+1, byRAM[i].name, byRAM[i].pid, byRAM[i].mem)
	}
	return b.String()
}

// GetServicesInfo — службы macOS: задачи launchd (launchctl list).
func GetServicesInfo() string {
	out, err := exec.Command("launchctl", "list").Output()
	if err != nil {
		return "Службы: не удалось получить информацию (launchctl)"
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	count := 0
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			count++
		}
	}
	return fmt.Sprintf("Службы (launchd): всего задач %d\nПримечание: службы в macOS управляются launchd —\nстатусы ошибок запуска видны в колонке статуса launchctl list", count)
}

// GetUpdatesInfo — установленные обновления ОС (softwareupdate --history).
func GetUpdatesInfo() string {
	out, err := exec.Command("softwareupdate", "--history").Output()
	if err != nil {
		return "Обновления ОС: журнал недоступен (softwareupdate --history)"
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return "Обновления ОС: журнал пуст"
	}
	lines := strings.Split(s, "\n")
	var b strings.Builder
	b.WriteString("Обновления ОС (softwareupdate --history, последние 10):\n")
	start := len(lines) - 10
	if start < 0 {
		start = 0
	}
	for _, l := range lines[start:] {
		b.WriteString("  " + strings.TrimSpace(l) + "\n")
	}
	return b.String()
}
