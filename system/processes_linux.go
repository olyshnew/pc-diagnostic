//go:build linux

package system

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/shirou/gopsutil/v3/process"
)

type procRow struct {
	name string
	pid  int32
	cpu  float64
	mem  float64 // МБ
}

// GetTopProcessesInfo — топ-10 процессов по накопленному CPU и по RAM.
func GetProcessesInfo() string {
	procs, err := process.Processes()
	if err != nil {
		return fmt.Sprintf("Процессы: ошибка получения списка: %v", err)
	}
	var rows []procRow
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
		rows = append(rows, procRow{name, p.Pid, t.Total(), float64(mi.RSS) / 1024 / 1024})

	}

	byCPU := append([]procRow(nil), rows...)
	sort.Slice(byCPU, func(i, j int) bool { return byCPU[i].cpu > byCPU[j].cpu })
	byRAM := append([]procRow(nil), rows...)
	sort.Slice(byRAM, func(i, j int) bool { return byRAM[i].mem > byRAM[j].mem })

	var b strings.Builder
	fmt.Fprintln(&b, "Топ-10 процессов по времени CPU:")
	for i := 0; i < len(byCPU) && i < 10; i++ {
		fmt.Fprintf(&b, "  %-25s PID %-7d CPU %.1f с\n", byCPU[i].name, byCPU[i].pid, byCPU[i].cpu)
	}
	fmt.Fprintln(&b, "Топ-10 процессов по RAM:")
	for i := 0; i < len(byRAM) && i < 10; i++ {
		fmt.Fprintf(&b, "  %-25s PID %-7d RAM %.0f МБ\n", byRAM[i].name, byRAM[i].pid, byRAM[i].mem)
	}
	return b.String()
}

// GetServicesInfo — сводка по службам systemd + упавшие юниты.
func GetServicesInfo() string {
	out, err := exec.Command("systemctl", "list-units", "--type=service", "--no-pager", "--all").Output()
	if err != nil {
		return "Службы: systemctl недоступен (система без systemd)"
	}
	total, running := 0, 0
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) >= 4 && strings.HasSuffix(f[0], ".service") {
			total++
			if f[3] == "running" {
				running++
			}
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Службы: всего %d, запущено %d, остановлено %d\n", total, running, total-running)
	failed, err2 := exec.Command("systemctl", "--failed", "--no-pager", "--plain", "--no-legend").Output()
	if err2 == nil && strings.TrimSpace(string(failed)) != "" {
		fmt.Fprintf(&b, "Службы с ошибкой запуска:\n%s", string(failed))
	} else {
		fmt.Fprintln(&b, "Службы с ошибкой запуска: не обнаружены")
	}
	return b.String()
}

// GetUpdatesInfo — установленные обновления/пакеты ОС (dpkg или rpm).
func GetUpdatesInfo() string {
	var lines []string
	if _, err := exec.LookPath("dpkg-query"); err == nil {
		out, err := exec.Command("sh", "-c", "dpkg-query -W -f='${Package}\n' | sort").Output()
		if err != nil {
			return "Обновления ОС: ошибка чтения списка пакетов"
		}
		lines = strings.Split(strings.TrimSpace(string(out)), "\n")
	} else if _, err := exec.LookPath("rpm"); err == nil {
		out, err := exec.Command("rpm", "-qa", "--qf", "%{NAME} %{VERSION}-%{RELEASE}\n").Output()
		if err != nil {
			return "Обновления ОС: ошибка чтения списка пакетов"
		}
		lines = strings.Split(strings.TrimSpace(string(out)), "\n")
	} else {
		return "Обновления ОС: менеджер пакетов не определён (нет dpkg и rpm)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Установленных пакетов: %d, последние 10:\n", len(lines))
	for i := len(lines) - 10; i < len(lines); i++ {
		if i >= 0 {
			fmt.Fprintf(&b, "  %s\n", lines[i])
		}
	}
	return b.String()
}
