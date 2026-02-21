package logs

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"dashi/internal/db"
	"dashi/internal/docker"
	"dashi/internal/models"
)

type Ingestor struct {
	repo *db.Repository
	dc   *docker.Client
	log  *slog.Logger

	mu      sync.Mutex
	workers map[string]context.CancelFunc
}

func NewIngestor(repo *db.Repository, dc *docker.Client, logger *slog.Logger) *Ingestor {
	return &Ingestor{repo: repo, dc: dc, log: logger, workers: map[string]context.CancelFunc{}}
}

func (i *Ingestor) Reconcile(ctx context.Context) {
	containers, err := i.dc.ListContainers(ctx)
	if err != nil {
		i.log.Warn("log reconcile list containers", "err", err)
		return
	}
	live := map[string]bool{}
	for _, c := range containers {
		live[c.ID] = true
		i.ensureWorker(ctx, c.ID, inferServiceName(c))
	}
	i.mu.Lock()
	for id, cancel := range i.workers {
		if !live[id] {
			cancel()
			delete(i.workers, id)
		}
	}
	i.mu.Unlock()
}

func (i *Ingestor) ensureWorker(parent context.Context, containerID, serviceID string) {
	i.mu.Lock()
	if _, ok := i.workers[containerID]; ok {
		i.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	i.workers[containerID] = cancel
	i.mu.Unlock()

	go i.runWorker(ctx, containerID, serviceID)
}

func (i *Ingestor) runWorker(ctx context.Context, containerID, serviceID string) {
	i.log.Info("start log worker", "container", containerID)
	defer i.log.Info("stop log worker", "container", containerID)
	entriesCh := make(chan models.LogEntry, 256)
	go i.flushLoop(ctx, entriesCh)

	since := time.Now().Add(-1 * time.Minute)
	first := true
	for {
		select {
		case <-ctx.Done():
			close(entriesCh)
			return
		default:
		}
		tail := 0
		if first {
			// Bootstrap initial UI visibility with recent history, then switch to incremental follow.
			since = time.Time{}
			tail = 500
			first = false
		}
		rc, err := i.dc.Logs(ctx, containerID, since, true, tail)
		if err != nil {
			i.log.Warn("open docker logs", "container", containerID, "err", err)
			time.Sleep(2 * time.Second)
			continue
		}
		err = ParseDockerStream(rc, serviceID, containerID, entriesCh)
		_ = rc.Close()
		if err != nil {
			i.log.Warn("parse docker stream", "container", containerID, "err", err)
			time.Sleep(1 * time.Second)
		}
		since = time.Now().Add(-30 * time.Second)
	}
}

func (i *Ingestor) flushLoop(ctx context.Context, in <-chan models.LogEntry) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	batch := make([]models.LogEntry, 0, 200)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := i.repo.InsertLogs(ctx, batch); err != nil {
			i.log.Error("insert logs", "err", err, "count", len(batch))
		}
		batch = batch[:0]
	}
	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case e, ok := <-in:
			if !ok {
				flush()
				return
			}
			batch = append(batch, e)
			if len(batch) >= 200 {
				flush()
			}
		case <-t.C:
			flush()
		}
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
