package daemon

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/crosskvm/crosskvm/internal/protocol"
	"github.com/crosskvm/crosskvm/internal/transport"
	"image/png"
	"time"
	"unicode/utf8"
)

const clipboardCapability = "clipboard-text-v1"
const maxClipboardText = 64 * 1024

type clipboardPayload struct {
	Text string `json:"text"`
}

func (ds *DaemonService) negotiateClipboard(conn *transport.Conn, capabilities []string) {
	if ds.activeConn.Load() != conn {
		return
	}
	for _, local := range ds.localDev.Capabilities {
		if local == fileTransferCapability {
			for _, remote := range capabilities {
				if remote == fileTransferCapability {
					ds.fileTransferConn.Store(conn)
				}
			}
		}
		if local == imageClipboardCapability {
			for _, remote := range capabilities {
				if remote == imageClipboardCapability {
					ds.imageClipboardConn.Store(conn)
				}
			}
		}
	}
	supported := false
	for _, c := range ds.localDev.Capabilities {
		if c == clipboardCapability {
			supported = true
		}
	}
	if !supported {
		return
	}
	for _, c := range capabilities {
		if c == clipboardCapability {
			ds.clipboardConn.Store(conn)
			ds.emit(IPCEvent{Event: "clipboard_ready", Data: map[string]bool{"enabled": true, "images": ds.imageClipboardConn.Load() == conn}})
			return
		}
	}
}

func validClipboardText(text string) bool {
	return len(text) <= maxClipboardText && utf8.ValidString(text)
}

func (ds *DaemonService) SendClipboardText(text string) error {
	if !validClipboardText(text) {
		return errors.New("clipboard text exceeds 64 KiB or is invalid UTF-8")
	}
	conn := ds.clipboardConn.Load()
	if conn == nil || conn != ds.activeConn.Load() || conn.IsClosed() {
		return errors.New("text clipboard is unavailable for this peer")
	}
	payload, _ := json.Marshal(clipboardPayload{Text: text})
	return conn.Send(protocol.Message{Type: protocol.MessageTypeClipboardText, Seq: ds.seqCounter.Add(1), Timestamp: time.Now().UnixNano(), Payload: payload})
}

func (ds *DaemonService) receiveClipboard(conn *transport.Conn, msg protocol.Message) {
	if conn == nil || conn != ds.clipboardConn.Load() || conn != ds.activeConn.Load() || conn.IsClosed() {
		return
	}
	var payload clipboardPayload
	if json.Unmarshal(msg.Payload, &payload) != nil || !validClipboardText(payload.Text) {
		return
	}
	ds.emit(IPCEvent{Event: "clipboard_text", Data: payload})
}

const imageClipboardCapability = "clipboard-image-v1"
const maxClipboardImage = 512 * 1024

type clipboardImagePayload struct {
	PNG string `json:"png"`
}

func validClipboardImage(data string) bool {
	if len(data) > base64.StdEncoding.EncodedLen(maxClipboardImage) {
		return false
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(data)
	if err != nil || len(raw) == 0 || len(raw) > maxClipboardImage {
		return false
	}
	config, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 16000000 {
		return false
	}
	return true
}
func (ds *DaemonService) SendClipboardImage(data string) error {
	if !validClipboardImage(data) {
		return errors.New("clipboard image must be a PNG up to 512 KiB and 16 megapixels")
	}
	conn := ds.imageClipboardConn.Load()
	if conn == nil || conn != ds.activeConn.Load() || conn.IsClosed() {
		return errors.New("image clipboard is unavailable for this peer")
	}
	payload, _ := json.Marshal(clipboardImagePayload{PNG: data})
	return conn.Send(protocol.Message{Type: protocol.MessageTypeClipboardImage, Seq: ds.seqCounter.Add(1), Timestamp: time.Now().UnixNano(), Payload: payload})
}
func (ds *DaemonService) receiveClipboardImage(conn *transport.Conn, msg protocol.Message) {
	if conn == nil || conn != ds.imageClipboardConn.Load() || conn != ds.activeConn.Load() || conn.IsClosed() {
		return
	}
	var payload clipboardImagePayload
	if json.Unmarshal(msg.Payload, &payload) != nil || !validClipboardImage(payload.PNG) {
		return
	}
	ds.emit(IPCEvent{Event: "clipboard_image", Data: payload})
}
