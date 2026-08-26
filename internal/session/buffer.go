package session

import (
	"bytes"
	"sync"
)

// RingBuffer is a size-capped byte buffer. Writes that would exceed cap
// discard the oldest bytes so the buffer never grows past cap.
type RingBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
	cap int
}

func NewRingBuffer(cap int) *RingBuffer {
	return &RingBuffer{cap: cap}
}

func (r *RingBuffer) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cap <= 0 {
		return r.buf.Write(p)
	}
	if len(p) >= r.cap {
		r.buf.Reset()
		return r.buf.Write(p[len(p)-r.cap:])
	}
	overflow := r.buf.Len() + len(p) - r.cap
	if overflow > 0 {
		b := r.buf.Bytes()
		r.buf.Reset()
		_, _ = r.buf.Write(b[overflow:])
	}
	return r.buf.Write(p)
}

func (r *RingBuffer) Bytes() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := make([]byte, r.buf.Len())
	copy(b, r.buf.Bytes())
	return b
}

func (r *RingBuffer) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.Len()
}

// ReplayBytes returns a copy of the buffer starting at a reasonably safe
// CSI/OSC boundary so a reconnect does not paint a truncated escape sequence.
func (r *RingBuffer) ReplayBytes() []byte {
	b := r.Bytes()
	if len(b) == 0 {
		return b
	}
	if i := safeReplayStart(b); i > 0 && i < len(b) {
		out := make([]byte, len(b)-i)
		copy(out, b[i:])
		return out
	}
	return b
}

func safeReplayStart(b []byte) int {
	window := b
	if len(window) > 8192 {
		window = b[len(b)-8192:]
	}
	offset := len(b) - len(window)
	for _, seq := range [][]byte{
		[]byte("\x1b[2J"),
		[]byte("\x1b[H\x1b[2J"),
		[]byte("\x1bc"),
		[]byte("\x1b[3J"),
	} {
		if i := bytes.LastIndex(window, seq); i >= 0 {
			return offset + i
		}
	}
	if b[0] != 0x1b {
		return 0
	}
	end := scanEscapeEnd(b)
	if end > 0 && end < len(b) {
		return end
	}
	return 0
}

func scanEscapeEnd(b []byte) int {
	if len(b) < 2 || b[0] != 0x1b {
		return 0
	}
	switch b[1] {
	case '[':
		for i := 2; i < len(b); i++ {
			if b[i] >= 0x40 && b[i] <= 0x7e {
				return i + 1
			}
		}
	case ']':
		for i := 2; i < len(b); i++ {
			if b[i] == 0x07 {
				return i + 1
			}
			if b[i] == 0x1b && i+1 < len(b) && b[i+1] == '\\' {
				return i + 2
			}
		}
	case 'P', 'X', '^', '_':
		for i := 2; i < len(b); i++ {
			if b[i] == 0x1b && i+1 < len(b) && b[i+1] == '\\' {
				return i + 2
			}
		}
	default:
		if len(b) >= 2 {
			return 2
		}
	}
	return 0
}
