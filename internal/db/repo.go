package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"dashi/internal/models"
)

type Repository struct {
	db *sql.DB
}

type ActiveAlertTarget struct {
	RuleID int64
	Target string
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) DB() *sql.DB { return r.db }

func (r *Repository) UpsertServiceAndContainer(ctx context.Context, svc models.Service, c models.Container) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `INSERT INTO services (id,name,image,labels_json,first_seen_at,last_seen_at,status)
		VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name,image=excluded.image,labels_json=excluded.labels_json,last_seen_at=excluded.last_seen_at,status=excluded.status`,
		svc.ID, svc.Name, svc.Image, svc.LabelsJSON, now, now, svc.Status)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO containers (id,service_id,name,status,started_at,last_seen_at,restart_count)
		VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET service_id=excluded.service_id,name=excluded.name,status=excluded.status,last_seen_at=excluded.last_seen_at,restart_count=excluded.restart_count`,
		c.ID, c.ServiceID, c.Name, c.Status, c.StartedAt, now, c.RestartCount)
	return err
}

func (r *Repository) MarkMissingContainers(ctx context.Context, seenIDs []string) error {
	if len(seenIDs) == 0 {
		_, err := r.db.ExecContext(ctx, `UPDATE containers SET status='missing' WHERE status!='missing'`)
		return err
	}
	placeholders := make([]string, len(seenIDs))
	args := make([]any, 0, len(seenIDs))
	for i, id := range seenIDs {
		placeholders[i] = "?" + strconv.Itoa(i+1)
		args = append(args, id)
	}
	query := fmt.Sprintf(`UPDATE containers SET status='missing' WHERE id NOT IN (%s) AND status!='missing'`, strings.Join(placeholders, ","))
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

func (r *Repository) InsertHostMetric(ctx context.Context, m models.HostMetric) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO host_metrics
		(ts,cpu_pct,mem_used_bytes,mem_total_bytes,net_rx_bytes,net_tx_bytes,disk_used_bytes,disk_total_bytes,load1,load5,load15,uptime_sec)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.TS.UTC(), m.CPUPct, m.MemUsedBytes, m.MemTotalBytes, m.NetRXBytes, m.NetTXBytes, m.DiskUsedBytes, m.DiskTotalBytes,
		m.Load1, m.Load5, m.Load15, m.UptimeSec)
	return err
}

func (r *Repository) InsertContainerMetric(ctx context.Context, m models.ContainerMetric) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO container_metrics
		(ts,container_id,cpu_pct,mem_used_bytes,mem_limit_bytes,net_rx_bytes,net_tx_bytes,blk_read_bytes,blk_write_bytes)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		m.TS.UTC(), m.ContainerID, m.CPUPct, m.MemUsedBytes, m.MemLimitBytes, m.NetRXBytes, m.NetTXBytes, m.BlkReadBytes, m.BlkWriteBytes)
	return err
}

func (r *Repository) InsertLogs(ctx context.Context, entries []models.LogEntry) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO logs (ts,service_id,container_id,level,stream,message) VALUES (?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, e := range entries {
		if _, err := stmt.ExecContext(ctx, e.TS.UTC(), e.ServiceID, e.ContainerID, e.Level, e.Stream, e.Message); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) LatestHostMetric(ctx context.Context) (models.HostMetric, error) {
	var m models.HostMetric
	err := r.db.QueryRowContext(ctx, `SELECT ts,cpu_pct,mem_used_bytes,mem_total_bytes,net_rx_bytes,net_tx_bytes,disk_used_bytes,disk_total_bytes,load1,load5,load15,uptime_sec FROM host_metrics ORDER BY ts DESC LIMIT 1`).
		Scan(&m.TS, &m.CPUPct, &m.MemUsedBytes, &m.MemTotalBytes, &m.NetRXBytes, &m.NetTXBytes, &m.DiskUsedBytes, &m.DiskTotalBytes, &m.Load1, &m.Load5, &m.Load15, &m.UptimeSec)
	return m, err
}

func (r *Repository) RecentHostMetrics(ctx context.Context, from time.Time, limit int) ([]models.HostMetric, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT ts,cpu_pct,mem_used_bytes,mem_total_bytes,net_rx_bytes,net_tx_bytes,disk_used_bytes,disk_total_bytes,load1,load5,load15,uptime_sec FROM host_metrics WHERE ts >= ? ORDER BY ts ASC LIMIT ?`, from.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.HostMetric, 0, limit)
	for rows.Next() {
		var m models.HostMetric
		if err := rows.Scan(&m.TS, &m.CPUPct, &m.MemUsedBytes, &m.MemTotalBytes, &m.NetRXBytes, &m.NetTXBytes, &m.DiskUsedBytes, &m.DiskTotalBytes, &m.Load1, &m.Load5, &m.Load15, &m.UptimeSec); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *Repository) RecentContainerMetrics(ctx context.Context, containerID string, from time.Time, limit int) ([]models.ContainerMetric, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT ts,container_id,cpu_pct,mem_used_bytes,mem_limit_bytes,net_rx_bytes,net_tx_bytes,blk_read_bytes,blk_write_bytes FROM container_metrics WHERE container_id = ? AND ts >= ? ORDER BY ts ASC LIMIT ?`, containerID, from.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.ContainerMetric, 0, limit)
	for rows.Next() {
		var m models.ContainerMetric
		if err := rows.Scan(&m.TS, &m.ContainerID, &m.CPUPct, &m.MemUsedBytes, &m.MemLimitBytes, &m.NetRXBytes, &m.NetTXBytes, &m.BlkReadBytes, &m.BlkWriteBytes); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *Repository) ListServicesWithHealth(ctx context.Context, minCPU float64, minMemBytes int64, limit int, includeMissing bool) ([]map[string]any, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	missingFilter := ""
	if !includeMissing {
		missingFilter = " AND c.status NOT IN ('missing','exited')"
	}
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`SELECT s.id,s.name,c.status,c.id,c.restart_count,c.last_seen_at,
		COALESCE((SELECT cpu_pct FROM container_metrics cm WHERE cm.container_id=c.id ORDER BY ts DESC LIMIT 1),0),
		COALESCE((SELECT mem_used_bytes FROM container_metrics cm WHERE cm.container_id=c.id ORDER BY ts DESC LIMIT 1),0),
		COALESCE((SELECT MAX(ts) FROM logs l WHERE l.container_id=c.id),'')
		FROM services s JOIN containers c ON c.service_id=s.id
		WHERE (
			COALESCE((SELECT cpu_pct FROM container_metrics cm WHERE cm.container_id=c.id ORDER BY ts DESC LIMIT 1),0) >= ?
			AND COALESCE((SELECT mem_used_bytes FROM container_metrics cm WHERE cm.container_id=c.id ORDER BY ts DESC LIMIT 1),0) >= ?
		)%s
		ORDER BY
			COALESCE((SELECT cpu_pct FROM container_metrics cm WHERE cm.container_id=c.id ORDER BY ts DESC LIMIT 1),0) DESC,
			COALESCE((SELECT mem_used_bytes FROM container_metrics cm WHERE cm.container_id=c.id ORDER BY ts DESC LIMIT 1),0) DESC,
			c.restart_count DESC
		LIMIT ?`, missingFilter), minCPU, minMemBytes, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var svcID, name, status, containerID string
		var restart int
		var lastSeen time.Time
		var cpu float64
		var mem int64
		var lastLog sql.NullString
		if err := rows.Scan(&svcID, &name, &status, &containerID, &restart, &lastSeen, &cpu, &mem, &lastLog); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"service_id":     svcID,
			"name":           name,
			"status":         status,
			"container_id":   containerID,
			"restart_count":  restart,
			"last_seen":      lastSeen,
			"cpu_pct":        cpu,
			"mem_used_bytes": mem,
			"last_log":       lastLog.String,
		})
	}
	return out, rows.Err()
}

func (r *Repository) QueryLogs(ctx context.Context, serviceID, q, level, stream string, from, to *time.Time, limit int) ([]models.LogEntry, error) {
	clauses, args := buildLogFilters(serviceID, q, level, stream, from, to)
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	args = append(args, limit)
	query := fmt.Sprintf(`SELECT ts,service_id,container_id,level,stream,message FROM logs WHERE %s ORDER BY ts DESC LIMIT ?`, strings.Join(clauses, " AND "))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.LogEntry, 0, limit)
	for rows.Next() {
		var e models.LogEntry
		if err := rows.Scan(&e.TS, &e.ServiceID, &e.ContainerID, &e.Level, &e.Stream, &e.Message); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) GroupLogs(ctx context.Context, groupBy, serviceID, q, level, stream string, from, to *time.Time, limit int) ([]map[string]any, error) {
	column := ""
	switch groupBy {
	case "service":
		column = "service_id"
	case "level":
		column = "level"
	case "stream":
		column = "stream"
	default:
		return nil, fmt.Errorf("unsupported group_by: %s", groupBy)
	}

	clauses, args := buildLogFilters(serviceID, q, level, stream, from, to)
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args = append(args, limit)

	query := fmt.Sprintf(`SELECT %s AS group_key, COUNT(*) AS count FROM logs WHERE %s GROUP BY %s ORDER BY count DESC, group_key ASC LIMIT ?`, column, strings.Join(clauses, " AND "), column)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]any, 0, limit)
	for rows.Next() {
		var key string
		var count int64
		if err := rows.Scan(&key, &count); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"key": key, "count": count})
	}
	return out, rows.Err()
}

func buildLogFilters(serviceID, q, level, stream string, from, to *time.Time) ([]string, []any) {
	clauses := []string{"1=1"}
	args := []any{}
	if serviceID != "" {
		clauses = append(clauses, "service_id = ?")
		args = append(args, serviceID)
	}
	if level != "" {
		clauses = append(clauses, "level = ?")
		args = append(args, strings.ToUpper(level))
	}
	if stream != "" {
		clauses = append(clauses, "stream = ?")
		args = append(args, strings.ToLower(stream))
	}
	if q != "" {
		clauses = append(clauses, "message LIKE ?")
		args = append(args, "%"+q+"%")
	}
	if from != nil {
		clauses = append(clauses, "ts >= ?")
		args = append(args, from.UTC())
	}
	if to != nil {
		clauses = append(clauses, "ts <= ?")
		args = append(args, to.UTC())
	}
	return clauses, args
}

func (r *Repository) ListRules(ctx context.Context) ([]models.AlertRule, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,name,target_type,target_id_nullable,metric_key,operator,threshold,for_seconds,cooldown_seconds,enabled FROM alert_rules ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.AlertRule
	for rows.Next() {
		var rule models.AlertRule
		var target sql.NullString
		var enabled int
		if err := rows.Scan(&rule.ID, &rule.Name, &rule.TargetType, &target, &rule.MetricKey, &rule.Operator, &rule.Threshold, &rule.ForSeconds, &rule.CooldownSeconds, &enabled); err != nil {
			return nil, err
		}
		if target.Valid {
			t := target.String
			rule.TargetID = &t
		}
		rule.Enabled = enabled == 1
		out = append(out, rule)
	}
	return out, rows.Err()
}

func (r *Repository) UpsertAlertState(ctx context.Context, ruleID int64, target, state string, since time.Time, lastFired, lastRecovered *time.Time) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO alert_states (rule_id,target_fingerprint,state,since_ts,last_fired_ts,last_recovered_ts)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(rule_id,target_fingerprint) DO UPDATE SET state=excluded.state,since_ts=excluded.since_ts,last_fired_ts=excluded.last_fired_ts,last_recovered_ts=excluded.last_recovered_ts`,
		ruleID, target, state, since.UTC(), lastFired, lastRecovered)
	return err
}

func (r *Repository) GetAlertState(ctx context.Context, ruleID int64, target string) (state string, since time.Time, lastFired, lastRecovered *time.Time, err error) {
	var fired, recovered sql.NullTime
	err = r.db.QueryRowContext(ctx, `SELECT state,since_ts,last_fired_ts,last_recovered_ts FROM alert_states WHERE rule_id=? AND target_fingerprint=?`, ruleID, target).
		Scan(&state, &since, &fired, &recovered)
	if err != nil {
		return "", time.Time{}, nil, nil, err
	}
	if fired.Valid {
		t := fired.Time
		lastFired = &t
	}
	if recovered.Valid {
		t := recovered.Time
		lastRecovered = &t
	}
	return
}

func (r *Repository) CreateAlert(ctx context.Context, ruleID int64, target, status, summary string, details map[string]any, started time.Time) (int64, error) {
	b, _ := json.Marshal(details)
	res, err := r.db.ExecContext(ctx, `INSERT INTO alerts (rule_id,target_fingerprint,status,started_ts,summary,details_json) VALUES (?,?,?,?,?,?)`, ruleID, target, status, started.UTC(), summary, string(b))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *Repository) CloseAlert(ctx context.Context, ruleID int64, target string, ended time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE alerts SET status='recovered', ended_ts_nullable=? WHERE rule_id=? AND target_fingerprint=? AND status='firing'`, ended.UTC(), ruleID, target)
	return err
}

func (r *Repository) RecentAlerts(ctx context.Context, since time.Time, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `SELECT a.id,a.status,a.started_ts,a.ended_ts_nullable,a.summary,r.name
		FROM alerts a JOIN alert_rules r ON r.id=a.rule_id
		WHERE a.started_ts >= ?
		ORDER BY a.started_ts DESC LIMIT ?`, since.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id int64
		var status, summary, ruleName string
		var started time.Time
		var ended sql.NullTime
		if err := rows.Scan(&id, &status, &started, &ended, &summary, &ruleName); err != nil {
			return nil, err
		}
		item := map[string]any{"id": id, "status": status, "started": started, "summary": summary, "rule_name": ruleName}
		if ended.Valid {
			item["ended"] = ended.Time
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) RecentRestartAlerts(ctx context.Context, since time.Time, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `SELECT a.id,a.target_fingerprint,a.status,a.started_ts,a.ended_ts_nullable,a.summary
		FROM alerts a
		JOIN alert_rules r ON r.id=a.rule_id
		WHERE r.metric_key='container_restarts' AND a.started_ts >= ?
		ORDER BY a.started_ts DESC
		LIMIT ?`, since.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]any, 0, limit)
	for rows.Next() {
		var id int64
		var target, status, summary string
		var started time.Time
		var ended sql.NullTime
		if err := rows.Scan(&id, &target, &status, &started, &ended, &summary); err != nil {
			return nil, err
		}
		item := map[string]any{"id": id, "target": target, "status": status, "started": started, "summary": summary}
		if ended.Valid {
			item["ended"] = ended.Time
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) DeleteRecoveredAlerts(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM alerts WHERE status='recovered'`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *Repository) DeleteAllAlerts(ctx context.Context) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `DELETE FROM alerts`)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM alert_states`); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *Repository) InsertNotificationEvent(ctx context.Context, alertID int64, channel, status string, attempts int, lastErr string, sent *time.Time) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO notification_events (alert_id,channel,status,attempts,last_error,sent_ts_nullable) VALUES (?,?,?,?,?,?)`, alertID, channel, status, attempts, lastErr, sent)
	return err
}

func (r *Repository) ActiveAlertCount(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM alerts WHERE status='firing'`).Scan(&n)
	return n, err
}

func (r *Repository) ActiveAlertTargetsByMetric(ctx context.Context, metricKey string) ([]ActiveAlertTarget, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT a.rule_id,a.target_fingerprint
		FROM alerts a
		JOIN alert_rules r ON r.id=a.rule_id
		WHERE a.status='firing' AND r.metric_key=?
		GROUP BY a.rule_id,a.target_fingerprint`, metricKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ActiveAlertTarget, 0, 16)
	for rows.Next() {
		var item ActiveAlertTarget
		if err := rows.Scan(&item.RuleID, &item.Target); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) ListContainers(ctx context.Context) ([]models.Container, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,service_id,name,status,started_at,last_seen_at,restart_count FROM containers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Container
	for rows.Next() {
		var c models.Container
		var started sql.NullTime
		if err := rows.Scan(&c.ID, &c.ServiceID, &c.Name, &c.Status, &started, &c.LastSeenAt, &c.RestartCount); err != nil {
			return nil, err
		}
		if started.Valid {
			t := started.Time
			c.StartedAt = &t
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateRuleThresholds(ctx context.Context, id int64, threshold float64, forSec, cooldown int, enabled bool) error {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	_, err := r.db.ExecContext(ctx, `UPDATE alert_rules SET threshold=?,for_seconds=?,cooldown_seconds=?,enabled=? WHERE id=?`, threshold, forSec, cooldown, enabledInt, id)
	return err
}

func (r *Repository) DeleteOlderThan(ctx context.Context, cutoff time.Time) error {
	queries := []string{
		`DELETE FROM host_metrics WHERE ts < ?`,
		`DELETE FROM container_metrics WHERE ts < ?`,
		`DELETE FROM logs WHERE ts < ?`,
		`DELETE FROM alerts WHERE started_ts < ? AND status='recovered'`,
	}
	for _, q := range queries {
		if _, err := r.db.ExecContext(ctx, q, cutoff.UTC()); err != nil {
			return err
		}
	}
	_, _ = r.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	_, _ = r.db.ExecContext(ctx, `PRAGMA optimize`)
	return nil
}

func (r *Repository) SaveTelegramSettings(ctx context.Context, token, chatID string) error {
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`)
	if err != nil {
		return err
	}
	for k, v := range map[string]string{"telegram_token": token, "telegram_chat_id": chatID} {
		if _, err := r.db.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, k, v); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) LoadTelegramSettings(ctx context.Context) (token, chatID string, err error) {
	_, err = r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`)
	if err != nil {
		return "", "", err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT key,value FROM settings WHERE key IN ('telegram_token','telegram_chat_id')`)
	if err != nil {
		return "", "", err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return "", "", err
		}
		if k == "telegram_token" {
			token = v
		}
		if k == "telegram_chat_id" {
			chatID = v
		}
	}
	return token, chatID, rows.Err()
}
