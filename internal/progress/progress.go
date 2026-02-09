// Package progress provides core infrastructure for tracking download progress
// in both TUI and CLI modes of fastbrew.
package progress

import (
	"sync"
	"time"
)

// ProgressTracker defines the interface for tracking download progress.
// Implementations can be used for both TUI and CLI rendering modes.
type ProgressTracker interface {
	// Start initializes the tracker with the total size to download
	Start(total int64)
	// Update updates the current progress
	Update(current int64)
	// Complete marks the download as successfully completed
	Complete()
	// Error marks the download as failed with the given error
	Error(err error)
	// GetID returns the unique identifier for this tracker
	GetID() string
	// GetDownloadProgress returns the current download progress state
	GetDownloadProgress() DownloadProgress
}

// DownloadProgress holds the state of a download operation
type DownloadProgress struct {
	ID              string
	URL             string
	TotalBytes      int64
	DownloadedBytes int64
	Speed           float64 // bytes per second
	ETA             time.Duration
	StartedAt       time.Time
	UpdatedAt       time.Time
	CompletedAt     time.Time
	Error           error
}

// CalculateProgress computes the completion percentage (0-100)
func (dp *DownloadProgress) CalculateProgress() float64 {
	if dp.TotalBytes <= 0 {
		return 0
	}
	percentage := float64(dp.DownloadedBytes) / float64(dp.TotalBytes) * 100
	if percentage > 100 {
		return 100
	}
	return percentage
}

// IsComplete returns true if the download has finished (successfully or with error)
func (dp DownloadProgress) IsComplete() bool {
	return !dp.CompletedAt.IsZero() || dp.Error != nil
}

// baseTracker is a basic implementation of ProgressTracker
type baseTracker struct {
	id       string
	url      string
	events   chan<- ProgressEvent
	progress DownloadProgress
	mu       sync.RWMutex
}

func (t *baseTracker) trySend(event ProgressEvent) {
	defer func() { recover() }()
	select {
	case t.events <- event:
	default:
	}
}

// NewProgressTracker creates a new ProgressTracker instance
func NewProgressTracker(id, url string, events chan<- ProgressEvent) ProgressTracker {
	return &baseTracker{
		id:     id,
		url:    url,
		events: events,
		progress: DownloadProgress{
			ID:  id,
			URL: url,
		},
	}
}

// Start initializes the tracker with the total size
func (t *baseTracker) Start(total int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.progress.TotalBytes = total
	t.progress.StartedAt = time.Now()
	t.progress.UpdatedAt = time.Now()

	t.trySend(ProgressEvent{
		Type:    EventDownloadStart,
		ID:      t.id,
		Message: "Download started",
		Current: 0,
		Total:   total,
	})
}

// Update updates the current progress
func (t *baseTracker) Update(current int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	bytesSinceLastUpdate := current - t.progress.DownloadedBytes
	timeSinceLastUpdate := now.Sub(t.progress.UpdatedAt).Seconds()

	if timeSinceLastUpdate > 0 {
		t.progress.Speed = float64(bytesSinceLastUpdate) / timeSinceLastUpdate
	}

	// Calculate ETA
	remainingBytes := t.progress.TotalBytes - current
	if t.progress.Speed > 0 && remainingBytes > 0 {
		t.progress.ETA = time.Duration(float64(remainingBytes)/t.progress.Speed) * time.Second
	}

	t.progress.DownloadedBytes = current
	t.progress.UpdatedAt = now

	t.trySend(ProgressEvent{
		Type:    EventDownloadProgress,
		ID:      t.id,
		Message: "Downloading...",
		Current: current,
		Total:   t.progress.TotalBytes,
	})
}

// Complete marks the download as successfully completed
func (t *baseTracker) Complete() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.progress.CompletedAt = time.Now()
	t.progress.DownloadedBytes = t.progress.TotalBytes

	t.trySend(ProgressEvent{
		Type:    EventDownloadComplete,
		ID:      t.id,
		Message: "Download complete",
		Current: t.progress.TotalBytes,
		Total:   t.progress.TotalBytes,
	})
}

// Error marks the download as failed
func (t *baseTracker) Error(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.progress.Error = err
	t.progress.CompletedAt = time.Now()

	t.trySend(ProgressEvent{
		Type:    EventDownloadError,
		ID:      t.id,
		Message: err.Error(),
		Current: t.progress.DownloadedBytes,
		Total:   t.progress.TotalBytes,
	})
}

// GetID returns the unique identifier
func (t *baseTracker) GetID() string {
	return t.id
}

// GetDownloadProgress returns the current progress state
func (t *baseTracker) GetDownloadProgress() DownloadProgress {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.progress
}
