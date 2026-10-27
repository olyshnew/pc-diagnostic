package web

import (
	"net/http"

	"pc-diagnostic/history"
)

// apiHistoryHandler отдаёт сводку по журналу мониторинга открытым текстом.
func apiHistoryHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(history.SummaryText()))
}

// historyPageHandler — страница «История мониторинга»: сводка по
// schedule_log.csv за весь период наблюдения.
func historyPageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	html := `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<title>История мониторинга — Диагностика ПК</title>
<style>
body { font-family: Arial, sans-serif; background:#f5f7fa; color:#333; padding: 20px; max-width: 900px; margin: 0 auto; }
h1 { color: #2c3e50; border-bottom: 2px solid #3498db; padding-bottom: 10px; }
.card { background:white; border-radius:8px; padding:20px; margin-bottom:20px; box-shadow:0 2px 5px rgba(0,0,0,0.1); }
#summary { background:#2c3e50; color:#ecf0f1; padding:15px; border-radius:5px; white-space:pre-wrap; word-wrap:break-word; font-family: monospace; font-size:13px; }
.warn-line { color:#f39c12; }
.crit-line { color:#ff7675; }
.ok-line { color:#55efc4; }
.hint { color:#777; font-size:13px; }
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

// Подсветка оценочной строки: критично — красным, предупреждение — жёлтым, норма — зелёным.
function renderSummary(text) {
    var el = document.getElementById('summary');
    el.innerHTML = '';
    var lines = text.split('\n');
    for (var i = 0; i < lines.length; i++) {
        var span = document.createElement('span');
        var l = lines[i];
        if (l.indexOf('[!]') === 0) span.className = 'crit-line';
        else if (l.indexOf('[i]') === 0) span.className = 'warn-line';
        else if (l.indexOf('[✓]') === 0) span.className = 'ok-line';
        span.textContent = l;
        el.appendChild(span);
        el.appendChild(document.createTextNode('\n'));
    }
}

function loadHistory() {
    fetch('/api/history')
        .then(function(r) { return r.text(); })
        .then(renderSummary)
        .catch(function(e) {
            document.getElementById('summary').textContent = 'Ошибка загрузки: ' + e;
        });
}
</script>
</head>
<body>
<div class="top-bar">
<h1>📈 История мониторинга</h1>
<button class="btn-exit" onclick="doExit()">✖ Выход</button>
</div>
<div class="nav-links" style="margin-bottom:15px;">
<a href="/">← Главная</a>
<a href="/monitor">📊 Мониторинг</a>
</div>

<div class="card">
<p class="hint">Сводка по журналу schedule_log.csv: средние и пиковые значения за весь период сбора метрик.</p>
<div id="summary">Загрузка...</div>
<p class="hint" style="margin-top:12px;">Журнал наполняется при запуске: <code>pc-diagnostic.exe --schedule=60</code></p>
</div>

<footer><hr><p>Диагностика ПК &copy; 2026</p></footer>
<script>
startHeartbeat();
loadHistory();
</script>
</body>
</html>`

	_, _ = w.Write([]byte(html))
}
