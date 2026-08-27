package session

import (
	"strings"
	"testing"
)

func prefixModes(t *testing.T, tr *ModeTracker) string {
	t.Helper()
	return string(tr.Prefix())
}

func TestModeTrackerBasicToggle(t *testing.T) {
	tr := NewModeTracker()
	tr.Write([]byte("\x1b[?1000h\x1b[?1006h"))
	if got := prefixModes(t, tr); got != "\x1b[?1000h\x1b[?1006h" {
		t.Fatalf("got %q", got)
	}
	tr.Write([]byte("\x1b[?1000l"))
	if got := prefixModes(t, tr); got != "\x1b[?1000l\x1b[?1006h" {
		t.Fatalf("mouse-off lost: %q", got)
	}
}

func TestModeTrackerLatestWinsPerMode(t *testing.T) {
	tr := NewModeTracker()
	for i := 0; i < 3; i++ {
		tr.Write([]byte("\x1b[?25l\x1b[?25h"))
	}
	if got := prefixModes(t, tr); got != "\x1b[?25h" {
		t.Fatalf("cursor-visibility should collapse to latest: %q", got)
	}
}

// Note on split sequences: the transport may cut a sequence anywhere,
// including between parameter DIGITS ("\x1b[?20"+"04h"). Such a split
// re-joins as a different-but-valid mode number for ANY parser, so it is
// protocol-ambiguous and cannot be recovered from bytes alone. Splits at
// parameter boundaries are lossless, which these tests cover.
func TestModeTrackerSplitAtParameterBoundary(t *testing.T) {
	tr := NewModeTracker()
	tr.Write([]byte("\x1b[?1002;"))
	if got := prefixModes(t, tr); got != "" {
		t.Fatalf("incomplete sequence must not apply: %q", got)
	}
	tr.Write([]byte("1006h"))
	got := prefixModes(t, tr)
	if !strings.Contains(got, "\x1b[?1002h") || !strings.Contains(got, "\x1b[?1006h") {
		t.Fatalf("sequence completed across chunks lost params: %q", got)
	}
}

func TestModeTrackerSplitTerminatorAcrossChunks(t *testing.T) {
	tr := NewModeTracker()
	tr.Write([]byte("\x1b[?1049"))
	tr.Write([]byte("h more output \x1b[31m"))
	if got := prefixModes(t, tr); got != "\x1b[?1049h" {
		t.Fatalf("terminator split lost: %q", got)
	}
}

func TestModeTrackerIgnoresOtherSequences(t *testing.T) {
	tr := NewModeTracker()
	tr.Write([]byte("\x1b[H\x1b[2J\x1b[38;5;196mhi\x1b[0m\x1bc\r\nplain text"))
	if got := prefixModes(t, tr); got != "" {
		t.Fatalf("tracked non-private sequences: %q", got)
	}
}

func TestModeTrackerMultiParamSequence(t *testing.T) {
	tr := NewModeTracker()
	tr.Write([]byte("\x1b[?1002;1006h"))
	got := prefixModes(t, tr)
	if !strings.Contains(got, "\x1b[?1002h") || !strings.Contains(got, "\x1b[?1006h") {
		t.Fatalf("multi-param enable not fully tracked: %q", got)
	}
}

func TestModeTrackerNoChangeKeepsCache(t *testing.T) {
	tr := NewModeTracker()
	tr.Write([]byte("\x1b[?1049h"))
	first := tr.Prefix()
	tr.Write([]byte("payload without modes"))
	if got := tr.Prefix(); &got[0] != &first[0] {
		t.Fatal("unchanged tracker rebuilt cached prefix")
	}
}

func TestAttachReplayComposesModePrefixWithHistory(t *testing.T) {
	// Build the manager by hand: NewManager touches the database, which
	// tests must not depend on.
	m := &Manager{sessions: make(map[string]*Session)}
	s := &Session{
		Buffer:  NewRingBuffer(1024),
		Modes:   NewModeTracker(),
		Clients: make(map[string]chan []byte),
		done:    make(chan struct{}),
		Status:  "running",
	}
	m.sessions[s.ID] = s

	s.emit([]byte("\x1b[?1006hspinner repaint\x1b[2Jlatest frame"))

	ch, remove, history, ok := m.AttachClient(s.ID)
	if !ok {
		t.Fatal("attach failed")
	}
	defer remove()

	want := "\x1b[?1006h\x1b[2Jlatest frame"
	if string(history) != want {
		t.Fatalf("history = %q, want %q", history, want)
	}
	// The live channel must not receive a duplicate of bytes already in
	// the snapshot.
	select {
	case b := <-ch:
		if b != nil && string(b) == "\x1b[2Jlatest frame" {
			t.Fatalf("duplicate delivery of history chunk: %q", b)
		}
	default:
	}
}
