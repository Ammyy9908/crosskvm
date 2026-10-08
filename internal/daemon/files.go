package daemon

import (
	"encoding/json"
	"errors"
	"github.com/crosskvm/crosskvm/internal/protocol"
	"github.com/crosskvm/crosskvm/internal/transport"
	"time"
)

const fileTransferCapability = "file-transfer-v1"

// The desktop process handles disk I/O and acknowledges each chunk only after
// writing it. Keep relay packets bounded so they cannot fill the input stream.
func validFilePacket(data json.RawMessage) bool {
	if len(data) > 96*1024 {
		return false
	}
	var p struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	}
	if json.Unmarshal(data, &p) != nil || len(p.ID) != 36 {
		return false
	}
	switch p.Kind {
	case "begin", "chunk", "end", "ack", "error", "cancel":
		return true
	}
	return false
}
func (ds *DaemonService) SendFilePacket(data json.RawMessage) error {
	if !validFilePacket(data) {
		return errors.New("invalid file transfer packet")
	}
	conn := ds.fileTransferConn.Load()
	if conn == nil || conn != ds.activeConn.Load() || conn.IsClosed() {
		return errors.New("file transfer unavailable; connect updated desktop apps")
	}
	return conn.Send(protocol.Message{Type: protocol.MessageTypeFileTransfer, Seq: ds.seqCounter.Add(1), Timestamp: time.Now().UnixNano(), Payload: data})
}
func (ds *DaemonService) receiveFileTransfer(conn *transport.Conn, msg protocol.Message) {
	if conn == nil || conn != ds.fileTransferConn.Load() || conn != ds.activeConn.Load() || conn.IsClosed() || !validFilePacket(msg.Payload) {
		return
	}
	ds.emit(IPCEvent{Event: "file_packet", Data: msg.Payload})
}
