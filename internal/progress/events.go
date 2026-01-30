package progress

import (
	"sync"
)

// EventType represents the type of progress event
type EventType string

const (
	// EventDownloadStart is sent when a download begins
	EventDownloadStart EventType = "download_start"
	// EventDownloadProgress is sent periodically during download
	EventDownloadProgress EventType = "download_progress"
	// EventDownloadComplete is sent when a download finishes successfully
	EventDownloadComplete EventType = "download_complete"
	// EventDownloadError is sent when a download fails
	EventDownloadError EventType = "download_error"
)

// ProgressEvent represents a single progress update event
type ProgressEvent struct {
	Type    EventType
	ID      string
	Message string
	Current int64
	Total   int64
}

// CalculatePercentage returns the progress percentage (0-100)
func (e ProgressEvent) CalculatePercentage() float64 {
	if e.Total <= 0 {
		return 0
	}
	percentage := float64(e.Current) / float64(e.Total) * 100
	if percentage > 100 {
		return 100
	}
	return percentage
}

// EventBus provides a thread-safe event channel system for progress updates
type EventBus struct {
	mu       sync.RWMutex
	channels map[string]chan<- ProgressEvent
}

// NewEventBus creates a new EventBus instance
func NewEventBus() *EventBus {
	return &EventBus{
		channels: make(map[string]chan<- ProgressEvent),
	}
}

// Subscribe registers a channel to receive progress events
func (eb *EventBus) Subscribe(id string, ch chan<- ProgressEvent) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.channels[id] = ch
}

// Unsubscribe removes a channel from receiving events
func (eb *EventBus) Unsubscribe(id string) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	delete(eb.channels, id)
}

// Publish sends an event to all subscribed channels
func (eb *EventBus) Publish(event ProgressEvent) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	for _, ch := range eb.channels {
		select {
		case ch <- event:
		default:
			// Channel is full, skip this subscriber
		}
	}
}

// GetSubscriberCount returns the number of active subscribers
func (eb *EventBus) GetSubscriberCount() int {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	return len(eb.channels)
}

// SafeEventChannel is a thread-safe wrapper around a progress event channel
type SafeEventChannel struct {
	mu     sync.Mutex
	ch     chan ProgressEvent
	closed bool
}

// NewSafeEventChannel creates a new SafeEventChannel with the specified buffer size
func NewSafeEventChannel(bufferSize int) *SafeEventChannel {
	return &SafeEventChannel{
		ch: make(chan ProgressEvent, bufferSize),
	}
}

// Send attempts to send an event, returns false if channel is closed or full
func (sec *SafeEventChannel) Send(event ProgressEvent) bool {
	sec.mu.Lock()
	defer sec.mu.Unlock()

	if sec.closed {
		return false
	}

	select {
	case sec.ch <- event:
		return true
	default:
		return false
	}
}

// Receive returns the underlying channel for receiving events
func (sec *SafeEventChannel) Receive() <-chan ProgressEvent {
	return sec.ch
}

// Close safely closes the channel
func (sec *SafeEventChannel) Close() {
	sec.mu.Lock()
	defer sec.mu.Unlock()

	if !sec.closed {
		close(sec.ch)
		sec.closed = true
	}
}

// IsClosed returns true if the channel is closed
func (sec *SafeEventChannel) IsClosed() bool {
	sec.mu.Lock()
	defer sec.mu.Unlock()
	return sec.closed
}
