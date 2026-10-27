package main

import (
	"fmt"
	"runtime"
	"time"
	"pc-diagnostic/system"
)

// getSystemInfo собирает базовую информацию о системе
func getSystemInfo() string {
	baseInfo := fmt.Sprintf(
		"ОС: %s\nАрхитектура: %s\nВремя запуска: %s",
		runtime.GOOS,
		runtime.GOARCH,
		time.Now().Format("2006-01-02 15:04:05"),
	)

	mainboardInfo := system.GetMainboardInfo()
	usbInfo := system.GetUSBInfo()

	return fmt.Sprintf("%s\n\n%s\n%s", baseInfo, mainboardInfo, usbInfo)
}
