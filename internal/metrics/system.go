package metrics

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SystemMetrics holds current system metrics
type SystemMetrics struct {
	Timestamp time.Time `json:"timestamp"`

	// CPU
	CPUUsagePercent float64 `json:"cpu_usage_percent"`
	CPUIdle         float64 `json:"cpu_idle"`
	CPUIOwait       float64 `json:"cpu_iowait"`

	// Memory
	MemTotalBytes     uint64  `json:"mem_total_bytes"`
	MemUsedBytes      uint64  `json:"mem_used_bytes"`
	MemFreeBytes      uint64  `json:"mem_free_bytes"`
	MemAvailableBytes uint64  `json:"mem_available_bytes"`
	MemUsagePercent   float64 `json:"mem_usage_percent"`
	SwapTotalBytes    uint64  `json:"swap_total_bytes"`
	SwapUsedBytes     uint64  `json:"swap_used_bytes"`

	// Disk
	DiskReadBytes    uint64  `json:"disk_read_bytes"`
	DiskWriteBytes   uint64  `json:"disk_write_bytes"`
	DiskReadPerSec   float64 `json:"disk_read_per_sec"`
	DiskWritePerSec  float64 `json:"disk_write_per_sec"`

	// Network
	NetRxBytes    uint64  `json:"net_rx_bytes"`
	NetTxBytes    uint64  `json:"net_tx_bytes"`
	NetRxPerSec   float64 `json:"net_rx_per_sec"`
	NetTxPerSec   float64 `json:"net_tx_per_sec"`

	// Load
	Load1  float64 `json:"load_1"`
	Load5  float64 `json:"load_5"`
	Load15 float64 `json:"load_15"`
}

// Collector collects system metrics
type Collector struct {
	mu sync.RWMutex

	current  SystemMetrics
	previous struct {
		cpuTotal, cpuIdle uint64
		diskRead, diskWrite uint64
		netRx, netTx uint64
		timestamp time.Time
	}
}

// NewCollector creates a new system metrics collector
func NewCollector() *Collector {
	c := &Collector{}
	// Initialize previous values
	c.previous.timestamp = time.Now()
	c.readCPU()
	c.readDisk()
	c.readNetwork()
	return c
}

// Collect gathers current system metrics
func (c *Collector) Collect() SystemMetrics {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(c.previous.timestamp).Seconds()
	if elapsed < 0.1 {
		elapsed = 0.1 // Avoid division by zero
	}

	c.current.Timestamp = now

	// CPU
	c.readCPU()

	// Memory
	c.readMemory()

	// Disk
	prevDiskRead, prevDiskWrite := c.previous.diskRead, c.previous.diskWrite
	c.readDisk()
	if prevDiskRead > 0 {
		c.current.DiskReadPerSec = float64(c.current.DiskReadBytes-prevDiskRead) / elapsed
		c.current.DiskWritePerSec = float64(c.current.DiskWriteBytes-prevDiskWrite) / elapsed
	}

	// Network
	prevNetRx, prevNetTx := c.previous.netRx, c.previous.netTx
	c.readNetwork()
	if prevNetRx > 0 {
		c.current.NetRxPerSec = float64(c.current.NetRxBytes-prevNetRx) / elapsed
		c.current.NetTxPerSec = float64(c.current.NetTxBytes-prevNetTx) / elapsed
	}

	// Load average
	c.readLoadAvg()

	c.previous.timestamp = now

	return c.current
}

// Get returns the most recent metrics without collecting new ones
func (c *Collector) Get() SystemMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.current
}

func (c *Collector) readCPU() {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) < 8 {
				continue
			}

			user, _ := strconv.ParseUint(fields[1], 10, 64)
			nice, _ := strconv.ParseUint(fields[2], 10, 64)
			system, _ := strconv.ParseUint(fields[3], 10, 64)
			idle, _ := strconv.ParseUint(fields[4], 10, 64)
			iowait, _ := strconv.ParseUint(fields[5], 10, 64)
			irq, _ := strconv.ParseUint(fields[6], 10, 64)
			softirq, _ := strconv.ParseUint(fields[7], 10, 64)

			total := user + nice + system + idle + iowait + irq + softirq
			idleTotal := idle + iowait

			if c.previous.cpuTotal > 0 {
				totalDelta := float64(total - c.previous.cpuTotal)
				idleDelta := float64(idleTotal - c.previous.cpuIdle)

				if totalDelta > 0 {
					c.current.CPUUsagePercent = 100 * (1 - idleDelta/totalDelta)
					c.current.CPUIdle = 100 * idleDelta / totalDelta
					c.current.CPUIOwait = 100 * float64(iowait) / totalDelta
				}
			}

			c.previous.cpuTotal = total
			c.previous.cpuIdle = idleTotal
			break
		}
	}
}

func (c *Collector) readMemory() {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return
	}
	defer f.Close()

	var memTotal, memFree, memAvailable, buffers, cached, swapTotal, swapFree uint64

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		value, _ := strconv.ParseUint(fields[1], 10, 64)
		value *= 1024 // Convert from KB to bytes

		switch fields[0] {
		case "MemTotal:":
			memTotal = value
		case "MemFree:":
			memFree = value
		case "MemAvailable:":
			memAvailable = value
		case "Buffers:":
			buffers = value
		case "Cached:":
			cached = value
		case "SwapTotal:":
			swapTotal = value
		case "SwapFree:":
			swapFree = value
		}
	}

	c.current.MemTotalBytes = memTotal
	c.current.MemFreeBytes = memFree
	c.current.MemAvailableBytes = memAvailable
	c.current.MemUsedBytes = memTotal - memFree - buffers - cached
	if memTotal > 0 {
		c.current.MemUsagePercent = 100 * float64(c.current.MemUsedBytes) / float64(memTotal)
	}
	c.current.SwapTotalBytes = swapTotal
	c.current.SwapUsedBytes = swapTotal - swapFree
}

func (c *Collector) readDisk() {
	f, err := os.Open("/proc/diskstats")
	if err != nil {
		return
	}
	defer f.Close()

	var totalRead, totalWrite uint64

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 14 {
			continue
		}

		// Only count real disks (sda, vda, nvme0n1, etc), not partitions
		name := fields[2]
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") {
			continue
		}
		// Skip partitions (e.g., sda1, nvme0n1p1)
		if len(name) > 0 {
			lastChar := name[len(name)-1]
			if lastChar >= '0' && lastChar <= '9' {
				// Check if it's a partition
				if strings.Contains(name, "sd") || strings.Contains(name, "vd") || strings.Contains(name, "hd") {
					if len(name) > 3 {
						continue
					}
				}
			}
		}

		// Fields: sectors read (5), sectors written (9)
		// Each sector is 512 bytes
		sectorsRead, _ := strconv.ParseUint(fields[5], 10, 64)
		sectorsWrite, _ := strconv.ParseUint(fields[9], 10, 64)

		totalRead += sectorsRead * 512
		totalWrite += sectorsWrite * 512
	}

	c.previous.diskRead = c.current.DiskReadBytes
	c.previous.diskWrite = c.current.DiskWriteBytes
	c.current.DiskReadBytes = totalRead
	c.current.DiskWriteBytes = totalWrite
}

func (c *Collector) readNetwork() {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return
	}
	defer f.Close()

	var totalRx, totalTx uint64

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum <= 2 {
			continue // Skip header lines
		}

		line := scanner.Text()
		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}

		iface := strings.TrimSpace(line[:colonIdx])
		// Skip loopback
		if iface == "lo" {
			continue
		}

		fields := strings.Fields(line[colonIdx+1:])
		if len(fields) < 10 {
			continue
		}

		rxBytes, _ := strconv.ParseUint(fields[0], 10, 64)
		txBytes, _ := strconv.ParseUint(fields[8], 10, 64)

		totalRx += rxBytes
		totalTx += txBytes
	}

	c.previous.netRx = c.current.NetRxBytes
	c.previous.netTx = c.current.NetTxBytes
	c.current.NetRxBytes = totalRx
	c.current.NetTxBytes = totalTx
}

func (c *Collector) readLoadAvg() {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return
	}

	fields := strings.Fields(string(data))
	if len(fields) >= 3 {
		c.current.Load1, _ = strconv.ParseFloat(fields[0], 64)
		c.current.Load5, _ = strconv.ParseFloat(fields[1], 64)
		c.current.Load15, _ = strconv.ParseFloat(fields[2], 64)
	}
}

// FormatBytes formats bytes to human readable string
func FormatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
