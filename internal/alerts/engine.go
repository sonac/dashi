package alerts

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"dashi/internal/db"
	"dashi/internal/models"
	"dashi/internal/notifier"
)

type Engine struct {
	repo     *db.Repository
	notify   *notifier.Telegram
	log      *slog.Logger
	now      func() time.Time
	lastHost map[string]float64
	lastRest map[string]int
	lastSvc  map[string]string
	debug    bool
}

func NewEngine(repo *db.Repository, notify *notifier.Telegram, logger *slog.Logger, debugRestartAlerts bool) *Engine {
	return &Engine{repo: repo, notify: notify, log: logger, now: time.Now, lastHost: map[string]float64{}, lastRest: map[string]int{}, lastSvc: map[string]string{}, debug: debugRestartAlerts}
}

func (e *Engine) Evaluate(ctx context.Context) {
	rules, err := e.repo.ListRules(ctx)
	if err != nil {
		e.log.Error("load rules", "err", err)
		return
	}
	latest, err := e.repo.LatestHostMetric(ctx)
	if err == nil {
		e.lastHost["host_cpu_pct"] = latest.CPUPct
		if latest.MemTotalBytes > 0 {
			e.lastHost["host_mem_pct"] = (float64(latest.MemUsedBytes) / float64(latest.MemTotalBytes)) * 100
		}
		if latest.DiskTotalBytes > 0 {
			e.lastHost["host_disk_pct"] = (float64(latest.DiskUsedBytes) / float64(latest.DiskTotalBytes)) * 100
		}
	}
	containers, _ := e.repo.ListContainers(ctx)

	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		switch r.TargetType {
		case "host":
			e.evalTarget(ctx, r.ID, "host", "host", r, e.lastHost[r.MetricKey])
		case "container":
			if r.MetricKey == "container_unavailable" {
				now := e.now().UTC()
				for _, c := range containers {
					v := 0.0
					if strings.EqualFold(c.Status, "running") && now.Sub(c.LastSeenAt) > 60*time.Second {
						v = 1
					}
					e.evalTarget(ctx, r.ID, c.ID, shortTarget(c.ID), r, v)
				}
			}
			if r.MetricKey == "container_restarts" {
				for _, c := range containers {
					prev, seen := e.lastRest[c.ID]
					restarted := 0.0
					if seen && c.RestartCount > prev {
						restarted = 1
					}
					reason := "counter"
					if prevID, ok := e.lastSvc[c.ServiceID]; ok && prevID != c.ID {
						restarted = 1
						reason = "service_container_changed"
					}
					e.lastSvc[c.ServiceID] = c.ID
					e.lastRest[c.ID] = c.RestartCount
					if e.debug {
						e.log.Info("restart eval",
							"service", c.ServiceID,
							"container", shortTarget(c.ID),
							"status", c.Status,
							"restart_count", c.RestartCount,
							"prev_restart_count", prev,
							"seen_before", seen,
							"triggered", restarted == 1,
							"reason", reason,
						)
					}
					e.evalTarget(ctx, r.ID, c.ID, shortTarget(c.ID), r, restarted)
				}
			}
		}
	}
}

func (e *Engine) evalTarget(ctx context.Context, ruleID int64, targetKey, targetLabel string, rule models.AlertRule, value float64) {
	if math.IsNaN(value) {
		return
	}
	shouldFire := compare(value, rule.Operator, rule.Threshold)
	now := e.now().UTC()
	state, since, lastFired, _, err := e.repo.GetAlertState(ctx, ruleID, targetKey)
	if err != nil && err != sql.ErrNoRows {
		e.log.Error("get alert state", "err", err, "rule_id", ruleID)
		return
	}
	if err == sql.ErrNoRows {
		state = "OK"
		since = now
	}

	if shouldFire {
		if state == "OK" {
			if rule.ForSeconds <= 0 {
				if lastFired != nil && now.Sub(*lastFired) < time.Duration(rule.CooldownSeconds)*time.Second {
					_ = e.repo.UpsertAlertState(ctx, ruleID, targetKey, "COOLDOWN", now, lastFired, nil)
					return
				}
				msg := fmt.Sprintf("ALERT %s [%s] value=%.2f threshold %s %.2f", rule.Name, targetLabel, value, rule.Operator, rule.Threshold)
				alertID, cErr := e.repo.CreateAlert(ctx, ruleID, targetKey, "firing", msg, map[string]any{"value": value, "target": targetLabel}, now)
				if cErr == nil {
					e.sendNotification(ctx, alertID, msg)
				}
				_ = e.repo.UpsertAlertState(ctx, ruleID, targetKey, "FIRING", now, &now, nil)
				return
			}
			_ = e.repo.UpsertAlertState(ctx, ruleID, targetKey, "PENDING", now, lastFired, nil)
			return
		}
		if state == "PENDING" && now.Sub(since) >= time.Duration(rule.ForSeconds)*time.Second {
			if lastFired != nil && now.Sub(*lastFired) < time.Duration(rule.CooldownSeconds)*time.Second {
				_ = e.repo.UpsertAlertState(ctx, ruleID, targetKey, "COOLDOWN", now, lastFired, nil)
				return
			}
			msg := fmt.Sprintf("ALERT %s [%s] value=%.2f threshold %s %.2f", rule.Name, targetLabel, value, rule.Operator, rule.Threshold)
			alertID, cErr := e.repo.CreateAlert(ctx, ruleID, targetKey, "firing", msg, map[string]any{"value": value, "target": targetLabel}, now)
			if cErr == nil {
				e.sendNotification(ctx, alertID, msg)
			}
			_ = e.repo.UpsertAlertState(ctx, ruleID, targetKey, "FIRING", since, &now, nil)
			return
		}
		return
	}

	if state == "FIRING" || state == "PENDING" || state == "COOLDOWN" {
		_ = e.repo.CloseAlert(ctx, ruleID, targetKey, now)
		rmsg := fmt.Sprintf("RECOVERY %s [%s] value=%.2f", rule.Name, targetLabel, value)
		if state == "FIRING" {
			e.sendNotification(ctx, 0, rmsg)
		}
		_ = e.repo.UpsertAlertState(ctx, ruleID, targetKey, "OK", now, lastFired, &now)
	}
}

func (e *Engine) sendNotification(ctx context.Context, alertID int64, msg string) {
	attempts := 0
	var err error
	for attempts < 3 {
		attempts++
		err = e.notify.Send(ctx, msg)
		if err == nil {
			now := e.now().UTC()
			_ = e.repo.InsertNotificationEvent(ctx, alertID, "telegram", "sent", attempts, "", &now)
			return
		}
		time.Sleep(time.Duration(attempts) * 300 * time.Millisecond)
	}
	_ = e.repo.InsertNotificationEvent(ctx, alertID, "telegram", "failed", attempts, err.Error(), nil)
	e.log.Warn("notify failed", "err", err)
}

func compare(v float64, op string, threshold float64) bool {
	switch op {
	case ">":
		return v > threshold
	case ">=":
		return v >= threshold
	case "<":
		return v < threshold
	case "<=":
		return v <= threshold
	case "==":
		return v == threshold
	default:
		return false
	}
}

func shortTarget(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
