package report

import (
	//"fmt"
	"strings"
	"time"

	"pc-diagnostic/hardware"
)

// BuildFromHardware строит секции из результатов hardware.RunDiagnostics()
func BuildFromHardware(results []hardware.DiagnosticResult) Report {
	r := Report{
		Title: "Отчёт диагностики ПК",
		Date:  time.Now().Format("02.01.2006 15:04:05"),
	}

	for _, res := range results {
		sec := Section{Title: res.Name}
		if res.Error != nil {
			sec.Rows = append(sec.Rows, Row{Label: "Ошибка", Value: res.Error.Error()})
		} else {
			sec.Rows = append(sec.Rows, parseTextBlock(res.Value)...)
		}
		r.Sections = append(r.Sections, sec)
	}
	return r
}

// AddText добавляет текстовый блок (например, из system.Get*Info())
func (r *Report) AddText(title, text string) {
	r.Sections = append(r.Sections, Section{
		Title: title,
		Rows:  parseTextBlock(text),
	})
}

// parseTextBlock превращает текст в пары «метка: значение».
// Строки без «:» сохраняются как безымянные строки значения.
func parseTextBlock(text string) []Row {
	var rows []Row
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if idx := strings.Index(line, ":"); idx > 0 {
			rows = append(rows, Row{
				Label: strings.TrimSpace(line[:idx]),
				Value: strings.TrimSpace(line[idx+1:]),
			})
		} else {
			rows = append(rows, Row{Value: line})
		}
	}
	return rows
}
