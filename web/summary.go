package web

import (
	"fmt"
	"net/http"
	"strings"

	"pc-diagnostic/hardware"
)

// apiSummaryHandler запускает диагностику оборудования и возвращает
// краткую сводку: по одной строке на модуль (аналог console-меню
// «Показать КРАТКУЮ сводку»).
func apiSummaryHandler(w http.ResponseWriter, r *http.Request) {
	hwResults := hardware.RunDiagnostics()

	var b strings.Builder
	for _, res := range hwResults {
		if res.Error != nil {
			fmt.Fprintf(&b, "- %s: ошибка (%v)\n", res.Name, res.Error)
			continue
		}
		lines := strings.Split(strings.TrimSpace(res.Value), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasSuffix(line, ":") {
				fmt.Fprintf(&b, "- %s: %s\n", res.Name, line)
				break
			}
		}
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

// summaryPageHandler — страница «Краткая сводка»: одна строка на модуль.
func summaryPageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	html := `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<title>Краткая сводка — Диагностика ПК</title>
<style>
body { font-family: Arial, sans-serif; background:#f5f7fa; color:#333; padding: 20px; max-width: 800px; margin: 0 auto; }
h1 { color: #2c3e50; border-bottom: 2px solid #3498db; padding-bottom: 10px; }
.card { background:white; border-radius:8px; padding:20px; margin-bottom:20px; box-shadow:0 2px 5px rgba(0,0,0,0.1); }
.sum-row { display:flex; align-items:baseline; gap:10px; padding:8px 0; border-bottom:1px solid #eee; }
.sum-row:last-child { border-bottom:none; }
.sum-name { font-weight:bold; color:#2c3e50; min-width:180px; }
.sum-val { color:#555; }
.sum-err { color:#e74c3c; }
.spinner { width: 40px; height: 40px; border: 4px solid #e0e0e0; border-top: 4px solid #3498db; border-radius: 50%; animation: spin 1s linear infinite; margin: 20px auto; }
@keyframes spin { 0% { transform: rotate(0deg); } 100% { transform: rotate(360deg); } }
#loading { text-align:center; padding: 30px; }
#loading p { color:#777; margin-top:10px; }
.elapsed { color:#777; font-size:13px; margin-top:10px; }
.nav-links a { color: #3498db; text-decoration: none; margin-right: 15px; }
.nav-links a:hover { text-decoration: underline; }
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
            if (!alive) return;
            alive = false;
            if (heartbeatTimer) clearInterval(heartbeatTimer);
            document.body.innerHTML = '<div style="text-align:center; padding-top:100px;">' +
                '<h2>Сервер остановлен</h2>' +
                '<p>Вернитесь в консоль и запустите веб-режим снова.</p></div>';
        });
    }, 3000);
}

function doExit() {
    document.body.innerHTML = '<div style="text-align:center; padding-top:100px;"><h2>Сервер останавливается...</h2><p>Можно закрыть вкладку.</p></div>';
    fetch('/shutdown');
}

var startedAt = Date.now();

function loadSummary() {
    fetch('/api/summary')
        .then(function(r) {
            if (!r.ok) throw new Error('HTTP ' + r.status);
            return r.text();
        })
        .then(function(text) {
            document.getElementById('loading').style.display = 'none';
            render(text);
        })
        .catch(function(err) {
            document.getElementById('loading').innerHTML =
                '<h2 style="color:#e74c3c;">Ошибка</h2><p>Не удалось получить данные: ' + err + '</p>';
        });
}

// Строки вида "- Модуль: значение" превращаем в строки «имя — значение».
function render(text) {
    var box = document.getElementById('results');
    box.innerHTML = '';
    var lines = text.split('\n');
    var count = 0;
    for (var i = 0; i < lines.length; i++) {
        var l = lines[i].trim();
        if (l === '') continue;
        count++;
        var body = l;
        if (body.indexOf('- ') === 0) body = body.substring(2);

        var row = document.createElement('div');
        row.className = 'sum-row';

        var sep = body.indexOf(':');
        var nameEl = document.createElement('span');
        nameEl.className = 'sum-name';
        var valEl = document.createElement('span');
        if (sep > 0) {
            nameEl.textContent = body.substring(0, sep);
            valEl.textContent = body.substring(sep + 1).trim();
        } else {
            nameEl.textContent = '';
            valEl.textContent = body;
        }
        if (body.indexOf('ошибка') >= 0) valEl.className = 'sum-err';
        else valEl.className = 'sum-val';

        row.appendChild(nameEl);
        row.appendChild(valEl);
        box.appendChild(row);
    }
    if (count === 0) {
        box.innerHTML = '<p>Данные не получены.</p>';
        return;
    }
    var elapsed = ((Date.now() - startedAt) / 1000).toFixed(0);
    document.getElementById('elapsed').textContent =
        'Модулей: ' + count + '. Время выполнения: ' + elapsed + ' сек.';
}
</script>
</head>
<body>
<div class="top-bar">
<h1>⚡ Краткая сводка</h1>
<button class="btn-exit" onclick="doExit()">✖ Выход</button>
</div>
<div class="nav-links" style="margin-bottom:15px;">
<a href="/">← Главная</a>
<a href="/diagnostic">📋 Полная диагностика</a>
</div>

<div id="loading">
    <div class="card">
        <div class="spinner"></div>
        <p>Сбор данных об оборудовании...</p>
    </div>
</div>

<div class="card">
    <div id="results"></div>
    <div class="elapsed" id="elapsed"></div>
</div>

<footer><hr><p>Диагностика ПК &copy; 2026</p></footer>
<script>
startHeartbeat();
loadSummary();
</script>
</body>
</html>`

	_, _ = w.Write([]byte(html))
}
