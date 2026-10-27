//go:build linux

package hardware

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// deviceIsSSD — определение SSD по /sys/block/<dev>/queue/rotational (0 = SSD).
func deviceIsSSD(dev string) bool {
	candidates := []string{dev}
	trimmed := dev
	for len(trimmed) > 0 {
		last := trimmed[len(trimmed)-1]
		if last >= '0' && last <= '9' {
			trimmed = trimmed[:len(trimmed)-1]
		} else {
			break
		}
	}
	if trimmed != dev && trimmed != "" {
		candidates = append(candidates, trimmed)
	}
	for _, c := range candidates {
		data, err := os.ReadFile("/sys/block/" + c + "/queue/rotational")
		if err == nil {
			return strings.TrimSpace(string(data)) == "0"
		}
	}
	return false
}

// physicalDisks — список физических дисков (без loop/ram/zram/dm).
func physicalDisks() []string {
	var out []string
	entries, _ := filepath.Glob("/sys/block/*")
	for _, e := range entries {
		name := filepath.Base(e)
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") ||
			strings.HasPrefix(name, "dm-") || strings.HasPrefix(name, "zram") || strings.HasPrefix(name, "sr") {
			continue
		}
		out = append(out, name)
	}
	return out
}

// mountedDevices — «устройство -> точка монтирования» из /proc/mounts.
func mountedDevices() map[string]string {
	m := map[string]string{}
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return m
	}
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) >= 3 {
			m[f[0]] = f[1]
		}
	}
	return m
}

// CheckFileSystemInfo — проверка файловой системы в режиме только чтения
// (Linux-аналог chkdsk /scan). Смонтированные разделы пропускаются:
// fsck работающего диска небезопасен. Полная проверка требует прав root.
func CheckFileSystemInfo() string {
	var b strings.Builder
	fmt.Fprintln(&b, "Проверка файловой системы (fsck -n, только чтение):")

	if os.Geteuid() != 0 {
		fmt.Fprintln(&b, "  Для проверки разделов требуются права администратора (root).")
		fmt.Fprintln(&b, "  Запустите программу с sudo — тогда несмонтированные разделы будут проверены.")
		return b.String()
	}

	// Составляем список разделов подкачки (swap) — их проверять не нужно:
	// это не файловая система с данными, а область для выгрузки памяти.
	swapDevs := map[string]bool{}
	if data, err := os.ReadFile("/proc/swaps"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			f := strings.Fields(strings.TrimSpace(line))
			if len(f) >= 1 && strings.HasPrefix(f[0], "/dev/") {
				swapDevs[f[0]] = true
			}
		}
	}

	mounts := mountedDevices()
	checked := 0
	for _, name := range physicalDisks() {
		dev := "/dev/" + name
		// swap и смонтированные разделы пропускаем
		if swapDevs[dev] {
			continue
		}
		if _, ok := mounts[dev]; ok {
			continue
		}
		out, err := exec.Command("fsck", "-n", dev).CombinedOutput()
		if err != nil {
			fmt.Fprintf(&b, "  %s: вывод fsck:\n%s\n", name, strings.TrimSpace(string(out)))
		} else {
			fmt.Fprintf(&b, "  %s: ошибок не найдено\n", name)
		}
		checked++
	}
	if checked == 0 {
		fmt.Fprintln(&b, "  Несмонтированных проверяемых разделов не найдено")
	}
	return b.String()
}


// GetSSDWearInfo — объём записи за жизнь (TBW) и Percentage Used через smartctl.
func GetSSDWearInfo() string {
	smartPath, err := exec.LookPath("smartctl")
	if err != nil {
		return "Износ SSD (TBW): утилита smartctl не найдена (установите пакет smartmontools 7.0+)"
	}
	var b strings.Builder
	fmt.Fprintln(&b, "Износ SSD (ресурс записи):")
	found := false
	for _, name := range physicalDisks() {
		if !deviceIsSSD(name) {
			continue
		}
		out, err := exec.Command(smartPath, "-A", "-j", "/dev/"+name).Output()
		if err != nil && len(out) == 0 {
			fmt.Fprintf(&b, "  /dev/%s: smartctl вернул ошибку: %v\n", name, err)
			continue
		}
		var doc map[string]interface{}
		if err := json.Unmarshal(out, &doc); err != nil {
			fmt.Fprintf(&b, "  /dev/%s: не удалось разобрать JSON smartctl\n", name)
			continue
		}
		found = true
		fmt.Fprintf(&b, "  /dev/%s:\n", name)
		if pu, ok := numField(doc, "percentage_used"); ok {
			status := "норма"
			if pu >= 90 {
				status = "КРИТИЧНО"
			} else if pu >= 80 {
				status = "ВНИМАНИЕ"
			}
			fmt.Fprintf(&b, "    Percentage Used: %.0f%% (%s)\n", pu, status)
		}
		if du, ok := numField(doc, "data_units_written"); ok {
			fmt.Fprintf(&b, "    Записано за жизнь: %.2f ТБ (TBW)\n", du*512000/1e12)
		}
		if attrs, ok := doc["attributes"].([]interface{}); ok {
			for _, a := range attrs {
				m, ok := a.(map[string]interface{})
				if !ok {
					continue
				}
				aname, _ := m["name"].(string)
				raw, _ := m["raw"].(map[string]interface{})
				rawStr, _ := raw["value"].(string)
				switch aname {
				case "Total_LBAs_Written":
					if n, err := strconv.ParseFloat(rawStr, 64); err == nil {
						fmt.Fprintf(&b, "    Записано за жизнь: %.2f ТБ (TBW)\n", n*512/1e12)
					}
				case "Host_Writes_32MiB":
					if n, err := strconv.ParseFloat(rawStr, 64); err == nil {
						fmt.Fprintf(&b, "    Записано за жизнь: %.2f ТБ (TBW)\n", n*33554432/1e12)
					}
				}
			}
		}
	}
	if !found {
		fmt.Fprintln(&b, "  SSD-диски не обнаружены или данные недоступны")
	}
	return b.String()
}

// numField — извлечение числового поля из JSON smartctl.
func numField(doc map[string]interface{}, key string) (float64, bool) {
	v, ok := doc[key]
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case float64:
		return x, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err == nil {
			return f, true
		}
	}
	return 0, false
}

// AnalyzeFragmentation — анализ фрагментации через e4defrag -c (ext4),
// SSD пропускаются.
func AnalyzeFragmentation() string {
	mounts := mountedDevices()
	var b strings.Builder
	fmt.Fprintln(&b, "Анализ фрагментации (только анализ, без дефрагментации):")
	analyzed, ssdSkipped := 0, 0
	for dev, mp := range mounts {
		base := filepath.Base(dev)
		if !strings.HasPrefix(dev, "/dev/sd") && !strings.HasPrefix(dev, "/dev/nvme") {
			continue
		}
		if !strings.ContainsAny(base[len(base)-1:], "123456789") {
			continue
		}
		if deviceIsSSD(base) {
			fmt.Fprintf(&b, "  %s (%s): SSD — пропущен (фрагментация не требуется)\n", dev, mp)
			ssdSkipped++
			continue
		}
		out, err := exec.Command("e4defrag", "-c", mp).CombinedOutput()
		analyzed++
		if err != nil && strings.Contains(string(out), "Permission denied") {
			fmt.Fprintf(&b, "  %s: требуются права администратора (запустите с sudo)\n", mp)
		} else if err != nil && len(out) == 0 {
			fmt.Fprintf(&b, "  %s: ошибка анализа: %v\n", mp, err)
		} else {
			fmt.Fprintf(&b, "  %s:\n%s\n", mp, strings.TrimSpace(string(out)))
		}
	}
	if analyzed == 0 && ssdSkipped == 0 {
		fmt.Fprintln(&b, "  Подходящих разделов не найдено")
	} else if analyzed == 0 && ssdSkipped > 0 {
		fmt.Fprintln(&b, "  Все разделы — SSD: фрагментация не анализируется")
	}
	return b.String()
}

// BenchmarkDiskRandom — тест случайного доступа блоками 4 КБ (чистый Go).
func BenchmarkDiskRandom() string {
	dir := os.TempDir()
	if wd, err := os.Getwd(); err == nil {
		dir = wd
	}
	path := filepath.Join(dir, "pcdiag_random_test.bin")
	const fileSize = int64(256) << 20
	f, err := os.Create(path)
	if err != nil {
		return "Тест случайного доступа 4K: не удалось создать тестовый файл: " + err.Error()
	}
	defer func() {
		f.Close()
		os.Remove(path)
	}()
	if err := f.Truncate(fileSize); err != nil {
		return "Тест случайного доступа 4K: не удалось выделить файл: " + err.Error()
	}
	const block = 4096
	buf := make([]byte, block)
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	maxOffset := fileSize - block

	start := time.Now()
	writes := 0
	for time.Since(start) < 3*time.Second {
		off := rng.Int63n(maxOffset)
		if _, err := f.WriteAt(buf, off); err != nil {
			break
		}
		writes++
	}
	f.Sync()

	rbuf := make([]byte, block)
	start = time.Now()
	reads := 0
	for time.Since(start) < 3*time.Second {
		off := rng.Int63n(maxOffset)
		if _, err := f.ReadAt(rbuf, off); err != nil {
			break
		}
		reads++
	}
	wMBs := float64(writes) * block / 1024 / 1024 / 3
	rMBs := float64(reads) * block / 1024 / 1024 / 3
	return fmt.Sprintf("Тест случайного доступа 4K:\n  Запись: %.1f МБ/с (IOPS ~%.0f)\n  Чтение: %.1f МБ/с (IOPS ~%.0f)\n  Примечание: тест идёт через кэш ОС — сравнивайте диски между собой, а не с абсолютными эталонами", wMBs, float64(writes)/3, rMBs, float64(reads)/3)
}
