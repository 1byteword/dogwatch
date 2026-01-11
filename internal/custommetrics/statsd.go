package custommetrics

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"
)

// StatsDReceiver listens for StatsD metrics over UDP
type StatsDReceiver struct {
	store    *Store
	conn     *net.UDPConn
	port     int
	stopChan chan struct{}
}

// NewStatsDReceiver creates a new StatsD receiver
func NewStatsDReceiver(store *Store, port int) *StatsDReceiver {
	return &StatsDReceiver{
		store:    store,
		port:     port,
		stopChan: make(chan struct{}),
	}
}

// Start begins listening for StatsD packets
func (r *StatsDReceiver) Start() error {
	addr := &net.UDPAddr{Port: r.port}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("listen UDP: %w", err)
	}
	r.conn = conn

	go r.listen()
	log.Printf("[statsd] Listening on UDP port %d", r.port)
	return nil
}

// Stop stops the receiver
func (r *StatsDReceiver) Stop() {
	close(r.stopChan)
	if r.conn != nil {
		r.conn.Close()
	}
}

func (r *StatsDReceiver) listen() {
	buf := make([]byte, 65535)

	for {
		select {
		case <-r.stopChan:
			return
		default:
		}

		r.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, _, err := r.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			continue
		}

		// Parse and record metrics
		lines := strings.Split(string(buf[:n]), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			dp, err := parseStatsDLine(line)
			if err != nil {
				continue // Silently ignore malformed lines
			}

			r.store.Record(dp)
		}
	}
}

// parseStatsDLine parses a StatsD protocol line
// Format: metric.name:value|type|@sample_rate|#tag1:value1,tag2:value2
// Examples:
//   page.views:1|c
//   fuel.level:0.5|g
//   song.length:240|h
//   request.latency:320|ms
//   users.online:100|g|#region:us-east,env:prod
func parseStatsDLine(line string) (DataPoint, error) {
	dp := DataPoint{
		Timestamp: time.Now(),
		Tags:      make(map[string]string),
	}

	// Split by | to get parts
	parts := strings.Split(line, "|")
	if len(parts) < 2 {
		return dp, fmt.Errorf("invalid format")
	}

	// Parse name:value
	nameValue := strings.SplitN(parts[0], ":", 2)
	if len(nameValue) != 2 {
		return dp, fmt.Errorf("invalid name:value")
	}

	dp.Name = strings.TrimSpace(nameValue[0])
	value, err := strconv.ParseFloat(strings.TrimSpace(nameValue[1]), 64)
	if err != nil {
		return dp, fmt.Errorf("invalid value: %w", err)
	}
	dp.Value = value

	// Parse type
	typeStr := strings.TrimSpace(parts[1])
	switch typeStr {
	case "c":
		dp.Type = Counter
	case "g":
		dp.Type = Gauge
	case "h", "ms", "d":
		dp.Type = Histogram
	case "s":
		dp.Type = Gauge // Sets treated as gauges
	default:
		dp.Type = Gauge
	}

	// Parse optional parts (sample rate and tags)
	for i := 2; i < len(parts); i++ {
		part := strings.TrimSpace(parts[i])

		// Sample rate: @0.5
		if strings.HasPrefix(part, "@") {
			// We ignore sample rate for now - could use it to scale counters
			continue
		}

		// DogStatsD tags: #tag1:value1,tag2:value2
		if strings.HasPrefix(part, "#") {
			tagsStr := strings.TrimPrefix(part, "#")
			tagPairs := strings.Split(tagsStr, ",")
			for _, pair := range tagPairs {
				kv := strings.SplitN(pair, ":", 2)
				if len(kv) == 2 {
					dp.Tags[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
				} else if len(kv) == 1 {
					dp.Tags[strings.TrimSpace(kv[0])] = "true"
				}
			}
		}
	}

	return dp, nil
}
