package protocol

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// MaxPayloadSize sets a sane safeguard against memory exhaustion (1 MB).
const MaxPayloadSize = 1024 * 1024

var (
	ErrPayloadTooLarge = errors.New("protocol: message payload exceeds maximum allowed size (1MB)")
	ErrEmptyPayload    = errors.New("protocol: payload length is zero")
)

var encBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 1024)
		return &b
	},
}

// Encoder encodes protocol Messages into a length-prefixed stream.
type Encoder struct {
	w  io.Writer
	mu sync.Mutex
}

// NewEncoder creates a new Encoder.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// Encode writes a length-prefixed Message to the underlying writer.
// Frame header (4-byte length) and payload are combined into a single atomic write.
// It is safe for concurrent use by multiple goroutines.
func (e *Encoder) Encode(msg Message) error {
	if err := msg.Validate(); err != nil {
		return fmt.Errorf("protocol: cannot encode invalid message: %w", err)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("protocol: json marshal error: %w", err)
	}

	length := len(data)
	if length > MaxPayloadSize {
		return ErrPayloadTooLarge
	}

	totalLen := 4 + length
	pBuf := encBufPool.Get().(*[]byte)
	buf := *pBuf
	if cap(buf) < totalLen {
		buf = make([]byte, totalLen)
	} else {
		buf = buf[:totalLen]
	}
	binary.BigEndian.PutUint32(buf[:4], uint32(length))
	copy(buf[4:], data)

	e.mu.Lock()
	_, writeErr := e.w.Write(buf)
	e.mu.Unlock()

	*pBuf = buf
	encBufPool.Put(pBuf)

	if writeErr != nil {
		return fmt.Errorf("protocol: write message error: %w", writeErr)
	}

	return nil
}

// Decoder reads and decodes length-prefixed Messages from a stream.
type Decoder struct {
	r      io.Reader
	mu     sync.Mutex
	header [4]byte
	buf    []byte
}

// NewDecoder creates a new Decoder.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{
		r:   r,
		buf: make([]byte, 0, 1024),
	}
}

// Decode reads the next length-prefixed Message from the underlying reader.
// Returns io.EOF when the stream is cleanly closed.
func (d *Decoder) Decode() (Message, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, err := io.ReadFull(d.r, d.header[:]); err != nil {
		return Message{}, err
	}

	length := binary.BigEndian.Uint32(d.header[:])
	if length == 0 {
		return Message{}, ErrEmptyPayload
	}
	if length > MaxPayloadSize {
		return Message{}, fmt.Errorf("%w (got %d bytes)", ErrPayloadTooLarge, length)
	}

	intLen := int(length)
	if cap(d.buf) < intLen {
		d.buf = make([]byte, intLen, intLen+512)
	} else {
		d.buf = d.buf[:intLen]
	}

	if _, err := io.ReadFull(d.r, d.buf); err != nil {
		return Message{}, fmt.Errorf("protocol: error reading payload: %w", err)
	}

	var msg Message
	if err := json.Unmarshal(d.buf, &msg); err != nil {
		return Message{}, fmt.Errorf("protocol: json unmarshal error: %w", err)
	}

	if err := msg.Validate(); err != nil {
		return Message{}, fmt.Errorf("protocol: decoded invalid message: %w", err)
	}

	return msg, nil
}
