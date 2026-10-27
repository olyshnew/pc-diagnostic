//go:build darwin

package hardware

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// listDisksDarwin — имена физических дисков (disk0, disk1...) из diskutil list.
func listDisksDarwin() []string {
	out, err := exec.Command("diskutil", "list").Output()
	if err != nil {
		return nil
	}
	var disks []string
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "physical") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 0 && strings.HasPrefix(fields[0], "/dev/disk") {
			disks = append(disks, strings.TrimPrefix(fields[0], "/dev/"))
		}
	}
	return disks
}

// physicalDisks — имя, которое ожидает disk_deep.go (RunSMARTDiagnostics).
func physicalDisks() []string { return listDisksDarwin() }

// CheckFileSystemInfo — проверка файловой системы (macOS: diskutil verifyVolume,
// только чтение — безопасный аналог chkdsk /scan).
func CheckFileSystemInfo() string {
	out, err := exec.Command("diskutil", "verifyVolume", "/").CombinedOutput()
	var b strings.Builder
	b.WriteString("Проверка файловой системы (diskutil verifyVolume, только чтение):\n")
	b.WriteString("Безопасно: ошибки не исправляются, том только анализируется.\n")
	if len(out) > 0 {
		b.WriteString(strings.TrimSpace(string(out)) + "\n")
	}
	if err != nil {
		b.WriteString("\nПРЕДУПРЕЖДЕНИЕ: обнаружены проблемы тома —\nвыполните восстановление из Дисковой утилиты (First Aid)\n")
	}
	return b.String()
}

// GetSSDWearInfo — объём записи за жизнь (TBW) и Percentage Used через smartctl.
func GetSSDWearInfo() string {
	smartPath, err := exec.LookPath("smartctl")
	if err != nil {
		return "Износ SSD (TBW): утилита smartctl не найдена (brew install smartmontools)"
	}
	var b strings.Builder
	fmt.Fprintln(&b, "Износ SSD (ресурс записи):")
	found := false
	for _, dev := range listDisksDarwin() {
		out, err := exec.Command(smartPath, "-A", "-j", "/dev/"+dev).Output()
		if err != nil && len(out) == 0 {
			fmt.Fprintf(&b, "  %s: smartctl вернул ошибку (для внутренних дисков нужен sudo): %v\n", dev, err)
			continue
		}
		var doc struct {
			RotationRate int `json:"rotation_rate"`
			NVMe         struct {
				PercentageUsed   int   `json:"percentage_used"`
				DataUnitsWritten int64 `json:"data_units_written"`
			} `json:"nvme_smart_health_information_log"`
		}
		if err := json.Unmarshal(out, &doc); err != nil {
			continue
		}
		found = true
		fmt.Fprintf(&b, "  %s:\n", dev)
		if doc.RotationRate == 0 && doc.NVMe.PercentageUsed == 0 && doc.NVMe.DataUnitsWritten == 0 {
			continue
		}
		if doc.RotationRate == 0 {
			b.WriteString("    Тип: SSD\n")
		}
		if doc.NVMe.PercentageUsed > 0 {
			status := "норма"
			if doc.NVMe.PercentageUsed >= 90 {
				status = "КРИТИЧНО"
			} else if doc.NVMe.PercentageUsed >= 80 {
				status = "ВНИМАНИЕ"
			}
			fmt.Fprintf(&b, "    Percentage Used: %d%% (%s)\n", doc.NVMe.PercentageUsed, status)
		}
		if doc.NVMe.DataUnitsWritten > 0 {
			// 1 data unit NVMe = 1 000 000 * 512 байт
			tbw := float64(doc.NVMe.DataUnitsWritten) * 512000 / 1e12
			fmt.Fprintf(&b, "    Объём записи за жизнь: %.2f ТБ\n", tbw)
		}
	}
	if !found {
		b.WriteString("  Данных SMART не получено (нужны права sudo)\n")
	}
	return b.String()
}

// AnalyzeFragmentation — на macOS не применимо: APFS не требует дефрагментации.
func AnalyzeFragmentation() string {
	return "Анализ фрагментации: на macOS не применим — APFS не требует дефрагментации"
}

// BenchmarkDiskRandom — тест случайного доступа 4K (чистый Go, как на Linux).
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
