package docker

import "testing"

func TestNormalizeStats(t *testing.T) {
	var s Stats
	s.CPUStats.SystemCPUUsage = 200
	s.PreCPUStats.SystemCPUUsage = 100
	s.CPUStats.CPUUsage.TotalUsage = 150
	s.PreCPUStats.CPUUsage.TotalUsage = 100
	s.CPUStats.OnlineCPUs = 2
	s.MemoryStats.Usage = 123
	s.MemoryStats.Limit = 456
	s.Networks = map[string]struct {
		RxBytes uint64 `json:"rx_bytes"`
		TxBytes uint64 `json:"tx_bytes"`
	}{"eth0": {RxBytes: 10, TxBytes: 20}}
	s.BlkioStats.IoServiceBytesRecursive = []struct {
		Op    string `json:"op"`
		Value uint64 `json:"value"`
	}{{Op: "Read", Value: 7}, {Op: "Write", Value: 8}}

	m := NormalizeStats("abc", s)
	if m.ContainerID != "abc" || m.MemUsedBytes != 123 || m.BlkWriteBytes != 8 {
		t.Fatalf("unexpected normalized data: %+v", m)
	}
}
