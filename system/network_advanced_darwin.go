//go:build darwin

package system

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

// GetTracerouteInfo — трассировка маршрута до 8.8.8.8 (traceroute встроен в macOS).
func GetTracerouteInfo() string {
	var b strings.Builder
	b.WriteString("Traceroute до 8.8.8.8 (до 12 прыжков):\n")

	if _, err := exec.LookPath("traceroute"); err != nil {
		return "Traceroute: утилита traceroute не найдена"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "traceroute", "-n", "-m", "12", "-w", "2", "8.8.8.8").CombinedOutput()
	if err != nil && len(out) == 0 {
		return "Traceroute: не удалось выполнить (прервано по таймауту)"
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

// ScanOpenPorts сканирует стандартные порты на localhost.
func ScanOpenPorts() string {
	var ports []int
	for p := range knownPorts {
		ports = append(ports, p)
	}
	sort.Ints(ports)

	var (
		mu    sync.Mutex
		found []int
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
			found = append(found, p)
			mu.Unlock()
		}(p)
	}
	wg.Wait()

	var b strings.Builder
	if len(found) == 0 {
		return "Открытые порты на localhost: открытых портов из стандартного списка не найдено"
	}
	fmt.Fprintf(&b, "Открытые порты на localhost (%d из %d проверенных):\n", len(found), len(ports))
	for _, p := range found {
		fmt.Fprintf(&b, "  %-6d %s\n", p, knownPorts[p])
	}
	return b.String()
}

// ---------- Активные соединения ----------

// GetActiveConnectionsInfo — сводка TCP-соединений (netstat) и список
// установленных соединений с привязкой к процессам (lsof).
func GetActiveConnectionsInfo() string {
	var b strings.Builder
	b.WriteString("Активные TCP-соединения:\n\n")

	if out, err := exec.Command("netstat", "-an", "-p", "tcp").Output(); err == nil {
		counts := map[string]int{}
		for _, line := range strings.Split(string(out), "\n") {
			f := strings.Fields(line)
			if len(f) >= 6 && (f[0] == "tcp4" || f[0] == "tcp6") {
				counts[f[5]]++
			}
		}
		for _, st := range []string{"ESTABLISHED", "LISTEN", "TIME_WAIT", "CLOSE_WAIT", "SYN_SENT", "CLOSED"} {
			if counts[st] > 0 {
				fmt.Fprintf(&b, "  %s: %d\n", st, counts[st])
			}
		}
		b.WriteString("\n")
	}

	if out, err := exec.Command("lsof", "-i", "-n", "-P").Output(); err == nil {
		seen := map[string]bool{}
		var established []string
		for _, line := range strings.Split(string(out), "\n") {
			if !strings.Contains(line, "ESTABLISHED") {
				continue
			}
			f := strings.Fields(line)
			if len(f) < 9 {
				continue
			}
			entry := f[0] + " (PID " + f[1] + ") " + f[8]
			if !seen[entry] {
				seen[entry] = true
				established = append(established, entry)
			}
		}
		if len(established) > 0 {
			b.WriteString("Установленные соединения (lsof, топ-15):\n")
			if len(established) > 15 {
				established = established[:15]
			}
			for _, e := range established {
				b.WriteString("  " + e + "\n")
			}
		}
	}
	return b.String()
}

// ---------- DNS ----------

// GetDNSInfo — DNS-серверы (scutil --dns) и проверка разрешения доменов.
func GetDNSInfo() string {
	var b strings.Builder
	b.WriteString("DNS-серверы:\n")

	out, err := exec.Command("scutil", "--dns").Output()
	if err != nil {
		return "DNS-диагностика: не удалось получить информацию (scutil --dns)"
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		t := strings.TrimSpace(line)
		if strings.Contains(t, "nameserver[") {
			parts := strings.SplitN(t, ":", 2)
			if len(parts) == 2 {
				addr := strings.TrimSpace(parts[1])
				if addr != "" && !seen[addr] {
					seen[addr] = true
					b.WriteString("  " + addr + "\n")
				}
			}
		}
	}

	b.WriteString("\nПроверка разрешения имён:\n")
	resolver := net.Resolver{}
	for _, host := range []string{"ya.ru", "google.com", "github.com"} {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		ips, err := resolver.LookupHost(ctx, host)
		cancel()
		if err == nil && len(ips) > 0 {
			fmt.Fprintf(&b, "  %s — %s\n", host, strings.Join(ips, ", "))
		} else {
			fmt.Fprintf(&b, "  %s — ошибка разрешения\n", host)
		}
	}
	return b.String()
}
