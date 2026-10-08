package control

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// LatencyStats contains statistical distribution metrics (in milliseconds).
type LatencyStats struct {
	LastMs float64
	AvgMs  float64
	MinMs  float64
	MaxMs  float64
	P50Ms  float64
	P95Ms  float64
	P99Ms  float64
	Count  uint64
}

// SampleReservoir maintains a circular buffer of recent duration samples for percentile calculation.
type SampleReservoir struct {
	mu      sync.Mutex
	samples []int64 // in microseconds
	idx     int
	count   uint64
	sum     int64
	min     int64
	max     int64
	last    int64
}

// NewSampleReservoir creates a reservoir with the given capacity.
func NewSampleReservoir(size int) *SampleReservoir {
	if size <= 0 {
		size = 1024
	}
	return &SampleReservoir{
		samples: make([]int64, size),
	}
}

// Record inserts a latency duration in microseconds into the reservoir.
func (r *SampleReservoir) Record(us int64) {
	if us <= 0 {
		return
	}
	r.mu.Lock()
	r.samples[r.idx] = us
	r.idx = (r.idx + 1) % len(r.samples)
	r.count++
	r.sum += us
	r.last = us
	if r.min == 0 || us < r.min {
		r.min = us
	}
	if us > r.max {
		r.max = us
	}
	r.mu.Unlock()
}

// Stats computes summary statistics and percentiles without mutating the underlying circular buffer.
func (r *SampleReservoir) Stats() LatencyStats {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.count == 0 {
		return LatencyStats{}
	}

	n := int(r.count)
	if n > len(r.samples) {
		n = len(r.samples)
	}

	cpy := make([]int64, n)
	copy(cpy, r.samples[:n])
	sort.Slice(cpy, func(i, j int) bool { return cpy[i] < cpy[j] })

	p50 := cpy[n*50/100]
	p95 := cpy[n*95/100]
	p99 := cpy[n*99/100]
	if n*99/100 >= n {
		p99 = cpy[n-1]
	}

	avgUs := r.sum / int64(r.count)

	return LatencyStats{
		LastMs: float64(r.last) / 1000.0,
		AvgMs:  float64(avgUs) / 1000.0,
		MinMs:  float64(r.min) / 1000.0,
		MaxMs:  float64(r.max) / 1000.0,
		P50Ms:  float64(p50) / 1000.0,
		P95Ms:  float64(p95) / 1000.0,
		P99Ms:  float64(p99) / 1000.0,
		Count:  r.count,
	}
}

// RateCounter computes sliding-window events-per-second rates.
type RateCounter struct {
	count     atomic.Uint64
	lastRate  atomic.Uint64
	lastReset atomic.Int64
}

// Inc increments the event counter and refreshes the rate window.
func (rc *RateCounter) Inc() {
	now := time.Now().UnixNano()
	last := rc.lastReset.Load()
	if last == 0 {
		rc.lastReset.Store(now)
	} else if now-last >= int64(time.Second) {
		if rc.lastReset.CompareAndSwap(last, now) {
			curr := rc.count.Swap(0)
			rc.lastRate.Store(curr)
		}
	}
	rc.count.Add(1)
}

// Rate returns the latest measured events per second.
func (rc *RateCounter) Rate() uint64 {
	now := time.Now().UnixNano()
	last := rc.lastReset.Load()
	if last > 0 && now-last > int64(2*time.Second) {
		rc.lastRate.Store(0)
	}
	return rc.lastRate.Load()
}

// MetricsTracker aggregates detailed pipeline stage latencies, event rates, and network performance.
type MetricsTracker struct {
	reconnectAttempts atomic.Uint64
	totalSent         atomic.Uint64
	totalRecv         atomic.Uint64
	lastRTTUs         atomic.Int64 // microseconds
	avgLatencyUs      atomic.Int64 // microseconds (one-way LAN estimate)
	clockOffsetNs     atomic.Int64 // estimated clock offset between peers

	// Pending ping tracking
	pingMu        sync.Mutex
	pingSentTimes map[uint64]int64

	// Stage latency reservoirs
	rttReservoir       *SampleReservoir
	injectionReservoir *SampleReservoir
	endToEndReservoir  *SampleReservoir
	queueReservoir     *SampleReservoir

	// Event rate counters
	captureRate RateCounter
	mouseRate   RateCounter
}

// NewMetricsTracker creates an initialized MetricsTracker.
func NewMetricsTracker() *MetricsTracker {
	return &MetricsTracker{
		pingSentTimes:      make(map[uint64]int64),
		rttReservoir:       NewSampleReservoir(1024),
		injectionReservoir: NewSampleReservoir(1024),
		endToEndReservoir:  NewSampleReservoir(1024),
		queueReservoir:     NewSampleReservoir(1024),
	}
}

// RecordPingSent records the timestamp when a keepalive ping was sent.
func (mt *MetricsTracker) RecordPingSent(seq uint64) {
	now := time.Now().UnixNano()
	mt.pingMu.Lock()
	mt.pingSentTimes[seq] = now
	// Clean up stale pings if map grows too large
	if len(mt.pingSentTimes) > 64 {
		for k, t := range mt.pingSentTimes {
			if now-t > int64(10*time.Second) {
				delete(mt.pingSentTimes, k)
			}
		}
	}
	mt.pingMu.Unlock()
}

// RecordPongRecv records the return of a keepalive pong and computes round-trip latency and clock offset.
func (mt *MetricsTracker) RecordPongRecv(seq uint64) time.Duration {
	return mt.RecordPongRecvWithRemoteTime(seq, 0)
}

// RecordPongRecvWithRemoteTime records pong return along with remote machine's timestamp for clock sync.
func (mt *MetricsTracker) RecordPongRecvWithRemoteTime(seq uint64, remoteTime int64) time.Duration {
	mt.pingMu.Lock()
	sentTime, ok := mt.pingSentTimes[seq]
	if ok {
		delete(mt.pingSentTimes, seq)
	}
	mt.pingMu.Unlock()

	if !ok {
		return 0
	}

	recvTime := time.Now().UnixNano()
	rtt := time.Duration(recvTime - sentTime)
	if rtt <= 0 {
		return 0
	}

	rttUs := rtt.Microseconds()
	mt.lastRTTUs.Store(rttUs)
	mt.rttReservoir.Record(rttUs)
	mt.RecordLatency(rtt / 2) // one-way approximate transit

	// Calibrate clock offset between local and remote system clocks:
	// theta = remoteTime - (sentTime + recvTime)/2
	if remoteTime > 0 {
		midpoint := (sentTime + recvTime) / 2
		offset := remoteTime - midpoint
		mt.clockOffsetNs.Store(offset)
	}

	return rtt
}

// RecordLatency updates the exponential moving average of one-way latency.
func (mt *MetricsTracker) RecordLatency(d time.Duration) {
	if d <= 0 {
		return
	}
	us := d.Microseconds()
	mt.lastRTTUs.Store(us * 2)

	prev := mt.avgLatencyUs.Load()
	if prev == 0 {
		mt.avgLatencyUs.Store(us)
	} else {
		// Exponential moving average: 80% old + 20% new
		updated := (prev*4 + us) / 5
		mt.avgLatencyUs.Store(updated)
	}
}

// RecordQueueDwell records outbound event queue wait time.
func (mt *MetricsTracker) RecordQueueDwell(d time.Duration) {
	if d > 0 {
		mt.queueReservoir.Record(d.Microseconds())
	}
}

// RecordInjectionLatency records native OS injection latency.
func (mt *MetricsTracker) RecordInjectionLatency(d time.Duration) {
	if d > 0 {
		mt.injectionReservoir.Record(d.Microseconds())
	}
}

// RecordEndToEndLatency records full capture-to-injection latency.
func (mt *MetricsTracker) RecordEndToEndLatency(d time.Duration) {
	if d > 0 {
		mt.endToEndReservoir.Record(d.Microseconds())
	}
}

// RecordCaptureEvent increments the capture event rate counter.
func (mt *MetricsTracker) RecordCaptureEvent() {
	mt.captureRate.Inc()
}

// RecordMouseEvent increments the mouse event rate counter.
func (mt *MetricsTracker) RecordMouseEvent() {
	mt.mouseRate.Inc()
}

// RecordSent increments total transmitted message count.
func (mt *MetricsTracker) RecordSent() {
	mt.totalSent.Add(1)
}

// RecordRecv increments total received message count.
func (mt *MetricsTracker) RecordRecv() {
	mt.totalRecv.Add(1)
}

// RecordReconnectAttempt increments reconnect count.
func (mt *MetricsTracker) RecordReconnectAttempt() {
	mt.reconnectAttempts.Add(1)
}

// AverageLatency returns the smoothed one-way LAN latency.
func (mt *MetricsTracker) AverageLatency() time.Duration {
	return time.Duration(mt.avgLatencyUs.Load()) * time.Microsecond
}

// ClockOffset returns estimated clock offset relative to remote peer.
func (mt *MetricsTracker) ClockOffset() time.Duration {
	return time.Duration(mt.clockOffsetNs.Load()) * time.Nanosecond
}

// ReconnectAttempts returns total reconnect attempts.
func (mt *MetricsTracker) ReconnectAttempts() uint64 {
	return mt.reconnectAttempts.Load()
}

// MetricsSnapshot represents an immutable point-in-time metrics report with full stage distributions.
type MetricsSnapshot struct {
	AvgLatencyMs      float64 // smoothed one-way network flight time
	LastRTTMs         float64 // last round trip time
	SentCount         uint64
	RecvCount         uint64
	ReconnectAttempts uint64

	// Stage latency distributions
	RTTStats        LatencyStats
	InjectionStats  LatencyStats
	EndToEndStats   LatencyStats
	QueueDwellStats LatencyStats

	// Event rates (events/second)
	CaptureRateSec uint64
	MouseRateSec   uint64

	// Queue stats
	QueueDepth     int
	CoalescedMoves uint64
	DroppedMoves   uint64
}

// Snapshot returns a copy of the current metrics.
func (mt *MetricsTracker) Snapshot() MetricsSnapshot {
	avgUs := mt.avgLatencyUs.Load()
	rttUs := mt.lastRTTUs.Load()

	return MetricsSnapshot{
		AvgLatencyMs:      float64(avgUs) / 1000.0,
		LastRTTMs:         float64(rttUs) / 1000.0,
		SentCount:         mt.totalSent.Load(),
		RecvCount:         mt.totalRecv.Load(),
		ReconnectAttempts: mt.reconnectAttempts.Load(),

		RTTStats:        mt.rttReservoir.Stats(),
		InjectionStats:  mt.injectionReservoir.Stats(),
		EndToEndStats:   mt.endToEndReservoir.Stats(),
		QueueDwellStats: mt.queueReservoir.Stats(),

		CaptureRateSec: mt.captureRate.Rate(),
		MouseRateSec:   mt.mouseRate.Rate(),
	}
}

// Summary returns a human-readable string of the current metrics.
func (mt *MetricsTracker) Summary() string {
	snap := mt.Snapshot()
	e2e := snap.EndToEndStats
	if e2e.Count > 0 {
		return fmt.Sprintf("E2E: avg=%.2fms (p95=%.2fms), RTT=%.2fms, Inj=%.2fms, Rate=%d/s",
			e2e.AvgMs, e2e.P95Ms, snap.LastRTTMs, snap.InjectionStats.AvgMs, snap.MouseRateSec)
	}
	return fmt.Sprintf("Latency: %.2fms, RTT: %.2fms, Sent: %d, Recv: %d",
		snap.AvgLatencyMs, snap.LastRTTMs, snap.SentCount, snap.RecvCount)
}
