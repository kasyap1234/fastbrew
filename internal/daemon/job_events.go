package daemon

import (
	"fmt"
	"sync"
	"time"

	"fastbrew/internal/brew"
	"fastbrew/internal/progress"
)

const (
	progressEventThrottlePct  = 1.0
	progressEventThrottleWait = 250 * time.Millisecond
)

type progressEmitState struct {
	lastPercent float64
	lastCurrent int64
	lastAt      time.Time
}

type progressThrottle struct {
	mu    sync.Mutex
	state map[string]progressEmitState
}

func newProgressThrottle() *progressThrottle {
	return &progressThrottle{
		state: make(map[string]progressEmitState),
	}
}

func (p *progressThrottle) shouldEmit(event progress.ProgressEvent) bool {
	if event.Type == progress.EventDownloadStart || event.Type == progress.EventDownloadComplete || event.Type == progress.EventDownloadError {
		p.mu.Lock()
		p.state[event.ID] = progressEmitState{
			lastPercent: event.CalculatePercentage(),
			lastCurrent: event.Current,
			lastAt:      time.Now(),
		}
		p.mu.Unlock()
		return true
	}

	now := time.Now()
	percent := event.CalculatePercentage()

	p.mu.Lock()
	defer p.mu.Unlock()

	state, ok := p.state[event.ID]
	if !ok {
		p.state[event.ID] = progressEmitState{
			lastPercent: percent,
			lastCurrent: event.Current,
			lastAt:      now,
		}
		return true
	}

	if event.Total > 0 && percent-state.lastPercent >= progressEventThrottlePct {
		p.state[event.ID] = progressEmitState{
			lastPercent: percent,
			lastCurrent: event.Current,
			lastAt:      now,
		}
		return true
	}

	if now.Sub(state.lastAt) >= progressEventThrottleWait {
		p.state[event.ID] = progressEmitState{
			lastPercent: percent,
			lastCurrent: event.Current,
			lastAt:      now,
		}
		return true
	}

	if event.Current < state.lastCurrent {
		p.state[event.ID] = progressEmitState{
			lastPercent: percent,
			lastCurrent: event.Current,
			lastAt:      now,
		}
		return true
	}

	return false
}

func mapProgressEventStatus(eventType progress.EventType) (string, string) {
	switch eventType {
	case progress.EventDownloadStart:
		return JobEventStatusRunning, "info"
	case progress.EventDownloadProgress:
		return JobEventStatusProgress, "info"
	case progress.EventDownloadComplete:
		return JobEventStatusSucceeded, "info"
	case progress.EventDownloadError:
		return JobEventStatusFailed, "error"
	default:
		return JobEventStatusProgress, "info"
	}
}

func mutationLevel(status string) string {
	switch status {
	case brew.MutationStatusFailed:
		return "error"
	case brew.MutationStatusSkipped:
		return "warn"
	default:
		return "info"
	}
}

func progressPointers(event progress.ProgressEvent) (*int64, *int64) {
	var currentPtr *int64
	var totalPtr *int64

	current := event.Current
	total := event.Total
	if current >= 0 {
		currentPtr = &current
	}
	if total >= 0 {
		totalPtr = &total
	}

	return currentPtr, totalPtr
}

func mutationPointers(event brew.MutationEvent) (*int64, *int64) {
	var currentPtr *int64
	var totalPtr *int64

	if event.Current != 0 || event.Total != 0 || event.Status == brew.MutationStatusProgress || event.Status == brew.MutationStatusRunning {
		current := event.Current
		total := event.Total
		currentPtr = &current
		totalPtr = &total
	}

	return currentPtr, totalPtr
}

func (s *Server) attachJobEventBridges(job *Job) func() {
	s.client.EnableProgress()

	pm := s.client.ProgressManager
	subID := fmt.Sprintf("job-progress-%s", job.id)
	progressCh := make(chan progress.ProgressEvent, 256)
	pm.SubscribeToEvents(subID, progressCh)

	throttle := newProgressThrottle()
	stopProgress := make(chan struct{})
	progressDone := make(chan struct{})

	go func() {
		defer close(progressDone)
		for {
			select {
			case <-stopProgress:
				return
			case event := <-progressCh:
				if !throttle.shouldEmit(event) {
					continue
				}

				status, level := mapProgressEventStatus(event.Type)
				currentPtr, totalPtr := progressPointers(event)
				job.addPackageEvent(
					level,
					event.ID,
					JobEventPhaseDownload,
					status,
					event.Message,
					currentPtr,
					totalPtr,
					"bytes",
				)
			}
		}
	}()

	s.client.SetMutationHook(func(event brew.MutationEvent) {
		if event.Package == "" {
			return
		}

		message := event.Message
		if message == "" {
			message = fmt.Sprintf("%s %s", event.Phase, event.Status)
		}

		currentPtr, totalPtr := mutationPointers(event)
		job.addPackageEvent(
			mutationLevel(event.Status),
			event.Package,
			event.Phase,
			event.Status,
			message,
			currentPtr,
			totalPtr,
			event.Unit,
		)
	})

	return func() {
		s.client.SetMutationHook(nil)
		pm.UnsubscribeFromEvents(subID)
		close(stopProgress)
		<-progressDone
	}
}
