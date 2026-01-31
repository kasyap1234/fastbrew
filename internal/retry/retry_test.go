package retry

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestDoSuccess(t *testing.T) {
	callCount := 0
	err := Do(context.Background(), func() error {
		callCount++
		return nil
	})

	if err != nil {
		t.Errorf("Do() returned error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

func TestDoRetriesOnError(t *testing.T) {
	callCount := 0
	cfg := Config{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		Multiplier:   1.0,
		JitterFactor: 0,
	}

	err := DoWithConfig(context.Background(), cfg, func() error {
		callCount++
		if callCount < 3 {
			return errors.New("transient error")
		}
		return nil
	})

	if err != nil {
		t.Errorf("Do() returned error: %v", err)
	}
	if callCount != 3 {
		t.Errorf("Expected 3 calls, got %d", callCount)
	}
}

func TestDoExhaustsRetries(t *testing.T) {
	callCount := 0
	cfg := Config{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		Multiplier:   1.0,
		JitterFactor: 0,
	}

	expectedErr := errors.New("persistent error")
	err := DoWithConfig(context.Background(), cfg, func() error {
		callCount++
		return expectedErr
	})

	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
	if callCount != 3 {
		t.Errorf("Expected 3 calls, got %d", callCount)
	}
}

func TestDoRespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	callCount := 0
	cfg := Config{
		MaxAttempts:  10,
		InitialDelay: 50 * time.Millisecond,
		Multiplier:   1.0,
		JitterFactor: 0,
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := DoWithConfig(ctx, cfg, func() error {
		callCount++
		return errors.New("keep retrying")
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
	if callCount > 5 {
		t.Errorf("Expected few calls before cancellation, got %d", callCount)
	}
}

func TestWithResultSuccess(t *testing.T) {
	callCount := 0
	result, err := WithResult(context.Background(), func() (int, error) {
		callCount++
		return 42, nil
	})

	if err != nil {
		t.Errorf("WithResult() returned error: %v", err)
	}
	if result != 42 {
		t.Errorf("Expected result 42, got %d", result)
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

func TestWithResultRetriesOnError(t *testing.T) {
	callCount := 0
	cfg := Config{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		Multiplier:   1.0,
		JitterFactor: 0,
	}

	result, err := WithResultConfig(context.Background(), cfg, func() (string, error) {
		callCount++
		if callCount < 3 {
			return "", errors.New("transient")
		}
		return "success", nil
	})

	if err != nil {
		t.Errorf("WithResult() returned error: %v", err)
	}
	if result != "success" {
		t.Errorf("Expected 'success', got %q", result)
	}
	if callCount != 3 {
		t.Errorf("Expected 3 calls, got %d", callCount)
	}
}

func TestNonRetryableError(t *testing.T) {
	callCount := 0
	cfg := Config{
		MaxAttempts:  5,
		InitialDelay: 1 * time.Millisecond,
		Multiplier:   1.0,
		JitterFactor: 0,
	}

	_, err := WithResultConfig(context.Background(), cfg, func() (int, error) {
		callCount++
		return 0, NonRetryable(errors.New("do not retry"))
	})

	if err == nil {
		t.Error("Expected error, got nil")
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call (no retries), got %d", callCount)
	}
}

func TestIsRetryable(t *testing.T) {
	regularErr := errors.New("regular error")
	nonRetryErr := NonRetryable(errors.New("non-retryable"))

	if !IsRetryable(regularErr) {
		t.Error("Regular error should be retryable")
	}
	if IsRetryable(nonRetryErr) {
		t.Error("NonRetryable error should not be retryable")
	}
}

func TestNonRetryableNil(t *testing.T) {
	if NonRetryable(nil) != nil {
		t.Error("NonRetryable(nil) should return nil")
	}
}

func TestNonRetryableUnwrap(t *testing.T) {
	original := errors.New("original")
	wrapped := NonRetryable(original)

	if !errors.Is(wrapped, original) {
		t.Error("Should be able to unwrap to original error")
	}
}

func TestDefaultConfig(t *testing.T) {
	if DefaultConfig.MaxAttempts != 3 {
		t.Errorf("Expected MaxAttempts=3, got %d", DefaultConfig.MaxAttempts)
	}
	if DefaultConfig.InitialDelay != 100*time.Millisecond {
		t.Errorf("Expected InitialDelay=100ms, got %v", DefaultConfig.InitialDelay)
	}
	if DefaultConfig.Multiplier != 2.0 {
		t.Errorf("Expected Multiplier=2.0, got %f", DefaultConfig.Multiplier)
	}
	if DefaultConfig.JitterFactor != 0.1 {
		t.Errorf("Expected JitterFactor=0.1, got %f", DefaultConfig.JitterFactor)
	}
}

func TestConcurrentRetries(t *testing.T) {
	cfg := Config{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		Multiplier:   1.0,
		JitterFactor: 0,
	}

	var wg sync.WaitGroup
	errCount := 0
	var mu sync.Mutex

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			callCount := 0
			_, err := WithResultConfig(context.Background(), cfg, func() (int, error) {
				callCount++
				if callCount < 2 {
					return 0, errors.New("retry once")
				}
				return id, nil
			})
			if err != nil {
				mu.Lock()
				errCount++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()
	if errCount > 0 {
		t.Errorf("Expected no errors, got %d", errCount)
	}
}
