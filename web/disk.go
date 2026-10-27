package web

import (
	"encoding/json"
	"net/http"
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v3/disk"
)

type diskInfo struct {
	Mount   string  `json:"mount"`
	Fs      string  `json:"fs"`
	TotalGB float64 `json:"total_gb"`
	UsedGB  float64 `json:"used_gb"`
	FreeGB  float64 `json:"free_gb"`
	UsedPct float64 `json:"used_pct"`
}

type diskPayload struct {
	OS    string     `json:"os"`
	Disks []diskInfo `json:"disks"`
}

// apiDiskHandler отдаёт JSON со сведениями о разделах дисков:
// монтирование, ФС, объём, занято/свободно и процент занятости.
func apiDiskHandler(w http.ResponseWriter, r *http.Request) {
	payload := diskPayload{OS: runtime.GOOS}

	parts, err := disk.Partitions(false)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(payload)
		return
	}

	seen := map[string]bool{}
	for _, p := range parts {
		if seen[p.Mountpoint] {
			continue
		}
		u, err := disk.Usage(p.Mountpoint)
		if err != nil || u.Total <= 0 {
			continue
		}
		seen[p.Mountpoint] = true

		payload.Disks = append(payload.Disks, diskInfo{
			Mount:   strings.TrimSuffix(p.Mountpoint, `\`) + `\`,
			Fs:      p.Fstype,
			TotalGB: float64(u.Total) / (1024 * 1024 * 1024),
			UsedGB:  float64(u.Used) / (1024 * 1024 * 1024),
			FreeGB:  float64(u.Free) / (1024 * 1024 * 1024),
			UsedPct: u.UsedPercent,
		})
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(payload)
}

// disksPageHandler — страница «Диски»: круговые диаграммы занятого
// места по разделам. Данные — с /api/disk, обновление раз в 30 секунд.
func disksPageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	html := `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<title>Диски — Диагностика ПК</title>
<style>
body { font-family: Arial, sans-serif; background:#f5f7fa; color:#333; padding: 20px; max-width: 900px; margin: 0 auto; }
h1 { color: #2c3e50; border-bottom: 2px solid #3498db; padding-bottom: 10px; }
.card { background:white; border-radius:8px; padding:20px; margin-bottom:20px; box-shadow:0 2px 5px rgba(0,0,0,0.1); }
.disks-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(260px, 1fr)); gap: 20px; }
.disk-card { text-align: center; }
.donut { width: 160px; height: 160px; border-radius: 50%; margin: 0 auto; position: relative; display: flex; align-items: center; justify-content: center; }
.donut::before { content: ''; position: absolute; width: 100px; height: 100px; background: white; border-radius: 50%; }
.donut-label { position: relative; font-size: 22px; font-weight: bold; color: #2c3e50; }
.disk-mount { font-size: 18px; font-weight: bold; color: #2c3e50; margin-top: 12px; }
.disk-detail { color: #777; font-size: 13px; margin-top: 4px; }
.note { color: #999; font-size: 12px; margin-top: 6px; }
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

document.addEventListener('visibilitychange', function() {
    if (!document.hidden && alive) {
        fetch('/heartbeat').then(function() { loadDisks(); }).catch(function() {});
    }
});

function doExit() {
    document.body.innerHTML = '<div style="text-align:center; padding-top:100px;"><h2>Сервер останавливается...</h2><p>Можно закрыть вкладку.</p></div>';
    fetch('/shutdown');
}

function diskColor(pct) {
    if (pct < 80) return '#2ecc71';
    if (pct < 92) return '#f39c12';
    return '#e74c3c';
}

function renderDisks(d) {
    var grid = document.getElementById('disks');
    if (!d.disks || d.disks.length === 0) {
        grid.innerHTML = '<p style="color:#777;">Данные о дисках недоступны.</p>';
        return;
    }
    var html = '';
    for (var i = 0; i < d.disks.length; i++) {
        var dk = d.disks[i];
        var deg = Math.min(dk.used_pct, 100) * 3.6;
        var color = diskColor(dk.used_pct);
        html += '<div class="disk-card">' +
            '<div class="donut" style="background: conic-gradient(' + color + ' 0deg ' + deg + 'deg, #e0e0e0 ' + deg + 'deg 360deg);">' +
            '<span class="donut-label">' + dk.used_pct.toFixed(1) + '%</span></div>' +
            '<div class="disk-mount">' + dk.mount.replace(/\\\\/g, '') + '</div>' +
            '<div class="disk-detail">' + dk.fs.toUpperCase() + ' &middot; занято ' + dk.used_gb.toFixed(1) +
            ' ГБ из ' + dk.total_gb.toFixed(1) + ' ГБ</div>' +
            '<div class="note">свободно: ' + dk.free_gb.toFixed(1) + ' ГБ</div>' +
            '</div>';
    }
    grid.innerHTML = html;
    var stamp = document.getElementById('updated');
    if (stamp) stamp.textContent = 'Обновлено: ' + new Date().toLocaleTimeString();
}

function loadDisks() {
    fetch('/api/disk')
        .then(function(r) { return r.json(); })
        .then(renderDisks)
        .catch(function(e) {
            var grid = document.getElementById('disks');
            if (grid) grid.innerHTML = '<p style="color:#e74c3c;">Не удалось получить данные: ' + e + '</p>';
        });
}
</script>
</head>
<body>
<div class="top-bar">
<h1>💾 Диски — занятое место</h1>
<button class="btn-exit" onclick="doExit()">✖ Выход</button>
</div>
<div class="nav-links" style="margin-bottom:15px;">
<a href="/">← Главная</a>
<a href="/monitor">📊 Мониторинг</a>
</div>

<div class="card">
<p style="color:#777; margin-top:0;" id="updated">Загрузка...</p>
<div class="disks-grid" id="disks">Загрузка данных...</div>
</div>

<footer><hr><p>Диагностика ПК &copy; 2026</p></footer>
<script>
startHeartbeat();
loadDisks();
setInterval(loadDisks, 30000);
</script>
</body>
</html>`

	_, _ = w.Write([]byte(html))
}
