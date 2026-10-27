//go:build linux

package system

import (
	"context"
	"fmt"
	stdnet "net"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
"strconv"
	gopsnet "github.com/shirou/gopsutil/v3/net"
	gopsproc "github.com/shirou/gopsutil/v3/process"
)

// GetTracerouteInfo — трассировка маршрута до 8.8.8.8 (traceroute, резерв tracepath).
func GetTracerouteInfo() string {
	if path, err := exec.LookPath("traceroute"); err == nil {
		out, err := exec.Command(path, "-m", "12", "-w", "2", "8.8.8.8").CombinedOutput()
		if err != nil && len(out) == 0 {
			return fmt.Sprintf("Трассировка маршрута: ошибка запуска: %v", err)
		}
		return "Трассировка маршрута до 8.8.8.8:\n" + strings.TrimSpace(string(out))
	}
	if path, err := exec.LookPath("tracepath"); err == nil {
		out, err := exec.Command(path, "8.8.8.8").CombinedOutput()
		if err == nil {
			return "Трассировка маршрута до 8.8.8.8 (tracepath):\n" + strings.TrimSpace(string(out))
		}
	}
	return "Трассировка маршрута: утилиты traceroute/tracepath не найдены (установите пакет traceroute)"
}

// GetActiveConnectionsInfo — активные TCP-соединения с привязкой к процессам
// (кроссплатформенно через gopsutil, без PowerShell).
func GetActiveConnectionsInfo() string {
	conns, err := gopsnet.Connections("all")
	if err != nil {
		return fmt.Sprintf("TCP-соединения: ошибка получения: %v", err)
	}
	byStatus := map[string]int{}
	var est []gopsnet.ConnectionStat
	for _, c := range conns {
		st := c.Status
		if st == "" {
			st = "NONE"
		}
		byStatus[st]++
		if st == "ESTABLISHED" {
			est = append(est, c)
		}
	}
	var b strings.Builder
	fmt.Fprintln(&b, "TCP-соединения по состояниям:")
	statuses := make([]string, 0, len(byStatus))
	for s := range byStatus {
		statuses = append(statuses, s)
	}
	sort.Strings(statuses)
	for _, s := range statuses {
		fmt.Fprintf(&b, "  %-15s %d\n", s, byStatus[s])
	}
	fmt.Fprintln(&b, "Установленные соединения (топ-15):")
	pcache := map[int32]string{}
	shown := 0
	for _, c := range est {
		if shown >= 15 {
			break
		}
		name := "?"
		if n, ok := pcache[c.Pid]; ok {
			name = n
		} else if c.Pid > 0 {
			if p, err := gopsproc.NewProcess(c.Pid); err == nil {
				name, _ = p.Name()
			}
			pcache[c.Pid] = name
		}
		fmt.Fprintf(&b, "  %s:%d -> %s:%d (%s, PID %d)\n", c.Laddr.IP, c.Laddr.Port, c.Raddr.IP, c.Raddr.Port, name, c.Pid)
		shown++
	}
	return b.String()
}

// GetDNSInfo — DNS-серверы из /etc/resolv.conf и проверка резолвинга доменов.
func GetDNSInfo() string {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return "DNS: файл /etc/resolv.conf недоступен"
	}
	var servers []string
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(strings.TrimSpace(line))
		if len(f) >= 2 && f[0] == "nameserver" {
			servers = append(servers, f[1])
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "DNS-серверы: %s\n", strings.Join(servers, ", "))
	for _, dom := range []string{"ya.ru", "google.com", "github.com"} {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		addrs, err := stdnet.DefaultResolver.LookupHost(ctx, dom)
		cancel()
		if err != nil {
			fmt.Fprintf(&b, "  %s — НЕ РЕШЕН (%v)\n", dom, err)
		} else {
			if len(addrs) > 2 {
				addrs = addrs[:2]
			}
			fmt.Fprintf(&b, "  %s — %s\n", dom, strings.Join(addrs, ", "))
		}
	}
	return b.String()
}
// ScanOpenPorts — сканирование стандартных портов на localhost.
func ScanOpenPorts() string {
	ports := map[int]string{
		20: "FTP-данные", 21: "FTP", 22: "SSH", 23: "Telnet", 25: "SMTP",
		53: "DNS", 80: "HTTP", 110: "POP3", 111: "RPC", 135: "RPC",
		139: "NetBIOS", 143: "IMAP", 443: "HTTPS", 445: "SMB", 465: "SMTPS",
		587: "SMTP (отправка)", 993: "IMAPS", 995: "POP3S", 1433: "MS SQL",
		3306: "MySQL", 3389: "RDP", 5432: "PostgreSQL", 5900: "VNC", 8080: "HTTP-альт",
	}
	type result struct {
		port int
		open bool
	}
	results := make(chan result, len(ports))
	for p := range ports {
		go func(p int) {
			conn, err := stdnet.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(p), 300*time.Millisecond)
			if err == nil {
				conn.Close()
				results <- result{p, true}
			} else {
				results <- result{p, false}
			}
		}(p)
	}
	var open []int
	for i := 0; i < len(ports); i++ {
		r := <-results
		if r.open {
			open = append(open, r.port)
		}
	}
	sort.Ints(open)
	var b strings.Builder
	if len(open) == 0 {
		return "Открытые порты на localhost: открытых портов из стандартного списка не найдено"
	}
	fmt.Fprintf(&b, "Открытые порты на localhost (%d из %d проверенных):\n", len(open), len(ports))
	for _, p := range open {
		fmt.Fprintf(&b, "  %-6d %s\n", p, ports[p])
	}
	return b.String()
}
