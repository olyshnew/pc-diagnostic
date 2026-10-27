package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"pc-diagnostic/archive"
	"pc-diagnostic/hardware"
	"pc-diagnostic/monitor"
	"pc-diagnostic/report"
	"pc-diagnostic/scheduler"
	"pc-diagnostic/stress"
	"pc-diagnostic/system"
	"pc-diagnostic/utils"
	"pc-diagnostic/web"
	"pc-diagnostic/history"
	"pc-diagnostic/baseline"

)

// --- ФЛАГИ КОМАНДНОЙ СТРОКИ ---

var (
	flagCPU    = flag.Bool("cpu", false, "Стресс-тест CPU (без интерактивного меню)")
	flagMemory = flag.Bool("memory", false, "Стресс-тест RAM")
	flagDisk   = flag.Bool("disk", false, "Стресс-тест диска")
	flagFull   = flag.Bool("full-test", false, "Полная диагностика с сохранением отчёта")
	flagJSON   = flag.Bool("json-report", false, "Сохранить отчёт в формате JSON (подразумевает --full-test)")
	flagWeb    = flag.Bool("web", false, "Запустить веб-интерфейс")
	flagDur    = flag.Int("dur", 60, "Длительность стресс-теста в секундах (10-600)")
)

// --- ФУНКЦИИ ОТЧЁТА ---

// getFullReport собирает полный текстовый отчёт и структурированный Report
// для сохранения в JSON/CSV/HTML. Возвращает обе версии.
func getFullReport(hwResults []hardware.DiagnosticResult) (string, report.Report) {
	var reportText strings.Builder
	reportText.WriteString("ОТЧЁТ ДИАГНОСТИКИ ПК\n")
	reportText.WriteString(fmt.Sprintf("Дата: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	reportText.WriteString(fmt.Sprintf("ОС: %s (%s)\n\n", runtime.GOOS, runtime.GOARCH))

	// Структурированный отчёт: секции из результатов оборудования
	structured := report.BuildFromHardware(hwResults)

	for _, res := range hwResults {
		reportText.WriteString(res.Name + ":\n")
		if res.Error != nil {
			reportText.WriteString(fmt.Sprintf(" Ошибка: %v\n", res.Error))
		} else {
			val := stripFirstLine(res.Value)
			if len(val) > 2000 {
				val = val[:2000] + "\n... [Вывод обрезан]"
			}
			reportText.WriteString(val)
		}
		reportText.WriteString("\n---\n\n")
	}

	reportText.WriteString("=== РАСШИРЕННАЯ ИНФОРМАЦИЯ ===\n\n")

	utils.Println(utils.Info("[•] Сбор системной информации (драйверы, автозагрузка, целостность...)..."))

	// Параллельный сбор системной информации.
	sysFuncs := []struct {
		Title string
		Fn    func() string
	}{
		{"Вентиляторы и напряжения", hardware.GetSensorsInfo},
		{"Диски (WMI)", system.GetDiskInfo},
		{"Материнская плата (WMI)", system.GetMainboardInfo},
		{"Драйверы (WMI)", system.GetDriversInfo},
		{"USB-устройства (WMI)", system.GetUSBInfo},
		{"Автозагрузка", system.GetAutostartInfo},
		{"Процессы", system.GetProcessesInfo},
		{"Службы", system.GetServicesInfo},
		{"Обновления ОС", system.GetUpdatesInfo},
		{"Целостность системы", system.CheckSystemIntegrity},
		{"Скорость интернета", system.TestInternetSpeed},
		{"Звуковые устройства", system.GetAudioInfo},

		// Диски, расширенная диагностика
		{"Проверка файловой системы", hardware.CheckFileSystemInfo},
		{"Износ SSD (TBW)", hardware.GetSSDWearInfo},
		{"Фрагментация", hardware.AnalyzeFragmentation},
		{"Случайный доступ 4K", hardware.BenchmarkDiskRandom},

		// Сеть, расширенная диагностика
		{"Traceroute", system.GetTracerouteInfo},
		{"Открытые порты", system.ScanOpenPorts},
		{"Активные соединения", system.GetActiveConnectionsInfo},
		{"DNS-диагностика", system.GetDNSInfo},
	}

	sysResults := make([]string, len(sysFuncs))
	var wg sync.WaitGroup

	for i, sf := range sysFuncs {
		wg.Add(1)
		go func(idx int, f func() string) {
			defer wg.Done()
			sysResults[idx] = f()
		}(i, sf.Fn)
	}

	wg.Wait()
	utils.Println(utils.Success("[✓] Системная информация собрана."))

	for i, text := range sysResults {
		reportText.WriteString(text + "\n")
		structured.AddText(sysFuncs[i].Title, text)
	}

	utils.Println(utils.Info("[•] Проверка дисков (SMART)..."))
	smartPath, err := exec.LookPath("smartctl")
	if err == nil && smartPath != "" {
		deepResults := hardware.RunSMARTDiagnostics()
		for _, res := range deepResults {
			reportText.WriteString(fmt.Sprintf("%s:\n", res.Name))
			if res.Error != nil {
				reportText.WriteString(fmt.Sprintf(" [ОШИБКА - от Администратора]: %v\n", res.Error))
				structured.AddText("S.M.A.R.T. — "+res.Name, "ОШИБКА: "+res.Error.Error())
			} else {
				clean := strings.TrimSpace(res.Value)
				reportText.WriteString(clean + "\n")
				structured.AddText("S.M.A.R.T. — "+res.Name, clean)
			}
			reportText.WriteString("\n")
		}

		utils.Println(utils.Info("[•] Тест скорости диска..."))
		wSpd, rSpd, benchErr := hardware.BenchmarkDisk(100)
		if benchErr == nil {
			benchLine := fmt.Sprintf("Запись: %.2f МБ/с | Чтение: %.2f МБ/с", wSpd, rSpd)
			reportText.WriteString(fmt.Sprintf("Тест скорости диска:\n %s\n", benchLine))
			structured.AddText("Тест скорости диска", benchLine)
		}
	} else {
		smartMsg := "Утилита 'smartctl' не найдена.\nСкачайте Smartmontools."
		reportText.WriteString("S.M.A.R.T.: " + smartMsg + "\n")
		structured.AddText("S.M.A.R.T. диагностика дисков", smartMsg)
	}

	utils.Println(utils.Success("[✓] Отчёт сформирован."))
	return reportText.String(), structured
}

func saveReportToFile(filename, reportText string) error {
	return os.WriteFile(filename, []byte(reportText), 0644)
}

// saveReportAnyFormat предлагает выбрать формат и сохраняет отчёт.
// Каждый успешно сохранённый отчёт дополнительно копируется в архив
// (logs/<timestamp>/) с манифестом и снимком журнала мониторинга.
func saveReportAnyFormat(rep report.Report, textReport string, reader *bufio.Reader) {
	utils.Println("")
	utils.Println(utils.Header("--- Выбор формата отчёта ---"))
	utils.Println(utils.Info("[1]") + " TXT (текстовый файл)")
	utils.Println(utils.Info("[2]") + " JSON")
	utils.Println(utils.Info("[3]") + " CSV (открывается в Excel)")
	utils.Println(utils.Info("[4]") + " HTML (открывается в браузере)")
	utils.PrintMenu("Выберите формат (1-4): ")

	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	switch choice {
	case "2", "3", "4":
		var rep2 report.Reporter
		switch choice {
		case "2":
			rep2 = report.JSONReporter{}
		case "3":
			rep2 = report.CSVReporter{}
		case "4":
			rep2 = report.HTMLReporter{}
		}

		data, err := rep2.Write(rep)
		if err != nil {
			utils.Println(utils.ErrorText(fmt.Sprintf("Ошибка формирования отчёта: %v", err)))
		} else {
			filename := "diagnostic_report." + rep2.Ext()
			if err := os.WriteFile(filename, data, 0644); err != nil {
				utils.Println(utils.ErrorText(fmt.Sprintf("Ошибка сохранения: %v", err)))
			} else {
				utils.Println(utils.Success(fmt.Sprintf("Отчёт сохранён в: %s", filename)))
				if dir, aerr := archive.Save(data, rep2.Ext()); aerr == nil {
					utils.Println(utils.Info("[i] Копия в архиве: " + dir))
				} else {
					utils.Println(utils.Warning("[!] Архив не создан: " + aerr.Error()))
				}
			}
		}

	default:
		// "1" и любой другой ввод — сохраняем как раньше, в TXT
		filename := "diagnostic_report.txt"
		if err := saveReportToFile(filename, textReport); err != nil {
			utils.Println(utils.ErrorText(fmt.Sprintf("Ошибка сохранения: %v", err)))
		} else {
			utils.Println(utils.Success(fmt.Sprintf("Полный отчёт сохранён в: %s", filename)))
			if dir, aerr := archive.Save([]byte(textReport), "txt"); aerr == nil {
			baseline.Run(dir) // сохранение baseline-снимка

				utils.Println(utils.Info("[i] Копия в архиве: " + dir))
			} else {
				utils.Println(utils.Warning("[!] Архив не создан: " + aerr.Error()))
			}
		}
	}
}

// colorizeReport добавляет ANSI-цвета в текст отчёта при выводе в консоль.
func colorizeReport(text string) string {
	lines := strings.Split(text, "\n")
	var result strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.Contains(trimmed, "ОТЧЁТ ДИАГНОСТИКИ ПК"):
			result.WriteString(utils.Title(line) + "\n")
		case strings.HasPrefix(trimmed, "===") && strings.HasSuffix(trimmed, "==="):
			result.WriteString(utils.Header(line) + "\n")
		case trimmed == "---":
			result.WriteString(utils.Dim(line) + "\n")
		case strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, " ") && !strings.HasPrefix(trimmed, "\t"):
			result.WriteString(utils.Bold(utils.Info(line)) + "\n")
		case strings.HasSuffix(trimmed, ":") && (strings.HasPrefix(trimmed, " ") || strings.HasPrefix(trimmed, "\t")):
			result.WriteString(utils.Cyan(line) + "\n")
		case strings.Contains(trimmed, "ОШИБКА") || strings.Contains(trimmed, "Ошибка соединения"):
			result.WriteString(utils.ErrorText(line) + "\n")
		case strings.Contains(trimmed, "ПРЕДУПРЕЖДЕНИЕ"):
			result.WriteString(utils.Warning(line) + "\n")
		case strings.Contains(trimmed, "Тест скорости диска"):
			result.WriteString(utils.Bold(line) + "\n")
		default:
			result.WriteString(line + "\n")
		}
	}

	return result.String()
}

func printCompactSummary(results []hardware.DiagnosticResult) {
	utils.Println("")
	utils.Println(utils.Header("Сбор данных завершён:"))
	for _, res := range results {
		cleanVal := strings.TrimSpace(res.Value)
		lines := strings.Split(cleanVal, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasSuffix(line, ":") {
				utils.Printf("- %s: %s", utils.Bold(res.Name), line)
				break
			}
		}
	}
}

// --- РЕЖИМ АВТОМАТИЗАЦИИ (CLI-ФЛАГИ) ---

// runCLIMode выполняет действия, заданные флагами, без интерактивного меню.
// Примеры: pc-diagnostic --full-test --json-report
//          pc-diagnostic --cpu --dur 120
func runCLIMode() {
	// Веб-режим: сервер сам открывает браузер, остальное не требуется.
	if *flagWeb {
		web.StartServer()
		return
	}

	dur := *flagDur
	if dur < 10 {
		dur = 10
	}
	if dur > 600 {
		dur = 600
	}

	// Стресс-тесты выполняются последовательно.
	if *flagCPU {
		utils.Println(utils.Info(fmt.Sprintf("[•] Стресс-тест CPU, %d сек...", dur)))
		utils.Println(stress.RunCPU(dur, 90))
	}
	if *flagMemory {
		utils.Println(utils.Info(fmt.Sprintf("[•] Стресс-тест RAM, %d сек...", dur)))
		utils.Println(stress.RunRAM(dur))
	}
	if *flagDisk {
		utils.Println(utils.Info(fmt.Sprintf("[•] Стресс-тест диска, %d сек...", dur)))
		utils.Println(stress.RunDisk(dur))
	}

	// Полная диагностика: собираем отчёт и сохраняем в файл.
	// --json-report подразумевает --full-test.
	if *flagFull || *flagJSON {
		utils.Println(utils.Info("[•] Полная диагностика..."))
		hwResults := hardware.RunDiagnostics()
		fullReport, structured := getFullReport(hwResults)

		var data []byte
		var err error
		ext := "txt"
		if *flagJSON {
			rep := report.JSONReporter{}
			data, err = rep.Write(structured)
			ext = rep.Ext()
		} else {
			data = []byte(fullReport)
		}

		if err != nil {
			utils.Println(utils.ErrorText(fmt.Sprintf("Ошибка формирования отчёта: %v", err)))
			os.Exit(1)
		}

		filename := "diagnostic_report." + ext
		if err := os.WriteFile(filename, data, 0644); err != nil {
			utils.Println(utils.ErrorText(fmt.Sprintf("Ошибка сохранения: %v", err)))
			os.Exit(1)
		}
		utils.Println(utils.Success("Отчёт сохранён в: " + filename))

		if dir, aerr := archive.Save(data, ext); aerr == nil {
			utils.Println(utils.Info("[i] Копия в архиве: " + dir))
			baseline.Run(dir) // сохранение baseline-снимка

		} else {
			utils.Println(utils.Warning("[!] Архив не создан: " + aerr.Error()))
		}
	}

	// Флаги заданы, но ни одно действие не выбрано — показываем справку.
	if !*flagCPU && !*flagMemory && !*flagDisk && !*flagFull && !*flagJSON && !*flagWeb {
		utils.Println(utils.Warning("Указаны параметры без действия. Доступные флаги:"))
		flag.Usage()
	}
}

// --- МЕНЮ ---

func printMainMenu() {
	utils.Println("")
	utils.Println(utils.Header("========================================"))
	utils.Println(utils.Title("    PC-Diagnostic   "))
	utils.Println(utils.Header("========================================"))
	utils.Println(utils.Info("[1]") + " Выполнить разовую диагностику (Создать отчёт)")
	utils.Println(utils.Info("[2]") + " Мониторинг реального времени (ASCII-графики)")
	utils.Println(utils.Info("[3]") + " Стресс-тест CPU (все ядра)")
	utils.Println(utils.Info("[4]") + " Стресс-тест RAM (память)")
	utils.Println(utils.Info("[5]") + " Стресс-тест диска (запись)")
	utils.Println(utils.Info("[6]") + " Комбинированный стресс-тест (CPU + RAM)")
	utils.Println(utils.Info("[7]") + " Стресс-тест GPU (WebGL, в браузере)")
	utils.Println(utils.Info("[8]") + " История мониторинга (сводка по журналу)")
	utils.Println(utils.Info("[9]") + " Архивы отчётов (список и сравнение)")
	utils.Println(utils.ErrorText("[Q]") + " Назад к выбору режима")
	utils.PrintMenu("Ваш выбор: ")
}

func printOutputMenu() {
	utils.Println("")
	utils.Println(utils.Header("--- Отчёт сформирован ---"))
	utils.Println(utils.Info("[1]") + " Показать КРАТКУЮ сводку в консоли")
	utils.Println(utils.Info("[2]") + " Вывести ПОЛНЫЙ текст отчёта в консоль")
	utils.Println(utils.Info("[3]") + " Сохранить в файл (TXT/JSON/CSV/HTML)")
	utils.Println(utils.ErrorText("[Q]") + " Вернуться в главное меню")
	utils.PrintMenu("Ваш выбор: ")
}

func printGlobalMenu() {
	utils.Println("")
	utils.Println(utils.Header("========================================"))
	utils.Println(utils.Title("    PC-Diagnostic     "))
	utils.Println(utils.Header("========================================"))
	utils.Println(utils.Info("[C]") + " Консольный режим (Отчёты)")
	utils.Println(utils.Info("[W]") + " Веб-интерфейс (http://localhost:9090)")
	utils.Println(utils.ErrorText("[Q]") + " Выход")
	utils.PrintMenu("Выберите режим работы [C/W/Q]: ")
}

// runArchivesMenu — пункт меню 9: список архивов и сравнение двух из них.
func runArchivesMenu(reader *bufio.Reader) {
	utils.Println("")
	dirs := archive.List()
	if len(dirs) == 0 {
		//utils.Println(utils.Info("[i] Архивов пока нет. Сохраните отчёт (пункт 4) — появится первый архив в папке logs/."))
		utils.Println(utils.Info("[i] Архивов пока нет. Сохраните отчёт (пункт 1, затем выбор формата 3) — появится первый архив в папке logs/."))

	} else {
		utils.Printf("Доступные архивы (%d):\n", len(dirs))
		for i, d := range dirs {
			utils.Printf("  [%d] %s\n", i+1, d)
		}
		utils.Println("")
		utils.Print("Номер первого архива для сравнения: ")
		line, _ := reader.ReadString('\n')
		a, err1 := strconv.Atoi(strings.TrimSpace(line))
		if err1 == nil && a >= 1 && a <= len(dirs) {
			utils.Print("Номер второго архива: ")
			line, _ = reader.ReadString('\n')
			b, err2 := strconv.Atoi(strings.TrimSpace(line))
			if err2 == nil && b >= 1 && b <= len(dirs) && b != a {
				archive.Compare(
					filepath.Join("logs", dirs[a-1]),
					filepath.Join("logs", dirs[b-1]),
				)
				fmt.Print(baseline.CompareDirs(
    filepath.Join("logs", dirs[a-1]),
    filepath.Join("logs", dirs[b-1]),
))

			} else {
				utils.Println(utils.Warning("Неверный номер или совпадает с первым."))
			}
		} else {
			utils.Println(utils.Warning("Неверный номер архива."))
		}
	}
	utils.Println("")
	utils.Println(utils.Dim("Нажмите Enter для возврата в меню..."))
	_, _ = reader.ReadString('\n')
}

func runConsoleMode(reader *bufio.Reader) {
	for {
		printMainMenu()
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(strings.ToLower(choice))

		switch choice {
		case "1":
			start := time.Now()
			utils.Println(utils.Info("[•] Сбор данных об оборудовании..."))
			hwResults := hardware.RunDiagnostics()
			elapsed := time.Since(start)
			utils.Println(utils.Success("[✓] Оборудование проверено."))

			// Отчёт собирается один раз: текстовая и структурированная версии
			fullReport, structuredReport := getFullReport(hwResults)

			// Внутренний цикл меню вывода — работает, пока не выберут выход.
		OUTPUT_MENU:
			for {
				printOutputMenu()
				outChoice, _ := reader.ReadString('\n')
				outChoice = strings.TrimSpace(strings.ToLower(outChoice))

				switch outChoice {
				case "1":
					printCompactSummary(hwResults)
					utils.Printf("\nВремя выполнения: %v\n", elapsed.Round(time.Second))
					_, _ = reader.ReadString('\n')
					// остаёмся в этом меню

				case "2":
					utils.Println("")
					utils.Println(utils.Header("====================== НАЧАЛО ОТЧЁТА ======================"))
					utils.Println(colorizeReport(fullReport))
					utils.Println(utils.Header("======================== КОНЕЦ ОТЧЁТА ====================="))
					_, _ = reader.ReadString('\n')
					// остаёмся в этом меню

				case "3":
					saveReportAnyFormat(structuredReport, fullReport, reader)
					// после сохранения снова показываем меню —
					// можно сохранить ещё в один формат

				case "q", "0":
					break OUTPUT_MENU // вернуться в главное меню консоли

				default:
					utils.Println(utils.Warning("Неверный ввод."))
					time.Sleep(1 * time.Second)
				}
			}

		case "2":
			utils.Println("")
			utils.Println(utils.Info("Запуск визуального мониторинга... Нажмите Enter для выхода."))
			utils.Println("")
			monitor.StartMonitoring()

		case "3":
			utils.Println("")
			utils.Println(utils.Info("Тест загрузит все ядра на 60 секунд. Не закрывайте окно."))
			result := stress.RunCPU(60, 90)
			utils.Println(result)
			utils.Println("")
			utils.Println(utils.Dim("Нажмите Enter для возврата в меню..."))
			_, _ = reader.ReadString('\n')

		case "4":
			utils.Println("")
			utils.Println(utils.Info("Тест запишет и проверит ~128 МБ буфера в цикле 60 секунд."))
			result := stress.RunRAM(60)
			utils.Println(result)
			utils.Println("")
			utils.Println(utils.Dim("Нажмите Enter для возврата в меню..."))
			_, _ = reader.ReadString('\n')

		case "5":
			utils.Println("")
			utils.Println(utils.Info("Тест создаст временный файл и запишет в него ~30 ГБ за 60 секунд, затем удалит его."))
			result := stress.RunDisk(60)
			utils.Println(result)
			utils.Println("")
			utils.Println(utils.Dim("Нажмите Enter для возврата в меню..."))
			_, _ = reader.ReadString('\n')

		case "6":
			utils.Println("")
			utils.Println(utils.Info("CPU на всех ядрах и запись RAM одновременно, 60 секунд."))
			result := stress.RunCombined(60, 90)
			utils.Println(result)
			utils.Println("")
			utils.Println(utils.Dim("Нажмите Enter для возврата в меню..."))
			_, _ = reader.ReadString('\n')

		case "7":
			utils.Println("")
			utils.Println(utils.Info("GPU-тест выполняется в браузере через WebGL."))
			utils.Println(utils.Info("Открываю веб-интерфейс со страницы стресс-тестов..."))
			web.SetStartPath("/stress?autostart=gpu")
			web.StartServer()
			utils.Println(utils.Dim("Возврат в меню..."))

		case "8":
			utils.Println("")
			history.Print()
			utils.Println("")
			utils.Println(utils.Dim("Нажмите Enter для возврата в меню..."))
			_, _ = reader.ReadString('\n')

		case "9":
			runArchivesMenu(reader)

		case "q", "quit", "exit", "0", "b", "back":
			return

		default:
			utils.Println(utils.Warning("Неверный ввод. Используйте 1-9 или Q."))
			time.Sleep(1 * time.Second)
		}
	}
}

// stripFirstLine удаляет первую строку, если она совпадает с названием секции
func stripFirstLine(text string) string {
	lines := strings.SplitN(text, "\n", 2)
	if len(lines) > 1 {
		first := strings.TrimSpace(lines[0])
		if strings.HasSuffix(first, ":") ||
			(strings.HasPrefix(first, "===") && strings.HasSuffix(first, "===")) {
			return strings.TrimSpace(lines[1])
		}
	}
	return strings.TrimSpace(text)
}

// printUsage — собственная справка: стандартная flag.Usage не знает про --schedule.
func printUsage() {
	fmt.Println("PC-Diagnostic — диагностика ПК, стресс-тесты и мониторинг")
	fmt.Println()
	fmt.Println("Без флагов: интерактивное меню (консольный или веб-режим).")
	fmt.Println()
	fmt.Println("Флаги:")
	fmt.Println("  --web              Запустить веб-интерфейс (http://localhost:9090)")
	fmt.Println("  --cpu              Стресс-тест CPU")
	fmt.Println("  --memory           Стресс-тест RAM")
	fmt.Println("  --disk             Стресс-тест диска")
	fmt.Println("  --dur N            Длительность стресс-тестов, сек (10-600, по умолчанию 60)")
	fmt.Println("  --full-test        Полная диагностика с сохранением отчёта (TXT)")
	fmt.Println("  --json-report      Отчёт в формате JSON (подразумевает --full-test)")
	fmt.Println("  --schedule[=N]     Сбор метрик по расписанию каждые N сек")
	fmt.Println("                     (по умолчанию 60, минимум 5). Интервал также можно")
	fmt.Println("                     задать в config.json (schedule.interval_seconds).")
	fmt.Println("                     Остановка — Ctrl+C. Журнал: schedule_log.csv")
	fmt.Println()
	fmt.Println("Примеры:")
	fmt.Println("  pc-diagnostic.exe --cpu --dur 120")
	fmt.Println("  pc-diagnostic.exe --full-test --json-report")
	fmt.Println("  pc-diagnostic.exe --schedule=30")
	fmt.Println("  pc-diagnostic.exe --web")
}

// --- MAIN ---

func main() {
	utils.EnsureWindowsANSI()
	if scheduler.RunIfRequested() {
		return // флаг --schedule: сбор запущен и завершён — выходим
	}
	flag.Usage = printUsage // своя справка, чтобы был виден --schedule
	flag.Parse()

	// Если задан хотя бы один флаг — режим автоматизации без меню.
	// Без флагов программа работает как раньше: интерактивные меню.
	if flag.NFlag() > 0 {
		runCLIMode()
		return
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		printGlobalMenu()
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(strings.ToLower(choice))

		switch choice {
		case "c", "console":
			runConsoleMode(reader)

		case "w", "web":
			utils.Println("")
			web.StartServer()

		case "q", "quit", "exit", "0":
			utils.Println(utils.Dim("Завершение работы."))
			os.Exit(0)

		default:
			utils.Println(utils.Warning("Неверный ввод. Используйте C, W или Q."))
			time.Sleep(1 * time.Second)
		}
	}
}
