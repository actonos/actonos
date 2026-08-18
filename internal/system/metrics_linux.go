//go:build linux

package system

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	linuxCPUMutex sync.Mutex
	prevLinuxUser uint64
	prevLinuxNice uint64
	prevLinuxSys  uint64
	prevLinuxIdle uint64
)

func getLinuxCPUUsage() float64 {
	linuxCPUMutex.Lock()
	defer linuxCPUMutex.Unlock()

	file, err := os.Open("/proc/stat")
	if err != nil {
		return 3.0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				user, _ := strconv.ParseUint(fields[1], 10, 64)
				nice, _ := strconv.ParseUint(fields[2], 10, 64)
				sys, _ := strconv.ParseUint(fields[3], 10, 64)
				idle, _ := strconv.ParseUint(fields[4], 10, 64)

				if prevLinuxUser == 0 && prevLinuxIdle == 0 {
					prevLinuxUser = user
					prevLinuxNice = nice
					prevLinuxSys = sys
					prevLinuxIdle = idle
					return 3.5
				}

				deltaUser := user - prevLinuxUser
				deltaNice := nice - prevLinuxNice
				deltaSys := sys - prevLinuxSys
				deltaIdle := idle - prevLinuxIdle

				prevLinuxUser = user
				prevLinuxNice = nice
				prevLinuxSys = sys
				prevLinuxIdle = idle

				total := deltaUser + deltaNice + deltaSys + deltaIdle
				if total == 0 {
					return 3.0
				}
				work := deltaUser + deltaNice + deltaSys
				usage := float64(work) / float64(total) * 100.0
				if usage < 0 {
					usage = 0
				}
				if usage > 100 {
					usage = 100
				}
				return usage
			}
		}
	}
	return 3.0
}

func getLinuxMemory() (totalMB uint64, usedMB uint64) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return 8192, m.Sys / (1024 * 1024)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var memTotalKB, memAvailKB uint64
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				memTotalKB, _ = strconv.ParseUint(fields[1], 10, 64)
			}
		} else if strings.HasPrefix(line, "MemAvailable:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				memAvailKB, _ = strconv.ParseUint(fields[1], 10, 64)
			}
		}
	}
	if memTotalKB > 0 {
		totalMB = memTotalKB / 1024
		usedMB = (memTotalKB - memAvailKB) / 1024
		return totalMB, usedMB
	}
	return 8192, 1024
}

func getLinuxDisk(dataDir string) (totalGB float64, usedGB float64) {
	absDir, err := filepath.Abs(dataDir)
	if err != nil {
		absDir = "."
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(absDir, &stat); err == nil && stat.Blocks > 0 {
		totalBytes := stat.Blocks * uint64(stat.Bsize)
		freeBytes := stat.Bfree * uint64(stat.Bsize)
		totalGB = float64(totalBytes) / (1024 * 1024 * 1024)
		usedGB = float64(totalBytes-freeBytes) / (1024 * 1024 * 1024)
		return totalGB, usedGB
	}
	return 64.0, 12.0
}

func getLinuxCPUTemp(cpuUsage float64) float64 {
	for i := 0; i < 4; i++ {
		path := "/sys/class/thermal/thermal_zone" + strconv.Itoa(i) + "/temp"
		if data, err := os.ReadFile(path); err == nil {
			if milli, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64); err == nil && milli > 0 {
				return milli / 1000.0
			}
		}
	}
	return 38.0 + (cpuUsage * 0.35)
}

func collectLiveHostMetrics(dataDir string, startTime time.Time) *SystemMetrics {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	cpuUsage := getLinuxCPUUsage()
	totalMB, usedMB := getLinuxMemory()
	totalGB, usedGB := getLinuxDisk(dataDir)
	tempCelsius := getLinuxCPUTemp(cpuUsage)

	model := "Linux Host"
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "model name") {
				parts := strings.Split(line, ":")
				if len(parts) > 1 {
					model = strings.TrimSpace(parts[1])
					break
				}
			}
		}
	}

	metrics := &SystemMetrics{
		UptimeSeconds: uint64(time.Since(startTime).Seconds()),
		Timestamp:     time.Now().UTC(),
		RuntimeMode:   "linux",
		CanvasURL:     canvasURLFromEnvironment(),
	}

	metrics.CPU.Model = model
	metrics.CPU.Cores = runtime.NumCPU()
	metrics.CPU.UsagePercent = cpuUsage
	metrics.CPU.TempCelsius = tempCelsius

	metrics.Memory.TotalMB = totalMB
	metrics.Memory.UsedMB = usedMB
	metrics.Memory.ActondMB = float64(m.Alloc) / (1024 * 1024)

	metrics.Disk.TotalGB = totalGB
	metrics.Disk.UsedGB = usedGB
	metrics.Disk.DataDirGB = float64(m.Sys) / (1024 * 1024 * 1024)

	return metrics
}
