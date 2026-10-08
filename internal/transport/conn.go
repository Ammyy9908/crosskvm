package transport

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"

	"github.com/crosskvm/crosskvm/internal/protocol"
)

var (
	ErrConnectionClosed = errors.New("transport: connection is closed")
)

// Conn wraps a net.Conn with a protocol Encoder and Decoder.
type Conn struct {
	rawConn net.Conn
	enc     *protocol.Encoder
	dec     *protocol.Decoder
	closed  atomic.Bool
	closeMu sync.Mutex
}

// NewConn initializes a framed protocol connection over a network stream.
func NewConn(raw net.Conn) *Conn {
	if tcp, ok := raw.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
		_ = tcp.SetKeepAlive(true)
	}
	return &Conn{
		rawConn: raw,
		enc:     protocol.NewEncoder(raw),
		dec:     protocol.NewDecoder(raw),
	}
}

// Send encodes and transmits a protocol Message.
func (c *Conn) Send(msg protocol.Message) error {
	if c.closed.Load() {
		return ErrConnectionClosed
	}
	return c.enc.Encode(msg)
}

// Receive decodes and returns the next protocol Message.
func (c *Conn) Receive() (protocol.Message, error) {
	if c.closed.Load() {
		return protocol.Message{}, ErrConnectionClosed
	}
	return c.dec.Decode()
}

// Close gracefully closes the network connection.
func (c *Conn) Close() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()

	if c.closed.Swap(true) {
		return nil
	}
	return c.rawConn.Close()
}

// IsClosed returns whether the connection has been closed.
func (c *Conn) IsClosed() bool {
	return c.closed.Load()
}

// RemoteAddr returns the remote network address.
func (c *Conn) RemoteAddr() net.Addr {
	return c.rawConn.RemoteAddr()
}

// LocalAddr returns the local network address.
func (c *Conn) LocalAddr() net.Addr {
	return c.rawConn.LocalAddr()
}
