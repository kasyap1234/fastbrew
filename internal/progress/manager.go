package progress

import (
	"sync"
	"time"
)

// Manager handles multiple concurrent download progress trackers
type Manager struct {
	mu       sync.RWMutex
	trackers map[string]ProgressTracker
	events   chan ProgressEvent
	eventBus *EventBus
}

// NewManager creates a new progress Manager instance
func NewManager() *Manager {
	return &Manager{
		trackers: make(map[string]ProgressTracker),
		events:   make(chan ProgressEvent, 100),
		eventBus: NewEventBus(),
	}
}

// NewManagerWithBuffer creates a new Manager with a custom event buffer size
func NewManagerWithBuffer(bufferSize int) *Manager {
	return &Manager{
		trackers: make(map[string]ProgressTracker),
		events:   make(chan ProgressEvent, bufferSize),
		eventBus: NewEventBus(),
	}
}

// Register creates and registers a new progress tracker for a download
func (m *Manager) Register(id, url string) ProgressTracker {
	m.mu.Lock()
	defer m.mu.Unlock()

	tracker := NewProgressTracker(id, url, m.events)
	m.trackers[id] = tracker
	return tracker
}

// Unregister removes a progress tracker from the manager
func (m *Manager) Unregister(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.trackers, id)
}

// GetTracker returns a tracker by ID, or nil if not found
func (m *Manager) GetTracker(id string) ProgressTracker {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.trackers[id]
}

// GetAllTrackers returns a slice of all registered trackers
func (m *Manager) GetAllTrackers() []ProgressTracker {
	m.mu.RLock()
	defer m.mu.RUnlock()

	trackers := make([]ProgressTracker, 0, len(m.trackers))
	for _, tracker := range m.trackers {
		trackers = append(trackers, tracker)
	}
	return trackers
}

// GetActiveTrackers returns trackers that are currently downloading
func (m *Manager) GetActiveTrackers() []ProgressTracker {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var active []ProgressTracker
	for _, tracker := range m.trackers {
		progress := tracker.GetDownloadProgress()
		if !progress.IsComplete() && progress.StartedAt.After(time.Time{}) {
			active = append(active, tracker)
		}
	}
	return active
}

// GetCompletedTrackers returns trackers that have finished
func (m *Manager) GetCompletedTrackers() []ProgressTracker {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var completed []ProgressTracker
	for _, tracker := range m.trackers {
		if tracker.GetDownloadProgress().IsComplete() {
			completed = append(completed, tracker)
		}
	}
	return completed
}

// GetEvents returns the events channel for receiving progress updates
func (m *Manager) GetEvents() <-chan ProgressEvent {
	return m.events
}

// Close closes the events channel and cleans up resources
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	close(m.events)
	m.trackers = make(map[string]ProgressTracker)
}

// AggregateProgress calculates overall progress across all downloads
type AggregateProgress struct {
	TotalDownloads     int
	ActiveDownloads    int
	CompletedDownloads int
	FailedDownloads    int
	TotalBytes         int64
	DownloadedBytes    int64
	OverallPercentage  float64
	AverageSpeed       float64 // bytes per second
}

// GetAggregateProgress calculates the aggregate progress of all trackers
func (m *Manager) GetAggregateProgress() AggregateProgress {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var totalBytes, downloadedBytes int64
	var totalSpeed float64
	var activeCount, completedCount, failedCount int

	for _, tracker := range m.trackers {
		progress := tracker.GetDownloadProgress()
		totalBytes += progress.TotalBytes
		downloadedBytes += progress.DownloadedBytes

		if progress.Error != nil {
			failedCount++
		} else if progress.IsComplete() {
			completedCount++
		} else if progress.StartedAt.After(time.Time{}) {
			activeCount++
			totalSpeed += progress.Speed
		}
	}

	overallPercentage := float64(0)
	if totalBytes > 0 {
		overallPercentage = float64(downloadedBytes) / float64(totalBytes) * 100
	}

	averageSpeed := float64(0)
	if activeCount > 0 {
		averageSpeed = totalSpeed / float64(activeCount)
	}

	return AggregateProgress{
		TotalDownloads:     len(m.trackers),
		ActiveDownloads:    activeCount,
		CompletedDownloads: completedCount,
		FailedDownloads:    failedCount,
		TotalBytes:         totalBytes,
		DownloadedBytes:    downloadedBytes,
		OverallPercentage:  overallPercentage,
		AverageSpeed:       averageSpeed,
	}
}

// IsComplete returns true if all downloads have finished
func (m *Manager) IsComplete() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.trackers) == 0 {
		return false
	}

	for _, tracker := range m.trackers {
		if !tracker.GetDownloadProgress().IsComplete() {
			return false
		}
	}
	return true
}

// SubscribeToEvents allows external components to subscribe to progress events
func (m *Manager) SubscribeToEvents(id string, ch chan<- ProgressEvent) {
	m.eventBus.Subscribe(id, ch)
}

// UnsubscribeFromEvents removes a subscription from the event bus
func (m *Manager) UnsubscribeFromEvents(id string) {
	m.eventBus.Unsubscribe(id)
}

// StartEventRouter starts a goroutine that routes events to the event bus
// Call this after setting up subscriptions to receive events
func (m *Manager) StartEventRouter() {
	go func() {
		for event := range m.events {
			m.eventBus.Publish(event)
		}
	}()
}
