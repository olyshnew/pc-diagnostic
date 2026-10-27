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

// GetMainboardInfo получает информацию о материнской плате
func GetMainboardInfo() string {
	var result strings.Builder

	if runtime.GOOS == "linux" {
		cmd := exec.Command("dmidecode", "-t", "2")
		output, err := cmd.Output()
		if err != nil {
			return "Материнская плата: не удалось получить информацию (dmidecode)"
	}
		result.WriteString("Информация о материнской плате (dmidecode):\n")
		result.WriteString(string(output))
		return result.String()
	} else if runtime.GOOS == "windows" {
		mbInfo, err := getMainboardViaWMI()
		if err == nil && mbInfo != "" {
			return mbInfo
		}
	}

	return "Материнская плата: не удалось получить информацию"
}

// +build windows
func getMainboardViaWMI() (string, error) {
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

	query := "SELECT Manufacturer, Product, SerialNumber, Version FROM Win32_BaseBoard"
	resultsRaw, err := oleutil.CallMethod(service, "ExecQuery", query)
	if err != nil {
		return "", fmt.Errorf("WMI запрос не выполнен: %v", err)
	}

	results := resultsRaw.ToIDispatch()
	defer results.Release()

	countVar, err := oleutil.GetProperty(results, "Count")
	if err != nil || countVar.Val == 0 {
		return "", fmt.Errorf("материнская плата не найдена через WMI")
	}

	var mbInfo []string
	mbInfo = append(mbInfo, "Материнская плата (WMI):")

	for i := 0; i < int(countVar.Val); i++ {
		itemRaw, err := oleutil.CallMethod(results, "ItemIndex", i)
		if err != nil {
			continue
		}
		item := itemRaw.ToIDispatch()
		defer item.Release() // явное освобождение ресурсов

		manufacturerRaw, err := oleutil.GetProperty(item, "Manufacturer")
		manufacturer := "Неизвестно"
		if err == nil {
			manufacturer = manufacturerRaw.ToString()
		}

		productRaw, err := oleutil.GetProperty(item, "Product")
		product := "Неизвестно"
		if err == nil {
			product = productRaw.ToString()
		}

		serialRaw, err := oleutil.GetProperty(item, "SerialNumber")
		serial := "Неизвестно"
		if err == nil {
			serial = serialRaw.ToString()
		}

		versionRaw, err := oleutil.GetProperty(item, "Version")
		version := "Неизвестно"
		if err == nil {
			version = versionRaw.ToString()
		}

		mbInfo = append(mbInfo, fmt.Sprintf("  Производитель: %s", manufacturer))
		mbInfo = append(mbInfo, fmt.Sprintf("  Модель: %s", product))
		mbInfo = append(mbInfo, fmt.Sprintf("  Серийный номер: %s", serial))
		mbInfo = append(mbInfo, fmt.Sprintf("  Версия: %s", version))
	}

	return strings.Join(mbInfo, "\n"), nil
}
