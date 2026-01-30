package progress

import (
	"errors"
	"testing"
	"time"
)

func TestProgressTracker_Start(t *testing.T) {
	events := make(chan ProgressEvent, 10)
	tracker := NewProgressTracker("test-1", "http://example.com/file.tar.gz", events)

	tracker.Start(1000)

	select {
	case event := <-events:
		if event.Type != EventDownloadStart {
			t.Errorf("Expected event type %s, got %s", EventDownloadStart, event.Type)
		}
		if event.ID != "test-1" {
			t.Errorf("Expected ID 'test-1', got %s", event.ID)
		}
		if event.Total != 1000 {
			t.Errorf("Expected Total 1000, got %d", event.Total)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected event to be sent")
	}

	progress := tracker.GetDownloadProgress()
	if progress.TotalBytes != 1000 {
		t.Errorf("Expected TotalBytes 1000, got %d", progress.TotalBytes)
	}
	if progress.URL != "http://example.com/file.tar.gz" {
		t.Errorf("Expected URL 'http://example.com/file.tar.gz', got %s", progress.URL)
	}
}

func TestProgressTracker_Update(t *testing.T) {
	events := make(chan ProgressEvent, 10)
	tracker := NewProgressTracker("test-2", "http://example.com/file.tar.gz", events)

	tracker.Start(1000)
	<-events // consume start event

	tracker.Update(500)

	select {
	case event := <-events:
		if event.Type != EventDownloadProgress {
			t.Errorf("Expected event type %s, got %s", EventDownloadProgress, event.Type)
		}
		if event.Current != 500 {
			t.Errorf("Expected Current 500, got %d", event.Current)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected event to be sent")
	}

	progress := tracker.GetDownloadProgress()
	if progress.DownloadedBytes != 500 {
		t.Errorf("Expected DownloadedBytes 500, got %d", progress.DownloadedBytes)
	}
}

func TestProgressTracker_Complete(t *testing.T) {
	events := make(chan ProgressEvent, 10)
	tracker := NewProgressTracker("test-3", "http://example.com/file.tar.gz", events)

	tracker.Start(1000)
	<-events // consume start event

	tracker.Update(500)
	<-events // consume progress event

	tracker.Complete()

	select {
	case event := <-events:
		if event.Type != EventDownloadComplete {
			t.Errorf("Expected event type %s, got %s", EventDownloadComplete, event.Type)
		}
		if event.Current != 1000 {
			t.Errorf("Expected Current 1000, got %d", event.Current)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected event to be sent")
	}

	progress := tracker.GetDownloadProgress()
	if !progress.IsComplete() {
		t.Error("Expected progress to be complete")
	}
	if progress.DownloadedBytes != 1000 {
		t.Errorf("Expected DownloadedBytes 1000, got %d", progress.DownloadedBytes)
	}
}

func TestProgressTracker_Error(t *testing.T) {
	events := make(chan ProgressEvent, 10)
	tracker := NewProgressTracker("test-4", "http://example.com/file.tar.gz", events)

	tracker.Start(1000)
	<-events // consume start event

	testErr := errors.New("download failed")
	tracker.Error(testErr)

	select {
	case event := <-events:
		if event.Type != EventDownloadError {
			t.Errorf("Expected event type %s, got %s", EventDownloadError, event.Type)
		}
		if event.Message != "download failed" {
			t.Errorf("Expected message 'download failed', got %s", event.Message)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected event to be sent")
	}

	progress := tracker.GetDownloadProgress()
	if !progress.IsComplete() {
		t.Error("Expected progress to be complete (with error)")
	}
	if progress.Error == nil {
		t.Error("Expected error to be set")
	}
}

func TestDownloadProgress_CalculateProgress(t *testing.T) {
	tests := []struct {
		name            string
		totalBytes      int64
		downloadedBytes int64
		expected        float64
	}{
		{"50% complete", 1000, 500, 50},
		{"0% complete", 1000, 0, 0},
		{"100% complete", 1000, 1000, 100},
		{"capped at 100%", 1000, 1500, 100},
		{"zero total", 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dp := DownloadProgress{
				TotalBytes:      tt.totalBytes,
				DownloadedBytes: tt.downloadedBytes,
			}
			result := dp.CalculateProgress()
			if result != tt.expected {
				t.Errorf("Expected %f, got %f", tt.expected, result)
			}
		})
	}
}

func TestProgressEvent_CalculatePercentage(t *testing.T) {
	tests := []struct {
		name     string
		current  int64
		total    int64
		expected float64
	}{
		{"50% complete", 500, 1000, 50},
		{"0% complete", 0, 1000, 0},
		{"100% complete", 1000, 1000, 100},
		{"capped at 100%", 1500, 1000, 100},
		{"zero total", 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := ProgressEvent{
				Current: tt.current,
				Total:   tt.total,
			}
			result := event.CalculatePercentage()
			if result != tt.expected {
				t.Errorf("Expected %f, got %f", tt.expected, result)
			}
		})
	}
}

func TestEventBus(t *testing.T) {
	bus := NewEventBus()

	ch1 := make(chan ProgressEvent, 10)
	ch2 := make(chan ProgressEvent, 10)

	bus.Subscribe("sub1", ch1)
	bus.Subscribe("sub2", ch2)

	if bus.GetSubscriberCount() != 2 {
		t.Errorf("Expected 2 subscribers, got %d", bus.GetSubscriberCount())
	}

	event := ProgressEvent{Type: EventDownloadStart, ID: "test"}
	bus.Publish(event)

	select {
	case e := <-ch1:
		if e.ID != "test" {
			t.Errorf("Expected ID 'test', got %s", e.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected event on ch1")
	}

	select {
	case e := <-ch2:
		if e.ID != "test" {
			t.Errorf("Expected ID 'test', got %s", e.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected event on ch2")
	}

	bus.Unsubscribe("sub1")
	if bus.GetSubscriberCount() != 1 {
		t.Errorf("Expected 1 subscriber after unsubscribe, got %d", bus.GetSubscriberCount())
	}
}

func TestManager(t *testing.T) {
	manager := NewManager()
	defer manager.Close()

	// Test registering trackers
	tracker1 := manager.Register("dl-1", "http://example.com/file1.tar.gz")
	tracker2 := manager.Register("dl-2", "http://example.com/file2.tar.gz")

	if manager.GetTracker("dl-1") == nil {
		t.Error("Expected tracker dl-1 to be registered")
	}
	if manager.GetTracker("dl-2") == nil {
		t.Error("Expected tracker dl-2 to be registered")
	}

	allTrackers := manager.GetAllTrackers()
	if len(allTrackers) != 2 {
		t.Errorf("Expected 2 trackers, got %d", len(allTrackers))
	}

	// Test progress updates
	tracker1.Start(1000)
	tracker1.Update(500)

	tracker2.Start(2000)
	tracker2.Update(1000)

	// Test aggregate progress
	agg := manager.GetAggregateProgress()
	if agg.TotalDownloads != 2 {
		t.Errorf("Expected TotalDownloads 2, got %d", agg.TotalDownloads)
	}
	if agg.TotalBytes != 3000 {
		t.Errorf("Expected TotalBytes 3000, got %d", agg.TotalBytes)
	}
	if agg.DownloadedBytes != 1500 {
		t.Errorf("Expected DownloadedBytes 1500, got %d", agg.DownloadedBytes)
	}

	// Complete one download
	tracker1.Complete()

	// Test completed trackers
	completed := manager.GetCompletedTrackers()
	if len(completed) != 1 {
		t.Errorf("Expected 1 completed tracker, got %d", len(completed))
	}

	// Test unregister
	manager.Unregister("dl-1")
	if manager.GetTracker("dl-1") != nil {
		t.Error("Expected tracker dl-1 to be unregistered")
	}
}

func TestManager_IsComplete(t *testing.T) {
	manager := NewManager()
	defer manager.Close()

	// Empty manager should not be complete
	if manager.IsComplete() {
		t.Error("Empty manager should not be complete")
	}

	tracker1 := manager.Register("dl-1", "http://example.com/file1.tar.gz")
	tracker2 := manager.Register("dl-2", "http://example.com/file2.tar.gz")

	tracker1.Start(1000)
	tracker2.Start(1000)

	// Both incomplete
	if manager.IsComplete() {
		t.Error("Manager should not be complete when downloads are in progress")
	}

	tracker1.Complete()

	// One still incomplete
	if manager.IsComplete() {
		t.Error("Manager should not be complete when one download is still in progress")
	}

	tracker2.Complete()

	// All complete
	if !manager.IsComplete() {
		t.Error("Manager should be complete when all downloads are finished")
	}
}

func TestSafeEventChannel(t *testing.T) {
	sec := NewSafeEventChannel(5)

	event := ProgressEvent{Type: EventDownloadStart, ID: "test"}

	// Test sending
	if !sec.Send(event) {
		t.Error("Expected Send to succeed")
	}

	// Test receiving
	select {
	case e := <-sec.Receive():
		if e.ID != "test" {
			t.Errorf("Expected ID 'test', got %s", e.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected to receive event")
	}

	// Test close
	sec.Close()
	if !sec.IsClosed() {
		t.Error("Expected channel to be closed")
	}

	// Sending to closed channel should fail
	if sec.Send(event) {
		t.Error("Expected Send to fail on closed channel")
	}

	// Double close should not panic
	sec.Close()
}

func TestManagerWithBuffer(t *testing.T) {
	manager := NewManagerWithBuffer(50)
	defer manager.Close()

	tracker := manager.Register("test", "http://example.com/file.tar.gz")
	tracker.Start(1000)

	// Just verify it doesn't panic and works correctly
	progress := tracker.GetDownloadProgress()
	if progress.TotalBytes != 1000 {
		t.Errorf("Expected TotalBytes 1000, got %d", progress.TotalBytes)
	}
}
