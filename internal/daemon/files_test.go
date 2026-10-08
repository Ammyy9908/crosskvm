package daemon

import (
	"encoding/json"
	"github.com/crosskvm/crosskvm/internal/discovery"
	"github.com/crosskvm/crosskvm/internal/transport"
	"net"
	"strings"
	"testing"
	"time"
)

func TestFileTransferNegotiationAndRelay(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	a.SetDeadline(time.Now().Add(time.Second))
	b.SetDeadline(time.Now().Add(time.Second))
	conn := transport.NewConn(a)
	ds := &DaemonService{localDev: &discovery.LocalDevice{Capabilities: []string{clipboardCapability, fileTransferCapability}}}
	ds.activeConn.Store(conn)
	data := json.RawMessage(`{"kind":"begin","id":"12345678-1234-1234-1234-123456789012","step":0,"name":"a","size":0}`)
	if ds.SendFilePacket(data) == nil {
		t.Fatal("unnegotiated send")
	}
	ds.negotiateClipboard(conn, []string{clipboardCapability})
	if ds.fileTransferConn.Load() != nil {
		t.Fatal("enabled with legacy peer")
	}
	ds.negotiateClipboard(conn, []string{clipboardCapability, fileTransferCapability})
	done := make(chan error, 1)
	go func() { done <- ds.SendFilePacket(data) }()
	msg, err := transport.NewConn(b).Receive()
	if err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	received := false
	ds.SetEmitter(func(e IPCEvent) { received = e.Event == "file_packet" })
	ds.receiveFileTransfer(conn, msg)
	if !received {
		t.Fatal("not relayed")
	}
	ds.fileTransferConn.Store(nil)
	received = false
	ds.receiveFileTransfer(conn, msg)
	if received {
		t.Fatal("relayed after disconnect")
	}
	if validFilePacket(json.RawMessage(strings.Repeat(" ", 100000))) {
		t.Fatal("oversized packet accepted")
	}
}
