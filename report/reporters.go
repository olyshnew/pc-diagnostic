package report

import (
	"encoding/csv"
	"encoding/json"
	"bytes"
	"fmt"
	"html/template"
	"strings"
)

// Reporter — интерфейс для любого формата отчёта
type Reporter interface {
	Write(r Report) ([]byte, error)
	Ext() string
}
// ---------- TXT ----------

type TXTReporter struct{}

func (TXTReporter) Ext() string { return "txt" }

func (TXTReporter) Write(r Report) ([]byte, error) {
	var b strings.Builder

	b.WriteString(r.Title + "\n")
	b.WriteString("Дата: " + r.Date + "\n\n")

	for _, sec := range r.Sections {
		b.WriteString(sec.Title + ":\n")
		for _, row := range sec.Rows {
			if row.Label != "" {
				b.WriteString(" " + row.Label + ": " + row.Value + "\n")
			} else {
				b.WriteString(" " + row.Value + "\n")
			}
		}
		b.WriteString("\n---\n\n")
	}

	return []byte(b.String()), nil
}
// ---------- JSON ----------

type JSONReporter struct{}

func (JSONReporter) Ext() string { return "json" }

func (JSONReporter) Write(r Report) ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// ---------- CSV ----------

type CSVReporter struct{}

func (CSVReporter) Ext() string { return "csv" }

func (CSVReporter) Write(r Report) ([]byte, error) {
	var buf bytes.Buffer

	// BOM, чтобы Excel корректно открыл кириллицу
	buf.WriteString("\uFEFF")

	w := csv.NewWriter(&buf)
	w.Comma = ';' // Excel по умолчанию понимает точку с запятой

	w.Write([]string{r.Title})
	w.Write([]string{"Дата", r.Date})
	w.Write([]string{})

	for _, sec := range r.Sections {
		w.Write([]string{"## " + sec.Title})
		for _, row := range sec.Rows {
			w.Write([]string{sec.Title, row.Label, row.Value})
		}
		w.Write([]string{})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("запись CSV: %w", err)
	}
	return buf.Bytes(), nil
}

// ---------- HTML ----------

type HTMLReporter struct{}

func (HTMLReporter) Ext() string { return "html" }

func (HTMLReporter) Write(r Report) ([]byte, error) {
	const tpl = `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<title>{{.Title}}</title>
<style>
body { font-family: Segoe UI, sans-serif; background:#f5f6fa; margin:24px; }
h1 { color:#2d3436; }
h2 { color:#0984e3; border-bottom:2px solid #dfe6e9; padding-bottom:6px; }
table { border-collapse:collapse; margin-bottom:24px; background:#fff;
        box-shadow:0 1px 3px rgba(0,0,0,.1); }
td { border:1px solid #dfe6e9; padding:6px 14px; }
td:first-child { font-weight:600; width:280px; }
.value { color:#636e72; }
</style>
</head>
<body>
<h1>{{.Title}}</h1>
<p>Дата: {{.Date}}</p>
{{range .Sections}}
<h2>{{.Title}}</h2>
<table>
{{range .Rows}}<tr><td>{{.Label}}</td><td class="value">{{.Value}}</td></tr>
{{end}}</table>
{{end}}
</body>
</html>`

	t := template.Must(template.New("report").Parse(tpl))
	var buf bytes.Buffer
	if err := t.Execute(&buf, r); err != nil {
		return nil, fmt.Errorf("рендер HTML: %w", err)
	}
	return buf.Bytes(), nil
}
