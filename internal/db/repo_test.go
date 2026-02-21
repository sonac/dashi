package db

import (
	"context"
	"testing"
	"time"

	"dashi/internal/models"
)

func TestQueryLogsFiltersByStreamLevelAndTime(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	now := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)
	seedContainer(t, repo, ctx, "svc-a", "c1", now)
	seedContainer(t, repo, ctx, "svc-b", "c2", now)

	err := repo.InsertLogs(ctx, []models.LogEntry{
		{TS: now.Add(-10 * time.Minute), ServiceID: "svc-a", ContainerID: "c1", Level: "INFO", Stream: "stdout", Message: "old entry"},
		{TS: now.Add(-2 * time.Minute), ServiceID: "svc-a", ContainerID: "c1", Level: "ERROR", Stream: "stderr", Message: "disk full"},
		{TS: now.Add(-1 * time.Minute), ServiceID: "svc-b", ContainerID: "c2", Level: "ERROR", Stream: "stdout", Message: "other service"},
	})
	if err != nil {
		t.Fatalf("insert logs: %v", err)
	}

	from := now.Add(-5 * time.Minute)
	entries, err := repo.QueryLogs(ctx, "svc-a", "disk", "ERROR", "stderr", &from, nil, 50)
	if err != nil {
		t.Fatalf("query logs: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
	if entries[0].Message != "disk full" {
		t.Fatalf("unexpected message: %q", entries[0].Message)
	}
}

func TestGroupLogsByLevel(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	now := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)
	seedContainer(t, repo, ctx, "svc", "c1", now)

	err := repo.InsertLogs(ctx, []models.LogEntry{
		{TS: now, ServiceID: "svc", ContainerID: "c1", Level: "ERROR", Stream: "stderr", Message: "boom"},
		{TS: now, ServiceID: "svc", ContainerID: "c1", Level: "ERROR", Stream: "stderr", Message: "boom again"},
		{TS: now, ServiceID: "svc", ContainerID: "c1", Level: "WARN", Stream: "stdout", Message: "careful"},
	})
	if err != nil {
		t.Fatalf("insert logs: %v", err)
	}

	groups, err := repo.GroupLogs(ctx, "level", "svc", "", "", "", nil, nil, 10)
	if err != nil {
		t.Fatalf("group logs: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("group len = %d, want 2", len(groups))
	}
	if groups[0]["key"] != "ERROR" || groups[0]["count"] != int64(2) {
		t.Fatalf("first group = %#v, want ERROR count 2", groups[0])
	}
}

func newTestRepo(t *testing.T) *Repository {
	t.Helper()
	sqldb, err := Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })
	if err := Migrate(sqldb); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	return NewRepository(sqldb)
}

func seedContainer(t *testing.T, repo *Repository, ctx context.Context, serviceID, containerID string, at time.Time) {
	t.Helper()
	err := repo.UpsertServiceAndContainer(ctx,
		models.Service{ID: serviceID, Name: serviceID, Image: "img", LabelsJSON: "{}", Status: "running"},
		models.Container{ID: containerID, ServiceID: serviceID, Name: containerID, Status: "running", LastSeenAt: at, RestartCount: 0},
	)
	if err != nil {
		t.Fatalf("seed container %s: %v", containerID, err)
	}
}
