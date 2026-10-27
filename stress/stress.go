// Package stress — стресс-тестирование компонентов ПК.
package stress

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"

	"pc-diagnostic/thresholds"
)

// statusRu переводит статус в русское слово для вывода.
func statusRu(s thresholds.Status) string {
	switch s {
	case thresholds.StatusNormal:
		return "НОРМА"
	case thresholds.StatusWarning:
		return "ВНИМАНИЕ"
	case thresholds.StatusCritical:
		return "КРИТИЧНО"
	}
	return "НЕТ ДАННЫХ"
}

// statusColored возвращает русское название статуса, окрашенное ANSI-цветом
// (зелёный/жёлтый/красный/серый). В веб-отчёте цветовые коды вычищаются
// регуляркой при выводе результата.
func statusColored(s thresholds.Status) string {
	colors := map[thresholds.Status]string{
		thresholds.StatusNormal:   "\033[32m",
		thresholds.StatusWarning:  "\033[33m",
		thresholds.StatusCritical: "\033[31m",
		thresholds.StatusUnknown:  "\033[90m",
	}
	return colors[s] + statusRu(s) + "\033[0m"
}

// tempStr форматирует температуру: -1 означает «нет данных».
func tempStr(t float64) string {
	if t < 0 {
		return "н/д"
	}
	return fmt.Sprintf("%.1f°C", t)
}

// cpuTemperature возвращает температуру CPU (°C) или -1, если недоступна.
func cpuTemperature() float64 {
	if runtime.GOOS == "windows" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command",
			"(Get-CimInstance -Namespace root/wmi -ClassName MSAcpi_ThermalZoneTemperature | Select-Object -First 1).CurrentTemperature").Output()
		if err != nil {
			return -1
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
		if err != nil {
			return -1
		}
		return v/10 - 273.15
	}
	entries, err := os.ReadDir("/sys/class/thermal")
	if err != nil {
		return -1
	}
	best := -1.0
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "thermal_zone") {
			continue
		}
		raw, err := os.ReadFile("/sys/class/thermal/" + e.Name() + "/temp")
		if err != nil {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
		if err != nil {
			continue
		}
		if c := v / 1000; c > best {
			best = c
		}
	}
	return best
}

// stats — статистика за время теста.
type stats struct {
	maxTemp  float64
	sumTemp  float64
	nTemp    int
	overheat bool
	lastLoad float64 // последняя измеренная загрузка CPU, %
	hasLoad  bool    // была ли хотя бы одна успешная выборка загрузки
}

func (s *stats) avg() float64 {
	if s.nTemp == 0 {
		return -1
	}
	return s.sumTemp / float64(s.nTemp)
}

// monitorLoop раз в секунду печатает загрузку, температуру и статус,
// при превышении предела отменяет тест.
func monitorLoop(ctx context.Context, maxTemp float64, st *stats, cancel context.CancelFunc, done chan<- struct{}) {
	defer close(done)
	st.maxTemp = -1
	start := time.Now()
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			temp := cpuTemperature()
			load := -1.0
			if p, err := cpu.Percent(0, false); err == nil && len(p) > 0 {
				load = p[0]
			}
			if temp > 0 {
				st.sumTemp += temp
				st.nTemp++
				if temp > st.maxTemp {
					st.maxTemp = temp
				}
			}
			if load >= 0 {
				st.lastLoad = load
				st.hasLoad = true
				stCPU := thresholds.ForCPU(load)
				fmt.Printf("\r  [%02ds] CPU %.0f%%  темп. %s  CPU: %s   ",
					int(time.Since(start).Seconds()), load, tempStr(temp), statusColored(stCPU))
			} else {
				fmt.Printf("\r  [%02ds] темп. %s   ",
					int(time.Since(start).Seconds()), tempStr(temp))
			}
			if maxTemp > 0 && temp >= maxTemp {
				st.overheat = true
				fmt.Println("\n  !! ПРЕДЕЛ ТЕМПЕРАТУРЫ ДОСТИГНУТ — тест остановлен")
				cancel()
				return
			}
		}
	}
}

// cpuLoadAll грузит заданное число ядер вычислениями до отмены контекста.
func cpuLoadAll(ctx context.Context, cores int, ops *uint64) {
	var wg sync.WaitGroup
	for i := 0; i < cores; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			x := 0.5
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				x = math.Sqrt(x*1.0000001 + 0.0000001)
				x = math.Mod(x*3.7, 2.0)
				_ = math.Sin(x) * math.Cos(x)
				atomic.AddUint64(ops, 1)
			}
		}()
	}
	wg.Wait()
}

// RunCPU — стресс-тест процессора: все ядра, живая строка прогресса,
// контроль температуры, сводка в конце.
func RunCPU(durationSec int, maxTemp float64) string {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return "Стресс-тест CPU: поддерживается на Windows и Linux"
	}
	cores := runtime.NumCPU()
	if maxTemp <= 0 {
		maxTemp = 90
	}
	dur := time.Duration(durationSec) * time.Second
	if dur < 5*time.Second {
		dur = 5 * time.Second
	}

	fmt.Printf("Стресс-тест CPU: %d ядер, %v (аварийная остановка при %s)\n",
		cores, dur, tempStr(maxTemp))
	before := cpuTemperature()

	ctx, cancel := context.WithTimeout(context.Background(), dur)
	defer cancel()

	var st stats
	var ops uint64
	monDone := make(chan struct{})
	go monitorLoop(ctx, maxTemp, &st, cancel, monDone)

	start := time.Now()
	cpuLoadAll(ctx, cores, &ops)
	elapsed := time.Since(start)
	cancel()  // гарантируем остановку монитора
	<-monDone // ждём, пока монитор напечатает свою последнюю строку
	fmt.Println()

	perCore := float64(atomic.LoadUint64(&ops)) / elapsed.Seconds() / float64(cores) / 1e6

	// Статус по данным, измеренным под нагрузкой (последняя выборка монитора),
	// а не по «остывшей» системе после остановки горутин.
	stCPU := thresholds.StatusUnknown
	if st.hasLoad {
		stCPU = thresholds.ForCPU(st.lastLoad)
	}
	overall := thresholds.Worst(stCPU, thresholds.ForTemp(st.maxTemp))

	var b strings.Builder
	b.WriteString("\nРезультаты стресс-теста CPU:\n")
	b.WriteString(fmt.Sprintf("  Ядер нагружено:        %d\n", cores))
	b.WriteString(fmt.Sprintf("  Длительность:          %v\n", elapsed.Round(time.Second)))
	b.WriteString(fmt.Sprintf("  Производительность:    %.1f млн операций/с на ядро\n", perCore))
	b.WriteString(fmt.Sprintf("  Температура до теста:  %s\n", tempStr(before)))
	b.WriteString(fmt.Sprintf("  Под нагрузкой:         средняя %s, макс. %s\n",
		tempStr(st.avg()), tempStr(st.maxTemp)))
	b.WriteString(fmt.Sprintf("  Состояние системы:     %s\n", statusColored(overall)))
	if st.overheat {
		b.WriteString("  ВНИМАНИЕ: тест остановлен по пределу температуры!\n")
	}
	return b.String()
}

// u64Equal сравнивает два среза uint64 поэлементно.
func u64Equal(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// RunRAM — стресс-тест оперативной памяти: многопоточная запись блоками по 8 байт,
// проверка целостности данных, защита от исчерпания свободной памяти.
func RunRAM(durationSec int) string {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return "Стресс-тест RAM: поддерживается на Windows и Linux"
	}
	dur := time.Duration(durationSec) * time.Second
	if dur < 5*time.Second {
		dur = 5 * time.Second
	}

	vm, err := mem.VirtualMemory()
	if err != nil {
		return fmt.Sprintf("Стресс-тест RAM: не удалось получить данные о памяти: %v", err)
	}
	fmt.Printf("Стресс-тест RAM: длительность %v, свободно %.1f ГБ из %.1f ГБ\n",
		dur, float64(vm.Available)/1e9, float64(vm.Total)/1e9)

	var lowMem int32 // 1 — памяти стало критически мало
	var maxUsedPct float64
	ctx, cancel := context.WithTimeout(context.Background(), dur)
	defer cancel()

	monDone := make(chan struct{})
	go func() {
		defer close(monDone)
		start := time.Now()
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				v, err := mem.VirtualMemory()
				if err != nil {
					continue
				}
				usedPct := 0.0
				if v.Total > 0 {
					usedPct = (1 - float64(v.Available)/float64(v.Total)) * 100
					if usedPct > maxUsedPct {
						maxUsedPct = usedPct
					}
				}
				fmt.Printf("\r  [%02ds] доступно: %.1f ГБ  RAM: %s   ",
					int(time.Since(start).Seconds()), float64(v.Available)/1e9,
					statusColored(thresholds.ForRAM(usedPct)))
				if v.Available < 500*1024*1024 {
					atomic.StoreInt32(&lowMem, 1)
					return
				}
			}
		}
	}()

	const chunkMB = 128
	buf := make([]uint64, chunkMB*1024*1024/8) // 128 МБ как массив 8-байтовых ячеек
	workers := runtime.NumCPU()
	segment := len(buf) / workers
	if segment < 1024 {
		segment = len(buf)
		workers = 1
	}
	const checkPerSegment = 512 // сколько ячеек проверяем в каждой части

	// Заполняет буфер по частям: каждый поток пишет свой сегмент.
	fillPattern := func() {
		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			w := w
			wg.Add(1)
			go func() {
				defer wg.Done()
				start := w * segment
				end := start + segment
				if w == workers-1 {
					end = len(buf)
				}
				for i := start; i < end; i++ {
					buf[i] = uint64(i)*0x9E3779B97F4A7C15 + 12345
				}
			}()
		}
		wg.Wait()
	}

	// Эталонные значения для проверки целостности: начало каждого сегмента.
	fillPattern()
	reference := make([]uint64, 0, workers*checkPerSegment)
	for w := 0; w < workers; w++ {
		reference = append(reference, buf[w*segment:w*segment+checkPerSegment]...)
	}

	start := time.Now()
	cycles := 0
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		default:
		}
		if atomic.LoadInt32(&lowMem) == 1 {
			break loop
		}
		fillPattern() // запись 128 МБ всеми потоками
		ok := true
		for w := 0; w < workers && ok; w++ {
			seg := buf[w*segment : w*segment+checkPerSegment]
			ref := reference[w*checkPerSegment : (w+1)*checkPerSegment]
			if !u64Equal(seg, ref) {
				ok = false
			}
		}
		if !ok {
			break loop
		}
		cycles++
	}
	elapsed := time.Since(start)
	cancel()
	<-monDone
	fmt.Println()

	vmAfter, err := mem.VirtualMemory()
	afterStr := "н/д"
	if err == nil {
		afterStr = fmt.Sprintf("%.1f ГБ свободно", float64(vmAfter.Available)/1e9)
	}
	overall := thresholds.StatusUnknown
	if maxUsedPct > 0 {
		overall = thresholds.ForRAM(maxUsedPct)
	}
	bw := float64(cycles) * chunkMB / elapsed.Seconds() // МБ/с записи

	var b strings.Builder
	b.WriteString("\nРезультаты стресс-теста RAM:\n")
	b.WriteString(fmt.Sprintf("  Длительность:        %v\n", elapsed.Round(time.Second)))
	b.WriteString(fmt.Sprintf("  Потоков записи:      %d (блоками по 8 байт)\n", workers))
	b.WriteString(fmt.Sprintf("  Полных циклов:       %d (запись 128 МБ + проверка)\n", cycles))
	b.WriteString(fmt.Sprintf("  Пропускная способность записи: %.0f МБ/с\n", bw))
	b.WriteString(fmt.Sprintf("  Свободная память до:   %.1f ГБ из %.1f ГБ\n", float64(vm.Available)/1e9, float64(vm.Total)/1e9))
	b.WriteString(fmt.Sprintf("  Свободная память после: %s\n", afterStr))
	b.WriteString(fmt.Sprintf("  Состояние системы:     %s\n", statusColored(overall)))
	if atomic.LoadInt32(&lowMem) == 1 {
		b.WriteString("  ВНИМАНИЕ: тест остановлен — свободной памяти стало слишком мало.\n")
	}
	return b.String()
}

// RunDisk — стресс-тест диска: запись файла-жертвы блоками,
// живой счётчик скорости, средняя скорость за тест, автопомойка.
func RunDisk(durationSec int) string {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return "Стресс-тест диска: поддерживается на Windows и Linux"
	}
	dur := time.Duration(durationSec) * time.Second
	if dur < 5*time.Second {
		dur = 5 * time.Second
	}

	path := filepath.Join(os.TempDir(), "pc-diagnostic-stress.tmp")
	fmt.Printf("Стресс-тест диска: длительность %v, файл: %s\n", dur, path)

	f, err := os.Create(path)
	if err != nil {
		return fmt.Sprintf("Стресс-тест диска: не удалось создать файл: %v", err)
	}
	// Уборка гарантирована при любом исходе: файл закрывается и удаляется.
	defer func() {
		f.Close()
		os.Remove(path)
	}()

	const chunkMB = 4
	chunk := make([]byte, chunkMB*1024*1024) // 4 МБ
	for i := range chunk {
		chunk[i] = byte(i * 31)
	}

	var totalWritten uint64 // МБ

	ctx, cancel := context.WithTimeout(context.Background(), dur)
	defer cancel()

	monDone := make(chan struct{})
	go func() {
		defer close(monDone)
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		var lastBytes uint64
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				b := atomic.LoadUint64(&totalWritten)
				fmt.Printf("\r  скорость: %6.0f МБ/с   ", float64(b-lastBytes))
				lastBytes = b
			}
		}
	}()

	start := time.Now()
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		default:
		}
		if _, err := f.Write(chunk); err != nil {
			cancel()
			<-monDone
			fmt.Println()
			return fmt.Sprintf("Стресс-тест диска: ошибка записи: %v", err)
		}
		atomic.AddUint64(&totalWritten, chunkMB)
	}
	elapsed := time.Since(start)
	cancel()
	<-monDone
	fmt.Println()

	totalMB := atomic.LoadUint64(&totalWritten)
	bw := float64(totalMB) / elapsed.Seconds()

	var b strings.Builder
	b.WriteString("\nРезультаты стресс-теста диска:\n")
	b.WriteString(fmt.Sprintf("  Длительность:         %v\n", elapsed.Round(time.Second)))
	b.WriteString(fmt.Sprintf("  Записано:             %.1f ГБ\n", float64(totalMB)/1024))
	b.WriteString(fmt.Sprintf("  Средняя скорость записи: %.0f МБ/с\n", bw))
	b.WriteString("  Файл-жертва удалён.\n")
	return b.String()
}

// RunCombined — комбинированный стресс-тест: нагрузка на все ядра CPU
// и запись RAM работают одновременно. Показывает, как компоненты
// влияют друг на друга при совместной нагрузке.
func RunCombined(durationSec int, maxTemp float64) string {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return "Комбинированный тест: поддерживается на Windows и Linux"
	}
	cores := runtime.NumCPU()
	if maxTemp <= 0 {
		maxTemp = 90
	}
	dur := time.Duration(durationSec) * time.Second
	if dur < 5*time.Second {
		dur = 5 * time.Second
	}

	vm0, err := mem.VirtualMemory()
	if err != nil {
		return fmt.Sprintf("Комбинированный тест: не удалось получить данные о памяти: %v", err)
	}
	fmt.Printf("Комбинированный стресс-тест: %d ядер CPU + запись RAM, %v (аварийная остановка при %s)\n",
		cores, dur, tempStr(maxTemp))
	fmt.Printf("Свободно памяти: %.1f ГБ из %.1f ГБ\n",
		float64(vm0.Available)/1e9, float64(vm0.Total)/1e9)

	before := cpuTemperature()

	ctx, cancel := context.WithTimeout(context.Background(), dur)
	defer cancel()

	var cpuOps uint64
	var ramMB uint64
	var ramCycles uint64
	var overheat int32

	// Нагрузка CPU: все ядра, как в RunCPU.
	go cpuLoadAll(ctx, cores, &cpuOps)

	// Нагрузка RAM: потоки пишут и проверяют буфер 128 МБ в цикле, как в RunRAM.
	ramDone := make(chan struct{})
	go func() {
		defer close(ramDone)
		const chunkMB = 128
		buf := make([]uint64, chunkMB*1024*1024/8)
		workers := runtime.NumCPU()
		segment := len(buf) / workers
		if segment < 1024 {
			segment = len(buf)
			workers = 1
		}
		fillOnce := func() {
			var wg sync.WaitGroup
			for w := 0; w < workers; w++ {
				w := w
				wg.Add(1)
				go func() {
					defer wg.Done()
					lo := w * segment
					hi := lo + segment
					if w == workers-1 {
						hi = len(buf)
					}
					for i := lo; i < hi; i++ {
						buf[i] = uint64(i)*0x9E3779B97F4A7C15 + 12345
					}
				}()
			}
			wg.Wait()
		}
		fillOnce()
		ref := make([]uint64, 512)
		copy(ref, buf[:512])
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			fillOnce()
			if !u64Equal(buf[:512], ref) {
				return // целостность данных нарушена — стоп
			}
			atomic.AddUint64(&ramMB, chunkMB)
			atomic.AddUint64(&ramCycles, 1)
		}
	}()

	// Монитор: раз в секунду — загрузка CPU, скорость RAM, память, температура.
	start := time.Now()
	monDone := make(chan struct{})
	var maxSeen float64
	var maxRamPct float64
	var tempSum float64
	var tempN int
	var cpuSum float64
	var cpuN int
	go func() {
		defer close(monDone)
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		var lastRam uint64
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				ram := atomic.LoadUint64(&ramMB)
				ramSpeed := float64(ram - lastRam)
				lastRam = ram
				pcts, _ := cpu.Percent(0, true)
				cpuAvg := 0.0
				for _, p := range pcts {
					cpuAvg += p
				}
				if len(pcts) > 0 {
					cpuAvg /= float64(len(pcts))
					cpuSum += cpuAvg
					cpuN++
				}
				temp := cpuTemperature()
				if temp > 0 {
					tempSum += temp
					tempN++
					if temp > maxSeen {
						maxSeen = temp
					}
				}
				vm, _ := mem.VirtualMemory()
				avail := 0.0
				if vm != nil {
					avail = float64(vm.Available) / 1e9
					if vm.Total > 0 {
						usedPct := (1 - float64(vm.Available)/float64(vm.Total)) * 100
						if usedPct > maxRamPct {
							maxRamPct = usedPct
						}
					}
				}
				stCPU := thresholds.ForCPU(cpuAvg)
				stRAM := thresholds.ForRAM(maxRamPct)
				fmt.Printf("\r  [%02ds] CPU %3.0f%%  RAM: %4.0f МБ/с  своб. %.1f ГБ  темп %s  %s   ",
					int(time.Since(start).Seconds()), cpuAvg, ramSpeed, avail,
					tempStr(temp), statusColored(thresholds.Worst(stCPU, stRAM)))
				if temp >= maxTemp && temp > 0 {
					atomic.StoreInt32(&overheat, 1)
					fmt.Printf("\r  [%02ds] ПРЕВЫШЕН ПРЕДЕЛ ТЕМПЕРАТУРЫ (%s), остановка   \n",
						int(time.Since(start).Seconds()), tempStr(temp))
					cancel()
					return
				}
			}
		}
	}()

	// Ждём окончания нагрузки (RAM-горутина завершается по таймауту
	// или при нарушении целостности).
	<-ramDone
	elapsed := time.Since(start)
	cancel()
	<-monDone
	fmt.Println()

	perCore := float64(atomic.LoadUint64(&cpuOps)) / elapsed.Seconds() / float64(cores) / 1e6
	ramBw := float64(atomic.LoadUint64(&ramMB)) / elapsed.Seconds()

	// Статус по данным, измеренным под нагрузкой.
	stCPU := thresholds.StatusUnknown
	if cpuN > 0 {
		stCPU = thresholds.ForCPU(cpuSum / float64(cpuN))
	}
	stRAM := thresholds.StatusUnknown
	if maxRamPct > 0 {
		stRAM = thresholds.ForRAM(maxRamPct)
	}
	stTemp := thresholds.StatusUnknown
	if tempN > 0 && maxSeen > 0 {
		stTemp = thresholds.ForTemp(maxSeen)
	}
	overall := thresholds.Worst(stCPU, stRAM, stTemp)

	var b strings.Builder
	b.WriteString("\nРезультаты комбинированного стресс-теста:\n")
	b.WriteString(fmt.Sprintf("  Длительность:           %v\n", elapsed.Round(time.Second)))
	b.WriteString(fmt.Sprintf("  CPU (%d ядер):           %.1f млн операций/с на ядро\n", cores, perCore))
	b.WriteString(fmt.Sprintf("  RAM (запись):           %.0f МБ/с\n", ramBw))
	b.WriteString(fmt.Sprintf("  Полных циклов RAM:      %d\n", atomic.LoadUint64(&ramCycles)))
	if cpuN > 0 {
		b.WriteString(fmt.Sprintf("  Загрузка CPU средняя:   %.0f%%\n", cpuSum/float64(cpuN)))
	}
	b.WriteString(fmt.Sprintf("  Температура до теста:   %s\n", tempStr(before)))
	if tempN > 0 {
		b.WriteString(fmt.Sprintf("  Под нагрузкой:          средняя %s, макс. %s\n",
			tempStr(tempSum/float64(tempN)), tempStr(maxSeen)))
	}
	vmAfter, err := mem.VirtualMemory()
	if err == nil {
		b.WriteString(fmt.Sprintf("  Свободная память после: %.1f ГБ из %.1f ГБ\n",
			float64(vmAfter.Available)/1e9, float64(vmAfter.Total)/1e9))
	}
	b.WriteString(fmt.Sprintf("  Состояние системы:      %s\n", statusColored(overall)))
	if atomic.LoadInt32(&overheat) == 1 {
		b.WriteString("  ВНИМАНИЕ: тест остановлен по пределу температуры!\n")
	}
	return b.String()
}
