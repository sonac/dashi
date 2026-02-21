package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"dashi/internal/db"
	"dashi/internal/docker"
	"dashi/internal/notifier"
)

//go:embed templates/*.html static/*
var webFS embed.FS

type Server struct {
	repo   *db.Repository
	docker *docker.Client
	notify *notifier.Telegram
	log    *slog.Logger
	tpl    *template.Template
}

func NewServer(repo *db.Repository, docker *docker.Client, notify *notifier.Telegram, logger *slog.Logger) *Server {
	tpl := template.Must(template.New("all").Funcs(template.FuncMap{
		"bytesToMB": func(v int64) string { return fmt.Sprintf("%.1f MB", float64(v)/1024.0/1024.0) },
		"pct":       func(v float64) string { return fmt.Sprintf("%.1f%%", v) },
		"timeago":   func(t time.Time) string { return time.Since(t).Round(time.Second).String() + " ago" },
	}).ParseFS(webFS, "templates/*.html"))
	return &Server{repo: repo, docker: docker, notify: notify, log: logger, tpl: tpl}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/fragments/overview", s.handleOverviewFragment)
	mux.HandleFunc("/fragments/services", s.handleServicesFragment)
	mux.HandleFunc("/fragments/alerts", s.handleAlertsFragment)
	mux.HandleFunc("/fragments/restarts", s.handleRestartAlertsFragment)
	mux.HandleFunc("/fragments/logs", s.handleLogsFragment)
	mux.HandleFunc("/fragments/service/", s.handleServiceSubroutes)
	mux.HandleFunc("/settings", s.handleSettings)
	mux.HandleFunc("/settings/telegram", s.handleSettingsTelegram)
	mux.HandleFunc("/settings/rules", s.handleSettingsRules)
	mux.HandleFunc("/api/metrics/host", s.handleHostMetricsAPI)
	mux.HandleFunc("/api/metrics/container/", s.handleContainerMetricsAPI)
	mux.HandleFunc("/api/logs", s.handleLogsAPI)
	mux.HandleFunc("/api/alerts/test-telegram", s.handleTestTelegram)
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)
	staticFS, _ := fs.Sub(webFS, "static")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	return logMiddleware(mux, s.log)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if err := s.tpl.ExecuteTemplate(w, "index.html", nil); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func (s *Server) handleOverviewFragment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	metric, err := s.repo.LatestHostMetric(ctx)
	if err != nil {
		http.Error(w, "no metrics yet", http.StatusServiceUnavailable)
		return
	}
	alerts, _ := s.repo.ActiveAlertCount(ctx)
	data := map[string]any{
		"metric":       metric,
		"mem_pct":      pct(metric.MemUsedBytes, metric.MemTotalBytes),
		"disk_pct":     pct(metric.DiskUsedBytes, metric.DiskTotalBytes),
		"activeAlerts": alerts,
	}
	_ = s.tpl.ExecuteTemplate(w, "fragment_overview.html", data)
}

func pct(used, total int64) float64 {
	if total == 0 {
		return 0
	}
	return (float64(used) / float64(total)) * 100
}

func (s *Server) handleServicesFragment(w http.ResponseWriter, r *http.Request) {
	minCPU := 0.0
	if v := r.URL.Query().Get("min_cpu"); v != "" {
		normalized := strings.ReplaceAll(v, ",", ".")
		if parsed, err := strconv.ParseFloat(normalized, 64); err == nil && parsed >= 0 {
			minCPU = parsed
		}
	}
	minMemMB := int64(0)
	if v := r.URL.Query().Get("min_mem_mb"); v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil && parsed >= 0 {
			minMemMB = parsed
		}
	}
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	includeMissing := r.URL.Query().Get("include_missing") == "1"
	rows, err := s.repo.ListServicesWithHealth(r.Context(), minCPU, minMemMB*1024*1024, limit, includeMissing)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = s.tpl.ExecuteTemplate(w, "fragment_services.html", map[string]any{
		"services":   rows,
		"minCPU":     minCPU,
		"minMemMB":   minMemMB,
		"limit":      limit,
		"serviceCnt": len(rows),
	})
}

func (s *Server) handleAlertsFragment(w http.ResponseWriter, r *http.Request) {
	alerts, err := s.repo.RecentAlerts(r.Context(), 100)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = s.tpl.ExecuteTemplate(w, "fragment_alerts.html", map[string]any{"alerts": alerts})
}

func (s *Server) handleRestartAlertsFragment(w http.ResponseWriter, r *http.Request) {
	restarts, err := s.repo.RecentRestartAlerts(r.Context(), 20)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = s.tpl.ExecuteTemplate(w, "fragment_restarts.html", map[string]any{"restarts": restarts})
}

func (s *Server) handleLogsFragment(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	serviceID := r.URL.Query().Get("service")
	level := r.URL.Query().Get("level")
	stream := r.URL.Query().Get("stream")
	from := queryRangeStart(r)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 150
	}
	entries, err := s.repo.QueryLogs(r.Context(), serviceID, q, level, stream, from, nil, limit)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	title := "Recent Logs"
	if serviceID != "" {
		title = "Logs for " + serviceID
	}
	_ = s.tpl.ExecuteTemplate(w, "fragment_logs.html", map[string]any{
		"entries":   entries,
		"serviceID": serviceID,
		"title":     title,
		"stream":    stream,
	})
}

func (s *Server) handleServiceSubroutes(w http.ResponseWriter, r *http.Request) {
	// /fragments/service/{id}/logs
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) == 4 && parts[0] == "fragments" && parts[1] == "service" && parts[3] == "logs" {
		svcID := parts[2]
		s.handleServiceLogsFragment(w, r.WithContext(context.WithValue(r.Context(), "serviceID", svcID)))
		return
	}
	http.NotFound(w, r)
}

func (s *Server) handleServiceLogsFragment(w http.ResponseWriter, r *http.Request) {
	svcID, _ := r.Context().Value("serviceID").(string)
	q := r.URL.Query().Get("q")
	level := r.URL.Query().Get("level")
	stream := r.URL.Query().Get("stream")
	from := queryRangeStart(r)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 200
	}
	entries, err := s.repo.QueryLogs(r.Context(), svcID, q, level, stream, from, nil, limit)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = s.tpl.ExecuteTemplate(w, "fragment_logs.html", map[string]any{"entries": entries, "serviceID": svcID, "title": "Logs for " + svcID})
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	token, chatID, _ := s.repo.LoadTelegramSettings(r.Context())
	rules, _ := s.repo.ListRules(r.Context())
	_ = s.tpl.ExecuteTemplate(w, "settings.html", map[string]any{"token": token, "chat_id": chatID, "rules": rules})
}

func (s *Server) handleSettingsTelegram(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	token := strings.TrimSpace(r.FormValue("token"))
	chatID := strings.TrimSpace(r.FormValue("chat_id"))
	if err := s.repo.SaveTelegramSettings(r.Context(), token, chatID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.notify.Update(token, chatID)
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (s *Server) handleSettingsRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
	th, _ := strconv.ParseFloat(r.FormValue("threshold"), 64)
	forSec, _ := strconv.Atoi(r.FormValue("for_seconds"))
	cooldown, _ := strconv.Atoi(r.FormValue("cooldown_seconds"))
	enabled := r.FormValue("enabled") == "on"
	if err := s.repo.UpdateRuleThresholds(r.Context(), id, th, forSec, cooldown, enabled); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (s *Server) handleHostMetricsAPI(w http.ResponseWriter, r *http.Request) {
	rng := parseRange(r.URL.Query().Get("range"))
	metrics, err := s.repo.RecentHostMetrics(r.Context(), time.Now().Add(-rng), 4096)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, metrics)
}

func (s *Server) handleContainerMetricsAPI(w http.ResponseWriter, r *http.Request) {
	containerID := path.Base(r.URL.Path)
	if containerID == "" || strings.Contains(containerID, "/") {
		http.NotFound(w, r)
		return
	}
	rng := parseRange(r.URL.Query().Get("range"))
	metrics, err := s.repo.RecentContainerMetrics(r.Context(), containerID, time.Now().Add(-rng), 4096)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, metrics)
}

func (s *Server) handleLogsAPI(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	serviceID := r.URL.Query().Get("service")
	level := r.URL.Query().Get("level")
	stream := r.URL.Query().Get("stream")
	groupBy := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("group_by")))
	from := queryRangeStart(r)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	if groupBy != "" {
		groups, err := s.repo.GroupLogs(r.Context(), groupBy, serviceID, q, level, stream, from, nil, limit)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, map[string]any{
			"group_by": groupBy,
			"filters":  map[string]any{"service": serviceID, "q": q, "level": level, "stream": stream, "range": r.URL.Query().Get("range")},
			"groups":   groups,
		})
		return
	}

	entries, err := s.repo.QueryLogs(r.Context(), serviceID, q, level, stream, from, nil, limit)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, entries)
}

func queryRangeStart(r *http.Request) *time.Time {
	v := strings.TrimSpace(r.URL.Query().Get("range"))
	if v == "" {
		return nil
	}
	from := time.Now().Add(-parseRange(v)).UTC()
	return &from
}

func (s *Server) handleTestTelegram(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	msg := "Dashi test alert: Telegram integration is working"
	err := s.notify.Send(r.Context(), msg)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if err := s.repo.DB().PingContext(r.Context()); err != nil {
		http.Error(w, "db not ready", 503)
		return
	}
	if err := s.docker.Ping(r.Context()); err != nil {
		http.Error(w, "docker not ready", 503)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func parseRange(v string) time.Duration {
	if v == "" {
		return time.Hour
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return time.Hour
	}
	if d <= 0 {
		return time.Hour
	}
	return d
}
