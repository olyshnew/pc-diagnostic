package report

// Row — одна строка секции отчёта
type Row struct {
	Label string
	Value string
}

// Section — логический блок отчёта («Оборудование», «Диски»...)
type Section struct {
	Title string
	Rows  []Row
}

// Report — весь отчёт целиком
type Report struct {
	Title    string
	Date     string
	Sections []Section
}
