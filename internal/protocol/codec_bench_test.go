package protocol

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/crosskvm/crosskvm/internal/input"
)

func BenchmarkInputEvent_MarshalJSON(b *testing.B) {
	ev := input.NewMouseMoveEvent(12, -8)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		data, err := json.Marshal(ev)
		if err != nil {
			b.Fatal(err)
		}
		if len(data) == 0 {
			b.Fatal("empty json")
		}
	}
}

func BenchmarkInputEvent_UnmarshalJSON(b *testing.B) {
	ev := input.NewMouseMoveEvent(12, -8)
	data, _ := json.Marshal(ev)

	b.ResetTimer()
	b.ReportAllocs()

	var target input.InputEvent
	for i := 0; i < b.N; i++ {
		if err := json.Unmarshal(data, &target); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMessage_MarshalJSON(b *testing.B) {
	ev := input.NewMouseMoveEvent(12, -8)
	msg := NewInputMessage(1, ev)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		data, err := json.Marshal(msg)
		if err != nil {
			b.Fatal(err)
		}
		if len(data) == 0 {
			b.Fatal("empty json")
		}
	}
}

func BenchmarkMessage_UnmarshalJSON(b *testing.B) {
	ev := input.NewMouseMoveEvent(12, -8)
	msg := NewInputMessage(1, ev)
	data, _ := json.Marshal(msg)

	b.ResetTimer()
	b.ReportAllocs()

	var target Message
	for i := 0; i < b.N; i++ {
		if err := json.Unmarshal(data, &target); err != nil {
			b.Fatal(err)
		}
	}
}

// discardWriter is a zero-allocation io.Writer for benchmarking encoders
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func BenchmarkFrame_Encode(b *testing.B) {
	enc := NewEncoder(discardWriter{})
	ev := input.NewMouseMoveEvent(12, -8)
	msg := NewInputMessage(1, ev)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if err := enc.Encode(msg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFrame_Decode(b *testing.B) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	ev := input.NewMouseMoveEvent(12, -8)
	msg := NewInputMessage(1, ev)
	_ = enc.Encode(msg)

	frameBytes := buf.Bytes()

	b.ResetTimer()
	b.ReportAllocs()

	reader := &repeatReader{data: frameBytes}
	dec := NewDecoder(reader)

	for i := 0; i < b.N; i++ {
		_, err := dec.Decode()
		if err != nil {
			b.Fatal(err)
		}
	}
}

type repeatReader struct {
	data []byte
	pos  int
}

func (r *repeatReader) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	copied := 0
	for copied < len(p) {
		avail := len(r.data) - r.pos
		toCopy := len(p) - copied
		if toCopy > avail {
			toCopy = avail
		}
		copy(p[copied:copied+toCopy], r.data[r.pos:r.pos+toCopy])
		copied += toCopy
		r.pos += toCopy
		if r.pos >= len(r.data) {
			r.pos = 0
		}
	}
	return copied, nil
}
