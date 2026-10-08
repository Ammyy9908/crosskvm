package transport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/crosskvm/crosskvm/internal/protocol"
)

var (
	ErrServerStopped = errors.New("transport: server is stopped")
)

// ServerHandler defines lifecycle callbacks for incoming connections and messages.
type ServerHandler interface {
	OnConnect(conn *Conn)
	OnMessage(conn *Conn, msg protocol.Message)
	OnDisconnect(conn *Conn, err error)
}

// ServerFuncHandler allows using functions as a ServerHandler.
type ServerFuncHandler struct {
	ConnectFunc    func(conn *Conn)
	MessageFunc    func(conn *Conn, msg protocol.Message)
	DisconnectFunc func(conn *Conn, err error)
}

func (h ServerFuncHandler) OnConnect(conn *Conn) {
	if h.ConnectFunc != nil {
		h.ConnectFunc(conn)
	}
}

func (h ServerFuncHandler) OnMessage(conn *Conn, msg protocol.Message) {
	if h.MessageFunc != nil {
		h.MessageFunc(conn, msg)
	}
}

func (h ServerFuncHandler) OnDisconnect(conn *Conn, err error) {
	if h.DisconnectFunc != nil {
		h.DisconnectFunc(conn, err)
	}
}

// Server manages TCP listening and connected peers.
type Server struct {
	addr     string
	handler  ServerHandler
	listener net.Listener
	mu       sync.Mutex
	conns    map[*Conn]struct{}
	closed   atomic.Bool
	readyCh  chan struct{}
}

// NewServer creates a new TCP server instance.
func NewServer(addr string, handler ServerHandler) *Server {
	if handler == nil {
		handler = ServerFuncHandler{}
	}
	return &Server{
		addr:    addr,
		handler: handler,
		conns:   make(map[*Conn]struct{}),
		readyCh: make(chan struct{}),
	}
}

// Ready returns a channel that is closed when the listener is active.
func (s *Server) Ready() <-chan struct{} {
	return s.readyCh
}

// Addr returns the bound listener address, or nil if not listening yet.
func (s *Server) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Addr()
	}
	return nil
}

// ListenAndServe starts listening on the configured TCP address and blocks until ctx is canceled or Stop() is called.
func (s *Server) ListenAndServe(ctx context.Context) error {
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", s.addr)
	if err != nil {
		return fmt.Errorf("transport: failed to listen on %s: %w", s.addr, err)
	}

	s.mu.Lock()
	if s.closed.Load() {
		s.mu.Unlock()
		_ = ln.Close()
		return ErrServerStopped
	}
	s.listener = ln
	s.mu.Unlock()

	close(s.readyCh)

	// Goroutine to watch context cancellation
	go func() {
		<-ctx.Done()
		_ = s.Stop()
	}()

	var wg sync.WaitGroup
	defer func() {
		_ = s.Stop()
		wg.Wait()
	}()

	for {
		rawConn, err := ln.Accept()
		if err != nil {
			if s.closed.Load() || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("transport: accept error: %w", err)
		}

		conn := NewConn(rawConn)
		s.addConn(conn)

		wg.Add(1)
		go func(c *Conn) {
			defer wg.Done()
			defer s.removeConn(c)
			s.handleConnection(ctx, c)
		}(conn)
	}
}

func (s *Server) handleConnection(ctx context.Context, conn *Conn) {
	s.handler.OnConnect(conn)

	var readErr error
	for {
		if ctx.Err() != nil || s.closed.Load() || conn.IsClosed() {
			break
		}

		msg, err := conn.Receive()
		if err != nil {
			readErr = err
			break
		}

		s.handler.OnMessage(conn, msg)
	}

	if errors.Is(readErr, io.EOF) || errors.Is(readErr, net.ErrClosed) || errors.Is(readErr, ErrConnectionClosed) {
		s.handler.OnDisconnect(conn, nil)
	} else {
		s.handler.OnDisconnect(conn, readErr)
	}
}

func (s *Server) addConn(conn *Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conns[conn] = struct{}{}
}

func (s *Server) removeConn(conn *Conn) {
	s.mu.Lock()
	delete(s.conns, conn)
	s.mu.Unlock()
	_ = conn.Close()
}

// ActiveConnections returns the number of currently connected clients.
func (s *Server) ActiveConnections() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.conns)
}

// Broadcast sends a message to all active connections.
func (s *Server) Broadcast(msg protocol.Message) error {
	s.mu.Lock()
	conns := make([]*Conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()

	var errs []error
	for _, c := range conns {
		if err := c.Send(msg); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("transport: broadcast failed for %d connections: %w", len(errs), errs[0])
	}
	return nil
}

// Stop closes the listener and all active connections.
func (s *Server) Stop() error {
	if s.closed.Swap(true) {
		return nil
	}

	s.mu.Lock()
	var err error
	if s.listener != nil {
		err = s.listener.Close()
	}

	for c := range s.conns {
		_ = c.Close()
	}
	s.conns = make(map[*Conn]struct{})
	s.mu.Unlock()

	return err
}
