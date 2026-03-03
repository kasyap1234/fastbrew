package daemon

import (
	"errors"
	"testing"
	"time"
)

func TestJobManagerSubmitSuccess(t *testing.T) {
	manager := NewJobManager()

	job := manager.Submit(JobOperationInstall, []string{"jq"}, func(job *Job) error {
		job.addEvent("info", "running")
		return nil
	})

	select {
	case <-job.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for job completion")
	}

	view, ok := manager.Status(job.id)
	if !ok {
		t.Fatalf("expected job %s to exist", job.id)
	}
	if view.Status != JobStatusSucceeded {
		t.Fatalf("expected succeeded status, got %s", view.Status)
	}

	_, events, ok := manager.Stream(job.id, 0, false)
	if !ok {
		t.Fatalf("expected stream data for job %s", job.id)
	}
	if len(events) == 0 {
		t.Fatal("expected at least one event from completed job")
	}
}

func TestJobManagerStreamBlockingWaitsForEvent(t *testing.T) {
	manager := NewJobManager()

	job := manager.Submit(JobOperationInstall, []string{"jq"}, func(job *Job) error {
		time.Sleep(120 * time.Millisecond)
		job.addEvent("info", "midpoint")
		time.Sleep(20 * time.Millisecond)
		return nil
	})

	start := time.Now()
	view, events, ok := manager.Stream(job.id, 2, true)
	if !ok {
		t.Fatalf("expected blocking stream data for job %s", job.id)
	}
	if time.Since(start) < 100*time.Millisecond {
		t.Fatal("expected blocking stream to wait for new events")
	}
	if view.Status == JobStatusQueued {
		t.Fatalf("expected running or terminal status, got %s", view.Status)
	}
	if len(events) == 0 {
		t.Fatal("expected events after blocking wait")
	}
}

func TestJobManagerStreamBlockingReturnsOnTerminalWithoutEvents(t *testing.T) {
	manager := NewJobManager()

	job := manager.Submit(JobOperationUpgrade, []string{"wget"}, func(job *Job) error {
		time.Sleep(120 * time.Millisecond)
		return nil
	})

	start := time.Now()
	view, events, ok := manager.Stream(job.id, 9999, true)
	if !ok {
		t.Fatalf("expected blocking stream data for job %s", job.id)
	}
	if time.Since(start) < 100*time.Millisecond {
		t.Fatal("expected blocking stream to wait for terminal status")
	}
	if view.Status != JobStatusSucceeded {
		t.Fatalf("expected succeeded status, got %s", view.Status)
	}
	if len(events) != 0 {
		t.Fatalf("expected zero events for far future seq, got %d", len(events))
	}
}

func TestJobManagerStreamNonBlockingIsImmediate(t *testing.T) {
	manager := NewJobManager()
	job := manager.Submit(JobOperationInstall, []string{"jq"}, func(job *Job) error {
		time.Sleep(300 * time.Millisecond)
		return nil
	})

	start := time.Now()
	_, _, ok := manager.Stream(job.id, 9999, false)
	if !ok {
		t.Fatalf("expected stream data for job %s", job.id)
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Fatal("non-blocking stream should return immediately")
	}
}

func TestJobManagerStreamSequenceMonotonic(t *testing.T) {
	manager := NewJobManager()

	job := manager.Submit(JobOperationInstall, []string{"jq"}, func(job *Job) error {
		job.addEvent("info", "phase 1")
		job.addEvent("info", "phase 2")
		job.addEvent("info", "phase 3")
		return nil
	})

	select {
	case <-job.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for job completion")
	}

	fromSeq := 0
	lastSeq := -1
	for {
		view, events, ok := manager.Stream(job.id, fromSeq, false)
		if !ok {
			t.Fatalf("expected stream data for job %s", job.id)
		}

		if len(events) == 0 {
			if view.Status == JobStatusSucceeded || view.Status == JobStatusFailed {
				break
			}
			continue
		}

		for _, event := range events {
			if event.Seq != fromSeq {
				t.Fatalf("expected seq %d, got %d", fromSeq, event.Seq)
			}
			if event.Seq <= lastSeq {
				t.Fatalf("expected monotonic increasing seq, last=%d current=%d", lastSeq, event.Seq)
			}
			lastSeq = event.Seq
			fromSeq = event.Seq + 1
		}
	}
}

func TestJobManagerSubmitFailure(t *testing.T) {
	manager := NewJobManager()
	expectedErr := errors.New("boom")

	job := manager.Submit(JobOperationUpgrade, []string{"wget"}, func(job *Job) error {
		return expectedErr
	})

	select {
	case <-job.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for failed job")
	}

	view, ok := manager.Status(job.id)
	if !ok {
		t.Fatalf("expected job %s to exist", job.id)
	}
	if view.Status != JobStatusFailed {
		t.Fatalf("expected failed status, got %s", view.Status)
	}
	if view.Error == "" {
		t.Fatal("expected failure error text to be set")
	}
}
