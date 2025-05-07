package schema

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// Scheduler handles periodic schema checks
type Scheduler struct {
	monitor *Monitor
	logger  *zap.SugaredLogger
	stop    chan struct{}
}

// NewScheduler creates a new schema check scheduler
func NewScheduler(monitor *Monitor, logger *zap.SugaredLogger) *Scheduler {
	return &Scheduler{
		monitor: monitor,
		logger:  logger,
		stop:    make(chan struct{}),
	}
}

// Start begins periodic schema checks
func (s *Scheduler) Start(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		// Run initial check
		s.runChecks()

		for {
			select {
			case <-ticker.C:
				s.runChecks()
			case <-s.stop:
				ticker.Stop()
				return
			}
		}
	}()
}

// Stop halts the scheduler
func (s *Scheduler) Stop() {
	close(s.stop)
}

// runChecks executes all schema checks
func (s *Scheduler) runChecks() {
	ctx := context.Background()
	if err := s.monitor.CheckOpenAISchema(ctx); err != nil {
		s.logger.Errorw("Failed to check OpenAI schema", "error", err)
	}
}
