package retry

import (
	"context"
	"errors"
	"math/rand"
	"time"
)

type Config struct {
	MaxAttempts  int
	InitialDelay time.Duration
	Multiplier   float64
	JitterFactor float64
}

var DefaultConfig = Config{
	MaxAttempts:  3,
	InitialDelay: 100 * time.Millisecond,
	Multiplier:   2.0,
	JitterFactor: 0.1,
}

func Do(ctx context.Context, fn func() error) error {
	return DoWithConfig(ctx, DefaultConfig, fn)
}

func DoWithConfig(ctx context.Context, cfg Config, fn func() error) error {
	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		if err := fn(); err != nil {
			lastErr = err

			if attempt == cfg.MaxAttempts {
				break
			}

			jitter := time.Duration(float64(delay) * cfg.JitterFactor * (rand.Float64()*2 - 1))
			sleepDuration := delay + jitter

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(sleepDuration):
			}

			delay = time.Duration(float64(delay) * cfg.Multiplier)
			continue
		}
		return nil
	}

	return lastErr
}

func WithResult[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	return WithResultConfig(ctx, DefaultConfig, fn)
}

func WithResultConfig[T any](ctx context.Context, cfg Config, fn func() (T, error)) (T, error) {
	var result T
	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}

		var err error
		result, err = fn()
		if err != nil {
			lastErr = err

			if attempt == cfg.MaxAttempts {
				break
			}

			if !IsRetryable(err) {
				return result, err
			}

			jitter := time.Duration(float64(delay) * cfg.JitterFactor * (rand.Float64()*2 - 1))
			sleepDuration := delay + jitter

			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case <-time.After(sleepDuration):
			}

			delay = time.Duration(float64(delay) * cfg.Multiplier)
			continue
		}
		return result, nil
	}

	return result, lastErr
}

type nonRetryableError struct {
	err error
}

func (e *nonRetryableError) Error() string { return e.err.Error() }
func (e *nonRetryableError) Unwrap() error { return e.err }

func NonRetryable(err error) error {
	if err == nil {
		return nil
	}
	return &nonRetryableError{err: err}
}

func IsRetryable(err error) bool {
	var nre *nonRetryableError
	return !errors.As(err, &nre)
}
