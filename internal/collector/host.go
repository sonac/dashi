package collector

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"dashi/internal/models"
)

type HostCollector struct {
	prevCPU *cpuSample
}

type cpuSample struct {
	total uint64
	idle  uint64
}

func NewHostCollector() *HostCollector { return &HostCollector{} }

func (h *HostCollector) Collect() (models.HostMetric, error) {
	total, idle, err := readCPU()
	if err != nil {
		return models.HostMetric{}, err
	}
	metric := models.HostMetric{TS: time.Now().UTC()}
	if h.prevCPU != nil {
		deltaTotal := total - h.prevCPU.total
		deltaIdle := idle - h.prevCPU.idle
		if deltaTotal > 0 {
			metric.CPUPct = 100 * (1 - float64(deltaIdle)/float64(deltaTotal))
		}
	}
	h.prevCPU = &cpuSample{total: total, idle: idle}

	memTotal, memAvail, err := readMem()
	if err == nil && memTotal > 0 {
		metric.MemTotalBytes = int64(memTotal)
		metric.MemUsedBytes = int64(memTotal - memAvail)
	}

	rx, tx, err := readNetDev()
	if err == nil {
		metric.NetRXBytes = int64(rx)
		metric.NetTXBytes = int64(tx)
	}

	totalDisk, usedDisk, err := readDiskUsage("/")
	if err == nil {
		metric.DiskTotalBytes = int64(totalDisk)
		metric.DiskUsedBytes = int64(usedDisk)
	}

	l1, l5, l15, err := readLoadAvg()
	if err == nil {
		metric.Load1, metric.Load5, metric.Load15 = l1, l5, l15
	}
	up, err := readUptimeSec()
	if err == nil {
		metric.UptimeSec = up
	}
	return metric, nil
}

func readCPU() (total, idle uint64, err error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "cpu ") {
			parts := strings.Fields(line)
			if len(parts) < 5 {
				return 0, 0, errors.New("invalid cpu line")
			}
			vals := make([]uint64, 0, len(parts)-1)
			for _, p := range parts[1:] {
				v, e := strconv.ParseUint(p, 10, 64)
				if e != nil {
					return 0, 0, e
				}
				vals = append(vals, v)
				total += v
			}
			idle = vals[3]
			if len(vals) > 4 {
				idle += vals[4]
			}
			return total, idle, nil
		}
	}
	if err := s.Err(); err != nil {
		return 0, 0, err
	}
	return 0, 0, errors.New("cpu line not found")
}

func readMem() (total, available uint64, err error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) < 2 {
			continue
		}
		if fields[0] == "MemTotal:" {
			total, _ = strconv.ParseUint(fields[1], 10, 64)
			total *= 1024
		}
		if fields[0] == "MemAvailable:" {
			available, _ = strconv.ParseUint(fields[1], 10, 64)
			available *= 1024
		}
	}
	if total == 0 {
		return 0, 0, errors.New("meminfo parse failed")
	}
	return total, available, nil
}

func readNetDev() (rx, tx uint64, err error) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		iface := strings.TrimSpace(parts[0])
		if iface == "lo" {
			continue
		}
		vals := strings.Fields(parts[1])
		if len(vals) < 16 {
			continue
		}
		r, _ := strconv.ParseUint(vals[0], 10, 64)
		t, _ := strconv.ParseUint(vals[8], 10, 64)
		rx += r
		tx += t
	}
	return rx, tx, s.Err()
}

func readDiskUsage(path string) (total, used uint64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	total = st.Blocks * uint64(st.Bsize)
	free := st.Bavail * uint64(st.Bsize)
	used = total - free
	return total, used, nil
}

func readLoadAvg() (float64, float64, float64, error) {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0, err
	}
	parts := strings.Fields(string(b))
	if len(parts) < 3 {
		return 0, 0, 0, fmt.Errorf("invalid loadavg")
	}
	l1, _ := strconv.ParseFloat(parts[0], 64)
	l5, _ := strconv.ParseFloat(parts[1], 64)
	l15, _ := strconv.ParseFloat(parts[2], 64)
	return l1, l5, l15, nil
}

func readUptimeSec() (int64, error) {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	parts := strings.Fields(string(b))
	if len(parts) == 0 {
		return 0, fmt.Errorf("invalid uptime")
	}
	f, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, err
	}
	return int64(f), nil
}
