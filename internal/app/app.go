package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"dashi/internal/alerts"
	"dashi/internal/collector"
	"dashi/internal/config"
	"dashi/internal/db"
	"dashi/internal/docker"
	"dashi/internal/logs"
	"dashi/internal/notifier"
	"dashi/internal/retention"
	"dashi/internal/web"
)

type App struct {
	cfg config.Config
	log *slog.Logger

	db     *db.Repository
	docker *docker.Client

	collector *collector.Service
	ingestor  *logs.Ingestor
	alerts    *alerts.Engine
	retention *retention.Service
	notify    *notifier.Telegram
	web       *web.Server

	httpSrv *http.Server
}

func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	sqldb, err := db.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(sqldb); err != nil {
		return nil, err
	}
	repo := db.NewRepository(sqldb)
	dc := docker.NewClient(cfg.DockerSocket)

	token, chatID, _ := repo.LoadTelegramSettings(context.Background())
	if token == "" {
		token = cfg.TelegramBotToken
	}
	if chatID == "" {
		chatID = cfg.TelegramChatID
	}
	n := notifier.NewTelegram(token, chatID)
	w := web.NewServer(repo, dc, n, logger)

	app := &App{
		cfg:       cfg,
		log:       logger,
		db:        repo,
		docker:    dc,
		collector: collector.NewService(repo, dc, logger.With("module", "collector")),
		ingestor:  logs.NewIngestor(repo, dc, logger.With("module", "logs"), cfg.SkipSelfLogs),
		alerts:    alerts.NewEngine(repo, n, logger.With("module", "alerts"), cfg.DebugRestarts),
		retention: retention.NewService(repo, cfg.RetentionDays, logger.With("module", "retention")),
		notify:    n,
		web:       w,
	}
	app.httpSrv = &http.Server{Addr: cfg.Addr, Handler: w.Routes()}
	return app, nil
}

func (a *App) Run(ctx context.Context) error {
	go func() {
		a.log.Info("http server listening", "addr", a.cfg.Addr)
		if err := a.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.log.Error("http server failed", "err", err)
		}
	}()

	metricsTicker := time.NewTicker(a.cfg.MetricsInterval)
	rulesTicker := time.NewTicker(a.cfg.RulesInterval)
	logsTicker := time.NewTicker(10 * time.Second)
	retentionTicker := time.NewTicker(6 * time.Hour)
	defer metricsTicker.Stop()
	defer rulesTicker.Stop()
	defer logsTicker.Stop()
	defer retentionTicker.Stop()

	// Immediate first run
	a.collector.Tick(ctx)
	a.ingestor.Reconcile(ctx)
	a.alerts.Evaluate(ctx)
	a.retention.Run(ctx)

	for {
		select {
		case <-ctx.Done():
			_ = a.httpSrv.Shutdown(context.Background())
			return a.db.DB().Close()
		case <-metricsTicker.C:
			a.collector.Tick(ctx)
		case <-rulesTicker.C:
			a.alerts.Evaluate(ctx)
		case <-logsTicker.C:
			a.ingestor.Reconcile(ctx)
		case <-retentionTicker.C:
			a.retention.Run(ctx)
		}
	}
}
