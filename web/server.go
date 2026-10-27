package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	
	"time"
	

	"pc-diagnostic/hardware"
	"pc-diagnostic/monitor"
	"pc-diagnostic/report"
	"pc-diagnostic/system"
	"pc-diagnostic/utils"
	"pc-diagnostic/thresholds"
		"pc-diagnostic/baseline"

)

type PageData struct {
	Title      string
	Date       string
	Hardware   []hardware.DiagnosticResult
	SystemInfo map[string]string
}

var (
	lastHeartbeat   time.Time
	mu              sync.Mutex
	server          *http.Server
	serverDone      chan struct{}
	lastReport      report.Report
	lastReportReady bool
)

func openBrowser(url string) {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		_ = exec.Command("open", url).Start()
	default:
		_ = exec.Command("xdg-open", url).Start()
	}
}


func StartServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", landingHandler)
	mux.HandleFunc("/diagnostic", renderHandler)
	mux.HandleFunc("/api/diagnostic", apiDiagnosticHandler)
	mux.HandleFunc("/monitor", monitorHandler)
	mux.HandleFunc("/api/stats", statsHandler)
		mux.HandleFunc("/api/disk", apiDiskHandler)
	mux.HandleFunc("/disks", disksPageHandler)
	mux.HandleFunc("/api/archives", apiArchivesHandler)
	mux.HandleFunc("/api/archives/compare", apiArchivesCompareHandler)
	mux.HandleFunc("/archives", archivesPageHandler)
	mux.HandleFunc("/api/history", apiHistoryHandler)
	mux.HandleFunc("/history", historyPageHandler)
		mux.HandleFunc("/api/summary", apiSummaryHandler)
	mux.HandleFunc("/summary", summaryPageHandler)

	mux.HandleFunc("/api/baseline", apiBaselineHandler)

	mux.HandleFunc("/stress", stressPageHandler)
	mux.HandleFunc("/api/stress/start", stressStartHandler)
	mux.HandleFunc("/api/stress/status", stressStatusHandler)
	mux.HandleFunc("/api/stress/gpu/start", stressGPUStartHandler)
	mux.HandleFunc("/api/stress/gpu/progress", stressGPUProgressHandler)
	mux.HandleFunc("/api/stress/gpu/finish", stressGPUFinishHandler)
	mux.HandleFunc("/api/report", apiReportHandler)
	mux.HandleFunc("/heartbeat", heartbeatHandler)
	mux.HandleFunc("/shutdown", shutdownHandler)

	const port = ":9090"

	mu.Lock()
	lastHeartbeat = time.Now()
	mu.Unlock()

	ln, err := net.Listen("tcp", port)
	if err != nil {
		fmt.Printf("Не удалось запустить сервер: %v\n", err)
		return
	}

	server = &http.Server{
		Handler:      withBasicAuth(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Minute,
		IdleTimeout:  120 * time.Second,
	}
	serverDone = make(chan struct{})

	go func() {
		err := server.Serve(ln)
		if err != nil && err != http.ErrServerClosed {
			fmt.Printf("\nСервер остановлен: %v\n", err)
		}
		close(serverDone)
	}()

	go func() {
		for {
			select {
			case <-serverDone:
				return
			case <-time.After(10 * time.Second):
				mu.Lock()
				idle := time.Since(lastHeartbeat)
				mu.Unlock()
				if idle > 60*time.Second {
					fmt.Println("\nВкладка браузера закрыта. Остановка сервера...")
					if server != nil {
						_ = server.Close()
					}
					return
				}
			}
		}
	}()

	fmt.Println("Веб-сервер запущен на http://localhost:9090")

	time.Sleep(500 * time.Millisecond)

	fmt.Println("Открываем браузер...")
	mu.Lock()
	target := startPath
	startPath = "/"
	mu.Unlock()
	openBrowser("http://localhost:9090" + target)

	<-serverDone
	fmt.Println("\nВозврат в меню...")
	time.Sleep(500 * time.Millisecond)
}

func heartbeatHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	lastHeartbeat = time.Now()
	mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func shutdownHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	go func() {
		time.Sleep(300 * time.Millisecond)
		if server != nil {
			_ = server.Close()
		}
	}()
}
	// apiBaselineHandler возвращает дельту текущих метрик относительно
// последнего сохранённого baseline-снимка (без создания нового).
func apiBaselineHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	newSnap := baseline.Take()
	if old, ok := baseline.LoadLast(); ok {
		_, _ = w.Write([]byte(baseline.CompareText(old, newSnap)))
		return
	}
	_, _ = w.Write([]byte("Baseline-снимка ещё нет. Скачайте отчёт (кнопки экспорта на странице «Диагностика») — снимок создастся автоматически.\n"))
}
func statsHandler(w http.ResponseWriter, r *http.Request) {
	stats := monitor.GetStats()

	// Оборачиваем статистику в map, чтобы добавить статусы,
	// не завися от внутреннего типа структуры monitor.Stats.
	data, _ := json.Marshal(stats)
	var m map[string]interface{}
	_ = json.Unmarshal(data, &m)

	getF := func(k string) float64 {
		v, _ := m[k].(float64)
		return v
	}

	stCPU := thresholds.ForCPU(getF("cpu_total"))
	stRAM := thresholds.ForRAM(getF("ram_used_pct"))
	stTemp := thresholds.ForTemp(getF("cpu_temp"))

	m["status_cpu"] = string(stCPU)
	m["status_ram"] = string(stRAM)
	m["status_temp"] = string(stTemp)
	m["status_overall"] = string(thresholds.Worst(stCPU, stRAM, stTemp))

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(m)
}

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

// =================== ЛЕНДИНГ ===================

func landingHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	html := `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<title>Диагностика ПК</title>
<style>
body { font-family: Arial, sans-serif; background:#f5f7fa; color:#333; padding: 20px; max-width: 700px; margin: 0 auto; }
h1 { color: #2c3e50; border-bottom: 2px solid #3498db; padding-bottom: 10px; }
.nav-card { background:white; border-radius:8px; padding:30px; margin:20px 0; box-shadow:0 2px 5px rgba(0,0,0,0.1); text-align:center; transition: transform 0.2s, box-shadow 0.2s; }
.nav-card:hover { transform: translateY(-3px); box-shadow:0 4px 12px rgba(0,0,0,0.15); }
.nav-card a { text-decoration: none; color: #2c3e50; display: block; }
.nav-card h2 { color: #3498db; margin: 0 0 8px 0; }
.nav-card p { color: #777; margin: 0; }
footer { margin-top:40px; color:#777; text-align:center; }
.top-bar { display: flex; justify-content: space-between; align-items: center; }
.btn-exit { background:#e74c3c; color:white; border:none; padding:8px 20px; border-radius:5px; cursor:pointer; font-size:14px; transition: background 0.2s; }
.btn-exit:hover { background:#c0392b; }
</style>
<script>
var heartbeatTimer = null;
var alive = true;

function startHeartbeat() {
    if (heartbeatTimer) clearInterval(heartbeatTimer);
    heartbeatTimer = setInterval(function() {
        if (!alive) return;
        fetch('/heartbeat').catch(function() {
            if (!alive) return;
            alive = false;
            if (heartbeatTimer) clearInterval(heartbeatTimer);
            document.body.innerHTML = '<div style="text-align:center; padding-top:100px;">' +
                '<h2>Сервер остановлен</h2>' +
                '<p>Вернитесь в консоль и запустите веб-режим снова.</p></div>';
        });
    }, 3000);
}

document.addEventListener('visibilitychange', function() {
    if (!document.hidden && alive) {
        fetch('/heartbeat').then(function() {}).catch(function() {});
    }
});

function doExit() {
    document.body.innerHTML = '<div style="text-align:center; padding-top:100px;"><h2>Сервер останавливается...</h2><p>Можно закрыть вкладку.</p></div>';
    fetch('/shutdown');
}
</script>
</head>
<body>
<div class="top-bar">
<h1>🖥️ Диагностика ПК</h1>
<button class="btn-exit" onclick="doExit()">✖ Выход</button>
</div>
<p>Выберите режим:</p>
<div class="nav-card"><a href="/diagnostic"><h2>📋 Диагностика ПК</h2><p>Полный отчёт об оборудовании и системе</p></a></div>
<div class="nav-card"><a href="/summary"><h2>⚡ Краткая сводка</h2><p>Одна строка на модуль диагностики</p></a></div>

<div class="nav-card"><a href="/monitor"><h2>📊 Мониторинг</h2><p>Показатели в реальном времени</p></a></div>
<div class="nav-card"><a href="/stress"><h2>🔥 Стресс-тесты</h2><p>Нагрузочные тесты CPU, RAM и диска</p></a></div>
<div class="nav-card"><a href="/disks"><h2>💾 Диски</h2><p>Круговые диаграммы занятого места</p></a></div>
<div class="nav-card"><a href="/archives"><h2>🗂️ Архивы</h2><p>Сравнение отчётов между прогонами</p></a></div>
<div class="nav-card"><a href="/history"><h2>📈 История мониторинга</h2><p>Сводка по журналу наблюдения</p></a></div>


<footer><hr><p>Диагностика ПК &copy; 2026</p></footer>
<script>
startHeartbeat();
</script>
</body>
</html>`

	_, _ = w.Write([]byte(html))
}

// =================== ДИАГНОСТИКА — СТРАНИЦА ЗАГРУЗКИ ===================

func renderHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/diagnostic" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	html := `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<title>Диагностика ПК</title>
<style>
body { font-family: Arial, sans-serif; background:#f5f7fa; color:#333; padding: 20px; }
h1 { color: #2c3e50; border-bottom: 2px solid #3498db; padding-bottom: 10px; }
h2 { color: #34495e; border-left: 4px solid #3498db; padding-left: 10px; }
h3 { color: #2c3e50; margin-top: 20px; }
pre { background:#f4f4f4; padding:10px; border-radius:5px; white-space: pre-wrap; word-wrap: break-word; font-size: 14px; line-height: 1.5; }
.error { color: #c0392b; }
.nav-links a { color: #3498db; text-decoration: none; margin-right: 15px; }
.nav-links a:hover { text-decoration: underline; }
.top-bar { display: flex; justify-content: space-between; align-items: center; }
.btn-exit { background:#e74c3c; color:white; border:none; padding:8px 20px; border-radius:5px; cursor:pointer; font-size:14px; transition: background 0.2s; }
.btn-exit:hover { background:#c0392b; }
.card { background:white; border-radius:8px; padding:20px; margin-bottom:20px; box-shadow:0 2px 5px rgba(0,0,0,0.1); }
.spinner { width: 50px; height: 50px; border: 5px solid #e0e0e0; border-top: 5px solid #3498db; border-radius: 50%; animation: spin 1s linear infinite; margin: 30px auto; }
@keyframes spin { 0% { transform: rotate(0deg); } 100% { transform: rotate(360deg); } }
#loading { text-align: center; padding: 40px; }
#loading p { color: #777; margin-top: 15px; }
.progress-msg { color: #3498db; font-size: 16px; margin-top: 10px; min-height: 24px; }
footer { margin-top:40px; color:#777; text-align:center; }
</style>
<script>
var heartbeatTimer = null;
var alive = true;

function startHeartbeat() {
    if (heartbeatTimer) clearInterval(heartbeatTimer);
    heartbeatTimer = setInterval(function() {
        if (!alive) return;
        fetch('/heartbeat').catch(function() {
            if (!alive) return;
            alive = false;
            if (heartbeatTimer) clearInterval(heartbeatTimer);
            document.body.innerHTML = '<div style="text-align:center; padding-top:100px;">' +
                '<h2>Сервер остановлен</h2>' +
                '<p>Вернитесь в консоль и запустите веб-режим снова.</p></div>';
        });
    }, 3000);
}

document.addEventListener('visibilitychange', function() {
    if (!document.hidden && alive) {
        fetch('/heartbeat').catch(function() {});
    }
});

function doExit() {
    document.body.innerHTML = '<div style="text-align:center; padding-top:100px;"><h2>Сервер останавливается...</h2><p>Можно закрыть вкладку.</p></div>';
    fetch('/shutdown');
}

var progressMessages = [
    "Сбор данных об оборудовании...",
    "Проверка драйверов, автозагрузки, USB...",
    "Проверка целостности системы (sfc /scannow)... ≈ 5-15 мин",
    "Тест скорости интернета...",
    "Проверка дисков (SMART)...",
    "Тест скорости диска...",
    "Формирование отчёта..."
];
var msgIndex = 0;
var progressInterval;

function startProgress() {
    var el = document.getElementById('progress-text');
    progressInterval = setInterval(function() {
        if (msgIndex < progressMessages.length) {
            el.textContent = progressMessages[msgIndex];
            msgIndex++;
        }
    }, 3000);
}

function stopProgress() {
    clearInterval(progressInterval);
}

function loadDiagnostics() {
    fetch('/api/diagnostic')
        .then(function(r) { return r.text(); })
        .then(function(html) {
            stopProgress();
            document.getElementById('loading').style.display = 'none';
            document.getElementById('results').innerHTML = html;
        })
        .catch(function(err) {
            stopProgress();
            document.getElementById('loading').innerHTML =
                '<h2 style="color:#e74c3c;">Ошибка</h2><p>Не удалось получить данные: ' + err + '</p>';
        });
}
</script>
</head>
<body>
<div class="top-bar">
<h1>🖥️ Отчёт диагностики ПК</h1>
<button class="btn-exit" onclick="doExit()">✖ Выход</button>
</div>
<div class="nav-links" style="margin-bottom:15px;">
<a href="/">← Главная</a>
<a href="/monitor">📊 Мониторинг</a>
<span style="color:#777;">Экспорт:</span>
<a href="/api/report?format=txt">TXT</a>
<a href="/api/report?format=json">JSON</a>
<a href="/api/report?format=csv">CSV</a>
<a href="/api/report?format=html">HTML</a>
</div>

<div id="loading">
    <div class="card">
        <h2>⏳ Сбор данных диагностики</h2>
        <div class="spinner"></div>
        <div class="progress-msg" id="progress-text">Подготовка...</div>
        <p>Это может занять 5–15 минут (особенно проверка целостности системы).</p>
    </div>
</div>

<div id="results"></div>

<footer style="margin-top:40px; color:#777; text-align:center;">
<hr><p>Диагностика ПК &copy; 2026</p>
</footer>

<script>
startHeartbeat();
startProgress();
loadDiagnostics();
</script>
</body>
</html>`

	_, _ = w.Write([]byte(html))
}

// =================== ДИАГНОСТИКА — API (ВЫПОЛНЯЕТ РАБОТУ) ===================

func apiDiagnosticHandler(w http.ResponseWriter, r *http.Request) {
	stopKeeper := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopKeeper:
				return
			case <-time.After(5 * time.Second):
				mu.Lock()
				lastHeartbeat = time.Now()
				mu.Unlock()
			}
		}
	}()

	utils.Println(utils.Info("[•] [Веб] Сбор данных об оборудовании..."))
	hwResults := hardware.RunDiagnostics()
	utils.Println(utils.Success("[✓] [Веб] Оборудование проверено."))

	utils.Println(utils.Info("[•] [Веб] Сбор системной информации..."))
	sysInfo := map[string]string{
		"Вентиляторы и напряжения": hardware.GetSensorsInfo(),
		"Диски":              system.GetDiskInfo(),
		"Материнская плата":  system.GetMainboardInfo(),
		"Драйверы":           system.GetDriversInfo(),
		"USB-устройства":     system.GetUSBInfo(),
		"Автозагрузка":             system.GetAutostartInfo(),
		"Процессы":                 system.GetProcessesInfo(),
		"Службы":                   system.GetServicesInfo(),
		"Обновления и пакеты":            system.GetUpdatesInfo(),
		"Целостность системы":      system.CheckSystemIntegrity(),
		"Скорость интернета":       system.TestInternetSpeed(),
		"Звуковые устройства":      system.GetAudioInfo(),

		"Проверка файловой системы": hardware.CheckFileSystemInfo(),
		"Износ SSD (TBW)":           hardware.GetSSDWearInfo(),
		"Фрагментация":              hardware.AnalyzeFragmentation(),
		"Случайный доступ 4K":       hardware.BenchmarkDiskRandom(),

		"Traceroute":          system.GetTracerouteInfo(),
		"Открытые порты":      system.ScanOpenPorts(),
		"Активные соединения": system.GetActiveConnectionsInfo(),
		"DNS-диагностика":     system.GetDNSInfo(),
	}
	utils.Println(utils.Success("[✓] [Веб] Системная информация собрана."))

	var smartResults []hardware.DiagnosticResult
	var diskBenchmark string
	smartPath, _ := exec.LookPath("smartctl")

	utils.Println(utils.Info("[•] [Веб] Проверка дисков (SMART)..."))
	if smartPath != "" {
		smartResults = hardware.RunSMARTDiagnostics()
		wSpd, rSpd, benchErr := hardware.BenchmarkDisk(100)
		if benchErr == nil {
			diskBenchmark = fmt.Sprintf("Запись: %.2f МБ/с | Чтение: %.2f МБ/с", wSpd, rSpd)
		}
	}
	utils.Println(utils.Success("[✓] [Веб] Отчёт сформирован."))

	// Собираем структурированный отчёт для экспорта (TXT/JSON/CSV/HTML)
	structured := report.BuildFromHardware(hwResults)

	// Фиксированный порядок секций (обход map даёт случайный порядок)
	sysOrder := []string{
		"Вентиляторы и напряжения",
		"Диски",
		"Материнская плата",
		"Драйверы",
		"USB-устройства",
		"Автозагрузка",
		"Процессы",
		"Службы",
		"Обновления и пакеты",
		"Целостность системы",
		"Скорость интернета",
		"Звуковые устройства",
		"Проверка файловой системы",
		"Износ SSD (TBW)",
		"Фрагментация",
		"Случайный доступ 4K",
		"Traceroute",
		"Открытые порты",
		"Активные соединения",
		"DNS-диагностика",
	}
	for _, key := range sysOrder {
		structured.AddText(key, sysInfo[key])
	}

	for _, res := range smartResults {
		if res.Error != nil {
			structured.AddText("S.M.A.R.T. — "+res.Name, "ОШИБКА: "+res.Error.Error())
		} else {
			structured.AddText("S.M.A.R.T. — "+res.Name, strings.TrimSpace(res.Value))
		}
	}
	if diskBenchmark != "" {
		structured.AddText("Тест скорости диска", diskBenchmark)
	}

	mu.Lock()
	lastReport = structured
	lastReportReady = true
	mu.Unlock()

	close(stopKeeper)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	var html strings.Builder

	if len(hwResults) > 0 {
		html.WriteString(`<div class="card"><h2>Оборудование</h2>`)
		for _, res := range hwResults {
			html.WriteString(fmt.Sprintf("<h3>%s</h3>\n", template.HTMLEscapeString(res.Name)))
			if res.Error != nil {
				html.WriteString(fmt.Sprintf("<p class='error'>Ошибка: %s</p>", template.HTMLEscapeString(res.Error.Error())))
			} else {
				safeVal := template.HTMLEscapeString(res.Value)
				safeVal = strings.ReplaceAll(safeVal, "\n", "<br>")
				html.WriteString("<pre>" + safeVal + "</pre>\n")
			}
		}
		html.WriteString("</div>\n")
	}

	if len(sysInfo) > 0 {
		html.WriteString(`<div class="card"><h2>Системная информация</h2>`)
		for _, key := range sysOrder {
			val := sysInfo[key]
			if strings.TrimSpace(val) == "" {
				continue // секция не заполнена — не рисуем пустой заголовок
			}
			safeKey := template.HTMLEscapeString(key)
			cleanVal := stripFirstLine(val)
			safeVal := template.HTMLEscapeString(cleanVal)
			safeVal = strings.ReplaceAll(safeVal, "\n", "<br>")
			html.WriteString(fmt.Sprintf("<h3>%s</h3><pre>%s</pre>\n", safeKey, safeVal))
		}
		html.WriteString("</div>\n")
	}

	if len(smartResults) > 0 {
		html.WriteString(`<div class="card"><h2>S.M.A.R.T. диагностика дисков</h2>`)
		for _, res := range smartResults {
			html.WriteString(fmt.Sprintf("<h3>%s</h3>\n", template.HTMLEscapeString(res.Name)))
			if res.Error != nil {
				html.WriteString(fmt.Sprintf("<p class='error'>Ошибка: %s</p>", template.HTMLEscapeString(res.Error.Error())))
			} else {
				safeVal := template.HTMLEscapeString(res.Value)
				safeVal = strings.ReplaceAll(safeVal, "\n", "<br>")
				html.WriteString("<pre>" + safeVal + "</pre>\n")
			}
		}
		if diskBenchmark != "" {
			html.WriteString(fmt.Sprintf("<h3>Тест скорости диска</h3><pre>%s</pre>\n", template.HTMLEscapeString(diskBenchmark)))
		}
		html.WriteString("</div>\n")
	} else if smartPath == "" {
		html.WriteString(`<div class="card"><h2>S.M.A.R.T.</h2><p>Утилита 'smartctl' не найдена. Скачайте <a href="https://www.smartmontools.org/" target="_blank">Smartmontools</a>.</p></div>`)
	}

	_, _ = w.Write([]byte(html.String()))
}

// =================== ЭКСПОРТ ОТЧЁТА ===================

// apiReportHandler отдаёт последний сформированный отчёт в выбранном формате.
// Формат: /api/report?format=json|csv|html|txt (по умолчанию json).
func apiReportHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	ready := lastReportReady
	rep := lastReport
	mu.Unlock()

	if !ready {
		http.Error(w, "Отчёт ещё не сформирован. Сначала откройте страницу «Диагностика ПК».", http.StatusServiceUnavailable)
		return
	}

	var rep2 report.Reporter
	var mime string
	switch r.URL.Query().Get("format") {
	case "csv":
		rep2 = report.CSVReporter{}
		mime = "text/csv; charset=utf-8"
	case "html":
		rep2 = report.HTMLReporter{}
		mime = "text/html; charset=utf-8"
	case "txt":
		rep2 = report.TXTReporter{}
		mime = "text/plain; charset=utf-8"
	default:
		rep2 = report.JSONReporter{}
		mime = "application/json; charset=utf-8"
	}

	data, err := rep2.Write(rep)
	if err != nil {
		http.Error(w, "Ошибка формирования отчёта: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Скачивание отчёта из веба = сохранение отчёта в консоли:
	// фиксируем baseline-снимок и печатаем дельту в консоль сервера.
	baseline.Run("")

	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", `attachment; filename="diagnostic_report.`+rep2.Ext()+`"`)
	_, _ = w.Write(data)
}

// =================== МОНИТОРИНГ ===================

func monitorHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	html := `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<title>Мониторинг — Диагностика ПК</title>
<style>
body { font-family: Arial, sans-serif; background:#f5f7fa; color:#333; padding: 20px; max-width: 900px; margin: 0 auto; }
h1 { color: #2c3e50; border-bottom: 2px solid #3498db; padding-bottom: 10px; }
h2 { color: #34495e; border-left: 4px solid #3498db; padding-left: 10px; }
.card { background:white; border-radius:8px; padding:20px; margin-bottom:20px; box-shadow:0 2px 5px rgba(0,0,0,0.1); }
.stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 15px; }
.stat-card { background:#f8f9fa; border-radius:8px; padding:15px; text-align:center; }
.stat-value { font-size: 28px; font-weight: bold; margin: 5px 0; transition: color 0.5s; }
.stat-label { color: #666; font-size: 12px; text-transform: uppercase; letter-spacing: 1px; }
.stat-sub { color: #888; font-size: 12px; margin-top: 4px; }
.progress-bar { background:#e0e0e0; border-radius:4px; overflow:hidden; height: 20px; margin-top:8px; }
.progress-fill { height: 100%; border-radius:4px; transition: width 0.5s ease, background 0.5s ease; }
.cores-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 10px; }
.core-item { display: flex; align-items: center; gap: 10px; }
.core-label { font-size: 13px; font-weight: bold; min-width: 55px; }
.core-bar { flex: 1; height: 16px; background:#e0e0e0; border-radius:4px; overflow:hidden; }
.core-fill { height: 100%; border-radius:4px; transition: width 0.5s ease, background 0.5s ease; }
.core-val { font-size: 12px; min-width: 45px; text-align: right; }
.nav-links a { color: #3498db; text-decoration: none; margin-right: 15px; }
.nav-links a:hover { text-decoration: underline; }
#chart { width: 100%; height: 200px; display: block; }
.legend { display: flex; gap: 20px; margin-top: 10px; }
.legend-item { display: flex; align-items: center; gap: 5px; font-size: 13px; }
.legend-color { width: 14px; height: 14px; border-radius: 2px; }
.clock { color: #888; font-size: 14px; }
.status-line { font-size: 18px; font-weight: bold; margin: 10px 0; }
footer { margin-top:40px; color:#777; text-align:center; }
.top-bar { display: flex; justify-content: space-between; align-items: center; }
.btn-exit { background:#e74c3c; color:white; border:none; padding:8px 20px; border-radius:5px; cursor:pointer; font-size:14px; transition: background 0.2s; }
.btn-exit:hover { background:#c0392b; }
</style>
<script>
var heartbeatTimer = null;
var alive = true;
var cpuHistory = [];
var ramHistory = [];
var tempHistory = [];

var MAX_HISTORY = 60;

function startHeartbeat() {
    if (heartbeatTimer) clearInterval(heartbeatTimer);
    heartbeatTimer = setInterval(function() {
        if (!alive) return;
        fetch('/heartbeat').catch(function() {
            if (!alive) return;
            alive = false;
            if (heartbeatTimer) clearInterval(heartbeatTimer);
            document.body.innerHTML = '<div style="text-align:center; padding-top:100px;">' +
                '<h2>Сервер остановлен</h2>' +
                '<p>Вернитесь в консоль и запустите веб-режим снова.</p></div>';
        });
    }, 3000);
}

document.addEventListener('visibilitychange', function() {
    if (!document.hidden && alive) {
        fetch('/heartbeat').then(function() {
            if (typeof fetchStats === 'function') fetchStats();
        }).catch(function() {});
    }
});

function doExit() {
    document.body.innerHTML = '<div style="text-align:center; padding-top:100px;"><h2>Сервер останавливается...</h2><p>Можно закрыть вкладку.</p></div>';
    fetch('/shutdown');
}

function getColor(pct) {
    if (pct < 50) return '#2ecc71';
    if (pct < 80) return '#f39c12';
    return '#e74c3c';
}

async function fetchStats() {
    try {
        var res = await fetch('/api/stats');
        var d = await res.json();

        var statusNames = { Normal: 'НОРМА', Warning: 'ВНИМАНИЕ', Critical: 'КРИТИЧНО', Unknown: 'НЕТ ДАННЫХ' };
        var statusColors = { Normal: '#2ecc71', Warning: '#f39c12', Critical: '#e74c3c', Unknown: '#888' };
        var stEl = document.getElementById('overall-status');
        if (stEl) {
            stEl.textContent = 'Состояние системы: ' + (statusNames[d.status_overall] || d.status_overall);
            stEl.style.color = statusColors[d.status_overall] || '#888';
        }

        document.getElementById('clock').textContent = 'Время: ' + d.timestamp;

        var cpuEl = document.getElementById('cpu-total');
        cpuEl.textContent = d.cpu_total.toFixed(1) + '%';
        cpuEl.style.color = getColor(d.cpu_total);
        var cpuBar = document.getElementById('cpu-bar');
        cpuBar.style.width = d.cpu_total + '%';
        cpuBar.style.background = getColor(d.cpu_total);

        var ramEl = document.getElementById('ram-pct');
        ramEl.textContent = d.ram_used_pct.toFixed(1) + '%';
        ramEl.style.color = getColor(d.ram_used_pct);
        document.getElementById('ram-detail').textContent =
            d.ram_used_gb.toFixed(2) + ' / ' + d.ram_total_gb.toFixed(2) + ' GB';
        var ramBar = document.getElementById('ram-bar');
        ramBar.style.width = d.ram_used_pct + '%';
        ramBar.style.background = getColor(d.ram_used_pct);

        if (d.swap_total_gb > 0) {
            var swapEl = document.getElementById('swap-pct');
            swapEl.textContent = d.swap_used_pct.toFixed(1) + '%';
            swapEl.style.color = getColor(d.swap_used_pct);
            document.getElementById('swap-detail').textContent =
                d.swap_used_gb.toFixed(2) + ' / ' + d.swap_total_gb.toFixed(2) + ' GB';
            var swapBar = document.getElementById('swap-bar');
            swapBar.style.width = d.swap_used_pct + '%';
            swapBar.style.background = getColor(d.swap_used_pct);
        } else {
            document.getElementById('swap-pct').textContent = '—';
            document.getElementById('swap-detail').textContent = 'не используется';
        }

        if (d.cpu_temp > 0) {
            var tempEl = document.getElementById('temp');
            tempEl.textContent = d.cpu_temp.toFixed(0) + ' °C';
            tempEl.style.color = d.cpu_temp > 80 ? '#e74c3c' : d.cpu_temp > 60 ? '#f39c12' : '#2ecc71';
            document.getElementById('temp-sub').textContent = '';
			            tempHistory.push(d.cpu_temp);

        }
        if (d.cpu_temp <= 0) tempHistory.push(null);
        if (tempHistory.length > MAX_HISTORY) tempHistory.shift();

        var coresHTML = '';
        for (var i = 0; i < d.cpu_cores.length; i++) {
            var pct = d.cpu_cores[i];
            var color = getColor(pct);
            coresHTML += '<div class="core-item">' +
                '<span class="core-label">Core ' + (i+1) + '</span>' +
                '<div class="core-bar"><div class="core-fill" style="width:' + pct + '%; background:' + color + ';"></div></div>' +
                '<span class="core-val">' + pct.toFixed(0) + '%</span>' +
                '</div>';
        }
        document.getElementById('cores').innerHTML = coresHTML;

        cpuHistory.push(d.cpu_total);
        ramHistory.push(d.ram_used_pct);
        if (cpuHistory.length > MAX_HISTORY) cpuHistory.shift();
        if (ramHistory.length > MAX_HISTORY) ramHistory.shift();
        drawChart();
    } catch(e) {
        console.error('Fetch error:', e);
    }
}

function drawChart() {
    var canvas = document.getElementById('chart');
    var ctx = canvas.getContext('2d');
    var w = canvas.width = canvas.offsetWidth;
    var h = canvas.height = 200;
    var pad = 35;

    ctx.clearRect(0, 0, w, h);

    ctx.strokeStyle = '#eee';
    ctx.lineWidth = 1;
    for (var i = 0; i <= 4; i++) {
        var y = (h / 4) * i;
        ctx.beginPath();
        ctx.moveTo(pad, y);
        ctx.lineTo(w, y);
        ctx.stroke();
    }

    ctx.fillStyle = '#999';
    ctx.font = '10px Arial';
    ctx.textAlign = 'right';
    for (var i = 0; i <= 4; i++) {
        var y = (h / 4) * i;
        ctx.fillText((100 - i * 25) + '%', pad - 5, y + 3);
    }

    drawLine(ctx, cpuHistory, w, h, '#3498db', pad);
    drawLine(ctx, ramHistory, w, h, '#e74c3c', pad);
	    drawLine(ctx, tempHistory, w, h, '#f39c12', pad);

}

function drawLine(ctx, data, w, h, color, pad) {
    if (data.length < 2) return;
    var chartW = w - pad;
    var step = chartW / (MAX_HISTORY - 1);

    ctx.strokeStyle = color;
    ctx.lineWidth = 2;
    var pen = false;
    for (var i = 0; i < data.length; i++) {
        var x = pad + i * step;
        if (data[i] === null) { pen = false; continue; }
        var y = h - (data[i] / 100) * h;
        if (!pen) { ctx.beginPath(); ctx.moveTo(x, y); pen = true; }
        else { ctx.lineTo(x, y); }
    }
    if (pen) ctx.stroke();
}

</script>
</head>
<body>
<div class="top-bar">
<h1>📊 Мониторинг реального времени</h1>
<button class="btn-exit" onclick="doExit()">✖ Выход</button>
</div>
<p class="clock" id="clock">Загрузка...</p>
<div class="status-line" id="overall-status">Состояние системы: —</div>

<div class="nav-links" style="margin-bottom:15px;">
<a href="/">← Главная</a>
<a href="/diagnostic">📋 Диагностика</a>
</div>

<div class="card">
<div class="stats-grid">
    <div class="stat-card">
        <div class="stat-label">CPU</div>
        <div class="stat-value" id="cpu-total">0%</div>
        <div class="progress-bar"><div class="progress-fill" id="cpu-bar" style="width:0%; background:#3498db;"></div></div>
    </div>
    <div class="stat-card">
        <div class="stat-label">RAM</div>
        <div class="stat-value" id="ram-pct">0%</div>
        <div class="stat-sub" id="ram-detail">0 / 0 GB</div>
        <div class="progress-bar"><div class="progress-fill" id="ram-bar" style="width:0%; background:#e74c3c;"></div></div>
    </div>
    <div class="stat-card">
        <div class="stat-label">Файл подкачки</div>
        <div class="stat-value" id="swap-pct">0%</div>
        <div class="stat-sub" id="swap-detail">0 / 0 GB</div>
        <div class="progress-bar"><div class="progress-fill" id="swap-bar" style="width:0%; background:#f39c12;"></div></div>
    </div>
    <div class="stat-card">
        <div class="stat-label">Температура CPU</div>
        <div class="stat-value" id="temp">—</div>
        <div class="stat-sub" id="temp-sub">недоступно</div>
    </div>
</div>
</div>

<div class="card">
<h2>Загрузка по ядрам</h2>
<div class="cores-grid" id="cores">Загрузка данных...</div>
</div>

<div class="card">
<h2>График (последние 60 секунд)</h2>
<canvas id="chart"></canvas>
<div class="legend">
<div class="legend-item"><div class="legend-color" style="background:#3498db;"></div>CPU %</div>
<div class="legend-item"><div class="legend-color" style="background:#e74c3c;"></div>RAM %</div>
<div class="legend-item"><div class="legend-color" style="background:#f39c12;"></div>Температура °C</div>

</div>
</div>

<footer><hr><p>Диагностика ПК &copy; 2026</p></footer>
<script>
startHeartbeat();
fetchStats();
setInterval(fetchStats, 1000);
</script>
</body>
</html>`

	_, _ = w.Write([]byte(html))
}
