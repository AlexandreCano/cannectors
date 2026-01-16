// Package scheduler provides CRON-based scheduling for pipeline execution.
// It allows pipelines to be executed on a recurring schedule.
//
// This package will be implemented in Story 4.1: Implement CRON Scheduler.
package scheduler

import (
	"errors"

	"github.com/canectors/runtime/pkg/connector"
)

// ErrNotImplemented is returned when a feature is not yet implemented.
var ErrNotImplemented = errors.New("not implemented: will be added in Story 4.1")

// Scheduler manages scheduled pipeline executions.
type Scheduler struct {
	// pipelines holds registered pipelines with their schedules
	pipelines map[string]*connector.Pipeline
}

// New creates a new scheduler instance.
func New() *Scheduler {
	return &Scheduler{
		pipelines: make(map[string]*connector.Pipeline),
	}
}

// Register adds a pipeline to the scheduler.
// TODO: Implement in Story 4.1
func (s *Scheduler) Register(_ *connector.Pipeline) error {
	return ErrNotImplemented
}

// Start begins executing scheduled pipelines.
// TODO: Implement in Story 4.1
func (s *Scheduler) Start() error {
	return ErrNotImplemented
}

// Stop halts all scheduled executions.
// TODO: Implement in Story 4.1
func (s *Scheduler) Stop() error {
	return ErrNotImplemented
}
