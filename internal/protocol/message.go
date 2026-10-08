package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/crosskvm/crosskvm/internal/input"
)

// CurrentProtocolVersion defines the supported wire protocol version.
const CurrentProtocolVersion = "1.0.0"

// MessageType indicates the nature of the protocol packet.
type MessageType string

const (
	MessageTypeFileTransfer   MessageType = "file_transfer"
	MessageTypeClipboardImage MessageType = "clipboard_image"
	MessageTypeClipboardText  MessageType = "clipboard_text"
	MessageTypeInput          MessageType = "input"
	MessageTypePing           MessageType = "ping"
	MessageTypePong           MessageType = "pong"
	MessageTypeHandshake      MessageType = "handshake"
	MessageTypeHandshakeAck   MessageType = "handshake_ack"
	MessageTypeSwitchControl  MessageType = "switch_control"
	MessageTypeReleaseControl MessageType = "release_control"
	MessageTypeLayoutUpdate   MessageType = "layout_update"
	MessageTypeError          MessageType = "error"
)

var (
	ErrUnknownMessageType          = errors.New("protocol: unknown message type")
	ErrMissingInputEvent           = errors.New("protocol: input message missing input event payload")
	ErrIncompatibleProtocolVersion = errors.New("protocol: incompatible protocol version")
	ErrInvalidHandshakePayload     = errors.New("protocol: invalid handshake payload")
)

// HandshakePayload represents initial connection negotiation data, including screen bounds and identity.
type HandshakePayload struct {
	Version      string   `json:"version"`
	DeviceID     string   `json:"device_id,omitempty"`
	PeerName     string   `json:"peer_name,omitempty"`
	OS           string   `json:"os,omitempty"`
	Arch         string   `json:"arch,omitempty"`
	ScreenWidth  int      `json:"screen_width,omitempty"`
	ScreenHeight int      `json:"screen_height,omitempty"`
	PeerSide     string   `json:"peer_side,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Timestamp    int64    `json:"timestamp"`
}

// HandshakeAckPayload represents the acknowledgement response to a handshake, including responder identity and screen bounds.
type HandshakeAckPayload struct {
	Success      bool     `json:"success"`
	Version      string   `json:"version"`
	DeviceID     string   `json:"device_id,omitempty"`
	Reason       string   `json:"reason,omitempty"`
	ScreenWidth  int      `json:"screen_width,omitempty"`
	ScreenHeight int      `json:"screen_height,omitempty"`
	PeerSide     string   `json:"peer_side,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// LayoutPayload carries screen arrangement layout updates between peers.
type LayoutPayload struct {
	PeerSide string `json:"peer_side"`
}

// IsCompatibleVersion checks if the peer protocol version is compatible with our version (matching major version).
func IsCompatibleVersion(peerVersion string) bool {
	if peerVersion == "" {
		return false
	}
	ourMajor := strings.Split(CurrentProtocolVersion, ".")[0]
	peerMajor := strings.Split(peerVersion, ".")[0]
	return ourMajor == peerMajor
}

// ValidateHandshake validates essential fields of an incoming HandshakePayload.
func ValidateHandshake(p HandshakePayload) error {
	if !IsCompatibleVersion(p.Version) {
		return fmt.Errorf("%w: peer version %s is incompatible with local %s",
			ErrIncompatibleProtocolVersion, p.Version, CurrentProtocolVersion)
	}
	if p.ScreenWidth <= 0 || p.ScreenHeight <= 0 {
		return fmt.Errorf("%w: invalid screen geometry (%dx%d)",
			ErrInvalidHandshakePayload, p.ScreenWidth, p.ScreenHeight)
	}
	return nil
}

// ControlPayload carries information about control transitions.
type ControlPayload struct {
	Side        string  `json:"side,omitempty"` // e.g. "left", "right"
	NormalizedX float64 `json:"normalized_x,omitempty"`
	NormalizedY float64 `json:"normalized_y,omitempty"`
	TargetX     int     `json:"target_x,omitempty"`
	TargetY     int     `json:"target_y,omitempty"`
	Reason      string  `json:"reason,omitempty"`
}

// ErrorPayload carries error details between peers.
type ErrorPayload struct {
	Message string `json:"message"`
	Code    int    `json:"code,omitempty"`
}

// Message represents the standard protocol envelope exchanged over the network.
type Message struct {
	SourceOS  string            `json:"source_os,omitempty"`
	Type      MessageType       `json:"type"`
	Seq       uint64            `json:"seq"`
	Timestamp int64             `json:"timestamp"`
	Input     *input.InputEvent `json:"input,omitempty"`
	Payload   json.RawMessage   `json:"payload,omitempty"`

	// Pipeline timestamps (nanoseconds) for microsecond-accurate latency profiling
	QueueTime int64 `json:"q_time,omitempty"` // UnixNano when enqueued into outbound queue
	WriteTime int64 `json:"w_time,omitempty"` // UnixNano just before TCP socket write
}

// PongPayload carries timing information for clock-synchronization and RTT measurement.
type PongPayload struct {
	PingSentTime int64 `json:"ping_sent_time"`
	RemoteTime   int64 `json:"remote_time"`
}

// NewInputMessage wraps a platform-neutral InputEvent into a protocol Message.
func NewInputMessage(seq uint64, event input.InputEvent) Message {
	return Message{
		Type:      MessageTypeInput,
		SourceOS:  runtime.GOOS,
		Seq:       seq,
		Timestamp: time.Now().UnixNano(),
		Input:     &event,
	}
}

// NewPingMessage constructs a keepalive ping.
func NewPingMessage(seq uint64) Message {
	return Message{
		Type:      MessageTypePing,
		Seq:       seq,
		Timestamp: time.Now().UnixNano(),
	}
}

// NewPongMessage constructs a keepalive pong in response to a ping.
func NewPongMessage(seq uint64) Message {
	return Message{
		Type:      MessageTypePong,
		Seq:       seq,
		Timestamp: time.Now().UnixNano(),
	}
}

// NewPongMessageWithTiming constructs a keepalive pong with round-trip and remote timing information.
func NewPongMessageWithTiming(seq uint64, pingSentTime int64) Message {
	payloadBytes, _ := json.Marshal(PongPayload{
		PingSentTime: pingSentTime,
		RemoteTime:   time.Now().UnixNano(),
	})
	return Message{
		Type:      MessageTypePong,
		Seq:       seq,
		Timestamp: time.Now().UnixNano(),
		Payload:   payloadBytes,
	}
}

// NewHandshakeMessage constructs a handshake message with peer screen bounds.
func NewHandshakeMessage(seq uint64, peerName, osName, arch string, screenWidth, screenHeight int) (Message, error) {
	return NewHandshakeMessageWithDevice(seq, "", peerName, osName, arch, screenWidth, screenHeight, []string{"input", "control"})
}

// NewHandshakeMessageWithDevice constructs a handshake message including device ID, capabilities and screen bounds.
func NewHandshakeMessageWithDevice(seq uint64, deviceID, peerName, osName, arch string, screenWidth, screenHeight int, capabilities []string) (Message, error) {
	payloadBytes, err := json.Marshal(HandshakePayload{
		Version:      CurrentProtocolVersion,
		DeviceID:     deviceID,
		PeerName:     peerName,
		OS:           osName,
		Arch:         arch,
		ScreenWidth:  screenWidth,
		ScreenHeight: screenHeight,
		Capabilities: capabilities,
		Timestamp:    time.Now().UnixNano(),
	})
	if err != nil {
		return Message{}, err
	}

	return Message{
		Type:      MessageTypeHandshake,
		Seq:       seq,
		Timestamp: time.Now().UnixNano(),
		Payload:   payloadBytes,
	}, nil
}

// NewHandshakeAckMessage constructs a handshake acknowledgement with responder screen bounds.
func NewHandshakeAckMessage(seq uint64, success bool, reason string, screenWidth, screenHeight int) (Message, error) {
	return NewHandshakeAckMessageWithDevice(seq, success, "", reason, screenWidth, screenHeight, []string{"input", "control"})
}

// NewHandshakeAckMessageWithDevice constructs a handshake acknowledgement with responder device ID and screen bounds.
func NewHandshakeAckMessageWithDevice(seq uint64, success bool, deviceID, reason string, screenWidth, screenHeight int, capabilities []string) (Message, error) {
	payloadBytes, err := json.Marshal(HandshakeAckPayload{
		Success:      success,
		Version:      CurrentProtocolVersion,
		DeviceID:     deviceID,
		Reason:       reason,
		ScreenWidth:  screenWidth,
		ScreenHeight: screenHeight,
		Capabilities: capabilities,
	})
	if err != nil {
		return Message{}, err
	}

	return Message{
		Type:      MessageTypeHandshakeAck,
		Seq:       seq,
		Timestamp: time.Now().UnixNano(),
		Payload:   payloadBytes,
	}, nil
}

// NewSwitchControlMessageAtPosition also preserves horizontal position for vertical layouts.
func NewSwitchControlMessageAtPosition(seq uint64, side string, normalizedX, normalizedY float64, reason string) (Message, error) {
	msg, err := NewSwitchControlMessage(seq, side, normalizedY, reason)
	if err != nil {
		return msg, err
	}
	var payload ControlPayload
	if err = json.Unmarshal(msg.Payload, &payload); err != nil {
		return msg, err
	}
	payload.NormalizedX = normalizedX
	msg.Payload, err = json.Marshal(payload)
	return msg, err
}

// NewSwitchControlMessage notifies peer to take over active input control at a given normalized vertical entry point.
func NewSwitchControlMessage(seq uint64, side string, normalizedY float64, reason string) (Message, error) {
	payloadBytes, err := json.Marshal(ControlPayload{
		Side:        side,
		NormalizedY: normalizedY,
		Reason:      reason,
	})
	if err != nil {
		return Message{}, err
	}

	return Message{
		Type:      MessageTypeSwitchControl,
		Seq:       seq,
		Timestamp: time.Now().UnixNano(),
		Payload:   payloadBytes,
	}, nil
}

// NewReleaseControlMessage requests control to return to local machine.
func NewReleaseControlMessage(seq uint64, reason string) (Message, error) {
	payloadBytes, err := json.Marshal(ControlPayload{
		Reason: reason,
	})
	if err != nil {
		return Message{}, err
	}

	return Message{
		Type:      MessageTypeReleaseControl,
		Seq:       seq,
		Timestamp: time.Now().UnixNano(),
		Payload:   payloadBytes,
	}, nil
}

// NewLayoutUpdateMessage constructs a layout update message to synchronize screen positions.
func NewLayoutUpdateMessage(seq uint64, peerSide string) (Message, error) {
	payloadBytes, err := json.Marshal(LayoutPayload{
		PeerSide: peerSide,
	})
	if err != nil {
		return Message{}, err
	}

	return Message{
		Type:      MessageTypeLayoutUpdate,
		Seq:       seq,
		Timestamp: time.Now().UnixNano(),
		Payload:   payloadBytes,
	}, nil
}

// NewErrorMessage constructs an error notification message.
func NewErrorMessage(seq uint64, errMsg string, code int) (Message, error) {
	payloadBytes, err := json.Marshal(ErrorPayload{
		Message: errMsg,
		Code:    code,
	})
	if err != nil {
		return Message{}, err
	}

	return Message{
		Type:      MessageTypeError,
		Seq:       seq,
		Timestamp: time.Now().UnixNano(),
		Payload:   payloadBytes,
	}, nil
}

// Validate checks the structural integrity of the message.
func (m Message) Validate() error {
	switch m.Type {
	case MessageTypeInput:
		if m.Input == nil {
			return ErrMissingInputEvent
		}
		return m.Input.Validate()
	case MessageTypePing, MessageTypePong, MessageTypeHandshake,
		MessageTypeHandshakeAck, MessageTypeSwitchControl, MessageTypeReleaseControl,
		MessageTypeLayoutUpdate, MessageTypeFileTransfer, MessageTypeClipboardImage, MessageTypeClipboardText, MessageTypeError:
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrUnknownMessageType, m.Type)
	}
}

// String returns a clean loggable representation of the message.
func (m Message) String() string {
	switch m.Type {
	case MessageTypeInput:
		if m.Input != nil {
			return fmt.Sprintf("Message[Seq=%d, Type=%s, Event=%s]", m.Seq, m.Type, m.Input.String())
		}
		return fmt.Sprintf("Message[Seq=%d, Type=%s, Event=nil]", m.Seq, m.Type)
	default:
		if len(m.Payload) > 0 {
			return fmt.Sprintf("Message[Seq=%d, Type=%s, Payload=%s]", m.Seq, m.Type, string(m.Payload))
		}
		return fmt.Sprintf("Message[Seq=%d, Type=%s]", m.Seq, m.Type)
	}
}
