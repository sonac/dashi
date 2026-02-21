package models

import "time"

type HostMetric struct {
	TS             time.Time
	CPUPct         float64
	MemUsedBytes   int64
	MemTotalBytes  int64
	NetRXBytes     int64
	NetTXBytes     int64
	DiskUsedBytes  int64
	DiskTotalBytes int64
	Load1          float64
	Load5          float64
	Load15         float64
	UptimeSec      int64
}

type ContainerMetric struct {
	TS            time.Time
	ContainerID   string
	CPUPct        float64
	MemUsedBytes  int64
	MemLimitBytes int64
	NetRXBytes    int64
	NetTXBytes    int64
	BlkReadBytes  int64
	BlkWriteBytes int64
}

type LogEntry struct {
	TS          time.Time
	ServiceID   string
	ContainerID string
	Level       string
	Stream      string
	Message     string
}

type Service struct {
	ID         string
	Name       string
	Image      string
	LabelsJSON string
	Status     string
}

type Container struct {
	ID           string
	ServiceID    string
	Name         string
	Status       string
	StartedAt    *time.Time
	LastSeenAt   time.Time
	RestartCount int
}

type AlertRule struct {
	ID              int64
	Name            string
	TargetType      string
	TargetID        *string
	MetricKey       string
	Operator        string
	Threshold       float64
	ForSeconds      int
	CooldownSeconds int
	Enabled         bool
}
