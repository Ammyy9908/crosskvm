package discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Peer represents a discovered CrossKVM instance on the local network.
type Peer struct {
	DeviceID        string    `json:"device_id"`
	DeviceName      string    `json:"device_name"`
	OS              string    `json:"os"`
	Arch            string    `json:"arch"`
	Port            int       `json:"port"`
	Address         string    `json:"address"` // e.g. "192.168.1.5:4545"
	IP              string    `json:"ip"`      // e.g. "192.168.1.5"
	ScreenWidth     int       `json:"screen_width"`
	ScreenHeight    int       `json:"screen_height"`
	ProtocolVersion string    `json:"protocol_version"`
	LastSeen        time.Time `json:"last_seen"`
}

// PeerStore maintains a thread-safe registry of discovered network peers.
type PeerStore struct {
	mu         sync.RWMutex
	peers      map[string]Peer // keyed by DeviceID
	persistDir string
}

// NewPeerStore creates a new PeerStore with optional file persistence.
func NewPeerStore() *PeerStore {
	configDir, _ := GetConfigDir()
	store := &PeerStore{
		peers:      make(map[string]Peer),
		persistDir: configDir,
	}
	store.loadPersistedPeers()
	return store
}

// NewEmptyPeerStore creates a purely in-memory PeerStore without loading existing persisted files.
func NewEmptyPeerStore() *PeerStore {
	return &PeerStore{
		peers: make(map[string]Peer),
	}
}

// AddOrUpdate inserts or updates a discovered peer. Returns true if newly added, false if updated.
func (ps *PeerStore) AddOrUpdate(p Peer) bool {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	p.DeviceID = strings.TrimSpace(p.DeviceID)
	if p.DeviceID == "" {
		return false
	}
	if p.LastSeen.IsZero() {
		p.LastSeen = time.Now()
	}

	_, exists := ps.peers[p.DeviceID]
	ps.peers[p.DeviceID] = p

	ps.savePersistedPeersLocked()
	return !exists
}

// Get looks up a peer by exact DeviceID.
func (ps *PeerStore) Get(deviceID string) (Peer, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	p, ok := ps.peers[deviceID]
	return p, ok
}

// FindByNameOrID looks up a peer by exact DeviceID, prefix ID, or DeviceName.
func (ps *PeerStore) FindByNameOrID(query string) (Peer, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	cleanQuery := strings.ToLower(strings.TrimSpace(query))
	if cleanQuery == "" {
		return Peer{}, false
	}

	// 1. Exact DeviceID match
	if p, ok := ps.peers[query]; ok {
		return p, true
	}

	// 2. Case-insensitive DeviceName match
	for _, p := range ps.peers {
		if strings.ToLower(p.DeviceName) == cleanQuery {
			return p, true
		}
	}

	// 3. Prefix ID match
	for id, p := range ps.peers {
		if strings.HasPrefix(strings.ToLower(id), cleanQuery) {
			return p, true
		}
	}

	return Peer{}, false
}

// List returns a snapshot slice of all known peers sorted by last seen.
func (ps *PeerStore) List() []Peer {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	result := make([]Peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		result = append(result, p)
	}
	return result
}

// Remove removes a peer from the store.
func (ps *PeerStore) Remove(deviceID string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.peers, deviceID)
	ps.savePersistedPeersLocked()
}

// Prune removes peers older than the given max age.
func (ps *PeerStore) Prune(maxAge time.Duration) int {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	now := time.Now()
	pruned := 0
	for id, p := range ps.peers {
		if now.Sub(p.LastSeen) > maxAge {
			delete(ps.peers, id)
			pruned++
		}
	}
	if pruned > 0 {
		ps.savePersistedPeersLocked()
	}
	return pruned
}

func (ps *PeerStore) loadPersistedPeers() {
	if ps.persistDir == "" {
		return
	}
	path := filepath.Join(ps.persistDir, "known_peers.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var list []Peer
	if err := json.Unmarshal(data, &list); err == nil {
		for _, p := range list {
			ps.peers[p.DeviceID] = p
		}
	}
}

func (ps *PeerStore) savePersistedPeersLocked() {
	if ps.persistDir == "" {
		return
	}
	path := filepath.Join(ps.persistDir, "known_peers.json")
	list := make([]Peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		list = append(list, p)
	}
	if data, err := json.MarshalIndent(list, "", "  "); err == nil {
		_ = os.WriteFile(path, data, 0644)
	}
}
