package web

import (
	"fmt"
	"net/http"
	"path/filepath"

	"pc-diagnostic/archive"
	"pc-diagnostic/baseline"
)

// apiArchivesHandler отдаёт список архивных папок (JSON).
func apiArchivesHandler(w http.ResponseWriter, r *http.Request) {
	dirs := archive.List()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprint(w, `{"archives":[`)
	for i, d := range dirs {
		if i > 0 {
			fmt.Fprint(w, ",")
		}
		fmt.Fprintf(w, "%q", d)
	}
	fmt.Fprint(w, "]}")
}

// apiArchivesCompareHandler возвращает diff двух архивов открытым текстом.
// Имена валидируются по фактическому списку архивов — путь за пределы
// logs/ подставить невозможно.
func apiArchivesCompareHandler(w http.ResponseWriter, r *http.Request) {
	lookup := map[string]bool{}
	for _, d := range archive.List() {
		lookup[d] = true
	}

	a := r.URL.Query().Get("a")
	b := r.URL.Query().Get("b")
	if a == "" || b == "" || !lookup[a] || !lookup[b] {
		http.Error(w, "Некорректные имена архивов.", http.StatusBadRequest)
		return
	}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	text := archive.CompareText(
		filepath.Join("logs", a),
		filepath.Join("logs", b),
	) + baseline.CompareDirs(
		filepath.Join("logs", a),
		filepath.Join("logs", b),
	)
	_, _ = w.Write([]byte(text))


}

// archivesPageHandler — страница «Архивы»: список прогонов и
// построчное сравнение двух выбранных.
func archivesPageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	html := `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<title>Архивы — Диагностика ПК</title>
<style>
body { font-family: Arial, sans-serif; background:#f5f7fa; color:#333; padding: 20px; max-width: 1000px; margin: 0 auto; }
h1 { color: #2c3e50; border-bottom: 2px solid #3498db; padding-bottom: 10px; }
.card { background:white; border-radius:8px; padding:20px; margin-bottom:20px; box-shadow:0 2px 5px rgba(0,0,0,0.1); }
select, button { font-size: 15px; padding: 8px; border-radius: 5px; border: 1px solid #ccc; }
button { background:#3498db; color:white; border:none; cursor:pointer; padding: 8px 20px; transition: background 0.2s; }
button:hover { background:#2980b9; }
button:disabled { background:#95a5a6; cursor: wait; }
.row { display: flex; gap: 15px; align-items: center; flex-wrap: wrap; }
.row label { font-weight: bold; }
#diff { background:#2c3e50; color:#ecf0f1; padding:15px; border-radius:5px; white-space:pre-wrap; word-wrap:break-word; font-family: monospace; font-size:13px; max-height: 500px; overflow-y: auto; display:none; }
.diff-line { display:block; }
.minus { color:#ff7675; }
.plus { color:#55efc4; }
.nav-links a { color: #3498db; text-decoration: none; margin-right: 15px; }
.nav-links a:hover { text-decoration: underline; }
.top-bar { display: flex; justify-content: space-between; align-items: center; }
.btn-exit { background:#e74c3c; color:white; border:none; padding:8px 20px; border-radius:5px; cursor:pointer; font-size:14px; transition: background 0.2s; }
.btn-exit:hover { background:#c0392b; }
footer { margin-top:40px; color:#777; text-align:center; }
.warn { color:#f39c12; }

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

var archives = [];

function loadArchives() {
    fetch('/api/archives')
        .then(function(r) { return r.json(); })
        .then(function(d) {
            archives = d.archives || [];
            var selA = document.getElementById('sel-a');
            var selB = document.getElementById('sel-b');
            selA.innerHTML = '';
            selB.innerHTML = '';
            if (archives.length === 0) {
                document.getElementById('hint').textContent =
                    'Архивов пока нет. Они создаются при каждом сохранении отчёта.';
                return;
            }
            document.getElementById('hint').textContent =
                'Найдено архивов: ' + archives.length + '. Выберите два для сравнения.';
            for (var i = 0; i < archives.length; i++) {
                selA.add(new Option(archives[i], archives[i]));
                selB.add(new Option(archives[i], archives[i]));
            }
            if (archives.length > 1) selB.selectedIndex = 1;
        })
        .catch(function(e) {
            document.getElementById('hint').textContent = 'Ошибка загрузки списка: ' + e;
        });
}

function compare() {
    var a = document.getElementById('sel-a').value;
    var b = document.getElementById('sel-b').value;
    if (!a || !b || a === b) {
        alert('Выберите два разных архива.');
        return;
    }
    var btn = document.getElementById('btn-compare');
    btn.disabled = true;
    btn.textContent = 'Сравнение...';
    fetch('/api/archives/compare?a=' + encodeURIComponent(a) + '&b=' + encodeURIComponent(b))
        .then(function(r) {
            if (!r.ok) throw new Error('HTTP ' + r.status);
            return r.text();
        })
        .then(function(text) {
            renderDiff(text);
        })
        .catch(function(e) {
            document.getElementById('diff').style.display = 'block';
            document.getElementById('diff').textContent = 'Ошибка сравнения: ' + e;
        })
        .then(function() {
            btn.disabled = false;
            btn.textContent = 'Сравнить';
        });
}

// Раскраска строк diff: минус — красным, плюс — зелёным.
function renderDiff(text) {
    var el = document.getElementById('diff');
    el.style.display = 'block';
    el.innerHTML = '';
    var lines = text.split('\n');
    for (var i = 0; i < lines.length; i++) {
        var div = document.createElement('span');
        div.className = 'diff-line';
        var l = lines[i];
               if (l.indexOf('- ') === 0) div.className += ' minus';
        else if (l.indexOf('+ ') === 0) div.className += ' plus';
        else if (l.indexOf('[!]') === 0) div.className += ' minus';
        else if (l.indexOf('[+]') === 0) div.className += ' plus';
        else if (l.indexOf('[-]') === 0) div.className += ' warn';

        div.textContent = l;
        el.appendChild(div);
    }
}
</script>
</head>
<body>
<div class="top-bar">
<h1>🗂️ Архивы отчётов</h1>
<button class="btn-exit" onclick="doExit()">✖ Выход</button>
</div>
<div class="nav-links" style="margin-bottom:15px;">
<a href="/">← Главная</a>
<a href="/diagnostic">📋 Диагностика</a>
</div>

<div class="card">
<p id="hint">Загрузка списка...</p>
<div class="row">
    <label>Старый архив:</label>
    <select id="sel-a" style="min-width:220px;"></select>
    <label>Новый архив:</label>
    <select id="sel-b" style="min-width:220px;"></select>
    <button id="btn-compare" onclick="compare()">Сравнить</button>
</div>
</div>

<pre id="diff"></pre>

<footer><hr><p>Диагностика ПК &copy; 2026</p></footer>
<script>
startHeartbeat();
loadArchives();
</script>
</body>
</html>`

	_, _ = w.Write([]byte(html))
}
