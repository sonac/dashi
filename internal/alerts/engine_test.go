package alerts

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"dashi/internal/db"
	"dashi/internal/models"
	"dashi/internal/notifier"
)

func TestCompare(t *testing.T) {
	cases := []struct {
		v, th float64
		op    string
		want  bool
	}{
		{91, 90, ">", true},
		{90, 90, ">=", true},
		{89, 90, "<", true},
		{90, 90, "==", true},
		{89, 90, ">", false},
	}
	for _, tc := range cases {
		if got := compare(tc.v, tc.op, tc.th); got != tc.want {
			t.Fatalf("compare(%v %s %v) got %v want %v", tc.v, tc.op, tc.th, got, tc.want)
		}
	}
}

func TestEvaluateContainerRestartsFiresOnIncrement(t *testing.T) {
	tmp := t.TempDir()
	sqldb, err := db.Open(tmp + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })
	if err := db.Migrate(sqldb); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	repo := db.NewRepository(sqldb)

	n := notifier.NewTelegram("token", "chat")
	n.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
	})}

	engine := NewEngine(repo, n, slog.New(slog.NewTextHandler(io.Discard, nil)), false)
	now := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)
	engine.now = func() time.Time { return now }

	var restartRule models.AlertRule
	rules, err := repo.ListRules(context.Background())
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	for _, r := range rules {
		if r.MetricKey == "container_restarts" {
			restartRule = r
			break
		}
	}
	if restartRule.ID == 0 {
		t.Fatal("missing container_restarts rule")
	}
	if err := repo.UpdateRuleThresholds(context.Background(), restartRule.ID, 1, 0, 0, true); err != nil {
		t.Fatalf("update restart rule: %v", err)
	}

	containerID := "container-abcdef123456"
	ctx := context.Background()
	if err := repo.UpsertServiceAndContainer(ctx,
		models.Service{ID: "svc", Name: "svc", Image: "img", LabelsJSON: "{}", Status: "running"},
		models.Container{ID: containerID, ServiceID: "svc", Name: "svc", Status: "running", LastSeenAt: now, RestartCount: 0},
	); err != nil {
		t.Fatalf("upsert container baseline: %v", err)
	}

	engine.Evaluate(ctx)
	assertRestartAlertCount(t, repo, 0)

	if err := repo.UpsertServiceAndContainer(ctx,
		models.Service{ID: "svc", Name: "svc", Image: "img", LabelsJSON: "{}", Status: "running"},
		models.Container{ID: containerID, ServiceID: "svc", Name: "svc", Status: "running", LastSeenAt: now, RestartCount: 1},
	); err != nil {
		t.Fatalf("upsert container restarted: %v", err)
	}
	engine.Evaluate(ctx)
	assertRestartAlertCount(t, repo, 1)
}

func TestEvaluateContainerRestartsFiresOnServiceContainerReplacement(t *testing.T) {
	tmp := t.TempDir()
	sqldb, err := db.Open(tmp + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })
	if err := db.Migrate(sqldb); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	repo := db.NewRepository(sqldb)

	n := notifier.NewTelegram("token", "chat")
	n.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
	})}

	engine := NewEngine(repo, n, slog.New(slog.NewTextHandler(io.Discard, nil)), false)
	now := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)
	engine.now = func() time.Time { return now }

	var restartRule models.AlertRule
	rules, err := repo.ListRules(context.Background())
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	for _, r := range rules {
		if r.MetricKey == "container_restarts" {
			restartRule = r
			break
		}
	}
	if restartRule.ID == 0 {
		t.Fatal("missing container_restarts rule")
	}
	if err := repo.UpdateRuleThresholds(context.Background(), restartRule.ID, 1, 0, 0, true); err != nil {
		t.Fatalf("update restart rule: %v", err)
	}

	ctx := context.Background()
	if err := repo.UpsertServiceAndContainer(ctx,
		models.Service{ID: "svc", Name: "svc", Image: "img", LabelsJSON: "{}", Status: "running"},
		models.Container{ID: "container-old", ServiceID: "svc", Name: "svc", Status: "running", LastSeenAt: now, RestartCount: 0},
	); err != nil {
		t.Fatalf("upsert baseline container: %v", err)
	}

	engine.Evaluate(ctx)
	assertRestartAlertCount(t, repo, 0)

	if err := repo.UpsertServiceAndContainer(ctx,
		models.Service{ID: "svc", Name: "svc", Image: "img", LabelsJSON: "{}", Status: "running"},
		models.Container{ID: "container-new", ServiceID: "svc", Name: "svc", Status: "running", LastSeenAt: now, RestartCount: 0},
	); err != nil {
		t.Fatalf("upsert replaced container: %v", err)
	}

	engine.Evaluate(ctx)
	assertRestartAlertCount(t, repo, 1)
}

func assertRestartAlertCount(t *testing.T, repo *db.Repository, want int) {
	t.Helper()
	var got int
	err := repo.DB().QueryRow(`SELECT COUNT(*) FROM alerts a JOIN alert_rules r ON r.id=a.rule_id WHERE r.metric_key='container_restarts'`).Scan(&got)
	if err != nil {
		t.Fatalf("count restart alerts: %v", err)
	}
	if got != want {
		t.Fatalf("restart alerts count = %d, want %d", got, want)
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
