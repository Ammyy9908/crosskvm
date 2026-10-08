package control

import (
	"fmt"
	"sync"
)

// State represents the current input ownership state.
type State string

const (
	// StateLocal indicates the local OS receives user input.
	StateLocal State = "local"
	// StateRemote indicates input is captured and forwarded to the remote peer.
	StateRemote State = "remote"
	// StateTransitioning indicates a handover between local and remote is in progress.
	StateTransitioning State = "transitioning"
)

// StateChangeListener is invoked whenever the control state changes.
type StateChangeListener func(oldState, newState State)

// StateManager manages thread-safe control transitions between local and remote nodes.
type StateManager struct {
	mu        sync.RWMutex
	current   State
	listeners []StateChangeListener
}

// NewStateManager creates a new StateManager initialized to StateLocal.
func NewStateManager() *StateManager {
	return &StateManager{
		current: StateLocal,
	}
}

// Current returns the current control state.
func (sm *StateManager) Current() State {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.current
}

// IsLocal returns true if control is currently local.
func (sm *StateManager) IsLocal() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.current == StateLocal
}

// IsRemote returns true if control is currently forwarded to remote peer.
func (sm *StateManager) IsRemote() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.current == StateRemote
}

// OnStateChange registers a callback to be notified of state transitions.
func (sm *StateManager) OnStateChange(listener StateChangeListener) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.listeners = append(sm.listeners, listener)
}

// Transition attempts to switch the control state.
func (sm *StateManager) Transition(target State) (bool, error) {
	sm.mu.Lock()
	oldState := sm.current
	if oldState == target {
		sm.mu.Unlock()
		return false, nil
	}

	sm.current = target
	listeners := make([]StateChangeListener, len(sm.listeners))
	copy(listeners, sm.listeners)
	sm.mu.Unlock()

	for _, l := range listeners {
		l(oldState, target)
	}

	return true, nil
}

// SwitchToRemote transitions control to the remote peer.
func (sm *StateManager) SwitchToRemote() error {
	_, err := sm.Transition(StateRemote)
	if err != nil {
		return fmt.Errorf("control: failed to switch to remote: %w", err)
	}
	return nil
}

// RestoreLocal transitions control back to the local machine (emergency return).
func (sm *StateManager) RestoreLocal() error {
	_, err := sm.Transition(StateLocal)
	if err != nil {
		return fmt.Errorf("control: failed to restore local control: %w", err)
	}
	return nil
}
