//go:build linux

package system

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GetAutostartInfo — программы в автозагрузке (Linux):
// .desktop-файлы, юниты systemd пользователя, cron.
func GetAutostartInfo() string {
	home, _ := os.UserHomeDir()
	var b strings.Builder
	count := 0

	for _, dir := range []string{filepath.Join(home, ".config/autostart"), "/etc/xdg/autostart"} {
		files, _ := filepath.Glob(filepath.Join(dir, "*.desktop"))
		for _, f := range files {
			name, execLine, hidden := "", "", false
			fh, err := os.Open(f)
			if err != nil {
				continue
			}
			sc := bufio.NewScanner(fh)
			for sc.Scan() {
				line := strings.TrimSpace(sc.Text())
				switch {
				case strings.HasPrefix(line, "Name="):
					name = strings.TrimPrefix(line, "Name=")
				case strings.HasPrefix(line, "Exec="):
					execLine = strings.TrimPrefix(line, "Exec=")
				case line == "Hidden=true" || line == "NoDisplay=true":
					hidden = true
				}
			}
			fh.Close()
			if hidden {
				continue
			}
			count++
			fmt.Fprintf(&b, "%d. %s — %s\n", count, orDash(name), orDash(execLine))
		}
	}

	units, _ := filepath.Glob(filepath.Join(home, ".config/systemd/user/*.service"))
	for _, u := range units {
		count++
		fmt.Fprintf(&b, "%d. systemd-user: %s\n", count, filepath.Base(u))
	}

	if out, err := exec.Command("crontab", "-l").Output(); err == nil && strings.TrimSpace(string(out)) != "" {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line != "" && !strings.HasPrefix(line, "#") {
				count++
				fmt.Fprintf(&b, "%d. cron: %s\n", count, line)
			}
		}
	}

	if count == 0 {
		return "Автозагрузка: записи не найдены"
	}
	return b.String()
}
