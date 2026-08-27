package session

import (
	"bytes"
	"sort"
	"strings"
	"sync"
)

// ModeTracker watches the raw PTY output stream for DEC private-mode
// sequences ("ESC [ ? Pm... h/l") and remembers the latest state of each
// mode: mouse tracking (1000/1002/1003/1006), bracketed paste (2004),
// focus reporting (1004), alternate screen (1049), application cursor
// keys (1), cursor visibility (25) and friends.
//
// Why this exists: when a browser reconnects (page refresh, tab restore)
// the session's scrollback ring is replayed into a fresh xterm.js. The
// replay often starts at a truncated boundary (see ReplayBytes) or after
// the oldest bytes were evicted from the ring — exactly where the
// application emitted its one-time mode setup. The client terminal then
// ends up with, say, mouse tracking disabled while the CLI on the PTY
// still expects SGR mouse reports: clicks silently do nothing until the
// session is restarted. Replaying the tracked final state before the
// history keeps both sides in agreement.
type ModeTracker struct {
	mu      sync.Mutex
	states  map[int]bool // DEC private mode number -> enabled
	order   []int        // stable emission order of first-seen modes
	pending []byte       // tail of a sequence split across Write chunks

	prefix []byte // cached rendering, invalidated on change
}

func NewModeTracker() *ModeTracker {
	return &ModeTracker{states: make(map[int]bool)}
}

// Write feeds another chunk of PTY output through the scanner.
// It tolerates escape sequences split across chunk boundaries.
func (t *ModeTracker) Write(p []byte) {
	if len(p) == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	data := p
	if len(t.pending) > 0 {
		data = append(t.pending, p...)
	}
	changed := t.scan(data)
	// Carry over an unterminated trailing escape prefix so a sequence split
	// across writes is completed by the next chunk.
	if keep := trailingEscapePrefix(data); keep > 0 {
		t.pending = append(t.pending[:0], data[len(data)-keep:]...)
	} else {
		t.pending = t.pending[:0]
	}
	if changed {
		t.prefix = nil
	}
}

// scan finds complete "ESC [ ? params [hl]" sequences in data and updates
// the tracked state. Returns true when any state changed.
func (t *ModeTracker) scan(data []byte) bool {
	changed := false
	i := 0
	for i < len(data) {
		j := bytes.IndexByte(data[i:], 0x1b)
		if j < 0 {
			break
		}
		start := i + j
		toggles, end := parseDECPrivateMode(data[start:])
		if len(toggles) > 0 {
			for _, m := range toggles {
				if old, seen := t.states[m.mode]; !seen || old != m.enable {
					changed = true
				}
				if _, seen := t.states[m.mode]; !seen {
					t.order = append(t.order, m.mode)
				}
				t.states[m.mode] = m.enable
			}
			i = start + end
			continue
		}
		// Not a private-mode sequence; skip past the introducer so nested
		// ESC bytes inside longer sequences are still examined individually.
		i = start + 1
	}
	return changed
}

type modeToggle struct {
	mode   int
	enable bool
}

// parseDECPrivateMode recognizes "\x1b[?p1;p2;...h" / "...l", the form the
// terminal uses for private modes. Every listed mode takes the action, e.g.
// "\x1b[?1002;1006h" enables both 1002 and 1006. Returns the toggles and the
// number of bytes consumed, or (nil, 0) when b does not (yet) hold a
// complete such sequence.
func parseDECPrivateMode(b []byte) ([]modeToggle, int) {
	if len(b) < 5 || b[0] != 0x1b || b[1] != '[' || b[2] != '?' {
		return nil, 0
	}
	end := -1
	for i := 3; i < len(b); i++ {
		c := b[i]
		if c == 'h' || c == 'l' {
			end = i + 1
			break // parameters end here; do not swallow what follows
		}
		if !(c >= '0' && c <= '9') && c != ';' {
			return nil, 0 // different CSI sequence
		}
	}
	if end < 0 {
		return nil, 0 // incomplete: waits for more bytes
	}
	var toggles []modeToggle
	cur := 0
	have := false
	enable := b[end-1] == 'h'
	for _, c := range b[3 : end-1] {
		if c == ';' {
			if have {
				toggles = append(toggles, modeToggle{mode: cur, enable: enable})
			}
			cur, have = 0, false
			continue
		}
		cur = cur*10 + int(c-'0')
		have = true
	}
	if have {
		toggles = append(toggles, modeToggle{mode: cur, enable: enable})
	}
	if len(toggles) == 0 {
		return nil, 0
	}
	return toggles, end
}

// Prefix renders the replayable state preamble: every tracked mode set to
// its latest observed value. The result is cached between changes.
func (t *ModeTracker) Prefix() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.prefix != nil {
		return t.prefix
	}
	if len(t.states) == 0 {
		t.prefix = []byte{}
		return t.prefix
	}
	nums := make([]int, len(t.order))
	copy(nums, t.order)
	sort.Ints(nums)
	var sb strings.Builder
	for _, m := range nums {
		sb.WriteString("\x1b[?")
		sb.WriteString(itoa(m))
		if t.states[m] {
			sb.WriteByte('h')
		} else {
			sb.WriteByte('l')
		}
	}
	t.prefix = []byte(sb.String())
	return t.prefix
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// trailingEscapePrefix reports how many trailing bytes of data may be the
// start of an unfinished "ESC [ ? ..." sequence. Only that suffix must be
// carried into the next scan; everything else can be dropped. When several
// lengths qualify, the longest wins so parameter bytes split across chunks
// survive.
func trailingEscapePrefix(data []byte) int {
	const maxWindow = 20 // '[', '?', plus generous room for parameters
	limit := len(data)
	if limit > maxWindow {
		limit = maxWindow
	}
	window := data[len(data)-limit:]
	for size := len(window); size >= 2; size-- {
		tail := window[len(window)-size:]
		if tail[0] == 0x1b && looksLikeUnfinishedDECPrivateMode(tail) {
			return size
		}
	}
	if len(window) > 0 && window[len(window)-1] == 0x1b {
		return 1
	}
	return 0
}

func looksLikeUnfinishedDECPrivateMode(tail []byte) bool {
	if len(tail) < 1 || tail[0] != 0x1b {
		return false
	}
	if len(tail) == 1 {
		return true
	}
	if tail[1] != '[' {
		return false
	}
	if len(tail) == 2 {
		return true
	}
	if tail[2] != '?' {
		return false
	}
	for _, c := range tail[3:] {
		if !(c >= '0' && c <= '9') && c != ';' {
			return false
		}
	}
	return true
}
