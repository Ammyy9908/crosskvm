package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/crosskvm/crosskvm/internal/discovery"
)

const (
	DefaultIPCPort  = 4548
	DefaultUnixSock = "/tmp/crosskvm.sock"
)

// IPCServer serves Electron requests over Unix domain socket and/or loopback TCP.
type IPCServer struct {
	mu           sync.Mutex
	service      *DaemonService
	logger       *log.Logger
	tcpAddr      string
	sockPath     string
	clients      map[net.Conn]chan []byte
	clientsMu    sync.RWMutex
	tcpListener  net.Listener
	unixListener net.Listener
	closed       atomic.Bool
	stopCh       chan struct{}
}

// NewIPCServer creates a new IPCServer bound to the DaemonService.
func NewIPCServer(service *DaemonService, tcpPort int, sockPath string, logger *log.Logger) *IPCServer {
	if tcpPort <= 0 {
		tcpPort = DefaultIPCPort
	}
	if sockPath == "" {
		sockPath = DefaultUnixSock
	}
	if logger == nil {
		logger = log.New(os.Stdout, "[CrossKVM-IPC] ", log.LstdFlags|log.Lmsgprefix)
	}

	server := &IPCServer{
		service:  service,
		logger:   logger,
		tcpAddr:  fmt.Sprintf("127.0.0.1:%d", tcpPort),
		sockPath: sockPath,
		clients:  make(map[net.Conn]chan []byte),
		stopCh:   make(chan struct{}),
	}

	service.SetEmitter(server.Broadcast)
	return server
}

// Start opens listeners and begins accepting Electron IPC connections.
func (s *IPCServer) Start(ctx context.Context) error {
	var startedCount int

	// 1. Unix Domain Socket listeners (macOS and Linux)
	if runtime.GOOS != "windows" {
		var sockPaths []string
		if cfgDir, err := discovery.GetConfigDir(); err == nil && cfgDir != "" {
			sockPaths = append(sockPaths, filepath.Join(cfgDir, "crosskvm.sock"))
		}
		if s.sockPath != "" {
			sockPaths = append(sockPaths, s.sockPath)
		}

		for _, p := range sockPaths {
			_ = os.Remove(p)
			unixLn, err := net.Listen("unix", p)
			if err == nil {
				_ = os.Chmod(p, 0777)
				s.logger.Printf("IPC Server listening on Unix domain socket: %s", p)
				go s.acceptLoop(unixLn)
				startedCount++
			} else {
				s.logger.Printf("[WARN] Failed to bind Unix domain socket %s: %v", p, err)
			}
		}
	}

	// 2. Loopback TCP listener (supported across macOS and Windows)
	tcpLn, err := net.Listen("tcp", s.tcpAddr)
	if err == nil {
		s.tcpListener = tcpLn
		s.logger.Printf("IPC Server listening on loopback TCP: %s", s.tcpAddr)
		go s.acceptLoop(tcpLn)
		startedCount++
	} else {
		s.logger.Printf("[WARN] Failed to bind loopback TCP IPC on %s: %v", s.tcpAddr, err)
	}

	if startedCount == 0 {
		return fmt.Errorf("failed to bind any IPC listener (tcp %s)", s.tcpAddr)
	}

	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()

	return nil
}

func (s *IPCServer) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if s.closed.Load() {
				return
			}
			s.logger.Printf("Accept error: %v", err)
			return
		}

		s.clientsMu.Lock()
		queue := make(chan []byte, 64)
		s.clients[conn] = queue
		s.clientsMu.Unlock()

		s.logger.Printf("Electron client connected: %s", conn.RemoteAddr())

		go s.writeClient(conn, queue)
		go s.handleClient(conn)
	}
}

func (s *IPCServer) handleClient(conn net.Conn) {
	defer func() {
		s.clientsMu.Lock()
		if queue := s.clients[conn]; queue != nil {
			close(queue)
		}
		delete(s.clients, conn)
		s.clientsMu.Unlock()
		_ = conn.Close()
		s.logger.Printf("Electron client disconnected: %s", conn.RemoteAddr())
	}()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req IPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendResponse(conn, IPCResponse{
				ID:    0,
				Error: fmt.Sprintf("invalid json: %v", err),
			})
			continue
		}

		resp := s.dispatch(req)
		s.sendResponse(conn, resp)
	}
}

// writeClient is the only socket writer for a desktop client. UI backpressure
// never runs on the input routing or ownership transition goroutine.
func (s *IPCServer) writeClient(conn net.Conn, queue <-chan []byte) {
	defer conn.Close()
	for data := range queue {
		_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
		if _, err := conn.Write(data); err != nil {
			return
		}
	}
}

func (s *IPCServer) enqueueClient(conn net.Conn, data []byte) {
	s.clientsMu.RLock()
	queue := s.clients[conn]
	if queue != nil {
		select {
		case queue <- data:
		default:
			_ = conn.Close()
		}
	}
	s.clientsMu.RUnlock()
}

func (s *IPCServer) sendResponse(conn net.Conn, resp IPCResponse) {
	data, err := json.Marshal(resp)
	if err == nil {
		s.enqueueClient(conn, append(data, '\n'))
	}
}

// Broadcast queues notifications without waiting for the desktop app.
func (s *IPCServer) Broadcast(evt IPCEvent) {
	data, err := json.Marshal(evt)
	if err != nil {
		return
	}
	data = append(data, '\n')
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	for client, queue := range s.clients {
		select {
		case queue <- data:
		default:
			_ = client.Close()
		}
	}
}

// dispatch handles incoming RPC methods.
func (s *IPCServer) dispatch(req IPCRequest) IPCResponse {
	resp := IPCResponse{ID: req.ID}

	switch req.Method {
	case "get_local_info":
		resp.Result = s.service.GetLocalInfo()

	case "get_peers":
		resp.Result = s.service.GetPeers()

	case "rescan":
		s.service.Rescan()
		resp.Result = map[string]bool{"success": true}

	case "get_status":
		resp.Result = s.service.GetStatus()

	case "get_metrics":
		resp.Result = s.service.GetMetrics()

	case "connect":
		var params ConnectParams
		if len(req.Params) > 0 {
			_ = json.Unmarshal(req.Params, &params)
		}
		if err := s.service.Connect(params.PeerID, params.Addr); err != nil {
			resp.Error = err.Error()
		} else {
			resp.Result = map[string]bool{"success": true}
		}

	case "disconnect":
		if err := s.service.Disconnect(); err != nil {
			resp.Error = err.Error()
		} else {
			resp.Result = map[string]bool{"success": true}
		}

	case "start_kvm":
		var params StartKVMParams
		if len(req.Params) > 0 {
			_ = json.Unmarshal(req.Params, &params)
		}
		if err := s.service.StartKVM(params.PeerID, params.Addr, params.Side); err != nil {
			resp.Error = err.Error()
		} else {
			resp.Result = map[string]bool{"success": true}
		}

	case "stop_kvm":
		if err := s.service.StopKVM(); err != nil {
			resp.Error = err.Error()
		} else {
			resp.Result = map[string]bool{"success": true}
		}

	case "set_peer_side":
		var params SetPeerSideParams
		if len(req.Params) > 0 {
			_ = json.Unmarshal(req.Params, &params)
		}
		if err := s.service.SetPeerSide(params.Side); err != nil {
			resp.Error = err.Error()
		} else {
			resp.Result = map[string]bool{"success": true}
		}

	case "shutdown":
		resp.Result = map[string]bool{"success": true}
		go func() {
			_ = s.service.Close()
			_ = s.Close()
			os.Exit(0)
		}()

	default:
		resp.Error = fmt.Sprintf("unknown method: %s", req.Method)
	}

	return resp
}

// Close terminates listeners and closes client connections.
func (s *IPCServer) Close() error {
	if s.closed.Swap(true) {
		return nil
	}

	s.clientsMu.Lock()
	for client, queue := range s.clients {
		_ = client.Close()
		close(queue)
	}
	s.clients = make(map[net.Conn]chan []byte)
	s.clientsMu.Unlock()

	if s.tcpListener != nil {
		_ = s.tcpListener.Close()
	}
	if s.unixListener != nil {
		_ = s.unixListener.Close()
		_ = os.Remove(s.sockPath)
	}

	return nil
}
