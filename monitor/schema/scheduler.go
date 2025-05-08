package schema

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// Scheduler handles periodic schema monitoring
type Scheduler struct {
	monitor  *Monitor
	logger   *zap.SugaredLogger
	interval time.Duration
}

// NewScheduler creates a new schema check scheduler
func NewScheduler(monitor *Monitor, logger *zap.SugaredLogger, interval time.Duration) *Scheduler {
	return &Scheduler{
		monitor:  monitor,
		logger:   logger,
		interval: interval,
	}
}

// Start begins periodic schema monitoring
func (s *Scheduler) Start(ctx context.Context) error {
	s.logger.Info("Starting schema monitor scheduler...")
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Do an initial check
	if err := s.monitor.CheckSchemas(ctx); err != nil {
		s.logger.Errorw("Initial schema check failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Stopping schema monitor scheduler...")
			return ctx.Err()
		case <-ticker.C:
			if err := s.monitor.CheckSchemas(ctx); err != nil {
				s.logger.Errorw("Schema check failed", "error", err)
			}
		}
	}
}
