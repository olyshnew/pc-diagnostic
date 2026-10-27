// +build linux windows

package system

import (
	"fmt"
	"os/exec"
	"strings"
	"runtime"
)

// +build windows
import (
	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// GetDiskInfo получает информацию о дисках и разделах
func GetDiskInfo() string {
	var result strings.Builder

	if runtime.GOOS == "linux" {
		cmd := exec.Command("lsblk", "-o", "NAME,SIZE,TYPE,MOUNTPOINT")
		output, err := cmd.Output()
		if err != nil {
			return "Диски: не удалось получить информацию (lsblk)"
	}
		result.WriteString("Информация о дисках (lsblk):\n")
		result.WriteString(string(output))
		return result.String()
	} else if runtime.GOOS == "windows" {
		// Метод 1: WMI для дисков
		diskInfo, err := getDiskViaWMI()
		if err == nil && diskInfo != "" {
			return diskInfo
		}

		// Резервный метод: через PowerShell
		psInfo, err := getDiskViaPowerShell()
		if err == nil && psInfo != "" {
			return psInfo
		}
	}

	return "Диски: не удалось получить информацию"
}

// +build windows
func getDiskViaWMI() (string, error) {
	err := ole.CoInitialize(0)
	if err != nil {
		return "", err
	}
	defer ole.CoUninitialize()

	unknown, err := oleutil.CreateObject("WbemScripting.SWbemLocator")
	if err != nil {
		return "", err
	}
	defer unknown.Release()

	locator, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return "", err
	}
	defer locator.Release()

	serviceRaw, err := oleutil.CallMethod(locator, "ConnectServer", ".", "root\\CIMV2")
	if err != nil {
		return "", fmt.Errorf("не удалось подключиться к WMI: %v", err)
	}

	service := serviceRaw.ToIDispatch()
	defer service.Release()

	query := "SELECT Name, Size, DeviceID, Model FROM Win32_DiskDrive"
	resultsRaw, err := oleutil.CallMethod(service, "ExecQuery", query)
	if err != nil {
		return "", fmt.Errorf("WMI запрос не выполнен: %v", err)
	}

	results := resultsRaw.ToIDispatch()
	defer results.Release()

	countVar, err := oleutil.GetProperty(results, "Count")
	if err != nil || countVar.Val == 0 {
		return "", fmt.Errorf("диски не найдены через WMI")
	}

	var disks []string
	disks = append(disks, "Диски (WMI):")

	for i := 0; i < int(countVar.Val); i++ {
		itemRaw, err := oleutil.CallMethod(results, "ItemIndex", i)
		if err != nil {
			continue
		}
		item := itemRaw.ToIDispatch()
		defer item.Release() // явное освобождение ресурсов

		nameRaw, err := oleutil.GetProperty(item, "Name")
		name := "Неизвестно"
		if err == nil {
			name = nameRaw.ToString()
		}

		sizeRaw, err := oleutil.GetProperty(item, "Size")
		size := "Неизвестно"
		if err == nil {
			size = sizeRaw.ToString()
	}

		deviceIDRaw, err := oleutil.GetProperty(item, "DeviceID")
		deviceID := "Неизвестно"
		if err == nil {
			deviceID = deviceIDRaw.ToString()
	}

		modelRaw, err := oleutil.GetProperty(item, "Model")
		model := "Неизвестно"
		if err == nil {
			model = modelRaw.ToString()
	}

		disks = append(disks, fmt.Sprintf("  %s (%s, ID: %s, Модель: %s)",
			name, size, deviceID, model))
	}

	return strings.Join(disks, "\n"), nil
}

// +build windows
func getDiskViaPowerShell() (string, error) {
	cmd := exec.Command("powershell", "-Command",
		"Get-Disk | Select-Object Number,Size,FriendlyName | Format-List")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("ошибка выполнения PowerShell-команды: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var result []string
	result = append(result, "Диски (PowerShell):")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			result = append(result, "  "+trimmed)
		}
	}

	return strings.Join(result, "\n"), nil
}
