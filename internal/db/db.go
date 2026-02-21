package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir data dir: %w", err)
	}
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000", path)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA synchronous=NORMAL; PRAGMA temp_store=MEMORY;`); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func Migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS services (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			image TEXT NOT NULL,
			labels_json TEXT NOT NULL,
			first_seen_at DATETIME NOT NULL,
			last_seen_at DATETIME NOT NULL,
			status TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS containers (
			id TEXT PRIMARY KEY,
			service_id TEXT NOT NULL,
			name TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at DATETIME,
			last_seen_at DATETIME NOT NULL,
			restart_count INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY(service_id) REFERENCES services(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS host_metrics (
			ts DATETIME NOT NULL,
			cpu_pct REAL NOT NULL,
			mem_used_bytes INTEGER NOT NULL,
			mem_total_bytes INTEGER NOT NULL,
			net_rx_bytes INTEGER NOT NULL,
			net_tx_bytes INTEGER NOT NULL,
			disk_used_bytes INTEGER NOT NULL,
			disk_total_bytes INTEGER NOT NULL,
			load1 REAL NOT NULL,
			load5 REAL NOT NULL,
			load15 REAL NOT NULL,
			uptime_sec INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS container_metrics (
			ts DATETIME NOT NULL,
			container_id TEXT NOT NULL,
			cpu_pct REAL NOT NULL,
			mem_used_bytes INTEGER NOT NULL,
			mem_limit_bytes INTEGER NOT NULL,
			net_rx_bytes INTEGER NOT NULL,
			net_tx_bytes INTEGER NOT NULL,
			blk_read_bytes INTEGER NOT NULL,
			blk_write_bytes INTEGER NOT NULL,
			FOREIGN KEY(container_id) REFERENCES containers(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ts DATETIME NOT NULL,
			service_id TEXT NOT NULL,
			container_id TEXT NOT NULL,
			level TEXT NOT NULL,
			stream TEXT NOT NULL,
			message TEXT NOT NULL,
			FOREIGN KEY(service_id) REFERENCES services(id) ON DELETE CASCADE,
			FOREIGN KEY(container_id) REFERENCES containers(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS alert_rules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			target_type TEXT NOT NULL,
			target_id_nullable TEXT,
			metric_key TEXT NOT NULL,
			operator TEXT NOT NULL,
			threshold REAL NOT NULL,
			for_seconds INTEGER NOT NULL,
			cooldown_seconds INTEGER NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1
		);`,
		`CREATE TABLE IF NOT EXISTS alert_states (
			rule_id INTEGER NOT NULL,
			target_fingerprint TEXT NOT NULL,
			state TEXT NOT NULL,
			since_ts DATETIME NOT NULL,
			last_fired_ts DATETIME,
			last_recovered_ts DATETIME,
			PRIMARY KEY(rule_id, target_fingerprint),
			FOREIGN KEY(rule_id) REFERENCES alert_rules(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS alerts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			rule_id INTEGER NOT NULL,
			target_fingerprint TEXT NOT NULL,
			status TEXT NOT NULL,
			started_ts DATETIME NOT NULL,
			ended_ts_nullable DATETIME,
			summary TEXT NOT NULL,
			details_json TEXT NOT NULL,
			FOREIGN KEY(rule_id) REFERENCES alert_rules(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS notification_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			alert_id INTEGER NOT NULL,
			channel TEXT NOT NULL,
			status TEXT NOT NULL,
			attempts INTEGER NOT NULL,
			last_error TEXT,
			sent_ts_nullable DATETIME,
			FOREIGN KEY(alert_id) REFERENCES alerts(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_logs_service_ts ON logs(service_id, ts DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_logs_container_ts ON logs(container_id, ts DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_host_metrics_ts ON host_metrics(ts DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_container_metrics_container_ts ON container_metrics(container_id, ts DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_status_started ON alerts(status, started_ts DESC);`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate failed: %w", err)
		}
	}
	return seedDefaultRules(db)
}

func seedDefaultRules(db *sql.DB) error {
	defaults := []struct {
		name, targetType, metricKey, op string
		th                              float64
		forSec, cooldown                int
	}{
		{"Host CPU high", "host", "host_cpu_pct", ">", 90, 120, 600},
		{"Host memory high", "host", "host_mem_pct", ">", 90, 120, 600},
		{"Host disk high", "host", "host_disk_pct", ">", 85, 300, 1800},
		{"Container unavailable", "container", "container_unavailable", ">=", 1, 60, 600},
		{"Container restarted", "container", "container_restarts", ">=", 1, 0, 60},
	}
	for _, r := range defaults {
		_, err := db.Exec(`INSERT INTO alert_rules (name,target_type,metric_key,operator,threshold,for_seconds,cooldown_seconds,enabled)
			SELECT ?,?,?,?,?,?,?,1 WHERE NOT EXISTS (SELECT 1 FROM alert_rules WHERE name = ?)`,
			r.name, r.targetType, r.metricKey, r.op, r.th, r.forSec, r.cooldown, r.name)
		if err != nil {
			return err
		}
	}
	return nil
}
