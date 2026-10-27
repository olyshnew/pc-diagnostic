package hardware

import (
    "context"
    "encoding/json"
    "fmt"
    "math/rand"
    "os"
    "os/exec"
    "runtime"
    "strconv"
    "strings"
    "time"

    "golang.org/x/text/encoding/charmap"
)

// fixedDriveList возвращает список локальных фиксированных дисков ("C:", "D:", ...).
func fixedDriveList() []string {
    if runtime.GOOS != "windows" {
        return []string{"/"}
    }
    var drives []string
    for c := 'C'; c <= 'Z'; c++ {
        root := string(c) + `:\`
        if fi, err := os.Stat(root); err == nil && fi.IsDir() {
            drives = append(drives, string(c)+":")
        }
    }
    return drives
}

// withTimeout — хелпер: таймаут-контекст для консольных команд.
func withTimeout(d time.Duration) (context.Context, context.CancelFunc) {
    return context.WithTimeout(context.Background(), d)
}

// decodeOEM866 превращает вывод консольных утилит Windows из OEM-кодировки
// (CP866) в UTF-8. chkdsk и defrag при выводе в канал пишут именно в CP866,
// поэтому русские пути без декодирования превращаются в кракозябры.
func decodeOEM866(b []byte) string {
    dec, err := charmap.CodePage866.NewDecoder().Bytes(b)
    if err != nil {
        return string(b)
    }
    return string(dec)
}

// runCmdRaw запускает консольную утилиту с таймаутом и декодирует
// её вывод из CP866. Используется для chkdsk и defrag.
func runCmdRaw(timeout time.Duration, name string, args ...string) (string, error) {
    ctx, cancel := withTimeout(timeout)
    defer cancel()
    out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
    return decodeOEM866(out), err
}

// summarizeChkdsk фильтрует вывод chkdsk: оставляет только значимые строки.
// chkdsk пишет прогресс через каретный возврат (\r), и находки
// («Found corrupt...») прилипают к той же строке, что и прогресс.
// Нормализуем \r в \n, чтобы находки стали самостоятельными строками.
func summarizeChkdsk(out string) string {
    out = strings.ReplaceAll(out, "\r\n", "\n")
    out = strings.ReplaceAll(out, "\r", "\n")

    var keep []string
    for _, line := range strings.Split(out, "\n") {
        l := strings.TrimSpace(line)
        if l == "" {
            continue
        }
        low := strings.ToLower(l)
        // Прогресс-строки chkdsk не несут информации о состоянии ФС.
        if strings.HasPrefix(low, "progress:") ||
            strings.Contains(low, "eta:") {
            continue
        }
        if strings.Contains(low, "%") ||
            strings.Contains(low, "ошиб") ||
            strings.Contains(low, "corrupt") ||
            strings.Contains(low, "проблем") ||
            strings.Contains(low, "готов") ||
            strings.Contains(low, "error") ||
            strings.Contains(low, "found") ||
            strings.Contains(low, "both own") ||
            strings.Contains(low, "bad index") ||
            strings.Contains(low, "unneeded link") ||
            strings.Contains(low, "records processed") ||
            strings.Contains(low, "байт") {
            keep = append(keep, "  "+l)
        }
    }
    if len(keep) == 0 {
        return "  (строк с проблемами нет — серьёзных ошибок не обнаружено)"
    }
    return strings.Join(keep, "\n")
}

// CheckFileSystemInfo проверяет файловые системы всех фиксированных дисков
// в режиме «только чтение» (chkdsk без ключей восстановления — безопасно).
func CheckFileSystemInfo() string {
    if runtime.GOOS != "windows" {
        return "Проверка файловой системы: доступно только на Windows"
    }

    var b strings.Builder
    b.WriteString("Проверка файловой системы (chkdsk, режим только чтения):\n")
    b.WriteString("Безопасно: ошибки не исправляются, диск только анализируется.\n")

    for _, d := range fixedDriveList() {
        b.WriteString(fmt.Sprintf("\nДиск %s:\n", d))
        out, err := runCmdRaw(10*time.Minute, "chkdsk", d, "/scan")
        low := strings.ToLower(out)
        switch {
        case err == nil:
            b.WriteString(summarizeChkdsk(out) + "\n")
        case strings.Contains(low, "corrupt") ||
            strings.Contains(low, "ошиб") ||
            strings.Contains(low, "error"):
            // chkdsk возвращает ненулевой код выхода, когда НАШЁЛ ошибки.
            // Это не сбой команды, а результат проверки.
            b.WriteString("  ОБНАРУЖЕНЫ ПРОБЛЕМЫ ФАЙЛОВОЙ СИСТЕМЫ:\n")
            b.WriteString(summarizeChkdsk(out) + "\n")
            b.WriteString("  Рекомендация: chkdsk " + d + " /f — исправление при перезагрузке.\n")
            b.WriteString("  Автоматически не запускаем: исправление требует размонтирования тома.\n")
        default:
            b.WriteString("  Не удалось выполнить проверку: " + strings.TrimSpace(out) + "\n")
        }
    }
    return b.String()
}

// ---------- Износ SSD (TBW) ----------

type smartScanOut struct {
    Devices []struct {
        Name string `json:"name"`
        Type string `json:"type"`
    } `json:"devices"`
}

type smartAttrOut struct {
    Device struct {
        Name      string `json:"name"`
        ModelName string `json:"model_name"`
    } `json:"device"`
    NVMe *struct {
        PercentageUsed   int   `json:"percentage_used"`
        DataUnitsWritten int64 `json:"data_units_written"`
    } `json:"nvme_smart_health_information_log"`
    ATA *struct {
        Table []struct {
            ID   int    `json:"id"`
            Name string `json:"name"`
            Val  int    `json:"value"`
            Raw  struct {
                Value  int64  `json:"value"`
                String string `json:"string"`
            } `json:"raw"`
        } `json:"table"`
    } `json:"ata_smart_attributes"`
}

// GetSSDWearInfo оценивает объём записанных на SSD данных (TBW).
// Остаток ресурса здесь не дублируем — он уже есть в базовом SMART-отчёте.
func GetSSDWearInfo() string {
    if runtime.GOOS != "windows" {
        return "Оценка износа SSD: доступно только на Windows"
    }

    smartctl, _ := exec.LookPath("smartctl")
    if smartctl == "" {
        return "Оценка износа SSD: утилита 'smartctl' не найдена. Скачайте Smartmontools (smartmontools.org)."
    }

    ctx, cancel := withTimeout(30 * time.Second)
    scanBytes, err := exec.CommandContext(ctx, smartctl, "--scan", "-j").Output()
    cancel()
    if err != nil {
        return "Оценка износа SSD: не удалось получить список дисков (smartctl --scan)"
    }
    var scan smartScanOut
    if err := json.Unmarshal(scanBytes, &scan); err != nil {
        return "Оценка износа SSD: неожиданный вывод smartctl --scan (нужна версия 7.0+)"
    }

    var b strings.Builder
    b.WriteString("Износ SSD (объём записи по SMART):\n")

    for _, dev := range scan.Devices {
        ctx, cancel := withTimeout(30 * time.Second)
        out, err := exec.CommandContext(ctx, smartctl, "-A", "-j", dev.Name).Output()
        cancel()
        if err != nil {
            b.WriteString(fmt.Sprintf("\nДиск %s: данные SMART недоступны\n", dev.Name))
            continue
        }

        var s smartAttrOut
        if err := json.Unmarshal(out, &s); err != nil {
            b.WriteString(fmt.Sprintf("\nДиск %s: не удалось разобрать вывод smartctl\n", dev.Name))
            continue
        }

        b.WriteString(fmt.Sprintf("\nДиск %s (%s):\n", dev.Name, s.Device.ModelName))

        if s.NVMe != nil {
            tbw := float64(s.NVMe.DataUnitsWritten) * 512000 / 1e12
            b.WriteString(fmt.Sprintf("  Записано за жизнь (TBW): %.2f ТБ\n", tbw))
            b.WriteString(fmt.Sprintf("  Использовано ресурса:    %d%%\n", s.NVMe.PercentageUsed))
            if remain := 100 - s.NVMe.PercentageUsed; remain < 10 {
                b.WriteString("  ВНИМАНИЕ: ресурс почти исчерпан, планируйте замену\n")
            }
        } else if s.ATA != nil {
            var tbw float64
            for _, a := range s.ATA.Table {
                if a.ID != 241 { // Total_LBAs_Written / Host_Writes_32MiB
                    continue
                }
                n, e := strconv.ParseInt(strings.TrimSpace(a.Raw.String), 10, 64)
                if e != nil || n <= 0 {
                    n = a.Raw.Value
                }
                if n <= 0 {
                    continue
                }
                // Единицы зависят от прошивки. Если имя атрибута сообщает
                // о блоках 32 МиБ (Kingston и др.) — считаем в них,
                // иначе это классические LBA по 512 байт.
                if strings.Contains(a.Name, "32MiB") {
                    tbw = float64(n) * 32 * 1024 * 1024 / 1e12
                } else {
                    tbw = float64(n) * 512 / 1e12
                }
            }
            if tbw > 0 {
                b.WriteString(fmt.Sprintf("  Записано за жизнь (TBW): %.2f ТБ\n", tbw))
            } else {
                b.WriteString("  Записано за жизнь (TBW): атрибут не найден\n")
            }
        }
    }
    return b.String()
}

// ---------- Фрагментация ----------

// driveMediaTypes возвращает соответствие «буква диска → тип носителя»
// ("SSD"/"HDD"/...) через PowerShell: Get-Partition даёт номер физического
// диска, Get-PhysicalDisk — его MediaType. nil — определить не удалось
// (PowerShell недоступен) — тогда анализ работает как раньше, по всем дискам.
func driveMediaTypes() map[string]string {
    diskOf, err := exec.Command("powershell", "-NoProfile", "-Command",
        `Get-Partition | Where-Object DriveLetter | ForEach-Object { "{0}|{1}" -f $_.DriveLetter, $_.DiskNumber }`).Output()
    if err != nil {
        return nil
    }
    mediaOf, err := exec.Command("powershell", "-NoProfile", "-Command",
        `Get-PhysicalDisk | ForEach-Object { "{0}|{1}" -f $_.DeviceId, $_.MediaType }`).Output()
    if err != nil {
        return nil
    }

    mediaByDisk := map[string]string{}
    for _, line := range strings.Split(string(mediaOf), "\n") {
        parts := strings.SplitN(strings.TrimSpace(line), "|", 2)
        if len(parts) == 2 && parts[0] != "" {
            mediaByDisk[parts[0]] = parts[1]
        }
    }

    res := map[string]string{}
    for _, line := range strings.Split(string(diskOf), "\n") {
        parts := strings.SplitN(strings.TrimSpace(line), "|", 2)
        if len(parts) == 2 && parts[0] != "" {
            res[parts[0]] = mediaByDisk[parts[1]]
        }
    }
    if len(res) == 0 {
        return nil
    }
    return res
}

// AnalyzeFragmentation запускает анализ фрагментации (defrag /A — только
// анализ, без дефрагментации). Требует прав администратора.
// SSD-диски пропускаются: дефрагментация им не нужна (контроллер сам
// распределяет записи), а запуск defrag расходует ресурс ячеек.
func AnalyzeFragmentation() string {
    if runtime.GOOS != "windows" {
        return "Анализ фрагментации: доступно только на Windows"
    }

    var b strings.Builder
    b.WriteString("Анализ фрагментации (defrag /A — только анализ):\n")

    media := driveMediaTypes()
    if media == nil {
        b.WriteString("  Не удалось определить тип носителя (Get-PhysicalDisk недоступен) —\n")
        b.WriteString("  анализ выполняется по всем фиксированным дискам, как раньше.\n")
    }

    anyRun := false
    for _, d := range fixedDriveList() {
        b.WriteString(fmt.Sprintf("\nДиск %s:\n", d))

        if media != nil {
            if m, ok := media[strings.TrimSuffix(d, ":")]; ok && strings.EqualFold(m, "SSD") {
                b.WriteString("  Пропущено: диск SSD — фрагментация для него нерелевантна,\n")
                b.WriteString("  а анализ defrag расходует ресурс записи ячеек.\n")
                continue
            }
        }
        anyRun = true

        out, err := runCmdRaw(5*time.Minute, "defrag", d, "/A", "/V")
        low := strings.ToLower(out)
        switch {
        case strings.Contains(low, "отказано") || strings.Contains(low, "denied"):
            b.WriteString("  Требуются права администратора — анализ пропущен.\n")
        case strings.TrimSpace(out) == "":
            if err != nil {
                b.WriteString(fmt.Sprintf("  Утилита не вернула данных (код выхода: %v).\n", err))
            } else {
                b.WriteString("  Утилита не вернула данных. Обычно это значит, что\n")
                b.WriteString("  анализ системного диска требует прав администратора.\n")
            }
        default:
            b.WriteString(out + "\n")
        }
    }

    if media != nil && !anyRun {
        b.WriteString("\nМагнитных дисков (HDD) не найдено: в системе только SSD — анализ завершён.\n")
    }
    return b.String()
}

// ---------- Тест случайного доступа (4K) ----------

// indentAll добавляет отступ к каждой строке многострочного текста.
func indentAll(s, prefix string) string {
    lines := strings.Split(s, "\n")
    for i, l := range lines {
        lines[i] = prefix + l
    }
    return strings.Join(lines, "\n")
}

// BenchmarkDiskRandom измеряет скорость случайных операций с блоками 4 КБ —
// главный показатель отзывчивости системы (загрузка ОС, открытие программ).
// Результаты идут через кэш ОС, поэтому завышены относительно прямых
// тестов (CrystalDiskMark), но годятся для сравнения дисков между собой.
func BenchmarkDiskRandom() string {
    var b strings.Builder
    b.WriteString("Тест случайного доступа 4K (3 сек запись + 3 сек чтение):\n")

    for _, d := range fixedDriveList() {
        dir := d + `\`
        if runtime.GOOS != "windows" {
            dir = "/"
        }
        b.WriteString(fmt.Sprintf("\nДиск %s:\n", d))
        w, r, err := measureRandom4K(dir)
        if err != nil {
            b.WriteString("  Ошибка теста: " + err.Error() + "\n")
            continue
        }
        b.WriteString(fmt.Sprintf("  Случайная запись:  %.2f МБ/с\n", w))
        b.WriteString(fmt.Sprintf("  Случайное чтение:  %.2f МБ/с\n", r))
    }
    return b.String()
}

// measureRandom4K выполняет тест на одном диске.
func measureRandom4K(dir string) (wMbps, rMbps float64, err error) {
    f, err := os.CreateTemp(dir, "pcdiag_rand_*.tmp")
    if err != nil {
        return 0, 0, err
    }
    name := f.Name()
    defer func() {
        f.Close()
        os.Remove(name)
    }()

    const blockSize = 4096
    const fileMB = 64
    size := int64(fileMB) * 1024 * 1024
    if err := f.Truncate(size); err != nil {
        return 0, 0, err
    }

    buf := make([]byte, blockSize)
    rnd := rand.New(rand.NewSource(time.Now().UnixNano()))

    // Запись
    deadline := time.Now().Add(3 * time.Second)
    ops := 0
    for time.Now().Before(deadline) {
        off := rnd.Int63n(fileMB) * blockSize
        if _, err := f.WriteAt(buf, off); err != nil {
            return 0, 0, err
        }
        ops++
    }
    _ = f.Sync()
    wMbps = float64(ops) * blockSize / (1024 * 1024) / 3.0

    // Чтение
    deadline = time.Now().Add(3 * time.Second)
    ops = 0
    for time.Now().Before(deadline) {
        off := rnd.Int63n(fileMB) * blockSize
        if _, err := f.ReadAt(buf, off); err != nil {
            return 0, 0, err
        }
        ops++
    }
    rMbps = float64(ops) * blockSize / (1024 * 1024) / 3.0

    return wMbps, rMbps, nil
}
