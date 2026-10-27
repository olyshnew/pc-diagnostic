package utils

import (
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/mattn/go-colorable"
)

var stdout io.Writer

func init() {
	if runtime.GOOS == "windows" {
		stdout = colorable.NewColorableStdout()
	} else {
		stdout = os.Stdout
	}
}

const (
	ColorReset    = "\033[0m"
	ColorBold     = "\033[1m"
	ColorDim      = "\033[2m"
	ColorRed      = "\033[31m"
	ColorGreen    = "\033[32m"
	ColorYellow   = "\033[33m"
	ColorBlue     = "\033[34m"
	ColorMagenta  = "\033[35m"
	ColorCyan     = "\033[36m"
	ColorWhite    = "\033[37m"
	ColorBgRed    = "\033[41m"
	ColorBgGreen  = "\033[42m"
	ColorBgYellow = "\033[43m"
	ColorBgBlue   = "\033[44m"
)

func Colored(text, color string) string {
	return color + text + ColorReset
}

func Bold(text string) string      { return Colored(text, ColorBold) }
func Header(text string) string    { return Colored(text, ColorBold+ColorCyan) }
func Title(text string) string     { return Colored(text, ColorBold+ColorMagenta) }
func Success(text string) string   { return Colored(text, ColorGreen) }
func Warning(text string) string   { return Colored(text, ColorYellow) }
func ErrorText(text string) string { return Colored(text, ColorRed) }
func Info(text string) string      { return Colored(text, ColorBlue) }
func Cyan(text string) string      { return Colored(text, ColorCyan) }
func Dim(text string) string       { return Colored(text, ColorDim) }


func ColorByPercent(value float64) string {
	switch {
	case value > 85:
		return Colored(fmt.Sprintf("%.1f%%", value), ColorRed)
	case value > 60:
		return Colored(fmt.Sprintf("%.1f%%", value), ColorYellow)
	default:
		return Colored(fmt.Sprintf("%.1f%%", value), ColorGreen)
	}
}

func ColorByTemp(temp float64) string {
	switch {
	case temp > 80:
		return Colored(fmt.Sprintf("%.1f C", temp), ColorRed)
	case temp > 60:
		return Colored(fmt.Sprintf("%.1f C", temp), ColorYellow)
	default:
		return Colored(fmt.Sprintf("%.1f C", temp), ColorGreen)
	}
}

func Separator(n int) string {
	return Colored(repeatChar('=', n), ColorBold+ColorCyan)
}

func repeatChar(ch rune, n int) string {
	result := make([]rune, n)
	for i := range result {
		result[i] = ch
	}
	return string(result)
}

func Println(text string) {
	fmt.Fprintln(stdout, text)
}

func Printf(format string, args ...interface{}) {
	fmt.Fprintf(stdout, format+"\n", args...)
}

func PrintMenu(text string) {
	fmt.Fprint(stdout, text)
}

func PressEnter() {
	fmt.Fprintln(stdout, Dim("Нажмите Enter для возврата в меню..."))
}

func EnsureWindowsANSI() {}
