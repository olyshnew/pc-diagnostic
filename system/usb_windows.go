// +build linux windows

package system

import (
	"fmt"
	"os/exec"
	"strings"
	"runtime"

	// Для WMI на Windows
	ole "github.com/go-ole/go-ole"
	oleutil "github.com/go-ole/go-ole/oleutil"
)

// GetUSBInfo получает информацию о подключённых USB‑устройствах
func GetUSBInfo() string {
	var result strings.Builder

	if runtime.GOOS == "linux" {
		cmd := exec.Command("lsusb")
		out, err := cmd.Output()
		if err != nil {
			return "USB‑устройства: не удалось получить информацию (команда lsusb недоступна или требуются права root)"
	}

		lines := strings.Split(string(out), "\n")
		result.WriteString("USB‑устройства (Linux):\n")

		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				result.WriteString("  " + trimmed + "\n")
		}
	}

		if result.Len() <= len("USB‑устройства (Linux):\n") {
			return "USB‑устройства: не обнаружены"
	}
	return result.String()

	} else if runtime.GOOS == "windows" {
		// Сначала пробуем WMI
		devices, err := getUSBViaWMI()
		if err == nil && len(devices) > 0 {
			result.WriteString("USB‑устройства (WMI):\n")
			for _, device := range devices {
				result.WriteString("  " + device + "\n")
		}
		return result.String()
	}

		// Если WMI не сработал, пробуем PowerShell
		devices, err = getUSBViaPowerShell()
		if err == nil && len(devices) > 0 {
			result.WriteString("USB‑устройства (PowerShell):\n")
			for _, device := range devices {
				result.WriteString("  " + device + "\n")
		}
		return result.String()
	}
	}

	return "USB‑устройства: не удалось получить информацию"
}

// getUSBViaWMI получает USB‑устройства через WMI (Win32_USBHub)
func getUSBViaWMI() ([]string, error) {
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

	query := "SELECT Name, DeviceID FROM Win32_USBHub"
	resultsRaw, err := oleutil.CallMethod(service, "ExecQuery", query)
	if err != nil {
		return nil, fmt.Errorf("WMI запрос не выполнен: %v", err)
	}

	results := resultsRaw.ToIDispatch()
	defer results.Release()

	countVar, err := oleutil.GetProperty(results, "Count")
	if err != nil || countVar.Val == 0 {
		return nil, fmt.Errorf("USB‑устройства не найдены через WMI")
	}

	var devices []string
	for i := 0; i < int(countVar.Val); i++ {
		itemRaw, err := oleutil.CallMethod(results, "ItemIndex", i)
		if err != nil {
			continue
	}
	item := itemRaw.ToIDispatch()
	defer item.Release()

	nameRaw, err := oleutil.GetProperty(item, "Name")
	name := "Неизвестно"
	if err == nil {
		name = nameRaw.ToString()
	}

	deviceIDRaw, err := oleutil.GetProperty(item, "DeviceID")
	deviceID := "Неизвестно"
	if err == nil {
		deviceID = deviceIDRaw.ToString()
	}

	devices = append(devices, fmt.Sprintf("%s (ID: %s)", name, deviceID))
	}

	return devices, nil
}

// getUSBViaPowerShell получает USB‑устройства через PowerShell
func getUSBViaPowerShell() ([]string, error) {
	cmd := exec.Command("powershell", "-Command",
		"Get-PnpDevice -Class USB | Where-Object {$_.Present -eq $true} | Select-Object -ExpandProperty Name")
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

	if len(devices) == 0 {
		return nil, fmt.Errorf("USB‑устройства не найдены через PowerShell")
	}

	return devices, nil
}
