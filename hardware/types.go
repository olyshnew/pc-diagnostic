package hardware

// DiagnosticResult структура для хранения результатов диагностики
type DiagnosticResult struct {
	Name  string
	Value string
	Error error
}
