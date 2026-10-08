package control

import (
	"context"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/crosskvm/crosskvm/internal/protocol"
	"github.com/crosskvm/crosskvm/internal/transport"
)

var (
	ErrHeartbeatTimeout = errors.New("connection: heartbeat timeout, peer unresponsive")
)

// ConnState represents the health and stage of the peer transport connection.
type ConnState string

const (
	ConnStateDisconnected ConnState = "disconnected"
	ConnStateConnecting   ConnState = "connecting"
	ConnStateConnected    ConnState = "connected"
	ConnStateReconnecting ConnState = "reconnecting"
)

// ConnectionConfig defines parameters for liveness detection and reconnection backoff.
type ConnectionConfig struct {
	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
}

// DefaultConnectionConfig provides sensible defaults for low-latency LAN operation.
func DefaultConnectionConfig() ConnectionConfig {
	return ConnectionConfig{
		HeartbeatInterval: 1 * time.Second,
		HeartbeatTimeout:  3500 * time.Millisecond,
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        10 * time.Second,
	}
}

// ConnectionManager oversees the peer connection lifecycle, heartbeats, and safe reconnection.
type ConnectionManager struct {
	mu            sync.RWMutex
	state         ConnState
	cfg           ConnectionConfig
	router        *Router
	metrics       *MetricsTracker
	logger        *log.Logger
	lastActivity  atomic.Int64 // unix nano
	stateChangeMu sync.Mutex
	stateHooks    []func(oldState, newState ConnState)

	ctx    context.Context
	cancel context.CancelFunc
}

// NewConnectionManager initializes a ConnectionManager.
func NewConnectionManager(router *Router, metrics *MetricsTracker, cfg ConnectionConfig, logger *log.Logger) *ConnectionManager {
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = 1 * time.Second
	}
	if cfg.HeartbeatTimeout <= 0 {
		cfg.HeartbeatTimeout = 3500 * time.Millisecond
	}
	if cfg.InitialBackoff <= 0 {
		cfg.InitialBackoff = 1 * time.Second
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 10 * time.Second
	}
	if metrics == nil {
		metrics = NewMetricsTracker()
	}

	ctx, cancel := context.WithCancel(context.Background())

	cm := &ConnectionManager{
		state:   ConnStateDisconnected,
		cfg:     cfg,
		router:  router,
		metrics: metrics,
		logger:  logger,
		ctx:     ctx,
		cancel:  cancel,
	}
	cm.lastActivity.Store(time.Now().UnixNano())

	return cm
}

// State returns current connection state.
func (cm *ConnectionManager) State() ConnState {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.state
}

// SetState updates the connection state and notifies listeners.
func (cm *ConnectionManager) SetState(newState ConnState) {
	cm.mu.Lock()
	oldState := cm.state
	if oldState == newState {
		cm.mu.Unlock()
		return
	}
	cm.state = newState
	cm.mu.Unlock()

	if cm.logger != nil {
		cm.logger.Printf("[CONN] State: %s -> %s", oldState, newState)
	}

	cm.stateChangeMu.Lock()
	hooks := make([]func(ConnState, ConnState), len(cm.stateHooks))
	copy(hooks, cm.stateHooks)
	cm.stateChangeMu.Unlock()

	for _, h := range hooks {
		h(oldState, newState)
	}
}

// OnStateChange registers a callback invoked when connection state changes.
func (cm *ConnectionManager) OnStateChange(fn func(oldState, newState ConnState)) {
	cm.stateChangeMu.Lock()
	defer cm.stateChangeMu.Unlock()
	cm.stateHooks = append(cm.stateHooks, fn)
}

// RecordActivity refreshes the last known message receive time from the peer.
func (cm *ConnectionManager) RecordActivity() {
	cm.lastActivity.Store(time.Now().UnixNano())
}

// StartHeartbeat starts periodic keepalive pings and liveness monitoring on a dedicated goroutine.
func (cm *ConnectionManager) StartHeartbeat(conn *transport.Conn) {
	go func() {
		ticker := time.NewTicker(cm.cfg.HeartbeatInterval)
		defer ticker.Stop()

		for {
			select {
			case <-cm.ctx.Done():
				return
			case <-ticker.C:
				if conn == nil || conn.IsClosed() {
					return
				}

				// Check liveness timeout
				lastNano := cm.lastActivity.Load()
				elapsed := time.Duration(time.Now().UnixNano() - lastNano)
				if elapsed > cm.cfg.HeartbeatTimeout {
					if cm.logger != nil {
						cm.logger.Printf("[WARN] Heartbeat timeout after %v without response. Restoring LOCAL.", elapsed)
					}
					// Dead peer detected! Trigger emergency disconnect recovery
					cm.handleDeadPeer(conn, ErrHeartbeatTimeout)
					return
				}

				// Send keepalive ping
				pingMsg := protocol.NewPingMessage(cm.router.seq.Add(1))
				cm.metrics.RecordPingSent(pingMsg.Seq)
				_ = conn.Send(pingMsg)
			}
		}
	}()
}

func (cm *ConnectionManager) handleDeadPeer(conn *transport.Conn, err error) {
	if conn != nil {
		_ = conn.Close()
	}

	// SAFETY INVARIANT: If no healthy peer exists, suppression MUST be false
	cm.router.HandleDisconnect(err)

	cm.SetState(ConnStateReconnecting)
}

// CalculateBackoff computes the exponential backoff duration for a given attempt count.
func (cm *ConnectionManager) CalculateBackoff(attempt int) time.Duration {
	backoff := cm.cfg.InitialBackoff
	for i := 1; i < attempt; i++ {
		backoff *= 2
		if backoff >= cm.cfg.MaxBackoff {
			return cm.cfg.MaxBackoff
		}
	}
	return backoff
}

// Metrics returns the underlying MetricsTracker.
func (cm *ConnectionManager) Metrics() *MetricsTracker {
	return cm.metrics
}

// Close gracefully terminates the connection manager and liveness workers.
func (cm *ConnectionManager) Close() error {
	cm.cancel()
	cm.SetState(ConnStateDisconnected)
	return nil
}
