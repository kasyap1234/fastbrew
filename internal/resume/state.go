package resume

import (
	"fmt"
	"time"
)

type DownloadState int

const (
	StatePending DownloadState = iota
	StateInProgress
	StateComplete
	StateFailed
)

func (ds DownloadState) String() string {
	switch ds {
	case StatePending:
		return "pending"
	case StateInProgress:
		return "in_progress"
	case StateComplete:
		return "complete"
	case StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

func ParseState(s string) DownloadState {
	switch s {
	case "pending":
		return StatePending
	case "in_progress":
		return StateInProgress
	case "complete":
		return StateComplete
	case "failed":
		return StateFailed
	default:
		return StateFailed
	}
}

var validTransitions = map[DownloadState][]DownloadState{
	StatePending:    {StateInProgress, StateFailed},
	StateInProgress: {StateComplete, StateFailed, StatePending},
	StateComplete:   {},
	StateFailed:     {StatePending, StateInProgress},
}

func ValidateStateTransition(from, to DownloadState) error {
	if from == to {
		return nil
	}

	validNextStates, exists := validTransitions[from]
	if !exists {
		return fmt.Errorf("invalid current state: %s", from.String())
	}

	for _, validState := range validNextStates {
		if validState == to {
			return nil
		}
	}

	return fmt.Errorf("invalid state transition from %s to %s", from.String(), to.String())
}

func IsTerminalState(state DownloadState) bool {
	return state == StateComplete || state == StateFailed
}

func CanResume(state DownloadState) bool {
	return state == StateFailed || state == StatePending
}

func IsActiveState(state DownloadState) bool {
	return state == StateInProgress
}

type StateTracker struct {
	CurrentState DownloadState
	History      []StateTransition
}

func NewStateTracker(initialState DownloadState) *StateTracker {
	now := time.Now()
	return &StateTracker{
		CurrentState: initialState,
		History: []StateTransition{
			{
				FromState: "",
				ToState:   initialState.String(),
				Timestamp: now,
			},
		},
	}
}

func (st *StateTracker) Transition(to DownloadState) error {
	if err := ValidateStateTransition(st.CurrentState, to); err != nil {
		return err
	}

	transition := StateTransition{
		FromState: st.CurrentState.String(),
		ToState:   to.String(),
		Timestamp: time.Now(),
	}

	st.History = append(st.History, transition)
	st.CurrentState = to

	return nil
}

func (st *StateTracker) GetTimeInState() time.Duration {
	if len(st.History) == 0 {
		return 0
	}

	lastTransition := st.History[len(st.History)-1]
	return time.Since(lastTransition.Timestamp)
}

func (st *StateTracker) GetTotalTime() time.Duration {
	if len(st.History) == 0 {
		return 0
	}

	return time.Since(st.History[0].Timestamp)
}

func (st *StateTracker) GetLastTransition() *StateTransition {
	if len(st.History) == 0 {
		return nil
	}
	transition := st.History[len(st.History)-1]
	return &transition
}

func (st *StateTracker) GetStateCount(state DownloadState) int {
	count := 0
	stateStr := state.String()

	for _, transition := range st.History {
		if transition.ToState == stateStr {
			count++
		}
	}

	return count
}
