package collector

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"dashi/internal/db"
	"dashi/internal/docker"
	"dashi/internal/models"
)

type Service struct {
	repo *db.Repository
	dc   *docker.Client
	log  *slog.Logger
	host *HostCollector
}

func NewService(repo *db.Repository, dc *docker.Client, logger *slog.Logger) *Service {
	return &Service{repo: repo, dc: dc, log: logger, host: NewHostCollector()}
}

func (s *Service) Tick(ctx context.Context) {
	hm, err := s.host.Collect()
	if err == nil {
		if err := s.repo.InsertHostMetric(ctx, hm); err != nil {
			s.log.Error("insert host metric", "err", err)
		}
	} else {
		s.log.Warn("collect host metric", "err", err)
	}

	containers, err := s.dc.ListContainers(ctx)
	if err != nil {
		s.log.Warn("list containers", "err", err)
		return
	}
	seen := make([]string, 0, len(containers))
	for _, c := range containers {
		seen = append(seen, c.ID)
		serviceName := inferServiceName(c)
		labelsJSON, _ := json.Marshal(c.Labels)
		svcID := serviceName
		inspect, err := s.dc.InspectContainer(ctx, c.ID)
		if err != nil {
			s.log.Warn("inspect container", "id", c.ID, "err", err)
			continue
		}
		var started *time.Time
		if t, err := time.Parse(time.RFC3339Nano, inspect.State.StartedAt); err == nil {
			t = t.UTC()
			started = &t
		}
		if err := s.repo.UpsertServiceAndContainer(ctx,
			models.Service{ID: svcID, Name: serviceName, Image: c.Image, LabelsJSON: string(labelsJSON), Status: c.State},
			models.Container{ID: c.ID, ServiceID: svcID, Name: strings.TrimPrefix(c.Names[0], "/"), Status: c.State, StartedAt: started, LastSeenAt: time.Now().UTC(), RestartCount: inspect.RestartCount},
		); err != nil {
			s.log.Error("upsert service/container", "id", c.ID, "err", err)
			continue
		}
		stats, err := s.dc.Stats(ctx, c.ID)
		if err != nil {
			s.log.Warn("container stats", "id", c.ID, "err", err)
			continue
		}
		m := docker.NormalizeStats(c.ID, stats)
		m.TS = time.Now().UTC()
		if err := s.repo.InsertContainerMetric(ctx, m); err != nil {
			s.log.Error("insert container metric", "id", c.ID, "err", err)
		}
	}
	if err := s.repo.MarkMissingContainers(ctx, seen); err != nil {
		s.log.Warn("mark missing containers", "err", err)
	}
}

func inferServiceName(c docker.ContainerSummary) string {
	if v := c.Labels["com.docker.compose.service"]; v != "" {
		return v
	}
	if len(c.Names) > 0 {
		return strings.TrimPrefix(c.Names[0], "/")
	}
	if len(c.ID) >= 12 {
		return c.ID[:12]
	}
	return c.ID
}
