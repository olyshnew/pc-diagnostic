// Package archive — версионирование отчётов.
// Каждый сохранённый отчёт попадает в logs/ГГГГ-ММ-ДД_ЧЧ-ММ-СС/:
// копия отчёта, снимок журнала schedule_log.csv и manifest.json
// с метаданными прогона. Плюс построчное сравнение двух архивов.
package archive

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type manifest struct {
	Time     string   `json:"time"`
	Hostname string   `json:"hostname"`
	OS       string   `json:"os"`
	Files    []string `json:"files"`
}

// Save сохраняет копию отчёта в папку logs/<timestamp>/.
// reportData — содержимое отчёта, ext — расширение (txt, json, csv, html).
// Возвращает путь к созданной папке.
func Save(reportData []byte, ext string) (string, error) {
	name := time.Now().Format("2006-01-02_15-04-05")
	dir := filepath.Join("logs", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("не удалось создать папку архива: %w", err)
	}

	files := []string{}

	rp := filepath.Join(dir, "diagnostic_report."+ext)
	if err := os.WriteFile(rp, reportData, 0o644); err != nil {
		return "", fmt.Errorf("не удалось записать отчёт: %w", err)
	}
	files = append(files, "diagnostic_report."+ext)

	// Снимок журнала мониторинга на момент прогона: дельта между
	// двумя архивами покажет, как менялись метрики между прогонами.
	if raw, err := os.ReadFile("schedule_log.csv"); err == nil {
		lg := filepath.Join(dir, "schedule_log.csv")
		if err := os.WriteFile(lg, raw, 0o644); err != nil {
			return "", fmt.Errorf("не удалось записать снимок журнала: %w", err)
		}
		files = append(files, "schedule_log.csv")
	}

	host, _ := os.Hostname()
	m := manifest{
		Time:     time.Now().Format("2006-01-02 15:04:05"),
		Hostname: host,
		OS:       runtime.GOOS + "/" + runtime.GOARCH,
		Files:    files,
	}
	mb, _ := json.MarshalIndent(m, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), mb, 0o644)

	return dir, nil
}

// List возвращает список имён архивных папок (отсортирован по времени,
// имя папки само по себе — это timestamp).
func List() []string {
	entries, err := os.ReadDir("logs")
	if err != nil {
		return nil // папки ещё нет — архивов нет
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	sort.Strings(dirs)
	return dirs
}

// Compare сравнивает два архива построчно и печатает различия
// для отчёта и журнала. Первый аргумент — «старый» архив,
// второй — «новый»: минус означает старую строку, плюс — новую.
//****
// Compare печатает построчные различия двух архивов в консоль.
func Compare(oldDir, newDir string) {
	fmt.Print(CompareText(oldDir, newDir))
}

// CompareText возвращает текст сравнения двух архивов строкой.
// Используется и консолью, и веб-интерфейсом.
func CompareText(oldDir, newDir string) string {
	var b strings.Builder
	b.WriteString(compareFileText(oldDir, newDir, "diagnostic_report.txt"))
	b.WriteString(compareFileText(oldDir, newDir, "schedule_log.csv"))
	return b.String()
}

func compareFileText(oldDir, newDir, name string) string {
	var b strings.Builder
	a, okA := readLines(oldDir, name)
	c, okB := readLines(newDir, name)
	if !okA && !okB {
		fmt.Fprintf(&b, "\n  %s: нет ни в одном из архивов — пропущено\n", name)
		return b.String()
	}
	if !okA || !okB {
		which := "старом"
		if !okB {
			which = "новом"
		}
		fmt.Fprintf(&b, "\n  %s: отсутствует в %s архиве — сравнение невозможно\n", name, which)
		return b.String()
	}

	fmt.Fprintf(&b, "\n----------------------------------------\n")
	fmt.Fprintf(&b, "СРАВНЕНИЕ: %s\n", name)
	fmt.Fprintf(&b, "старый: %s\nновый:  %s\n", oldDir, newDir)
	fmt.Fprintf(&b, "----------------------------------------\n")

	changes := diffLines(a, c)
	if len(changes) == 0 {
		b.WriteString("  Различий не найдено.\n")
		return b.String()
	}
	const maxOut = 80
	for i, line := range changes {
		if i == maxOut {
			fmt.Fprintf(&b, "  ... и ещё %d строк различий (вывод ограничен)\n", len(changes)-maxOut)
			return b.String()
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

//***
func readLines(dir, name string) ([]string, bool) {
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil, false
	}
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return nil, true // файл есть, но пустой
	}
	return strings.Split(s, "\n"), true
}

// diffLines — построчный diff через наибольшую общую подпоследовательность.
// Ограничение: суммарно до 6000 строк (иначе файл сравнивается целиком
// как заменённый) — защита от квадратичного роста памяти.
func diffLines(a, b []string) []string {
	if len(a)+len(b) > 6000 {
		return []string{"  [!] Файлы слишком большие для построчного сравнения"}
	}

	n, m := len(a), len(b)
	// lcs[i][j] — длина наибольшей общей подпоследовательности a[i:], b[j:].
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	var out []string
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, "- "+strings.TrimRight(a[i], "\r"))
			i++
		default:
			out = append(out, "+ "+strings.TrimRight(b[j], "\r"))
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, "- "+strings.TrimRight(a[i], "\r"))
	}
	for ; j < m; j++ {
		out = append(out, "+ "+strings.TrimRight(b[j], "\r"))
	}
	return out
}
