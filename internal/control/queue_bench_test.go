package control

import (
	"testing"

	"github.com/crosskvm/crosskvm/internal/input"
	"github.com/crosskvm/crosskvm/internal/protocol"
)

func BenchmarkEventQueue_PushPop(b *testing.B) {
	q := NewEventQueue(2048)
	msg := protocol.NewInputMessage(1, input.NewMouseMoveEvent(10, 5))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		q.Enqueue(msg)
		q.Dequeue()
	}
}

func BenchmarkEventQueue_CoalescingUnderPressure(b *testing.B) {
	q := NewEventQueue(512)
	q.SetPressureThreshold(2) // low threshold so every subsequent move coalesces

	// Seed with 2 items to trigger pressure
	q.Enqueue(protocol.NewInputMessage(1, input.NewMouseMoveEvent(1, 1)))
	q.Enqueue(protocol.NewInputMessage(2, input.NewMouseMoveEvent(1, 1)))

	move := protocol.NewInputMessage(3, input.NewMouseMoveEvent(2, 3))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		q.Enqueue(move)
	}
}

func BenchmarkEventQueue_NoCoalescingNormal(b *testing.B) {
	q := NewEventQueue(2048)
	q.SetPressureThreshold(100) // high threshold: normal LAN operations (depth ~ 0-5)
	msg := protocol.NewInputMessage(1, input.NewMouseMoveEvent(2, 3))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		q.Enqueue(msg)
		if q.Len() > 5 {
			q.Dequeue()
		}
	}
}
