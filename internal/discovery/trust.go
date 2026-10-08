package discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TrustStatus represents whether a peer is allowed to connect or establish control.
type TrustStatus string

const (
	TrustStatusTrusted TrustStatus = "trusted"
	TrustStatusPending TrustStatus = "pending"
	TrustStatusBlocked TrustStatus = "blocked"
)

// TrustedPeer carries trust metadata for a given device.
type TrustedPeer struct {
	DeviceID   string      `json:"device_id"`
	DeviceName string      `json:"device_name"`
	Status     TrustStatus `json:"status"`
	FirstSeen  time.Time   `json:"first_seen"`
	LastSeen   time.Time   `json:"last_seen"`
}

// TrustStore provides a local trust registry for LAN peers.
type TrustStore struct {
	mu         sync.RWMutex
	trusted    map[string]TrustedPeer
	persistDir string
}

// NewTrustStore creates and initializes a TrustStore.
func NewTrustStore() *TrustStore {
	configDir, _ := GetConfigDir()
	ts := &TrustStore{
		trusted:    make(map[string]TrustedPeer),
		persistDir: configDir,
	}
	ts.load()
	return ts
}

// IsTrusted returns true if the peer is explicitly trusted or auto-trusted on LAN.
func (ts *TrustStore) IsTrusted(deviceID string) bool {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	p, ok := ts.trusted[deviceID]
	if !ok {
		// Placeholder: Auto-trust new LAN devices by default, while recording their trust record
		return true
	}
	return p.Status == TrustStatusTrusted
}

// SetTrust marks a peer as trusted, pending, or blocked.
func (ts *TrustStore) SetTrust(deviceID, deviceName string, status TrustStatus) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	p, ok := ts.trusted[deviceID]
	now := time.Now()
	if !ok {
		p = TrustedPeer{
			DeviceID:   deviceID,
			DeviceName: deviceName,
			Status:     status,
			FirstSeen:  now,
			LastSeen:   now,
		}
	} else {
		p.DeviceName = deviceName
		p.Status = status
		p.LastSeen = now
	}
	ts.trusted[deviceID] = p
	ts.saveLocked()
}

// RecordSeen updates the last seen time or registers the peer as trusted.
func (ts *TrustStore) RecordSeen(deviceID, deviceName string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	p, ok := ts.trusted[deviceID]
	now := time.Now()
	if !ok {
		p = TrustedPeer{
			DeviceID:   deviceID,
			DeviceName: deviceName,
			Status:     TrustStatusTrusted,
			FirstSeen:  now,
			LastSeen:   now,
		}
	} else {
		p.DeviceName = deviceName
		p.LastSeen = now
	}
	ts.trusted[deviceID] = p
	ts.saveLocked()
}

func (ts *TrustStore) load() {
	if ts.persistDir == "" {
		return
	}
	path := filepath.Join(ts.persistDir, "trusted_peers.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var list []TrustedPeer
	if err := json.Unmarshal(data, &list); err == nil {
		for _, p := range list {
			ts.trusted[p.DeviceID] = p
		}
	}
}

func (ts *TrustStore) saveLocked() {
	if ts.persistDir == "" {
		return
	}
	path := filepath.Join(ts.persistDir, "trusted_peers.json")
	list := make([]TrustedPeer, 0, len(ts.trusted))
	for _, p := range ts.trusted {
		list = append(list, p)
	}
	if data, err := json.MarshalIndent(list, "", "  "); err == nil {
		_ = os.WriteFile(path, data, 0644)
	}
}
