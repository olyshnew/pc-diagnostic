package hardware

import (
    "bytes"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
    "strconv"
    "strings"
    "time"
)

// RunSMARTDiagnostics выполняет глубокую диагностику всех физических дисков через smartctl
func RunSMARTDiagnostics() []DiagnosticResult {
    var results []DiagnosticResult

    if _, err := exec.LookPath("smartctl"); err != nil {
        results = append(results, DiagnosticResult{
            Name:  "SMART диагностика",
            Value: "",
            Error: fmt.Errorf("утилита smartctl не найдена в PATH. Установите Smartmontools"),
        })
        return results
    }

    devices, err := listBlockDevices()
    if err != nil {
        results = append(results, DiagnosticResult{
            Name:  "SMART диагностика",
            Value: "",
            Error: fmt.Errorf("не удалось определить диски: %v", err),
        })
        return results
    }

    for _, dev := range devices {
        res := diagnoseSingleDisk(dev)
        results = append(results, res)
    }
    return results
}

// diagnoseSingleDisk запускает smartctl и возвращает читаемый результат
func diagnoseSingleDisk(devicePath string) DiagnosticResult {
    cmd := exec.Command("smartctl", "-a", "-d", "auto", devicePath, "--json=c")
    var out bytes.Buffer
    cmd.Stdout = &out
    cmd.Stderr = &out

    err := cmd.Run()
    jsonOutput := out.String()
    summary := parseSMARTJSON(jsonOutput)

    if err != nil {
        if exitErr, ok := err.(*exec.ExitError); ok {
            code := exitErr.ExitCode()
            if summary != "" {
                return DiagnosticResult{Name: devicePath, Value: summary, Error: nil}
            }
            return DiagnosticResult{
                Name:  devicePath,
                Value: jsonOutput,
                Error: fmt.Errorf("smartctl вернул код %d", code),
            }
        }
        return DiagnosticResult{Name: devicePath, Value: jsonOutput, Error: err}
    }

    return DiagnosticResult{Name: devicePath, Value: summary, Error: nil}
}

// parseSMARTJSON извлекает ключевые параметры из JSON-вывода smartctl
func parseSMARTJSON(jsonStr string) string {
    getJSONValue := func(key string) string {
        search := "\"" + key + "\":"
        idx := strings.Index(jsonStr, search)
        if idx == -1 {
            return ""
        }
        start := idx + len(search)
        for start < len(jsonStr) && jsonStr[start] == ' ' {
            start++
        }
        if start >= len(jsonStr) {
            return ""
        }
        if jsonStr[start] == '"' {
            end := start + 1
            for end < len(jsonStr) && jsonStr[end] != '"' {
                end++
            }
            return jsonStr[start+1:end]
        }
        end := start
        for end < len(jsonStr) && jsonStr[end] != ',' && jsonStr[end] != '}' && jsonStr[end] != '\n' {
            end++
        }
        return strings.TrimSpace(jsonStr[start:end])
    }

    getNestedValue := func(parent, key string) string {
        idx := strings.Index(jsonStr, "\""+parent+"\":{")
        if idx == -1 {
            return ""
        }
        sub := jsonStr[idx:]
        search := "\"" + key + "\":"
        subIdx := strings.Index(sub, search)
        if subIdx == -1 {
            return ""
        }
        start := subIdx + len(search)
        for start < len(sub) && sub[start] == ' ' {
            start++
        }
        if start >= len(sub) {
            return ""
        }
        if sub[start] == '"' {
            end := start + 1
                      for end < len(sub) && sub[end] != '"' {
                end++
            }
            return sub[start+1 : end]
        }
        end := start
        for end < len(sub) && sub[end] != ',' && sub[end] != '}' && sub[end] != '\n' {
            end++
        }
        return strings.TrimSpace(sub[start:end])
    }

    var sb strings.Builder

    model := getJSONValue("model_name")
    if model != "" {
        sb.WriteString(fmt.Sprintf("  Модель: %s\n", model))
    }
    serial := getJSONValue("serial_number")
    if serial != "" {
        sb.WriteString(fmt.Sprintf("  Серийный номер: %s\n", serial))
    }
    fw := getJSONValue("firmware_version")
    if fw != "" {
        sb.WriteString(fmt.Sprintf("  Прошивка: %s\n", fw))
    }

    // Тип носителя: SSD или HDD (см. isSSDDevice ниже)
    if isSSDDevice(jsonStr) {
        sb.WriteString("  Тип: SSD\n")
    }

    capBytes := getJSONValue("bytes")
    if capBytes != "" {
        gb := parseFloat64(capBytes) / (1024 * 1024 * 1024)
        if gb > 0 {
            sb.WriteString(fmt.Sprintf("  Ёмкость: %.0f ГБ\n", gb))
        }
    }

    passed := getNestedValue("smart_status", "passed")
    if passed == "true" {
        sb.WriteString("  SMART: PASSED (диск здоров)\n")
    } else if passed == "false" {
        sb.WriteString("  SMART: FAILED (диск неисправен!)\n")
    }

    temp := getNestedValue("temperature", "current")
    if temp != "" {
        sb.WriteString(fmt.Sprintf("  Температура: %s°C\n", temp))
    }

    hours := getNestedValue("power_on_time", "hours")
    if hours != "" {
        sb.WriteString(fmt.Sprintf("  Время работы: %s часов\n", hours))
    }

    cycles := getJSONValue("power_cycle_count")
    if cycles != "" {
        sb.WriteString(fmt.Sprintf("  Циклов включения: %s\n", cycles))
    }

    endurance := getNestedValue("endurance_used", "current_percent")
    if endurance != "" {
        sb.WriteString(fmt.Sprintf("  Износ SSD: %s%%\n", endurance))
    }

    // Percentage Used (NVMe): сколько ресурса записи использовано.
    // Пороги: > 80% — внимание, > 90% — критично.
    pctUsed := getNestedValue("nvme_smart_health_information_log", "percentage_used")
    if pctUsed != "" {
        sb.WriteString(fmt.Sprintf("  Использовано ресурса (Percentage Used): %s%%\n", pctUsed))
        if p, e := strconv.Atoi(pctUsed); e == nil {
            switch {
            case p >= 90:
                sb.WriteString("  КРИТИЧНО: ресурс почти исчерпан — планируйте замену SSD\n")
            case p >= 80:
                sb.WriteString("  ВНИМАНИЕ: использовано более 80% ресурса\n")
            }
        }
    }

    remainingPerc := extractAttrValue(jsonStr, "Remaining_Lifetime_Perc")
    if remainingPerc != "" {
        sb.WriteString(fmt.Sprintf("  Остаток ресурса: %s%%\n", remainingPerc))
    }

    ssdLife := extractAttrValue(jsonStr, "SSD_Life_Left")
    if ssdLife != "" {
        sb.WriteString(fmt.Sprintf("  SSD Life Left: %s%%\n", ssdLife))
        if v, e := strconv.Atoi(strings.TrimSpace(ssdLife)); e == nil {
            switch {
            case v <= 10:
                sb.WriteString("  КРИТИЧНО: осталось менее 10% ресурса\n")
            case v <= 20:
                sb.WriteString("  ВНИМАНИЕ: осталось менее 20% ресурса\n")
            }
        }
    }

    realloc := extractAttrValue(jsonStr, "Reallocated_Sector_Ct")
    if realloc != "" {
        sb.WriteString(fmt.Sprintf("  Переназначенные сектора: %s\n", realloc))
    }

    crcErrors := extractAttrValue(jsonStr, "UDMA_CRC_Error_Count")
    if crcErrors != "" {
        sb.WriteString(fmt.Sprintf("  Ошибки передачи (UDMA CRC): %s\n", crcErrors))
    }

    result := sb.String()
    if strings.TrimSpace(result) == "" {
        return "  Не удалось извлечь данные SMART\n"
    }
    return result
}

// isSSDDevice определяет SSD по JSON-выводу smartctl:
// - наличие блока nvme_smart_health_information_log (NVMe — всегда SSD);
// - rotation_rate == 0 (SATA SSD; у магнитных дисков — 5400/7200).
// Если ничего не найдено, возвращает false — тип считается неопределённым.
func isSSDDevice(jsonStr string) bool {
    if strings.Contains(jsonStr, `"nvme_smart_health_information_log"`) {
        return true
    }
    const key = `"rotation_rate":`
    idx := strings.Index(jsonStr, key)
    if idx == -1 {
        return false
    }
    rest := strings.TrimLeft(jsonStr[idx+len(key):], " \t")
    return strings.HasPrefix(rest, "0") || strings.HasPrefix(rest, `"0`)
}

// extractAttrValue ищет raw_value для атрибута по имени
func extractAttrValue(jsonStr, attrName string) string {
    search := "\"name\":\"" + attrName + "\""
    idx := strings.Index(jsonStr, search)
    if idx == -1 {
        return ""
    }
    sub := jsonStr[idx:]
    rawSearch := "\"raw\":{\"value\":"
    rawIdx := strings.Index(sub, rawSearch)
    if rawIdx == -1 {
        return ""
    }
    start := rawIdx + len(rawSearch)
    end := start
    for end < len(sub) && sub[end] != ',' && sub[end] != '}' {
        end++
    }
    return strings.TrimSpace(sub[start:end])
}

// parseFloat64 — простой парсер float
func parseFloat64(s string) float64 {
    var f float64
    fmt.Sscanf(s, "%f", &f)
    return f
}

// BenchmarkDisk измеряет скорость линейного чтения/записи
func BenchmarkDisk(sizeMB int) (writeSpeed float64, readSpeed float64, err error) {
    writeDir := os.Getenv("TEMP")
    if writeDir == "" {
        writeDir = "."
    }
    tempFile := filepath.Join(writeDir, "pcdiag_benchmark.tmp")

    buffer := make([]byte, 32*1024)
    sizeBytes := int64(sizeMB * 1024 * 1024)

    fWrite, err := os.OpenFile(tempFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
    if err != nil {
        return 0, 0, fmt.Errorf("ошибка создания файла для записи: %w", err)
    }

    startWrite := time.Now()
    written := int64(0)
    for written < sizeBytes {
        n, werr := fWrite.Write(buffer)
        if werr != nil {
            fWrite.Close()
            return 0, 0, fmt.Errorf("ошибка записи: %w", werr)
        }
        written += int64(n)
    }
    writeDuration := time.Since(startWrite).Seconds()
    fWrite.Close()

    if writeDuration > 0 {
        writeSpeed = float64(sizeMB) / writeDuration
    }

    fRead, err := os.Open(tempFile)
    if err != nil {
        _ = os.Remove(tempFile)
        return writeSpeed, 0, fmt.Errorf("ошибка открытия файла для чтения: %w", err)
    }

    defer func() {
        fRead.Close()
        os.Remove(tempFile)
    }()

    readBuffer := make([]byte, 32*1024)
    toRead := sizeBytes
    startRead := time.Now()
    for toRead > 0 {
        n, rerr := fRead.Read(readBuffer)
        if rerr != nil && !strings.Contains(rerr.Error(), "EOF") {
            return writeSpeed, 0, fmt.Errorf("ошибка чтения: %w", rerr)
        }
        toRead -= int64(n)
    }
    readDuration := time.Since(startRead).Seconds()
    if readDuration > 0 {
        readSpeed = float64(sizeMB) / readDuration
    }

    return writeSpeed, readSpeed, nil
}

// listBlockDevices определяет список физических дисков системы
func listBlockDevices() ([]string, error) {
    switch runtime.GOOS {
    case "windows":
        cmd := exec.Command("smartctl", "--scan")
        var out bytes.Buffer
        cmd.Stdout = &out

        if err := cmd.Run(); err != nil {
            psCmd := exec.Command("powershell", "-Command", "(Get-PhysicalDisk).DeviceId")
            var psOut bytes.Buffer
            psCmd.Stdout = &psOut
            if err := psCmd.Run(); err != nil {
                return nil, fmt.Errorf("ошибка определения дисков: %v", err)
            }
            lines := strings.Split(strings.TrimSpace(psOut.String()), "\r\n")
            var disks []string
            for _, line := range lines {
                line = strings.TrimSpace(line)
                if line != "" {
                    disks = append(disks, fmt.Sprintf("\\\\.\\PhysicalDrive%s", line))
                }
            }
            return disks, nil
        }

        lines := strings.Split(strings.TrimSpace(out.String()), "\n")
        var disks []string
        for _, line := range lines {
            line = strings.TrimSpace(line)
            if line == "" || strings.HasPrefix(line, "#") {
                continue
            }
            parts := strings.Fields(line)
            if len(parts) > 0 {
                disks = append(disks, parts[0])
            }
        }
        return disks, nil

    case "linux":
        out, err := exec.Command("lsblk", "-d", "-n", "-o", "NAME,TYPE").Output()
        if err != nil {
            return nil, err
        }
        lines := strings.Split(string(out), "\n")
        var disks []string
        for _, line := range lines {
            if strings.Contains(line, " disk") {
                parts := strings.Fields(line)
                if len(parts) > 0 {
                    disks = append(disks, "/dev/"+parts[0])
                }
            }
        }
        return disks, nil
    case "darwin":
        return []string{"/dev/disk0"}, nil
    default:
        return nil, fmt.Errorf("unsupported OS")
    }
}

