// Package baseline — структурный снимок ключевых показателей системы
// и сравнение прогонов: свободная RAM, температура CPU, открытые порты,
// автозагрузка. Снимок хранится в baseline.json (корень проекта) и копией
// в архив отчёта, поэтому дельты доступны и между любыми двумя архивами
// (консольный пункт [9], веб-страница «Архивы»).
package baseline

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"pc-diagnostic/monitor"
	"pc-diagnostic/utils"
)

const lastFile = "baseline.json"

// Snapshot — снимок ключевых метрик прогона.
type Snapshot struct {
	Time       string   `json:"time"`
	RamFreeGB  float64  `json:"ram_free_gb"`
	RamUsedGB  float64  `json:"ram_used_gb"`
	RamTotalGB float64  `json:"ram_total_gb"`
	SwapUsedGB float64  `json:"swap_used_gb"`
	CpuTempC   float64  `json:"cpu_temp_c"`
	Ports      []int    `json:"open_ports"`
	Autostart  []string `json:"autostart"`
}

// Take собирает текущий снимок. RAM и температура берутся из monitor.GetStats
// (тот же источник, что в веб-мониторинге), порты сканируются самостоятельно,
// автозагрузка читается из реестра через reg query.
func Take() Snapshot {
	s := Snapshot{Time: time.Now().Format("2006-01-02 15:04:05")}

	// Поля структуры monitor.Stats читаем через map, чтобы не зависеть
	// от имён полей структуры (паттерн как в web.statsHandler).
	if data, err := json.Marshal(monitor.GetStats()); err == nil {
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err == nil {
			num := func(k string) float64 {
				v, _ := m[k].(float64)
				return v
			}
			s.RamTotalGB = num("ram_total_gb")
			s.RamUsedGB = num("ram_used_gb")
			s.RamFreeGB = s.RamTotalGB - s.RamUsedGB
			s.SwapUsedGB = num("swap_used_gb")
			s.CpuTempC = num("cpu_temp")
		}
	}

	s.Ports = scanPorts()
	s.Autostart = readAutostart()

	sort.Slice(s.Ports, func(i, j int) bool { return s.Ports[i] < s.Ports[j] })
	sort.Strings(s.Autostart)
	return s
}

// ---------- Хранение ----------

// SaveLast перезаписывает baseline.json — снимок «последнего прогона».
func SaveLast(s Snapshot) {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(lastFile, data, 0644)
}

// SaveToDir копирует снимок в архивную папку (рядом с отчётом).
func SaveToDir(s Snapshot, dir string) {
	if dir == "" {
		return
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(filepath.Join(dir, "baseline.json"), data, 0644); err != nil {
		fmt.Printf("[!] Не удалось сохранить baseline-снимок в архив: %v\n", err)
	}
}

// LoadLast читает последний снимок из baseline.json.
func LoadLast() (Snapshot, bool) {
	return loadFrom(lastFile)
}

func loadFrom(path string) (Snapshot, bool) {
	var s Snapshot
	data, err := os.ReadFile(path)
	if err != nil {
		return s, false
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, false
	}
	return s, true
}

// ---------- Сравнение ----------

// CompareText возвращает структурные дельты двух снимков открытым текстом.
func CompareText(old, new Snapshot) string {
	var b strings.Builder

	fmt.Fprintln(&b, "----------------------------------------")
	fmt.Fprintln(&b, "BASELINE: ключевые метрики")
	fmt.Fprintf(&b, "старый: %s (RAM свободно %.2f ГБ, портов %d, автозапуск %d)\n",
		old.Time, old.RamFreeGB, len(old.Ports), len(old.Autostart))
	fmt.Fprintf(&b, "новый:  %s (RAM свободно %.2f ГБ, портов %d, автозапуск %d)\n",
		new.Time, new.RamFreeGB, len(new.Ports), len(new.Autostart))
	fmt.Fprintln(&b, "----------------------------------------")

	fmt.Fprintf(&b, "RAM (свободно):      %.2f → %.2f ГБ  (%+.2f)\n",
		old.RamFreeGB, new.RamFreeGB, new.RamFreeGB-old.RamFreeGB)
	fmt.Fprintf(&b, "Swap (используется): %.2f → %.2f ГБ  (%+.2f)\n",
		old.SwapUsedGB, new.SwapUsedGB, new.SwapUsedGB-old.SwapUsedGB)

	// Температура: сравниваем только если оба датчика вернули данные.
	switch {
	case old.CpuTempC > 0 && new.CpuTempC > 0:
		fmt.Fprintf(&b, "CPU (температура):   %.0f → %.0f °C  (%+.0f)\n",
			old.CpuTempC, new.CpuTempC, new.CpuTempC-old.CpuTempC)
	default:
		fmt.Fprintln(&b, "CPU (температура):   нет данных в одном из снимков")
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Порты (localhost):")
	oldPorts := toSet(old.Ports)
	newPorts := toSet(new.Ports)
	added, removed := 0, 0
	for _, p := range new.Ports {
		if !oldPorts[p] {
			fmt.Fprintf(&b, "  [+] %d %s — порт ОТКРЫЛСЯ\n", p, portName(p))
			added++
		}
	}
	for _, p := range old.Ports {
		if !newPorts[p] {
			fmt.Fprintf(&b, "  [-] %d %s — порт закрылся\n", p, portName(p))
			removed++
		}
	}
	if added == 0 && removed == 0 {
		fmt.Fprintf(&b, "  совпадает (%d портов)\n", len(new.Ports))
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Автозагрузка:")
	oldAuto := toStrSet(old.Autostart)
	newAuto := toStrSet(new.Autostart)
	aAdded, aRemoved := 0, 0
	for _, e := range new.Autostart {
		if !oldAuto[e] {
			fmt.Fprintf(&b, "  [+] %s — новая запись\n", e)
			aAdded++
		}
	}
	for _, e := range old.Autostart {
		if !newAuto[e] {
			fmt.Fprintf(&b, "  [-] %s — запись удалена\n", e)
			aRemoved++
		}
	}
	if aAdded == 0 && aRemoved == 0 {
		fmt.Fprintf(&b, "  изменений нет (%d записей)\n", len(new.Autostart))
	}

	// Итоговый вердикт по значимым изменениям.
	fmt.Fprintln(&b)
	switch {
	case added > 0:
		fmt.Fprintln(&b, "[!] Внимание: появились новые открытые порты — проверьте, какие программы их слушают.")
	case aAdded > 0:
		fmt.Fprintln(&b, "[!] Внимание: в автозагрузке появились новые записи.")
	case removed > 0 || aRemoved > 0:
		fmt.Fprintln(&b, "[i] Часть элементов (порты/автозагрузка) исчезла по сравнению с прошлым прогоном.")
	default:
		fmt.Fprintln(&b, "[✓] Значимых изменений в портах и автозагрузке нет.")
	}
	return b.String()
}

// CompareDirs сравнивает baseline.json из двух архивных папок.
func CompareDirs(oldDir, newDir string) string {
	o, okO := loadFrom(filepath.Join(oldDir, "baseline.json"))
	n, okN := loadFrom(filepath.Join(newDir, "baseline.json"))
	switch {
	case !okO && !okN:
		return "\n  Baseline-снимок: отсутствует в обоих архивах (снимки ведутся только в новых прогонах)\n"
	case !okN:
		return "\n  Baseline-снимок: отсутствует в новом архиве\n"
	case !okO:
		return "\n  Baseline-снимок: отсутствует в старом архиве (baseline появился позже)\n"
	}
	return CompareText(o, n)
}

// Run — полный цикл для момента сохранения отчёта: снимок → сравнение
// с предыдущим → печать дельты в консоль → сохранение нового снимка
// (в baseline.json и копией в архив). Вызывается из main.go.
func Run(archiveDir string) {
	newSnap := Take()

	fmt.Println()
	if old, ok := LoadLast(); ok {
		printColored(CompareText(old, newSnap))
	} else {
		fmt.Println(utils.Info("[i] Baseline-снимок создан (baseline.json). Дельты появятся при следующем сохранении отчёта."))
	}

	SaveLast(newSnap)
	SaveToDir(newSnap, archiveDir)
}

// printColored печатает дельту с раскраской: [!] красным, [+] зелёным,
// [-] жёлтым.
func printColored(text string) {
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "[!]"):
			fmt.Println(utils.ErrorText(line))
		case strings.HasPrefix(line, "  [+]"):
			fmt.Println(utils.Success(line))
		case strings.HasPrefix(line, "  [-]"):
			fmt.Println(utils.Warning(line))
		default:
			fmt.Println(line)
		}
	}
}

// ---------- Сбор данных ----------

// portList — сканируемый набор портов. Список фиксирован: дельты корректны,
// только если между прогонами сканируется один и тот же набор.
var portList = []int{
	21, 22, 23, 25, 53, 80, 110, 135, 139, 143, 443, 445,
	993, 995, 1433, 1723, 3306, 3389, 5432, 5900, 6379, 8080, 8443, 27017,
}

// portNames — короткие названия для дельты (только добавленные порты).
func portName(p int) string {
	names := map[int]string{
		21: "FTP", 22: "SSH", 23: "Telnet", 25: "SMTP", 53: "DNS",
		80: "HTTP", 110: "POP3", 135: "RPC", 139: "NetBIOS", 143: "IMAP",
		443: "HTTPS", 445: "SMB", 993: "IMAPS", 995: "POP3S", 1433: "MSSQL",
		1723: "PPTP", 3306: "MySQL", 3389: "RDP", 5432: "PostgreSQL",
		5900: "VNC", 6379: "Redis", 8080: "HTTP-alt", 8443: "HTTPS-alt",
		27017: "MongoDB",
	}
	if n, ok := names[p]; ok {
		return "(" + n + ")"
	}
	return ""
}

// scanPorts проверяет стандартные порты на localhost конкурентно.
func scanPorts() []int {
	var mu sync.Mutex
	var wg sync.WaitGroup
	var open []int

	for _, p := range portList {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			conn, err := net.DialTimeout("tcp", "127.0.0.1:"+fmt.Sprint(port), 300*time.Millisecond)
			if err == nil {
				conn.Close()
				mu.Lock()
				open = append(open, port)
				mu.Unlock()
			}
		}(p)
	}
	wg.Wait()
	return open
}

// readAutostart читает записи автозагрузки из реестра (HKCU и HKLM, ключ Run)
// через reg query. На Linux возвращает пустой список.
func readAutostart() []string {
	if runtime.GOOS != "windows" {
		return nil
	}

	var out []string
	seen := map[string]bool{}
	for _, key := range []string{
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
		`HKLM\Software\Microsoft\Windows\CurrentVersion\Run`,
	} {
		res, err := exec.Command("reg", "query", key).Output()
		if err != nil {
			continue // ключа может не быть — это нормально
		}
		for _, line := range strings.Split(string(res), "\n") {
			line = strings.TrimSpace(line)
			// Формат строки reg query: "Имя    REG_SZ    значение"
			if !strings.Contains(line, "REG_SZ") && !strings.Contains(line, "REG_EXPAND_SZ") {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) < 3 {
				continue
			}
			entry := parts[0] + ": " + strings.Join(parts[2:], " ")
			if !seen[entry] {
				seen[entry] = true
				out = append(out, entry)
			}
		}
	}
	return out
}

// toSet / toStrSet — вспомогательные множества для сравнения.
func toSet(nums []int) map[int]bool {
	m := make(map[int]bool, len(nums))
	for _, n := range nums {
		m[n] = true
	}
	return m
}

func toStrSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}
