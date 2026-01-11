package containers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Container represents a Docker container
type Container struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Image     string            `json:"image"`
	Status    string            `json:"status"`
	State     string            `json:"state"` // running, paused, exited, etc.
	Created   time.Time         `json:"created"`
	StartedAt time.Time         `json:"started_at"`
	Labels    map[string]string `json:"labels"`
	Ports     []PortMapping     `json:"ports"`
}

// PortMapping represents a container port mapping
type PortMapping struct {
	ContainerPort int    `json:"container_port"`
	HostPort      int    `json:"host_port"`
	Protocol      string `json:"protocol"`
	HostIP        string `json:"host_ip"`
}

// ContainerStats represents container resource usage
type ContainerStats struct {
	ContainerID   string    `json:"container_id"`
	ContainerName string    `json:"container_name"`
	Timestamp     time.Time `json:"timestamp"`

	// CPU
	CPUPercent    float64 `json:"cpu_percent"`
	CPUCores      float64 `json:"cpu_cores"`
	CPUSystemNano uint64  `json:"cpu_system_nano"`
	CPUTotalNano  uint64  `json:"cpu_total_nano"`

	// Memory
	MemoryUsage   uint64  `json:"memory_usage"`
	MemoryLimit   uint64  `json:"memory_limit"`
	MemoryPercent float64 `json:"memory_percent"`
	MemoryCache   uint64  `json:"memory_cache"`

	// Network
	NetworkRxBytes   uint64 `json:"network_rx_bytes"`
	NetworkTxBytes   uint64 `json:"network_tx_bytes"`
	NetworkRxPackets uint64 `json:"network_rx_packets"`
	NetworkTxPackets uint64 `json:"network_tx_packets"`

	// Block I/O
	BlockReadBytes  uint64 `json:"block_read_bytes"`
	BlockWriteBytes uint64 `json:"block_write_bytes"`

	// PIDs
	PIDs int `json:"pids"`
}

// Collector collects Docker container metrics
type Collector struct {
	client     *http.Client
	socketPath string
	stopChan   chan struct{}
	wg         sync.WaitGroup

	// Current state
	containers map[string]*Container
	stats      map[string]*ContainerStats
	history    []ContainerStats // Rolling history
	mu         sync.RWMutex

	// Previous CPU readings for calculating percent
	prevCPU map[string]cpuReading
}

type cpuReading struct {
	totalNano  uint64
	systemNano uint64
	timestamp  time.Time
}

// NewCollector creates a new container metrics collector
func NewCollector() *Collector {
	socketPath := "/var/run/docker.sock"

	// Create HTTP client that uses Unix socket
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 10 * time.Second,
	}

	return &Collector{
		client:     client,
		socketPath: socketPath,
		stopChan:   make(chan struct{}),
		containers: make(map[string]*Container),
		stats:      make(map[string]*ContainerStats),
		history:    make([]ContainerStats, 0, 1000),
		prevCPU:    make(map[string]cpuReading),
	}
}

// Start begins collecting container metrics
func (c *Collector) Start() error {
	// Check if Docker is available
	if !c.isDockerAvailable() {
		return fmt.Errorf("Docker socket not available at %s", c.socketPath)
	}

	c.wg.Add(1)
	go c.runLoop()
	log.Println("[containers] Collector started")
	return nil
}

// Stop stops the collector
func (c *Collector) Stop() {
	close(c.stopChan)
	c.wg.Wait()
	log.Println("[containers] Collector stopped")
}

func (c *Collector) isDockerAvailable() bool {
	resp, err := c.doRequest("GET", "/_ping")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (c *Collector) runLoop() {
	defer c.wg.Done()

	// Collect immediately on start
	c.collect()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.collect()
		}
	}
}

func (c *Collector) collect() {
	// List containers
	containers, err := c.listContainers()
	if err != nil {
		log.Printf("[containers] Failed to list containers: %v", err)
		return
	}

	c.mu.Lock()
	c.containers = make(map[string]*Container)
	for _, container := range containers {
		c.containers[container.ID] = container
	}
	c.mu.Unlock()

	// Collect stats for each running container
	for _, container := range containers {
		if container.State == "running" {
			stats, err := c.getContainerStats(container.ID)
			if err != nil {
				log.Printf("[containers] Failed to get stats for %s: %v", container.Name, err)
				continue
			}
			stats.ContainerName = container.Name

			c.mu.Lock()
			c.stats[container.ID] = stats
			// Add to history (keep last 1000 entries)
			c.history = append(c.history, *stats)
			if len(c.history) > 1000 {
				c.history = c.history[len(c.history)-1000:]
			}
			c.mu.Unlock()
		}
	}
}

func (c *Collector) listContainers() ([]*Container, error) {
	resp, err := c.doRequest("GET", "/containers/json?all=true")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var dockerContainers []struct {
		ID      string            `json:"Id"`
		Names   []string          `json:"Names"`
		Image   string            `json:"Image"`
		Status  string            `json:"Status"`
		State   string            `json:"State"`
		Created int64             `json:"Created"`
		Labels  map[string]string `json:"Labels"`
		Ports   []struct {
			PrivatePort int    `json:"PrivatePort"`
			PublicPort  int    `json:"PublicPort"`
			Type        string `json:"Type"`
			IP          string `json:"IP"`
		} `json:"Ports"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&dockerContainers); err != nil {
		return nil, err
	}

	containers := make([]*Container, len(dockerContainers))
	for i, dc := range dockerContainers {
		name := ""
		if len(dc.Names) > 0 {
			name = strings.TrimPrefix(dc.Names[0], "/")
		}

		ports := make([]PortMapping, len(dc.Ports))
		for j, p := range dc.Ports {
			ports[j] = PortMapping{
				ContainerPort: p.PrivatePort,
				HostPort:      p.PublicPort,
				Protocol:      p.Type,
				HostIP:        p.IP,
			}
		}

		containers[i] = &Container{
			ID:      dc.ID[:12], // Short ID
			Name:    name,
			Image:   dc.Image,
			Status:  dc.Status,
			State:   dc.State,
			Created: time.Unix(dc.Created, 0),
			Labels:  dc.Labels,
			Ports:   ports,
		}
	}

	return containers, nil
}

func (c *Collector) getContainerStats(containerID string) (*ContainerStats, error) {
	// Use stream=false to get a single stats reading
	resp, err := c.doRequest("GET", fmt.Sprintf("/containers/%s/stats?stream=false", containerID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var dockerStats struct {
		Read     time.Time `json:"read"`
		CPUStats struct {
			CPUUsage struct {
				TotalUsage uint64 `json:"total_usage"`
			} `json:"cpu_usage"`
			SystemCPUUsage uint64 `json:"system_cpu_usage"`
			OnlineCPUs     int    `json:"online_cpus"`
		} `json:"cpu_stats"`
		PreCPUStats struct {
			CPUUsage struct {
				TotalUsage uint64 `json:"total_usage"`
			} `json:"cpu_usage"`
			SystemCPUUsage uint64 `json:"system_cpu_usage"`
		} `json:"precpu_stats"`
		MemoryStats struct {
			Usage uint64 `json:"usage"`
			Limit uint64 `json:"limit"`
			Stats struct {
				Cache uint64 `json:"cache"`
			} `json:"stats"`
		} `json:"memory_stats"`
		Networks map[string]struct {
			RxBytes   uint64 `json:"rx_bytes"`
			TxBytes   uint64 `json:"tx_bytes"`
			RxPackets uint64 `json:"rx_packets"`
			TxPackets uint64 `json:"tx_packets"`
		} `json:"networks"`
		BlkioStats struct {
			IoServiceBytesRecursive []struct {
				Op    string `json:"op"`
				Value uint64 `json:"value"`
			} `json:"io_service_bytes_recursive"`
		} `json:"blkio_stats"`
		PidsStats struct {
			Current int `json:"current"`
		} `json:"pids_stats"`
	}

	if err := json.Unmarshal(body, &dockerStats); err != nil {
		return nil, err
	}

	stats := &ContainerStats{
		ContainerID: containerID[:12],
		Timestamp:   dockerStats.Read,
	}

	// Calculate CPU percent
	cpuDelta := float64(dockerStats.CPUStats.CPUUsage.TotalUsage - dockerStats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(dockerStats.CPUStats.SystemCPUUsage - dockerStats.PreCPUStats.SystemCPUUsage)
	if systemDelta > 0 && cpuDelta > 0 {
		stats.CPUPercent = (cpuDelta / systemDelta) * float64(dockerStats.CPUStats.OnlineCPUs) * 100.0
	}
	stats.CPUCores = float64(dockerStats.CPUStats.OnlineCPUs)
	stats.CPUTotalNano = dockerStats.CPUStats.CPUUsage.TotalUsage
	stats.CPUSystemNano = dockerStats.CPUStats.SystemCPUUsage

	// Memory
	stats.MemoryUsage = dockerStats.MemoryStats.Usage
	stats.MemoryLimit = dockerStats.MemoryStats.Limit
	if stats.MemoryLimit > 0 {
		stats.MemoryPercent = float64(stats.MemoryUsage) / float64(stats.MemoryLimit) * 100
	}
	stats.MemoryCache = dockerStats.MemoryStats.Stats.Cache

	// Network (sum all interfaces)
	for _, net := range dockerStats.Networks {
		stats.NetworkRxBytes += net.RxBytes
		stats.NetworkTxBytes += net.TxBytes
		stats.NetworkRxPackets += net.RxPackets
		stats.NetworkTxPackets += net.TxPackets
	}

	// Block I/O
	for _, bio := range dockerStats.BlkioStats.IoServiceBytesRecursive {
		switch bio.Op {
		case "Read":
			stats.BlockReadBytes += bio.Value
		case "Write":
			stats.BlockWriteBytes += bio.Value
		}
	}

	// PIDs
	stats.PIDs = dockerStats.PidsStats.Current

	return stats, nil
}

func (c *Collector) doRequest(method, path string) (*http.Response, error) {
	req, err := http.NewRequest(method, "http://localhost"+path, nil)
	if err != nil {
		return nil, err
	}
	return c.client.Do(req)
}

// GetContainers returns all containers
func (c *Collector) GetContainers() []*Container {
	c.mu.RLock()
	defer c.mu.RUnlock()

	containers := make([]*Container, 0, len(c.containers))
	for _, container := range c.containers {
		containers = append(containers, container)
	}
	return containers
}

// GetContainer returns a specific container
func (c *Collector) GetContainer(id string) *Container {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.containers[id]
}

// GetStats returns current stats for all containers
func (c *Collector) GetStats() map[string]*ContainerStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]*ContainerStats)
	for k, v := range c.stats {
		result[k] = v
	}
	return result
}

// GetContainerStats returns stats for a specific container
func (c *Collector) GetContainerStats(containerID string) *ContainerStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats[containerID]
}

// GetHistory returns stats history for a container
func (c *Collector) GetHistory(containerID string, since time.Duration) []ContainerStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cutoff := time.Now().Add(-since)
	var result []ContainerStats

	for _, s := range c.history {
		if s.ContainerID == containerID && s.Timestamp.After(cutoff) {
			result = append(result, s)
		}
	}
	return result
}

// Summary provides an overview of container status
type Summary struct {
	TotalContainers   int     `json:"total_containers"`
	RunningContainers int     `json:"running_containers"`
	StoppedContainers int     `json:"stopped_containers"`
	TotalCPUPercent   float64 `json:"total_cpu_percent"`
	TotalMemoryUsage  uint64  `json:"total_memory_usage"`
	TotalMemoryLimit  uint64  `json:"total_memory_limit"`
}

// GetSummary returns a summary of container metrics
func (c *Collector) GetSummary() Summary {
	c.mu.RLock()
	defer c.mu.RUnlock()

	summary := Summary{}
	summary.TotalContainers = len(c.containers)

	for _, container := range c.containers {
		if container.State == "running" {
			summary.RunningContainers++
		} else {
			summary.StoppedContainers++
		}
	}

	for _, stats := range c.stats {
		summary.TotalCPUPercent += stats.CPUPercent
		summary.TotalMemoryUsage += stats.MemoryUsage
		summary.TotalMemoryLimit += stats.MemoryLimit
	}

	return summary
}
