package system

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// runPS выполняет PowerShell-команду и возвращает вывод как строку.
// Кодировка принудительно UTF-8 — иначе русские названия процессов
// и служб на Windows превращаются в кракозябры.
func runPS(command string) (string, error) {
	full := `[Console]::OutputEncoding=[Text.Encoding]::UTF8; ` + command
	out, err := exec.Command("powershell", "-NoProfile", "-Command", full).Output()
	return string(out), err
}

// GetProcessesInfo получает информацию о запущенных процессах:
// топ-10 по CPU и топ-10 по памяти.
func GetProcessesInfo() string {
	var result strings.Builder

	if runtime.GOOS == "linux" {
		out, err := exec.Command("ps", "aux").Output()
		if err != nil {
			return "Процессы: не удалось получить информацию (ps aux)"
		}
		result.WriteString("Запущенные процессы (Linux, ps aux):\n")
		result.WriteString(string(out))
		return result.String()
	}

	if runtime.GOOS != "windows" {
		return "Процессы: не удалось получить информацию для текущей ОС"
	}

	// Топ-10 по CPU. Фильтр по $null нужен: у свежезапущенных процессов
	// счётчик CPU ещё не заполнен и сортировать их бессмысленно.
	psTop := `Get-Process | Where-Object { $_.CPU -ne $null } | ` +
		`Sort-Object CPU -Descending | Select-Object -First 10 ` +
		`@{n='Имя';e={$_.ProcessName}}, ` +
		`@{n='PID';e={$_.Id}}, ` +
		`@{n='CPU(сек)';e={[math]::Round($_.CPU,1)}}, ` +
		`@{n='RAM(МБ)';e={[math]::Round($_.WorkingSet64/1MB,0)}} | ` +
		`Format-Table -AutoSize | Out-String -Width 120`

	out, err := runPS(psTop)
	if err != nil {
		return "Процессы: не удалось получить информацию (PowerShell)"
	}
	result.WriteString("Процессы — топ-10 по CPU:\n")
	result.WriteString(strings.TrimSpace(out) + "\n")

	// Топ-10 по памяти
	psRam := `Get-Process | Sort-Object WorkingSet64 -Descending | ` +
		`Select-Object -First 10 ` +
		`@{n='Имя';e={$_.ProcessName}}, ` +
		`@{n='PID';e={$_.Id}}, ` +
		`@{n='RAM(МБ)';e={[math]::Round($_.WorkingSet64/1MB,0)}} | ` +
		`Format-Table -AutoSize | Out-String -Width 120`

	out, err = runPS(psRam)
	if err == nil {
		result.WriteString("\nПроцессы — топ-10 по памяти:\n")
		result.WriteString(strings.TrimSpace(out) + "\n")
	}

	return result.String()
}

// GetServicesInfo получает сводку по службам:
// общая статистика + службы с автозапуском, упавшие с ошибкой.
func GetServicesInfo() string {
	if runtime.GOOS != "windows" {
		return "Службы: доступно только на Windows"
	}

	var result strings.Builder

	// Общая статистика
	psStats := `$all = Get-Service; ` +
		`$running = ($all | Where-Object {$_.Status -eq 'Running'}).Count; ` +
		`$stopped = ($all | Where-Object {$_.Status -eq 'Stopped'}).Count; ` +
		`Write-Output ("Всего служб: " + $all.Count + " | Запущено: " + $running + " | Остановлено: " + $stopped)`

	out, err := runPS(psStats)
	if err == nil {
		result.WriteString("Службы — сводка:\n " + strings.TrimSpace(out) + "\n")
	} else {
		result.WriteString("Службы: не удалось получить статистику\n")
	}

	// Автозапуск, но не запущены с кодом ошибки — признак проблем.
	// Фильтр по ExitCode отсеивает службы, которые стоят легитимно
	// (отложенный старт, триггеры запуска).
	psAuto := `Get-WmiObject Win32_Service | ` +
		`Where-Object { $_.StartMode -eq 'Auto' -and $_.State -ne 'Running' -and $_.ExitCode -ne 0 } | ` +
		`Sort-Object DisplayName | Select-Object -First 15 ` +
		`@{n='Имя';e={$_.DisplayName}}, ` +
		`@{n='Состояние';e={$_.State}}, ` +
		`@{n='Код выхода';e={$_.ExitCode}} | ` +
		`Format-Table -AutoSize | Out-String -Width 120`

	out, err = runPS(psAuto)
	if err != nil {
		result.WriteString("Службы: не удалось получить список проблемных служб\n")
	} else if clean := strings.TrimSpace(out); clean == "" {
		result.WriteString("Службы с автозапуском, не запустившиеся с ошибкой: не обнаружены\n")
	} else {
		result.WriteString("Службы — автозапуск, но не запущены (с кодом ошибки):\n")
		result.WriteString(clean + "\n")
	}

	return result.String()
}

// GetUpdatesInfo получает список установленных обновлений ОС.
func GetUpdatesInfo() string {
	if runtime.GOOS != "windows" {
		return "Обновления ОС: доступно только на Windows"
	}

	psUpd := `$hf = Get-HotFix | Sort-Object InstalledOn -Descending; ` +
		`Write-Output ("Всего обновлений в системе: " + $hf.Count); ` +
		`Write-Output ""; ` +
		`$hf | Select-Object -First 10 ` +
		`@{n='Обновление';e={$_.HotFixID}}, ` +
		`@{n='Тип';e={$_.Description}}, ` +
		`@{n='Дата';e={if ($_.InstalledOn) { $_.InstalledOn.ToString('dd.MM.yyyy') } else { 'н/д' }}} | ` +
		`Format-Table -AutoSize | Out-String -Width 120`

	out, err := runPS(psUpd)
	if err != nil {
		return "Обновления ОС: не удалось получить список (Get-HotFix)"
	}

	return "Обновления ОС:\n" + strings.TrimSpace(out) + "\n"
}

// GetProcesses — оставлена для совместимости, если она использовалась где-то ещё.
// Актуальная версия для отчётов — GetProcessesInfo.
func GetProcesses() string {
	return GetProcessesInfo()
}

var _ = fmt.Sprintf // заглушка, чтобы импорт fmt не мешал, если уберёте строку с ним
