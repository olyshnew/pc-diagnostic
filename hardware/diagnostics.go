package hardware

import "sync"

// RunDiagnostics запускает все диагностики оборудования параллельно через горутины
func RunDiagnostics() []DiagnosticResult {
	collectors := []func() DiagnosticResult{
		DiagnoseDisk,
		DiagnoseMemory,
		DiagnoseNetwork,
		GetCPUInfo,
		DiagnoseGPU,
		DiagnoseTemperature,
	}

	results := make([]DiagnosticResult, len(collectors))
	var wg sync.WaitGroup

	for i, c := range collectors {
		wg.Add(1)
		go func(idx int, fn func() DiagnosticResult) {
			defer wg.Done()
			results[idx] = fn()
		}(i, c)
	}

	wg.Wait()
	return results
}

