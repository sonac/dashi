package retention

import (
	"context"
	"log/slog"
	"time"

	"dashi/internal/db"
)

type Service struct {
	repo          *db.Repository
	retentionDays int
	log           *slog.Logger
}

func NewService(repo *db.Repository, days int, logger *slog.Logger) *Service {
	if days <= 0 {
		days = 14
	}
	return &Service{repo: repo, retentionDays: days, log: logger}
}

func (s *Service) Run(ctx context.Context) {
	cutoff := time.Now().UTC().AddDate(0, 0, -s.retentionDays)
	if err := s.repo.DeleteOlderThan(ctx, cutoff); err != nil {
		s.log.Error("retention cleanup failed", "err", err)
	} else {
		s.log.Info("retention cleanup completed", "cutoff", cutoff)
	}
}
