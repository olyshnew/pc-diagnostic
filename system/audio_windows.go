// +build windows

package system

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"runtime"
)

// +build windows
import (
	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// GetAudioInfo получает информацию о звуковой подсистеме
func GetAudioInfo() string {
	var result strings.Builder

	if runtime.GOOS == "linux" {
		// Метод 1: aplay -l (ALSA)
		cmd := exec.Command("aplay", "-l")
		out, err := cmd.Output()
		if err == nil && len(out) > 0 {
			lines := strings.Split(string(out), "\n")
			result.WriteString("Звуковые устройства (ALSA):\n")
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if trimmed != "" {
					result.WriteString("  " + trimmed + "\n")
			}
		}
		return result.String()
	}

		// Метод 2: информация из /proc/asound/
		procInfo, err := getAudioFromProc()
		if err == nil && procInfo != "" {
			return procInfo
	}

	} else if runtime.GOOS == "windows" {
		// Методы для Windows
		devices, err := getAudioViaWMI()
		if err == nil && len(devices) > 0 {
			result.WriteString("Звуковые устройства (WMI):\n")
			for _, device := range devices {
				result.WriteString("  " + device + "\n")
		}
		return result.String()
	}

		devices, err = getAudioViaPowerShell()
		if err == nil && len(devices) > 0 {
			result.WriteString("Звуковые устройства (PowerShell):\n")
			for _, device := range devices {
				result.WriteString("  " + device + "\n")
		}
		return result.String()
	}
	}

	return "Звуковая подсистема: не удалось получить информацию"
}

// Linux-специфичная функция
// +build linux
func getAudioFromProc() (string, error) {
	var result strings.Builder
	var hasData bool

	cardsData, err := readFileSafely("/proc/asound/cards")
	if err == nil {
		result.WriteString("Звуковые карты (/proc/asound/cards):\n")
		cardLines := strings.Split(cardsData, "\n")
		for _, line := range cardLines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				result.WriteString("  " + trimmed + "\n")
			hasData = true
		}
	}
	}

	pcmData, err := readFileSafely("/proc/asound/pcm")
	if err == nil {
		if hasData {
			result.WriteString("\n") // разделитель между секциями
		}
		result.WriteString("PCM устройства (/proc/asound/pcm):\n")
		pcmLines := strings.Split(pcmData, "\n")
		for _, line := range pcmLines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				result.WriteString("  " + trimmed + "\n")
			hasData = true
		}
	}
	}

	if !hasData {
		return "", fmt.Errorf("нет данных в /proc/asound/")
	}

	return result.String(), nil
}

// Windows-специфичные функции
// +build windows
func getAudioViaWMI() ([]string, error) {
	err := ole.CoInitialize(0)
	if err != nil {
		return nil, err
	}
	defer ole.CoUninitialize()

	unknown, err := oleutil.CreateObject("WbemScripting.SWbemLocator")
	if err != nil {
		return nil, err
	}
	defer unknown.Release()

	locator, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, err
	}
	defer locator.Release()

	serviceRaw, err := oleutil.CallMethod(locator, "ConnectServer", ".", "root\\CIMV2")
	if err != nil {
		return nil, fmt.Errorf("не удалось подключиться к WMI: %v", err)
	}

	service := serviceRaw.ToIDispatch()
	defer service.Release()

	query := "SELECT Name, Manufacturer, DeviceID, Status FROM Win32_SoundDevice"
	resultsRaw, err := oleutil.CallMethod(service, "ExecQuery", query)
	if err != nil {
		return nil, fmt.Errorf("WMI запрос не выполнен: %v", err)
	}

	results := resultsRaw.ToIDispatch()
	defer results.Release()

	countVar, err := oleutil.GetProperty(results, "Count")
	if err != nil || countVar.Val == 0 {
		return nil, fmt.Errorf("звуковые устройства не найдены через WMI")
	}

	var devices []string
	for i := 0; i < int(countVar.Val); i++ {
		itemRaw, err := oleutil.CallMethod(results, "ItemIndex", i)
		if err != nil {
			continue
	}
	item := itemRaw.ToIDispatch()

	nameRaw, err := oleutil.GetProperty(item, "Name")
	name := "Неизвестно"
	if err == nil {
		name = nameRaw.ToString()
	}

	manufacturerRaw, err := oleutil.GetProperty(item, "Manufacturer")
	manufacturer := "Неизвестно"
	if err == nil {
		manufacturer = manufacturerRaw.ToString()
	}

	deviceIDRaw, err := oleutil.GetProperty(item, "DeviceID")
	deviceID := "Неизвестно"
	if err == nil {
		deviceID = deviceIDRaw.ToString()
	}

	statusRaw, err := oleutil.GetProperty(item, "Status")
	status := "Неизвестно"
	if err == nil {
		status = statusRaw.ToString()
	}

	devices = append(devices, fmt.Sprintf("%s (%s, ID: %s, Статус: %s)",
		name, manufacturer, deviceID, status))

	item.Release() // явное освобождение ресурсов
	}

	return devices, nil
}

// +build windows
func getAudioViaPowerShell() ([]string, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		`[Console]::OutputEncoding=[System.Text.Encoding]::UTF8; Get-PnpDevice -Class AudioEndpoint | Where-Object {$_.Present -eq $true} | Select-Object -ExpandProperty Name`)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var devices []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && trimmed != "Name" {
			devices = append(devices, trimmed)
		}
	}
	return devices, nil
}


func readFileSafely(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
