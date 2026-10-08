package main

import (
	"testing"

	"github.com/crosskvm/crosskvm/internal/control"
)

func TestMain_Banner(t *testing.T) {
	if len(banner) == 0 {
		t.Errorf("Banner string should not be empty")
	}
}

func TestMain_PrintMetricsDashboard(t *testing.T) {
	snap := control.MetricsSnapshot{
		AvgLatencyMs:      1.25,
		LastRTTMs:         0.85,
		SentCount:         150,
		RecvCount:         145,
		ReconnectAttempts: 0,
		CaptureRateSec:    250,
		MouseRateSec:      245,
		EndToEndStats: control.LatencyStats{
			AvgMs:  1.35,
			P50Ms:  1.20,
			P95Ms:  1.80,
			P99Ms:  2.10,
			MinMs:  0.80,
			MaxMs:  2.50,
			LastMs: 1.25,
			Count:  100,
		},
		InjectionStats: control.LatencyStats{
			AvgMs:  0.45,
			P50Ms:  0.40,
			P95Ms:  0.65,
			P99Ms:  0.85,
			Count:  100,
		},
		QueueDwellStats: control.LatencyStats{
			AvgMs: 0.08,
			P50Ms: 0.05,
			P95Ms: 0.15,
			Count: 100,
		},
		RTTStats: control.LatencyStats{
			AvgMs: 0.85,
			P50Ms: 0.80,
			P95Ms: 1.10,
			Count: 50,
		},
	}

	// Verify printMetricsDashboard does not panic
	printMetricsDashboard(snap, "connected", 0, 0, 0)
}
