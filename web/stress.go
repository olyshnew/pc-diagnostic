package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"pc-diagnostic/stress"
)

// =================== СТРЕСС-ТЕСТЫ ===================

// Состояние стресс-теста: одна копия на программу.
// Одновременно может выполняться только один тест.
var (
	stressMu        sync.Mutex
	stressRunning   bool
	stressTestType  string // cpu | ram | disk | combined | gpu
	stressDuration  int
	stressStartedAt time.Time
	stressResult    string

	// Живые данные GPU-теста (обновляются из браузера через /api/stress/gpu/*)
	gpuFps     float64
	gpuFrames  int64
	gpuElapsed int

	// Путь, который открывается в браузере при запуске сервера.
	// Меняется через SetStartPath (консольный пункт «GPU-тест»).
	startPath = "/"
)

// SetStartPath задаёт страницу, которую StartServer откроет в браузере.
func SetStartPath(p string) { startPath = p }

type stressStatus struct {
	Running  bool    `json:"running"`
	Type     string  `json:"type"`
	Duration int     `json:"duration"`
	Elapsed  int     `json:"elapsed"`
	Progress int     `json:"progress"`
	Fps      float64 `json:"fps"`
	Result   string  `json:"result"`
}

// stressStartHandler запускает стресс-тест (cpu/ram/disk/combined) в фоновом режиме.
// Параметры: ?type=cpu|ram|disk|combined&dur=60
func stressStartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}

	testType := r.URL.Query().Get("type")
	dur := 60
	if d, err := strconv.Atoi(r.URL.Query().Get("dur")); err == nil && d >= 10 && d <= 600 {
		dur = d
	}

	stressMu.Lock()
	if stressRunning {
		stressMu.Unlock()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": "Тест уже выполняется. Дождитесь завершения.",
		})
		return
	}
	stressRunning = true
	stressTestType = testType
	stressDuration = dur
	stressResult = ""
	stressStartedAt = time.Now()
	stressMu.Unlock()

	fmt.Printf("[•] [Веб] Запущен стресс-тест: %s, %d сек\n", testType, dur)

	go func() {
		var res string
		switch testType {
		case "cpu":
			res = stress.RunCPU(dur, 90)
		case "ram":
			res = stress.RunRAM(dur)
		case "disk":
			res = stress.RunDisk(dur)
		case "combined":
			res = stress.RunCombined(dur, 90)
		default:
			res = "Неизвестный тип теста: " + testType
		}
		stressMu.Lock()
		stressResult = res
		stressRunning = false
		stressMu.Unlock()
		fmt.Printf("[✓] [Веб] Стресс-тест %s завершён.\n", testType)
	}()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

// stressGPUStartHandler отмечает запуск GPU-теста. Сам тест выполняется
// в браузере (WebGL-шейдер), сервер только принимает прогресс и результат.
func stressGPUStartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}

	dur := 60
	if d, err := strconv.Atoi(r.URL.Query().Get("dur")); err == nil && d >= 10 && d <= 600 {
		dur = d
	}

	stressMu.Lock()
	if stressRunning {
		stressMu.Unlock()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": "Тест уже выполняется. Дождитесь завершения.",
		})
		return
	}
	stressRunning = true
	stressTestType = "gpu"
	stressDuration = dur
	stressResult = ""
	stressStartedAt = time.Now()
	gpuFps = 0
	gpuFrames = 0
	gpuElapsed = 0
	stressMu.Unlock()

	fmt.Printf("[•] [Веб] Запущен стресс-тест: gpu (WebGL в браузере), %d сек\n", dur)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

type gpuProgressMsg struct {
	Elapsed int     `json:"elapsed"`
	Fps     float64 `json:"fps"`
	Frames  int64   `json:"frames"`
}

// stressGPUProgressHandler принимает живые данные от браузера раз в секунду.
func stressGPUProgressHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var m gpuProgressMsg
	_ = json.NewDecoder(r.Body).Decode(&m)

	stressMu.Lock()
	if stressRunning && stressTestType == "gpu" {
		gpuFps = m.Fps
		gpuFrames = m.Frames
		gpuElapsed = m.Elapsed
	}
	stressMu.Unlock()

	w.WriteHeader(http.StatusOK)
}

type gpuFinishMsg struct {
	AvgFps   float64 `json:"avg_fps"`
	Frames   int64   `json:"frames"`
	Seconds  float64 `json:"seconds"`
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	Iters    int     `json:"iterations"`
	Renderer string  `json:"renderer"`
	Aborted  bool    `json:"aborted"`
}

// stressGPUFinishHandler принимает итоговые данные теста и формирует сводку.
func stressGPUFinishHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var m gpuFinishMsg
	_ = json.NewDecoder(r.Body).Decode(&m)

	stressMu.Lock()
	defer stressMu.Unlock()

	// Тест не запущен или уже завершён (например, страница перезагрузилась) — игнорируем.
	if !stressRunning || stressTestType != "gpu" {
		w.WriteHeader(http.StatusOK)
		return
	}

	durStr := time.Duration(m.Seconds * float64(time.Second)).Round(time.Second)
	if m.Aborted {
		stressResult = "Тест GPU прерван до завершения (вкладка закрыта или WebGL недоступен).\n"
		if m.Frames > 0 {
			stressResult += fmt.Sprintf("  Частичные данные: кадров %d, средний FPS %.0f, время %v\n",
				m.Frames, m.AvgFps, durStr)
		}
	} else {
		// Грубая оценка мощности шейдера: пиксели × итерации × FPS × ~8 операций
		// на итерацию Мандельброта (3 умножения, 3 сложения, сравнение и пр.)
		ops := float64(m.Width) * float64(m.Height) * float64(m.Iters) * m.AvgFps * 8.0
		stressResult = fmt.Sprintf(
			"\nРезультаты стресс-теста GPU (WebGL):\n"+
				"  Длительность:            %v\n"+
				"  Разрешение рендера:      %dx%d, %d итераций на пиксель\n"+
				"  Средний FPS:             %.1f\n"+
				"  Отрендерено кадров:      %d\n"+
				"  Оценка мощности шейдера: %.1f GFLOPS (приблизительно)\n"+
				"  GPU: %s\n",
			durStr, m.Width, m.Height, m.Iters,
			m.AvgFps, m.Frames, ops/1e9, m.Renderer)
	}
	stressRunning = false
	w.WriteHeader(http.StatusOK)
}

// stressStatusHandler отдаёт текущее состояние теста в JSON.
func stressStatusHandler(w http.ResponseWriter, r *http.Request) {
	stressMu.Lock()
	st := stressStatus{
		Running:  stressRunning,
		Type:     stressTestType,
		Duration: stressDuration,
		Result:   stressResult,
		Fps:      gpuFps,
	}
	started := stressStartedAt
	isGPU := stressRunning && stressTestType == "gpu"
	if isGPU {
		st.Elapsed = gpuElapsed
	}
	stressMu.Unlock()

	if st.Running {
		if !isGPU {
			st.Elapsed = int(time.Since(started).Seconds())
		}
		if st.Duration > 0 {
			p := st.Elapsed * 100 / st.Duration
			if p > 100 {
				p = 100
			}
			st.Progress = p
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "status": st})
}

// stressPageHandler — страница стресс-тестов.
func stressPageHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/stress" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	html := `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<title>Стресс-тесты — Диагностика ПК</title>
<style>
body { font-family: Arial, sans-serif; background:#f5f7fa; color:#333; padding: 20px; max-width: 900px; margin: 0 auto; }
h1 { color: #2c3e50; border-bottom: 2px solid #3498db; padding-bottom: 10px; }
.card { background:white; border-radius:8px; padding:20px; margin-bottom:20px; box-shadow:0 2px 5px rgba(0,0,0,0.1); }
.btn { border:none; padding:12px 20px; border-radius:5px; cursor:pointer; font-size:15px; color:white; margin: 5px; transition: opacity 0.2s; }
.btn:hover { opacity: 0.85; }
.btn:disabled { opacity: 0.4; cursor: not-allowed; }
.btn-cpu { background:#3498db; }
.btn-ram { background:#e67e22; }
.btn-disk { background:#27ae60; }
.btn-comb { background:#8e44ad; }
.btn-gpu { background:#c0392b; }
select { padding:8px; border-radius:5px; border:1px solid #ccc; font-size:14px; }
.progress-bar { background:#e0e0e0; border-radius:4px; overflow:hidden; height: 22px; margin-top:8px; }
.progress-fill { height: 100%; background:#3498db; border-radius:4px; transition: width 1s ease; }
pre { background:#f4f4f4; padding:12px; border-radius:5px; white-space: pre-wrap; word-wrap: break-word; font-size: 14px; line-height: 1.5; }
.live-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 12px; margin-top:12px; }
.live-item { background:#f8f9fa; border-radius:8px; padding:12px; text-align:center; }
.live-value { font-size: 24px; font-weight: bold; }
.live-label { color:#666; font-size:11px; text-transform:uppercase; letter-spacing:1px; }
.status-line { color:#3498db; font-size:16px; min-height:24px; }
.nav-links a { color: #3498db; text-decoration: none; margin-right: 15px; }
.nav-links a:hover { text-decoration: underline; }
.hidden { display: none; }
.top-bar { display: flex; justify-content: space-between; align-items: center; }
.btn-exit { background:#e74c3c; color:white; border:none; padding:8px 20px; border-radius:5px; cursor:pointer; font-size:14px; transition: background 0.2s; }
.btn-exit:hover { background:#c0392b; }
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
            alive = false;
            document.body.innerHTML = '<div style="text-align:center; padding-top:100px;">' +
                '<h2>Сервер остановлен</h2>' +
                '<p>Вернитесь в консоль и выберите режим «W» снова.</p></div>';
        });
    }, 3000);
}

document.addEventListener('visibilitychange', function() {
    if (!document.hidden && alive) {
        fetch('/heartbeat').catch(function() { alive = false; });
    }
});

function doExit() {
    document.body.innerHTML = '<div style="text-align:center; padding-top:100px;"><h2>Сервер останавливается...</h2><p>Можно закрыть вкладку.</p></div>';
    fetch('/shutdown');
}

function startTest(type) {
    var dur = parseInt(document.getElementById('duration').value);
    fetch('/api/stress/start?type=' + type + '&dur=' + dur, { method: 'POST' })
        .then(function(r) { return r.json(); })
        .then(function(d) {
            if (!d.ok) alert(d.error);
        })
        .catch(function(err) { alert('Ошибка запуска: ' + err); });
}

function setButtons(enabled) {
    var btns = document.querySelectorAll('.btn');
    for (var i = 0; i < btns.length; i++) btns[i].disabled = !enabled;
}

function getColor(pct) {
    if (pct < 50) return '#2ecc71';
    if (pct < 80) return '#f39c12';
    return '#e74c3c';
}

function stripAnsi(s) {
    var esc = String.fromCharCode(27); // ESC-символ ANSI-кодов
    var parts = s.split(esc);
    var out = parts[0];
    for (var i = 1; i < parts.length; i++) {
        var p = parts[i];
        // пропускаем тело кода до его завершающей буквы (m, K, H и т. п.)
        var j = 0;
        while (j < p.length && !((p[j] >= 'A' && p[j] <= 'Z') || (p[j] >= 'a' && p[j] <= 'z'))) j++;
        if (j < p.length) j++;
        out += p.slice(j);
    }
    return out;
}


function typeName(t) {
    var names = { cpu: 'CPU', ram: 'RAM', disk: 'Диск', combined: 'CPU + RAM', gpu: 'GPU (WebGL)' };
    return names[t] || t;
}

function fetchLive() {
    fetch('/api/stats').then(function(r) { return r.json(); }).then(function(d) {
        var cpu = document.getElementById('live-cpu');
        cpu.textContent = d.cpu_total.toFixed(0) + '%';
        cpu.style.color = getColor(d.cpu_total);
        var ram = document.getElementById('live-ram');
        ram.textContent = d.ram_used_pct.toFixed(0) + '%';
        ram.style.color = getColor(d.ram_used_pct);
        if (d.cpu_temp > 0) {
            document.getElementById('live-temp').textContent = d.cpu_temp.toFixed(0) + ' °C';
        }
        var statusNames = { Normal: 'НОРМА', Warning: 'ВНИМАНИЕ', Critical: 'КРИТИЧНО', Unknown: 'НЕТ ДАННЫХ' };
        var statusColors = { Normal: '#2ecc71', Warning: '#f39c12', Critical: '#e74c3c', Unknown: '#888' };
        var stEl = document.getElementById('live-status');
        if (stEl) {
            stEl.textContent = statusNames[d.status_overall] || '—';
            stEl.style.color = statusColors[d.status_overall] || '#888';
        }
    }).catch(function() {});
}

function pollStatus() {
    fetch('/api/stress/status')
        .then(function(r) { return r.json(); })
        .then(function(d) {
            var st = d.status;
            var runBox = document.getElementById('running');
            var resBox = document.getElementById('result-box');
            var idleBox = document.getElementById('idle');

            if (st.running) {
                setButtons(false);
                idleBox.classList.add('hidden');
                resBox.classList.add('hidden');
                runBox.classList.remove('hidden');
                document.getElementById('status-type').textContent = typeName(st.type);
                document.getElementById('status-elapsed').textContent =
                    st.elapsed + ' из ' + st.duration + ' сек';
                document.getElementById('status-progress').style.width = st.progress + '%';
                var fpsEl = document.getElementById('live-fps');
                if (st.type === 'gpu') {
                    fpsEl.textContent = st.fps > 0 ? st.fps.toFixed(0) + ' FPS' : '—';
                    document.getElementById('gpu-wrap').classList.remove('hidden');
                } else {
                    fpsEl.textContent = '—';
                    document.getElementById('gpu-wrap').classList.add('hidden');
                }
                fetchLive();
            } else {
                setButtons(true);
                runBox.classList.add('hidden');
                idleBox.classList.remove('hidden');
                if (st.result) {
                    resBox.classList.remove('hidden');
                   document.getElementById('result-text').textContent = stripAnsi(st.result);

                } else {
                    resBox.classList.add('hidden');
                }
            }
        })
        .catch(function() {});
}

// =================== GPU-ТЕСТ (WEBGL) ===================

var gpuRunning = false;
var gpuFrameCount = 0;
var gpuFramesThisSecond = 0;
var gpuFpsSum = 0;
var gpuFpsSamples = 0;
var gpuStartTime = 0;
var gpuDur = 60;
var gpuRAF = null;
var gpuTimer = null;
var gpuRenderer = 'неизвестно';
var GPU_ITER = 400;
var GPU_W = 640, GPU_H = 480;

function startGPUTest() {
    var dur = parseInt(document.getElementById('duration').value);
    fetch('/api/stress/gpu/start?dur=' + dur, { method: 'POST' })
        .then(function(r) { return r.json(); })
        .then(function(d) {
            if (!d.ok) { alert(d.error); return; }
            runGPU(dur);
        })
        .catch(function(err) { alert('Ошибка запуска: ' + err); });
}

function runGPU(dur) {
    gpuDur = dur;
    var canvas = document.getElementById('gpu-canvas');
    var gl = canvas.getContext('webgl') || canvas.getContext('experimental-webgl');
    if (!gl) {
        alert('WebGL недоступен в этом браузере');
        finishGPUTest(true);
        return;
    }
    var dbg = gl.getExtension('WEBGL_debug_renderer_info');
    if (dbg) {
        try { gpuRenderer = gl.getParameter(dbg.UNMASKED_RENDERER_WEBGL) || 'неизвестно'; } catch(e) {}
    }

    var vsSrc = 'attribute vec2 p; void main(){ gl_Position = vec4(p, 0.0, 1.0); }';
    var fsSrc = 'precision highp float;\n' +
        'uniform vec2 u_res;\n' +
        'uniform float u_time;\n' +
        'void main() {\n' +
        '  vec2 uv = (gl_FragCoord.xy * 2.0 - u_res) / min(u_res.x, u_res.y);\n' +
        '  float scale = 1.5 + 0.3 * sin(u_time * 0.4);\n' +
        '  vec2 c = uv * scale + vec2(-0.6, 0.0);\n' +
        '  vec2 z = vec2(0.0);\n' +
        '  float it = 0.0;\n' +
        '  for (float i = 0.0; i < 400.0; i++) {\n' +
        '    z = vec2(z.x*z.x - z.y*z.y, 2.0*z.x*z.y) + c;\n' +
        '    if (dot(z, z) > 4.0) break;\n' +
        '    it = i;\n' +
        '  }\n' +
        '  float col = it / 400.0;\n' +
        '  gl_FragColor = vec4(col, col * 0.6, 1.0 - col, 1.0);\n' +
        '}';

    function compile(type, src) {
        var s = gl.createShader(type);
        gl.shaderSource(s, src);
        gl.compileShader(s);
        if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) {
            console.error(gl.getShaderInfoLog(s));
            return null;
        }
        return s;
    }
    var vs = compile(gl.VERTEX_SHADER, vsSrc);
    var fs = compile(gl.FRAGMENT_SHADER, fsSrc);
    if (!vs || !fs) {
        alert('Ошибка компиляции шейдера');
        finishGPUTest(true);
        return;
    }
    var prog = gl.createProgram();
    gl.attachShader(prog, vs);
    gl.attachShader(prog, fs);
    gl.linkProgram(prog);
    if (!gl.getProgramParameter(prog, gl.LINK_STATUS)) {
        alert('Ошибка линковки программы WebGL');
        finishGPUTest(true);
        return;
    }
    gl.useProgram(prog);

    var buf = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, buf);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1,-1, 3,-1, -1,3]), gl.STATIC_DRAW);
    var locP = gl.getAttribLocation(prog, 'p');
    gl.enableVertexAttribArray(locP);
    gl.vertexAttribPointer(locP, 2, gl.FLOAT, false, 0, 0);
    var uRes = gl.getUniformLocation(prog, 'u_res');
    var uTime = gl.getUniformLocation(prog, 'u_time');

    canvas.width = GPU_W;
    canvas.height = GPU_H;
    gl.viewport(0, 0, GPU_W, GPU_H);

    gpuRunning = true;
    gpuFrameCount = 0;
    gpuFramesThisSecond = 0;
    gpuFpsSum = 0;
    gpuFpsSamples = 0;
    gpuStartTime = performance.now();

    function frame(t) {
        if (!gpuRunning) return;
        gl.uniform2f(uRes, canvas.width, canvas.height);
        gl.uniform1f(uTime, (t - gpuStartTime) / 1000.0);
        gl.drawArrays(gl.TRIANGLES, 0, 3);
        gpuFrameCount++;
        gpuFramesThisSecond++;
        gpuRAF = requestAnimationFrame(frame);
    }
    gpuRAF = requestAnimationFrame(frame);

    gpuTimer = setInterval(function() {
        var elapsed = Math.round((performance.now() - gpuStartTime) / 1000);
        var fps = gpuFramesThisSecond;
        gpuFramesThisSecond = 0;
        gpuFpsSum += fps;
        gpuFpsSamples++;
        fetch('/api/stress/gpu/progress', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ elapsed: elapsed, fps: fps, frames: gpuFrameCount })
        }).catch(function() {});
        if (elapsed >= gpuDur) {
            finishGPUTest(false);
        }
    }, 1000);
}

function finishGPUTest(aborted) {
    if (!gpuRunning) return;
    gpuRunning = false;
    if (gpuTimer) clearInterval(gpuTimer);
    if (gpuRAF) cancelAnimationFrame(gpuRAF);
    var elapsed = Math.min(gpuDur, Math.round((performance.now() - gpuStartTime) / 1000));
    var avg = gpuFpsSamples > 0 ? gpuFpsSum / gpuFpsSamples : 0;
    fetch('/api/stress/gpu/finish', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            avg_fps: avg, frames: gpuFrameCount, seconds: elapsed,
            width: GPU_W, height: GPU_H, iterations: GPU_ITER,
            renderer: gpuRenderer, aborted: aborted
        })
    }).catch(function() {});
}

// Если вкладка закрыли посреди GPU-теста — сообщаем серверу.
window.addEventListener('beforeunload', function() {
    if (gpuRunning) {
        var avg = gpuFpsSamples > 0 ? gpuFpsSum / gpuFpsSamples : 0;
        navigator.sendBeacon('/api/stress/gpu/finish', JSON.stringify({
            avg_fps: avg, frames: gpuFrameCount, seconds: gpuDur,
            width: GPU_W, height: GPU_H, iterations: GPU_ITER,
            renderer: gpuRenderer, aborted: true
        }));
    }
});
</script>
</head>
<body>
<div class="top-bar">
<h1>🔥 Стресс-тесты</h1>
<button class="btn-exit" onclick="doExit()">✖ Выход</button>
</div>
<p class="nav-links">
<a href="/">← Главная</a>
<a href="/monitor">📊 Мониторинг</a>
<a href="/diagnostic">📋 Диагностика</a>
</p>

<div class="card" id="idle">
<h2>Выберите тест</h2>
<p>Длительность:
<select id="duration">
<option value="30">30 секунд</option>
<option value="60" selected>60 секунд</option>
<option value="120">120 секунд</option>
</select>
</p>
<button class="btn btn-cpu" onclick="startTest('cpu')">CPU — все ядра</button>
<button class="btn btn-ram" onclick="startTest('ram')">RAM — запись и проверка</button>
<button class="btn btn-disk" onclick="startTest('disk')">Диск — запись</button>
<button class="btn btn-comb" onclick="startTest('combined')">Комбинированный (CPU + RAM)</button>
<button class="btn btn-gpu" onclick="startGPUTest()">GPU — WebGL шейдер</button>
<p style="color:#777; font-size:13px;">Во время теста живые показатели можно смотреть на странице «Мониторинг» — графики покажут нагрузку в реальном времени.</p>
</div>

<div class="card hidden" id="running">
<h2>⏳ Выполняется: <span id="status-type"></span></h2>
<div class="status-line">Прошло: <span id="status-elapsed"></span></div>
<div class="progress-bar"><div class="progress-fill" id="status-progress" style="width:0%;"></div></div>
<div class="live-grid">
    <div class="live-item"><div class="live-label">CPU</div><div class="live-value" id="live-cpu">—</div></div>
    <div class="live-item"><div class="live-label">RAM</div><div class="live-value" id="live-ram">—</div></div>
    <div class="live-item"><div class="live-label">Температура</div><div class="live-value" id="live-temp">—</div></div>
    <div class="live-item"><div class="live-label">GPU FPS</div><div class="live-value" id="live-fps">—</div></div>
    <div class="live-item"><div class="live-label">Статус</div><div class="live-value" id="live-status">—</div></div>
</div>
<div id="gpu-wrap" class="hidden" style="margin-top:12px;">
    <canvas id="gpu-canvas" style="width:100%; max-width:640px; border-radius:5px; display:block; margin:0 auto; background:#000;"></canvas>
    <p style="color:#777; font-size:13px; text-align:center;">Шейдер Мандельброта нагружает GPU. Если вкладка неактивна, браузер приостанавливает рендер — держите вкладку открытой.</p>
</div>
<p style="color:#777; font-size:13px;">Откройте «Мониторинг» в новой вкладке, чтобы увидеть графики нагрузки.</p>
</div>

<div class="card hidden" id="result-box">
<h2>✅ Результат</h2>
<pre id="result-text"></pre>
</div>

<footer><hr><p>Диагностика ПК &copy; 2026</p></footer>
<script>
startHeartbeat();
pollStatus();
setInterval(pollStatus, 1000);
// Автозапуск GPU-теста из консольного меню (пункт 7)
try {
    var params = new URLSearchParams(window.location.search);
    if (params.get('autostart') === 'gpu') startGPUTest();
} catch(e) {}
</script>
</body>
</html>`

	_, _ = w.Write([]byte(html))
}
