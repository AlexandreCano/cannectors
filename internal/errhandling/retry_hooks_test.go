package errhandling

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cannectors/runtime/pkg/connector"
)

func fastCfg() connector.RetryConfig {
	return connector.RetryConfig{
		MaxAttempts:       3,
		DelayMs:           1,
		BackoffMultiplier: 1,
		MaxDelayMs:        1000,
	}
}

func TestExecuteWithHooks_ExtractRetryAfterOverridesDelay(t *testing.T) {
	e := NewRetryExecutor(fastCfg())

	var observedDelays []time.Duration
	var attempts int
	_, err := e.ExecuteWithHooks(context.Background(),
		func(ctx context.Context) (any, error) {
			attempts++
			return nil, &ClassifiedError{
				Category:   CategoryRateLimit,
				Retryable:  true,
				StatusCode: 429,
				Message:    "rate limited",
			}
		},
		Hooks{
			ExtractRetryAfter: func(err error) (time.Duration, bool) {
				return 42 * time.Millisecond, true
			},
		},
		func(attempt int, err error, nextDelay time.Duration) {
			if err != nil && nextDelay > 0 {
				observedDelays = append(observedDelays, nextDelay)
			}
		},
	)
	if err == nil {
		t.Fatal("expected exhaustion error")
	}
	if attempts != fastCfg().MaxAttempts+1 {
		t.Errorf("attempts = %d, want %d", attempts, fastCfg().MaxAttempts+1)
	}
	for _, d := range observedDelays {
		if d != 42*time.Millisecond {
			t.Errorf("delay = %v, want 42ms override from ExtractRetryAfter", d)
		}
	}
}

func TestExecuteWithHooks_ExtractRetryAfterCappedByMaxDelay(t *testing.T) {
	cfg := fastCfg()
	cfg.MaxDelayMs = 10

	e := NewRetryExecutor(cfg)
	var observed time.Duration
	_, _ = e.ExecuteWithHooks(context.Background(),
		func(ctx context.Context) (any, error) {
			return nil, &ClassifiedError{Retryable: true, Category: CategoryServer, StatusCode: 503}
		},
		Hooks{
			ExtractRetryAfter: func(err error) (time.Duration, bool) {
				return 5 * time.Second, true
			},
		},
		func(_ int, _ error, d time.Duration) {
			if d > 0 {
				observed = d
			}
		},
	)
	if observed != 10*time.Millisecond {
		t.Errorf("delay = %v, want 10ms (capped by MaxDelayMs)", observed)
	}
}

func TestExecuteWithHooks_NegativeRetryAfterClampedToZero(t *testing.T) {
	e := NewRetryExecutor(fastCfg())

	var observed time.Duration
	_, _ = e.ExecuteWithHooks(context.Background(),
		func(ctx context.Context) (any, error) {
			return nil, &ClassifiedError{Retryable: true, Category: CategoryServer}
		},
		Hooks{
			ExtractRetryAfter: func(err error) (time.Duration, bool) {
				return -2 * time.Second, true
			},
		},
		func(_ int, _ error, d time.Duration) {
			observed = d
		},
	)
	if observed != 0 {
		t.Errorf("negative Retry-After should clamp to 0, got %v", observed)
	}
}

func TestExecuteWithHooks_NilHookIsNoop(t *testing.T) {
	e := NewRetryExecutor(fastCfg())
	_, err := e.ExecuteWithHooks(context.Background(),
		func(ctx context.Context) (any, error) {
			return "ok", nil
		},
		Hooks{},
		nil,
	)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExecuteWithCallback_BackwardCompat(t *testing.T) {
	e := NewRetryExecutor(fastCfg())
	var calls int
	_, _ = e.ExecuteWithCallback(context.Background(),
		func(ctx context.Context) (any, error) {
			calls++
			return nil, errors.New("transient")
		},
		nil,
	)
	if calls != fastCfg().MaxAttempts+1 {
		t.Errorf("calls = %d, want %d", calls, fastCfg().MaxAttempts+1)
	}
}
