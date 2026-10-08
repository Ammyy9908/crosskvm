package transport

import (
	"context"
	"fmt"
	"net"
	"time"
)

// Client handles outgoing TCP connections to a remote peer.
type Client struct {
	dialer net.Dialer
}

// NewClient creates a new TCP Client.
func NewClient(timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Client{
		dialer: net.Dialer{
			Timeout:   timeout,
			KeepAlive: 15 * time.Second,
		},
	}
}

// Connect dials the remote address using the provided context and returns an active framed Conn.
func (c *Client) Connect(ctx context.Context, addr string) (*Conn, error) {
	rawConn, err := c.dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("transport: failed to connect to %s: %w", addr, err)
	}

	return NewConn(rawConn), nil
}
