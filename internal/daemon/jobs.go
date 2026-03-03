package daemon

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type Job struct {
	mu sync.RWMutex

	id        string
	operation string
	packages  []string
	status    string
	errText   string

	submitted  time.Time
	startedAt  *time.Time
	finishedAt *time.Time

	events  []JobEvent
	nextSeq int
	waiters map[chan struct{}]struct{}

	done chan struct{}
}

func newJob(id, operation string, packages []string) *Job {
	j := &Job{
		id:        id,
		operation: operation,
		packages:  cloneSlice(packages),
		status:    JobStatusQueued,
		submitted: time.Now(),
		done:      make(chan struct{}),
		waiters:   make(map[chan struct{}]struct{}),
	}
	j.addEvent("info", fmt.Sprintf("Job queued: %s %v", operation, packages))
	return j
}

func (j *Job) markRunning() {
	j.mu.Lock()
	defer j.mu.Unlock()
	now := time.Now()
	j.startedAt = &now
	j.status = JobStatusRunning
	j.notifyWaitersLocked()
}

func (j *Job) markFinished(err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	now := time.Now()
	j.finishedAt = &now
	if err != nil {
		j.status = JobStatusFailed
		j.errText = err.Error()
		j.notifyWaitersLocked()
		return
	}
	j.status = JobStatusSucceeded
	j.notifyWaitersLocked()
}

func (j *Job) addEvent(level, message string) {
	j.addEventWithDetails(JobEvent{
		Level:     level,
		Message:   message,
		Kind:      JobEventKindJob,
		Operation: j.operation,
	})
}

func (j *Job) addPackageEvent(level, pkg, phase, status, message string, current, total *int64, unit string) {
	j.addEventWithDetails(JobEvent{
		Level:     level,
		Message:   message,
		Kind:      JobEventKindPackage,
		Operation: j.operation,
		Package:   pkg,
		Phase:     phase,
		Status:    status,
		Current:   current,
		Total:     total,
		Unit:      unit,
	})
}

func (j *Job) addEventWithDetails(event JobEvent) {
	j.mu.Lock()
	defer j.mu.Unlock()
	event.Seq = j.nextSeq
	event.Timestamp = time.Now()
	if event.Level == "" {
		event.Level = "info"
	}
	j.events = append(j.events, event)
	j.nextSeq++
	j.notifyWaitersLocked()
}

func (j *Job) notifyWaitersLocked() {
	for waiter := range j.waiters {
		close(waiter)
		delete(j.waiters, waiter)
	}
}

func (j *Job) snapshot() JobView {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.snapshotLocked()
}

func (j *Job) snapshotLocked() JobView {
	return JobView{
		ID:         j.id,
		Operation:  j.operation,
		Packages:   cloneSlice(j.packages),
		Status:     j.status,
		Error:      j.errText,
		Submitted:  j.submitted,
		StartedAt:  cloneTimePtr(j.startedAt),
		FinishedAt: cloneTimePtr(j.finishedAt),
	}
}

func (j *Job) eventsSince(fromSeq int) []JobEvent {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.eventsSinceLocked(fromSeq)
}

func (j *Job) eventsSinceLocked(fromSeq int) []JobEvent {
	if fromSeq < 0 {
		fromSeq = 0
	}
	if fromSeq >= len(j.events) {
		return nil
	}

	out := make([]JobEvent, len(j.events[fromSeq:]))
	copy(out, j.events[fromSeq:])
	return out
}

func (j *Job) isTerminalLocked() bool {
	return j.status == JobStatusSucceeded || j.status == JobStatusFailed
}

func (j *Job) stream(fromSeq int, blocking bool) (JobView, []JobEvent) {
	if !blocking {
		return j.snapshot(), j.eventsSince(fromSeq)
	}

	deadline := time.Now().Add(30 * time.Second)
	for {
		j.mu.Lock()
		view := j.snapshotLocked()
		events := j.eventsSinceLocked(fromSeq)
		if len(events) > 0 || j.isTerminalLocked() {
			j.mu.Unlock()
			return view, events
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			j.mu.Unlock()
			return view, nil
		}

		waiter := make(chan struct{})
		j.waiters[waiter] = struct{}{}
		j.mu.Unlock()

		timer := time.NewTimer(remaining)
		select {
		case <-waiter:
		case <-timer.C:
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}

		j.mu.Lock()
		delete(j.waiters, waiter)
		j.mu.Unlock()
	}
}

type JobManager struct {
	mu sync.RWMutex

	sequence atomic.Uint64
	jobs     map[string]*Job
}

type JobStats struct {
	Total   int
	Running int
	Failed  int
}

func NewJobManager() *JobManager {
	return &JobManager{
		jobs: make(map[string]*Job),
	}
}

func (m *JobManager) Submit(operation string, packages []string, runner func(*Job) error) *Job {
	id := fmt.Sprintf("job-%d-%d", time.Now().Unix(), m.sequence.Add(1))
	job := newJob(id, operation, packages)

	m.mu.Lock()
	m.jobs[id] = job
	m.mu.Unlock()

	go func() {
		job.markRunning()
		job.addEvent("info", fmt.Sprintf("Job started: %s", operation))
		err := runner(job)
		if err != nil {
			job.addEvent("error", err.Error())
		} else {
			job.addEvent("info", "Job completed successfully")
		}
		job.markFinished(err)
		close(job.done)
	}()

	return job
}

func (m *JobManager) Get(jobID string) (*Job, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	job, ok := m.jobs[jobID]
	return job, ok
}

func (m *JobManager) Status(jobID string) (JobView, bool) {
	job, ok := m.Get(jobID)
	if !ok {
		return JobView{}, false
	}
	return job.snapshot(), true
}

func (m *JobManager) Stream(jobID string, fromSeq int, blocking bool) (JobView, []JobEvent, bool) {
	job, ok := m.Get(jobID)
	if !ok {
		return JobView{}, nil, false
	}
	view, events := job.stream(fromSeq, blocking)
	return view, events, true
}

func (m *JobManager) Stats() JobStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := JobStats{
		Total: len(m.jobs),
	}
	for _, job := range m.jobs {
		view := job.snapshot()
		switch view.Status {
		case JobStatusRunning, JobStatusQueued:
			stats.Running++
		case JobStatusFailed:
			stats.Failed++
		}
	}
	return stats
}

func cloneTimePtr(in *time.Time) *time.Time {
	if in == nil {
		return nil
	}
	t := *in
	return &t
}
