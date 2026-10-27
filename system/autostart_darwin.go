//go:build darwin

package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GetAutostartInfo — автозагрузка (macOS: LaunchAgents/LaunchDaemons + launchctl list).
func GetAutostartInfo() string {
	var b strings.Builder
	b.WriteString("Автозагрузка (LaunchAgents/LaunchDaemons):\n")

	home, _ := os.UserHomeDir()
	dirs := []string{
		filepath.Join(home, "Library", "LaunchAgents"),
		"/Library/LaunchAgents",
		"/Library/LaunchDaemons",
		"/System/Library/LaunchAgents",
		"/System/Library/LaunchDaemons",
	}
	var names []string
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if filepath.Ext(e.Name()) == ".plist" {
				names = append(names, dir+string(filepath.Separator)+e.Name())
			}
		}
	}
	if len(names) == 0 {
		b.WriteString("  Записей не найдено\n")
	}
	for i, n := range names {
		if i >= 50 {
			fmt.Fprintf(&b, "  ... и ещё %d записей\n", len(names)-50)
			break
		}
		b.WriteString("  " + n + "\n")
	}

	if out, err := exec.Command("launchctl", "list").Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		fmt.Fprintf(&b, "\nЗадачи launchctl (загружено: %d):\n", len(lines)-1)
		max := 16
		for i, line := range lines {
			if i >= max {
				fmt.Fprintf(&b, "  ... и ещё %d\n", len(lines)-max)
				break
			}
			b.WriteString("  " + line + "\n")
		}
	}
	return b.String()
}
