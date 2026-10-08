package daemon

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"github.com/crosskvm/crosskvm/internal/discovery"
	"github.com/crosskvm/crosskvm/internal/protocol"
	"github.com/crosskvm/crosskvm/internal/transport"
	"image"
	"image/png"
	"net"
	"strings"
	"testing"
	"time"
)

func TestClipboardNegotiationAndRoundTrip(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	_ = a.SetDeadline(time.Now().Add(time.Second))
	_ = b.SetDeadline(time.Now().Add(time.Second))
	conn := transport.NewConn(a)
	ds := &DaemonService{localDev: &discovery.LocalDevice{Capabilities: []string{clipboardCapability}}}
	ds.activeConn.Store(conn)
	if ds.SendClipboardText("before negotiation") == nil {
		t.Fatal("sent without negotiated capability")
	}
	ds.negotiateClipboard(conn, []string{"input"})
	if ds.clipboardConn.Load() != nil {
		t.Fatal("enabled for an older peer")
	}
	ds.negotiateClipboard(conn, []string{clipboardCapability})
	want := "Hello\nनमस्ते 👋\n你好"
	done := make(chan error, 1)
	go func() { done <- ds.SendClipboardText(want) }()
	msg, err := transport.NewConn(b).Receive()
	if err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	var got string
	ds.SetEmitter(func(e IPCEvent) {
		if e.Event == "clipboard_text" {
			got = e.Data.(clipboardPayload).Text
		}
	})
	ds.receiveClipboard(conn, msg)
	if got != want {
		t.Fatalf("round trip mismatch: %q", got)
	}
	if ds.SendClipboardText(strings.Repeat("é", 40000)) == nil {
		t.Fatal("oversized clipboard accepted")
	}
	got = ""
	msg.Payload, _ = json.Marshal(clipboardPayload{Text: strings.Repeat("x", maxClipboardText+1)})
	ds.receiveClipboard(conn, msg)
	if got != "" {
		t.Fatal("oversized incoming clipboard accepted")
	}
	ds.clipboardConn.Store(nil)
	msg.Payload, _ = json.Marshal(clipboardPayload{Text: want})
	ds.receiveClipboard(conn, msg)
	if got != "" {
		t.Fatal("clipboard applied after disconnect")
	}
	if msg.Type != protocol.MessageTypeClipboardText {
		t.Fatal("incorrect message type")
	}
}

func TestImageClipboardNegotiationAndValidation(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	_ = a.SetDeadline(time.Now().Add(time.Second))
	_ = b.SetDeadline(time.Now().Add(time.Second))
	conn := transport.NewConn(a)
	ds := &DaemonService{localDev: &discovery.LocalDevice{Capabilities: []string{clipboardCapability, imageClipboardCapability}}}
	ds.activeConn.Store(conn)
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	data := base64.StdEncoding.EncodeToString(pngBytes.Bytes())
	if ds.SendClipboardImage(data) == nil {
		t.Fatal("sent before negotiation")
	}
	ds.negotiateClipboard(conn, []string{clipboardCapability})
	if ds.imageClipboardConn.Load() != nil {
		t.Fatal("enabled images for text-only peer")
	}
	ds.negotiateClipboard(conn, []string{clipboardCapability, imageClipboardCapability})
	done := make(chan error, 1)
	go func() { done <- ds.SendClipboardImage(data) }()
	msg, err := transport.NewConn(b).Receive()
	if err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	var got string
	ds.SetEmitter(func(e IPCEvent) {
		if e.Event == "clipboard_image" {
			got = e.Data.(clipboardImagePayload).PNG
		}
	})
	ds.receiveClipboardImage(conn, msg)
	if got != data {
		t.Fatal("image roundtrip mismatch")
	}
	if validClipboardImage("bad") || validClipboardImage(base64.StdEncoding.EncodeToString(make([]byte, maxClipboardImage+1))) {
		t.Fatal("invalid image accepted")
	}
	ds.imageClipboardConn.Store(nil)
	got = ""
	ds.receiveClipboardImage(conn, msg)
	if got != "" {
		t.Fatal("received after disconnect")
	}
}
