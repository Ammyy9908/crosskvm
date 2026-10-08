package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"github.com/crosskvm/crosskvm/internal/input"
)

func TestCodec_RoundTrip_InputMessage(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	dec := NewDecoder(&buf)

	event := input.NewMouseMoveEvent(42, -18)
	originalMsg := NewInputMessage(1, event)

	if err := enc.Encode(originalMsg); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decodedMsg, err := dec.Decode()
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decodedMsg.Type != originalMsg.Type {
		t.Errorf("Type mismatch: got %v, want %v", decodedMsg.Type, originalMsg.Type)
	}
	if decodedMsg.Seq != originalMsg.Seq {
		t.Errorf("Seq mismatch: got %v, want %v", decodedMsg.Seq, originalMsg.Seq)
	}
	if decodedMsg.Input == nil {
		t.Fatalf("Expected Input payload, got nil")
	}
	if decodedMsg.Input.DX != 42 || decodedMsg.Input.DY != -18 {
		t.Errorf("Input DX/DY mismatch: got (%d, %d), want (42, -18)", decodedMsg.Input.DX, decodedMsg.Input.DY)
	}
}

func TestCodec_RoundTrip_VariousMessages(t *testing.T) {
	handshake, err := NewHandshakeMessage(1, "macbook-pro", "darwin", "arm64", 1920, 1080)
	if err != nil {
		t.Fatalf("NewHandshakeMessage failed: %v", err)
	}

	handshakeAck, err := NewHandshakeAckMessage(2, true, "ready", 2560, 1440)
	if err != nil {
		t.Fatalf("NewHandshakeAckMessage failed: %v", err)
	}

	switchCtrl, err := NewSwitchControlMessage(3, "left", 0.5, "cursor crossed edge")
	if err != nil {
		t.Fatalf("NewSwitchControlMessage failed: %v", err)
	}

	releaseCtrl, err := NewReleaseControlMessage(4, "emergency hotkey")
	if err != nil {
		t.Fatalf("NewReleaseControlMessage failed: %v", err)
	}

	errMsg, err := NewErrorMessage(5, "failed to inject input", 500)
	if err != nil {
		t.Fatalf("NewErrorMessage failed: %v", err)
	}

	messages := []Message{
		NewPingMessage(10),
		NewPongMessage(11),
		handshake,
		handshakeAck,
		switchCtrl,
		releaseCtrl,
		errMsg,
		NewInputMessage(12, input.NewKeyDownEvent("A", input.ModCtrl)),
		NewInputMessage(13, input.NewKeyUpEvent("A", 0)),
		NewInputMessage(14, input.NewMouseButtonDownEvent(input.MouseButtonLeft)),
		NewInputMessage(15, input.NewMouseWheelEvent(0, 120)),
	}

	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	dec := NewDecoder(&buf)

	for _, msg := range messages {
		if err := enc.Encode(msg); err != nil {
			t.Fatalf("Encode of %s failed: %v", msg.Type, err)
		}
	}

	for i, expected := range messages {
		decoded, err := dec.Decode()
		if err != nil {
			t.Fatalf("[%d] Decode of %s failed: %v", i, expected.Type, err)
		}
		if decoded.Type != expected.Type {
			t.Errorf("[%d] Type mismatch: got %v, want %v", i, decoded.Type, expected.Type)
		}
		if decoded.Seq != expected.Seq {
			t.Errorf("[%d] Seq mismatch: got %v, want %v", i, decoded.Seq, expected.Seq)
		}
	}

	// Next decode should return EOF
	_, err = dec.Decode()
	if !errors.Is(err, io.EOF) {
		t.Errorf("Expected EOF at end of stream, got: %v", err)
	}
}

func TestCodec_PayloadTooLarge(t *testing.T) {
	var buf bytes.Buffer
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, MaxPayloadSize+10)
	buf.Write(header)

	dec := NewDecoder(&buf)
	_, err := dec.Decode()
	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("Expected ErrPayloadTooLarge, got %v", err)
	}
}

func TestCodec_EmptyPayload(t *testing.T) {
	var buf bytes.Buffer
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, 0)
	buf.Write(header)

	dec := NewDecoder(&buf)
	_, err := dec.Decode()
	if !errors.Is(err, ErrEmptyPayload) {
		t.Fatalf("Expected ErrEmptyPayload, got %v", err)
	}
}

func TestCodec_CorruptedPayload(t *testing.T) {
	var buf bytes.Buffer
	header := make([]byte, 4)
	payload := []byte("not valid json")
	binary.BigEndian.PutUint32(header, uint32(len(payload)))
	buf.Write(header)
	buf.Write(payload)

	dec := NewDecoder(&buf)
	_, err := dec.Decode()
	if err == nil {
		t.Fatal("Expected error on invalid JSON, got nil")
	}
}

func TestCodec_TruncatedStream(t *testing.T) {
	var buf bytes.Buffer
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, 50)
	buf.Write(header)
	buf.Write([]byte("short")) // only 5 bytes instead of 50

	dec := NewDecoder(&buf)
	_, err := dec.Decode()
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("Expected io.ErrUnexpectedEOF, got %v", err)
	}
}

func TestProtocol_HandshakeValidation_And_VersionCompatibility(t *testing.T) {
	// Valid handshake
	valid := HandshakePayload{
		Version:      "1.0.0",
		DeviceID:     "xkvm-123",
		PeerName:     "HostA",
		OS:           "darwin",
		ScreenWidth:  1920,
		ScreenHeight: 1080,
	}
	if err := ValidateHandshake(valid); err != nil {
		t.Errorf("Expected valid handshake to pass, got: %v", err)
	}

	// Incompatible version (major version 2.0.0 vs 1.0.0)
	incompat := valid
	incompat.Version = "2.0.0"
	if err := ValidateHandshake(incompat); !errors.Is(err, ErrIncompatibleProtocolVersion) {
		t.Errorf("Expected ErrIncompatibleProtocolVersion, got: %v", err)
	}

	// Compatible minor version (1.1.0 vs 1.0.0)
	compatMinor := valid
	compatMinor.Version = "1.1.0"
	if err := ValidateHandshake(compatMinor); err != nil {
		t.Errorf("Expected minor version 1.1.0 to be compatible, got: %v", err)
	}

	// Invalid screen geometry
	invalidGeom := valid
	invalidGeom.ScreenWidth = 0
	if err := ValidateHandshake(invalidGeom); !errors.Is(err, ErrInvalidHandshakePayload) {
		t.Errorf("Expected ErrInvalidHandshakePayload for 0 width, got: %v", err)
	}
}
