package docker

import "dashi/internal/models"

func NormalizeStats(id string, s Stats) models.ContainerMetric {
	var cpuPct float64
	sysDelta := float64(s.CPUStats.SystemCPUUsage - s.PreCPUStats.SystemCPUUsage)
	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage - s.PreCPUStats.CPUUsage.TotalUsage)
	cpus := float64(s.CPUStats.OnlineCPUs)
	if cpus == 0 {
		cpus = float64(len(s.CPUStats.CPUUsage.PercpuUsage))
		if cpus == 0 {
			cpus = 1
		}
	}
	if sysDelta > 0 && cpuDelta >= 0 {
		cpuPct = (cpuDelta / sysDelta) * cpus * 100
	}

	var rx, tx, br, bw uint64
	for _, n := range s.Networks {
		rx += n.RxBytes
		tx += n.TxBytes
	}
	for _, io := range s.BlkioStats.IoServiceBytesRecursive {
		switch io.Op {
		case "Read":
			br += io.Value
		case "Write":
			bw += io.Value
		}
	}
	return models.ContainerMetric{
		ContainerID:   id,
		CPUPct:        cpuPct,
		MemUsedBytes:  int64(s.MemoryStats.Usage),
		MemLimitBytes: int64(s.MemoryStats.Limit),
		NetRXBytes:    int64(rx),
		NetTXBytes:    int64(tx),
		BlkReadBytes:  int64(br),
		BlkWriteBytes: int64(bw),
	}
}
