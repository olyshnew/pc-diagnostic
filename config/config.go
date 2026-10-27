package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// AppConfig структура конфигурации приложения
type AppConfig struct {
	ParallelExecution bool   `json:"parallel_execution"`
	OutputFormat    string `json:"output_format"`
	OutputFile      string `json:"output_file"`
	Mode string `json:"mode"`
}

// Default конфигурация по умолчанию
var Default = AppConfig{
	ParallelExecution: true,
	OutputFormat:    "text",
	OutputFile:      "diagnostic_report.txt",
}

// LoadFromFile загружает конфигурацию из JSON‑файла
func LoadFromFile(filename string) (AppConfig, error) {
	// Проверяем существование файла
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return Default, fmt.Errorf("файл конфигурации не найден: %s. Используется конфигурация по умолчанию", filename)
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return Default, fmt.Errorf("не удалось прочитать файл конфигурации %s: %w", filename, err)
	}

	var cfg AppConfig
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return Default, fmt.Errorf("ошибка парсинга JSON в файле %s: %w", filename, err)
	}

	// Применяем значения по умолчанию для отсутствующих полей
	if cfg.ParallelExecution == false && !isFieldPresent(data, "parallel_execution") {
		cfg.ParallelExecution = Default.ParallelExecution
	}
	if cfg.OutputFormat == "" && !isFieldPresent(data, "output_format") {
		cfg.OutputFormat = Default.OutputFormat
	}
	if cfg.OutputFile == "" && !isFieldPresent(data, "output_file") {
		cfg.OutputFile = Default.OutputFile
	}

	return cfg, nil
}

// isFieldPresent проверяет, присутствует ли поле в JSON‑данных
func isFieldPresent(jsonData []byte, fieldName string) bool {
	var data map[string]interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return false
	}
	_, exists := data[fieldName]
	return exists
}
