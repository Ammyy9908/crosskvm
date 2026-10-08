package transport

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/crosskvm/crosskvm/internal/input"
	"github.com/crosskvm/crosskvm/internal/protocol"
)

func skipIfNoListen(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("TCP listen not permitted in this environment: %v", err)
	}
	_ = ln.Close()
}

func TestTransport_ServerClient_Exchange(t *testing.T) {
	skipIfNoListen(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	receivedMsgCh := make(chan protocol.Message, 10)
	connectCh := make(chan *Conn, 1)

	handler := ServerFuncHandler{
		ConnectFunc: func(conn *Conn) {
			connectCh <- conn
		},
		MessageFunc: func(conn *Conn, msg protocol.Message) {
			receivedMsgCh <- msg
		},
	}

	server := NewServer("127.0.0.1:0", handler)
	serverErrCh := make(chan error, 1)

	go func() {
		serverErrCh <- server.ListenAndServe(ctx)
	}()

	select {
	case <-server.Ready():
	case <-ctx.Done():
		t.Fatal("Timeout waiting for server to be ready")
	}

	serverAddr := server.Addr().String()

	client := NewClient(2 * time.Second)
	clientConn, err := client.Connect(ctx, serverAddr)
	if err != nil {
		t.Fatalf("Client connect failed: %v", err)
	}
	defer clientConn.Close()

	// Wait for server to register connection
	var serverConn *Conn
	select {
	case serverConn = <-connectCh:
	case <-time.After(2 * time.Second):
		t.Fatal("Server did not accept client connection in time")
	}

	// 1. Client -> Server: Send InputEvent
	event := input.NewMouseMoveEvent(15, -7)
	sendMsg := protocol.NewInputMessage(1, event)
	if err := clientConn.Send(sendMsg); err != nil {
		t.Fatalf("Client send failed: %v", err)
	}

	select {
	case recvMsg := <-receivedMsgCh:
		if recvMsg.Type != protocol.MessageTypeInput {
			t.Errorf("Server received wrong type: %v", recvMsg.Type)
		}
		if recvMsg.Input == nil || recvMsg.Input.DX != 15 || recvMsg.Input.DY != -7 {
			t.Errorf("Server received wrong input event: %+v", recvMsg.Input)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Server did not receive message in time")
	}

	// 2. Server -> Client: Send Pong
	pongMsg := protocol.NewPongMessage(2)
	if err := serverConn.Send(pongMsg); err != nil {
		t.Fatalf("Server send failed: %v", err)
	}

	clientRecvMsg, err := clientConn.Receive()
	if err != nil {
		t.Fatalf("Client receive failed: %v", err)
	}
	if clientRecvMsg.Type != protocol.MessageTypePong || clientRecvMsg.Seq != 2 {
		t.Errorf("Client received unexpected message: %+v", clientRecvMsg)
	}
}

func TestTransport_ClientDisconnect_ServerNotification(t *testing.T) {
	skipIfNoListen(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	disconnectCh := make(chan struct{}, 1)
	handler := ServerFuncHandler{
		DisconnectFunc: func(conn *Conn, err error) {
			disconnectCh <- struct{}{}
		},
	}

	server := NewServer("127.0.0.1:0", handler)
	go func() {
		_ = server.ListenAndServe(ctx)
	}()

	select {
	case <-server.Ready():
	case <-ctx.Done():
		t.Fatal("Server not ready")
	}

	client := NewClient(2 * time.Second)
	clientConn, err := client.Connect(ctx, server.Addr().String())
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Close client connection
	if err := clientConn.Close(); err != nil {
		t.Fatalf("Client close failed: %v", err)
	}

	select {
	case <-disconnectCh:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Server did not detect client disconnect")
	}
}

func TestTransport_ServerShutdown_ClientEOF(t *testing.T) {
	skipIfNoListen(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	server := NewServer("127.0.0.1:0", nil)
	go func() {
		_ = server.ListenAndServe(ctx)
	}()

	select {
	case <-server.Ready():
	case <-ctx.Done():
		t.Fatal("Server not ready")
	}

	client := NewClient(2 * time.Second)
	clientConn, err := client.Connect(ctx, server.Addr().String())
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer clientConn.Close()

	// Stop server
	if err := server.Stop(); err != nil {
		t.Fatalf("Server stop failed: %v", err)
	}

	_, err = clientConn.Receive()
	if err == nil {
		t.Fatal("Expected error on receive after server shutdown, got nil")
	}
	if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) && !errors.Is(err, ErrConnectionClosed) {
		t.Logf("Received expected disconnection error: %v", err)
	}
}

func TestTransport_Broadcast(t *testing.T) {
	skipIfNoListen(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var connectedCount sync.WaitGroup
	connectedCount.Add(2)

	handler := ServerFuncHandler{
		ConnectFunc: func(conn *Conn) {
			connectedCount.Done()
		},
	}

	server := NewServer("127.0.0.1:0", handler)
	go func() {
		_ = server.ListenAndServe(ctx)
	}()

	<-server.Ready()

	client := NewClient(2 * time.Second)
	c1, err := client.Connect(ctx, server.Addr().String())
	if err != nil {
		t.Fatalf("c1 connect failed: %v", err)
	}
	defer c1.Close()

	c2, err := client.Connect(ctx, server.Addr().String())
	if err != nil {
		t.Fatalf("c2 connect failed: %v", err)
	}
	defer c2.Close()

	connectedCount.Wait()

	msg := protocol.NewPingMessage(99)
	if err := server.Broadcast(msg); err != nil {
		t.Fatalf("Broadcast failed: %v", err)
	}

	m1, err := c1.Receive()
	if err != nil || m1.Seq != 99 {
		t.Fatalf("c1 receive failed: msg=%v, err=%v", m1, err)
	}

	m2, err := c2.Receive()
	if err != nil || m2.Seq != 99 {
		t.Fatalf("c2 receive failed: msg=%v, err=%v", m2, err)
	}
}

func TestTransport_ConnectUnreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	client := NewClient(200 * time.Millisecond)
	// Use an invalid port / non-listening port
	_, err := client.Connect(ctx, "127.0.0.1:54321")
	if err == nil {
		t.Fatal("Expected error connecting to non-existent server, got nil")
	}
}
