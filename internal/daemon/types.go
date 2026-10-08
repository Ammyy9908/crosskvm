package daemon

import (
	"encoding/json"
	"time"

	"github.com/crosskvm/crosskvm/internal/control"
	"github.com/crosskvm/crosskvm/internal/discovery"
	"github.com/crosskvm/crosskvm/internal/input"
)

// IPCRequest represents an incoming command from Electron.
type IPCRequest struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// IPCResponse represents the reply sent back to Electron.
type IPCResponse struct {
	ID     int64       `json:"id"`
	Result interface{} `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`
}

// IPCEvent represents an asynchronous push notification to Electron.
type IPCEvent struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

// LocalInfoResponse provides details about the local machine.
type LocalInfoResponse struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	OS              string `json:"os"`
	Arch            string `json:"arch"`
	ScreenWidth     int    `json:"screenWidth"`
	ScreenHeight    int    `json:"screenHeight"`
	ProtocolVersion string `json:"protocolVersion"`
	Port            int    `json:"port"`
}

// PeerInfo wraps discovery.Peer with UI-convenient fields.
type PeerInfo struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	OS              string    `json:"os"`
	Arch            string    `json:"arch"`
	Address         string    `json:"address"`
	IP              string    `json:"ip"`
	Port            int       `json:"port"`
	ScreenWidth     int       `json:"screenWidth"`
	ScreenHeight    int       `json:"screenHeight"`
	ProtocolVersion string    `json:"protocolVersion"`
	LastSeen        time.Time `json:"lastSeen"`
	Online          bool      `json:"online"`
}

// FromDiscoveryPeer converts discovery.Peer to PeerInfo.
func FromDiscoveryPeer(p discovery.Peer) PeerInfo {
	return PeerInfo{
		ID:              p.DeviceID,
		Name:            p.DeviceName,
		OS:              p.OS,
		Arch:            p.Arch,
		Address:         p.Address,
		IP:              p.IP,
		Port:            p.Port,
		ScreenWidth:     p.ScreenWidth,
		ScreenHeight:    p.ScreenHeight,
		ProtocolVersion: p.ProtocolVersion,
		LastSeen:        p.LastSeen,
		Online:          time.Since(p.LastSeen) < 10*time.Second,
	}
}

// StatusResponse encapsulates current KVM and connection state.
type StatusResponse struct {
	ConnectionState string             `json:"connectionState"` // disconnected, connecting, connected, reconnecting
	InputError      string             `json:"inputError,omitempty"`
	KVMActive       bool               `json:"kvmActive"`
	ControlState    string             `json:"controlState"` // local, remote, transitioning
	PeerSide        string             `json:"peerSide"`     // left, right, none
	CurrentPeer     *PeerInfo          `json:"currentPeer,omitempty"`
	LocalBounds     input.ScreenBounds `json:"localBounds"`
	RemoteBounds    input.ScreenBounds `json:"remoteBounds"`
}

// ConnectParams specifies parameters for the connect method.
type ConnectParams struct {
	PeerID string `json:"peerId,omitempty"`
	Addr   string `json:"addr,omitempty"`
}

// StartKVMParams specifies parameters for starting KVM routing.
type StartKVMParams struct {
	PeerID string `json:"peerId,omitempty"`
	Addr   string `json:"addr,omitempty"`
	Side   string `json:"side"` // "left" or "right"
}

// SetPeerSideParams specifies parameter for setPeerSide.
type SetPeerSideParams struct {
	Side string `json:"side"` // "left" or "right"
}

// ControlSwitchedEvent data payload.
type ControlSwitchedEvent struct {
	State string `json:"state"` // "local" or "remote"
	X     int    `json:"x"`
	Y     int    `json:"y"`
}

// ConnectedEvent data payload.
type ConnectedEvent struct {
	Peer PeerInfo `json:"peer"`
	Side string   `json:"side"`
}

// DisconnectedEvent data payload.
type DisconnectedEvent struct {
	Reason string `json:"reason"`
}

// ReconnectingEvent data payload.
type ReconnectingEvent struct {
	Attempt int `json:"attempt"`
}

// MetricsEvent data payload wraps control.MetricsSnapshot.
type MetricsEvent struct {
	control.MetricsSnapshot
}
