package control

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/crosskvm/crosskvm/internal/input"
	"github.com/crosskvm/crosskvm/internal/protocol"
)

// Default constants for queue tuning.
const (
	DefaultQueueCapacity          = 1024
	DefaultCoalescePressureLimit = 8
)

// EventQueue protects the network pipe by maintaining strict FIFO ordering for all events
// and coalescing high-frequency relative mouse moves only when the queue is under backpressure.
// Critical control handoffs, mouse clicks, and keyboard events are NEVER coalesced or dropped.
type EventQueue struct {
	mu                sync.Mutex
	items             []protocol.Message
	cond              *sync.Cond
	capacity          int
	pressureThreshold int
	closed            bool
	droppedMoves      atomic.Uint64
	coalescedMoves    atomic.Uint64
}

// NewEventQueue creates an EventQueue with the specified capacity and default pressure threshold.
func NewEventQueue(capacity int) *EventQueue {
	if capacity <= 0 {
		capacity = DefaultQueueCapacity
	}
	eq := &EventQueue{
		capacity:          capacity,
		pressureThreshold: DefaultCoalescePressureLimit,
		items:             make([]protocol.Message, 0, 64),
	}
	eq.cond = sync.NewCond(&eq.mu)
	return eq
}

// SetPressureThreshold adjusts the depth at which mouse movement coalescing begins.
func (eq *EventQueue) SetPressureThreshold(threshold int) {
	eq.mu.Lock()
	defer eq.mu.Unlock()
	if threshold > 0 {
		eq.pressureThreshold = threshold
	}
}

// IsCritical returns true if the message must NEVER be coalesced or dropped.
func IsCritical(msg protocol.Message) bool {
	switch msg.Type {
	case protocol.MessageTypeSwitchControl,
		protocol.MessageTypeReleaseControl,
		protocol.MessageTypeHandshake,
		protocol.MessageTypeHandshakeAck,
		protocol.MessageTypePing,
		protocol.MessageTypePong:
		return true
	case protocol.MessageTypeInput:
		if msg.Input == nil {
			return false
		}
		switch msg.Input.Type {
		case input.EventTypeKeyDown,
			input.EventTypeKeyUp,
			input.EventTypeMouseButtonDown,
			input.EventTypeMouseButtonUp:
			return true
		default:
			return false
		}
	default:
		return false
	}
}

// Enqueue inserts a message into the FIFO queue.
// Preserves absolute chronological ordering across all events.
// Coalescing is applied ONLY to adjacent relative mouse moves when the queue is under pressure.
func (eq *EventQueue) Enqueue(msg protocol.Message) bool {
	eq.mu.Lock()
	defer eq.mu.Unlock()

	if eq.closed {
		return false
	}

	now := time.Now().UnixNano()
	if msg.Timestamp == 0 {
		msg.Timestamp = now
	}
	msg.QueueTime = now

	// 1. Critical events (keys, clicks, handoffs) are never dropped or coalesced
	if IsCritical(msg) {
		eq.items = append(eq.items, msg)
		eq.cond.Signal()
		return true
	}

	// 2. Relative mouse move: coalesce only if queue is under pressure and immediately follows another mouse move
	if msg.Type == protocol.MessageTypeInput && msg.Input != nil && msg.Input.Type == input.EventTypeMouseMove {
		if len(eq.items) >= eq.pressureThreshold && len(eq.items) > 0 {
			lastIdx := len(eq.items) - 1
			lastMsg := eq.items[lastIdx]
			if lastMsg.Type == protocol.MessageTypeInput && lastMsg.Input != nil && lastMsg.Input.Type == input.EventTypeMouseMove {
				// Coalesce DX/DY deltas into single pending move to eliminate queue lag
				lastMsg.Input.DX += msg.Input.DX
				lastMsg.Input.DY += msg.Input.DY
				lastMsg.Timestamp = msg.Timestamp
				eq.items[lastIdx] = lastMsg
				eq.coalescedMoves.Add(1)
				return true
			}
		}

		// If queue is full, drop excess mouse move
		if len(eq.items) >= eq.capacity {
			eq.droppedMoves.Add(1)
			return false
		}
	}

	// Normal enqueue
	if len(eq.items) >= eq.capacity {
		eq.droppedMoves.Add(1)
		return false
	}

	eq.items = append(eq.items, msg)
	eq.cond.Signal()
	return true
}

// Dequeue blocks until a message is available or the queue is closed.
// Guarantees strict FIFO dequeuing.
func (eq *EventQueue) Dequeue() (protocol.Message, bool) {
	eq.mu.Lock()
	defer eq.mu.Unlock()

	for len(eq.items) == 0 && !eq.closed {
		eq.cond.Wait()
	}

	if len(eq.items) == 0 && eq.closed {
		return protocol.Message{}, false
	}

	msg := eq.items[0]
	eq.items = eq.items[1:]

	// Periodic slice compaction if underlying array grew excessively large and queue drained
	if len(eq.items) == 0 && cap(eq.items) > 2048 {
		eq.items = make([]protocol.Message, 0, 64)
	}

	return msg, true
}

// Len returns total pending items in the queue.
func (eq *EventQueue) Len() int {
	eq.mu.Lock()
	defer eq.mu.Unlock()
	return len(eq.items)
}

// DroppedCount returns number of dropped mouse moves.
func (eq *EventQueue) DroppedCount() uint64 {
	return eq.droppedMoves.Load()
}

// Drops returns number of dropped mouse moves (alias for DroppedCount).
func (eq *EventQueue) Drops() uint64 {
	return eq.droppedMoves.Load()
}

// CoalescedCount returns number of coalesced mouse moves.
func (eq *EventQueue) CoalescedCount() uint64 {
	return eq.coalescedMoves.Load()
}

// Close closes the queue and wakes up blocked dequeue workers.
func (eq *EventQueue) Close() {
	eq.mu.Lock()
	defer eq.mu.Unlock()
	if eq.closed {
		return
	}
	eq.closed = true
	eq.cond.Broadcast()
}
