package system

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// GetTracerouteInfo строит трассировку маршрута до внешнего узла.
func GetTracerouteInfo() string {
	var b strings.Builder
	b.WriteString("Traceroute до 8.8.8.8 (без разрешения имён, до 12 прыжков):\n")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if runtime.GOOS == "windows" {
		out, err := exec.CommandContext(ctx, "tracert", "-d", "-h", "12", "-w", "500", "8.8.8.8").Output()
		if err != nil {
			return "Traceroute: не удалось выполнить (прервано по таймауту или tracert недоступен)"
		}
		b.WriteString(strings.TrimSpace(string(out)))
		return b.String()
	}

	out, err := exec.CommandContext(ctx, "traceroute", "-n", "-m", "12", "-w", "2", "8.8.8.8").Output()
	if err != nil {
		if strings.Contains(err.Error(), "executable file not found") || strings.Contains(err.Error(), "no such file") {
			return "Traceroute: утилита traceroute не установлена (apt install traceroute)"
		}
		return "Traceroute: не удалось выполнить"
	}
	b.WriteString(strings.TrimSpace(string(out)))
	return b.String()
}

// ---------- Открытые порты ----------

var knownPorts = map[int]string{
	21: "FTP", 22: "SSH", 23: "Telnet", 25: "SMTP", 53: "DNS",
	80: "HTTP", 110: "POP3", 135: "RPC", 139: "NetBIOS", 143: "IMAP",
	443: "HTTPS", 445: "SMB", 465: "SMTPS", 587: "SMTP (submit)",
	993: "IMAPS", 995: "POP3S", 1433: "MS SQL", 3306: "MySQL",
	3389: "RDP", 5432: "PostgreSQL", 5900: "VNC", 6379: "Redis",
	8080: "HTTP-alt", 9100: "Принтер",
}

// ScanOpenPorts сканирует стандартные порты на localhost — показывает,
// какие службы принимают подключения на этой машине.
func ScanOpenPorts() string {
	var ports []int
	for p := range knownPorts {
		ports = append(ports, p)
	}
	sort.Ints(ports)

	type openPort struct {
		port  int
		state string
	}
	var (
		mu    sync.Mutex
		found []openPort
		wg    sync.WaitGroup
	)

	for _, p := range ports {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", p), 300*time.Millisecond)
			if err != nil {
				return
			}
			conn.Close()
			mu.Lock()
			found = append(found, openPort{port: p, state: knownPorts[p]})
			mu.Unlock()
		}(p)
	}
	wg.Wait()

	var b strings.Builder
	if len(found) == 0 {
		return "Открытые порты (localhost): слушающих портов из стандартного списка не обнаружено"
	}
	b.WriteString(fmt.Sprintf("Открытые порты (localhost): найдено %d из %d проверенных\n\n", len(found), len(ports)))
	b.WriteString("  Порт    Служба\n")
	for _, op := range found {
		b.WriteString(fmt.Sprintf("  %-8d%s\n", op.port, op.state))
	}
	return b.String()
}

// ---------- Активные соединения ----------

// GetActiveConnectionsInfo сводка по TCP-соединениям:
// распределение по состояниям + список установленных с процессами.
func GetActiveConnectionsInfo() string {
	if runtime.GOOS != "windows" {
		return "Активные соединения: сводка доступна на Windows (netstat/ss — вручную)"
	}

	var b strings.Builder
	b.WriteString("Активные TCP-соединения:\n\n")

	psStates := `$c = Get-NetTCPConnection; ` +
		`$c | Group-Object State | Sort-Object Count -Descending | ` +
		`ForEach-Object { Write-Output ("  " + $_.Name + ": " + $_.Count) }`
	if out, err := runPS(psStates); err == nil {
		b.WriteString(strings.TrimSpace(out) + "\n\n")
	}

	psEstab := `$c = Get-NetTCPConnection | Where-Object State -eq 'Established'; ` +
		`$c | ForEach-Object { ` +
		`$p = (Get-Process -Id $_.OwningProcess -ErrorAction SilentlyContinue).ProcessName; ` +
		`if (-not $p) { $p = 'unknown' }; ` +
		`Write-Output ("  " + ($_.RemoteAddress + ":" + $_.RemotePort) + "  <-  " + $p) } | ` +
		`Group-Object | Sort-Object Count -Descending | Select-Object -First 15 | ` +
		`ForEach-Object { Write-Output $_.Name }`
	if out, err := runPS(psEstab); err == nil {
		b.WriteString("Установленные соединения (кто с кем общается):\n")
		b.WriteString(strings.TrimSpace(out) + "\n")
	}
	return b.String()
}

// ---------- DNS ----------

// GetDNSInfo показывает используемые DNS-серверы и проверяет,
// что распознавание имён работает на несколько внешних доменов.
func GetDNSInfo() string {
	var b strings.Builder
	b.WriteString("Диагностика DNS:\n\n")

	if runtime.GOOS == "windows" {
		psDNS := `Get-DnsClientServerAddress -AddressFamily IPv4 | ` +
			`Where-Object { $_.ServerAddresses } | ` +
			`ForEach-Object { Write-Output ("  Интерфейс " + $_.InterfaceAlias + ": " + ($_.ServerAddresses -join ', ')) }`
		if out, err := runPS(psDNS); err == nil {
			b.WriteString("DNS-серверы:\n" + strings.TrimSpace(out) + "\n\n")
		}
	}

	domains := []string{"ya.ru", "google.com", "github.com"}
	b.WriteString("Проверка распознавания имён:\n")
	for _, d := range domains {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		ips, err := net.DefaultResolver.LookupHost(ctx, d)
		cancel()
		if err != nil || len(ips) == 0 {
			b.WriteString(fmt.Sprintf("  %-12s ОШИБКА: %v\n", d, err))
			continue
		}
		if len(ips) > 3 {
			ips = ips[:3]
		}
		b.WriteString(fmt.Sprintf("  %-12s %s\n", d, strings.Join(ips, ", ")))
	}
	return b.String()
}
