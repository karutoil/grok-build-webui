package session

import "testing"

func TestRingBufferCapsAndKeepsNewest(t *testing.T) {
	r := NewRingBuffer(8)
	if _, err := r.Write([]byte("abcdef")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Write([]byte("ghij")); err != nil {
		t.Fatal(err)
	}
	got := string(r.Bytes())
	if got != "cdefghij" {
		t.Fatalf("got %q want %q", got, "cdefghij")
	}
	if r.Len() != 8 {
		t.Fatalf("len %d want 8", r.Len())
	}
}

func TestRingBufferWriteLargerThanCap(t *testing.T) {
	r := NewRingBuffer(4)
	if _, err := r.Write([]byte("hello-world")); err != nil {
		t.Fatal(err)
	}
	got := string(r.Bytes())
	if got != "orld" {
		t.Fatalf("got %q want %q", got, "orld")
	}
}

func TestRingBufferExactCap(t *testing.T) {
	r := NewRingBuffer(4)
	if _, err := r.Write([]byte("abcd")); err != nil {
		t.Fatal(err)
	}
	if got := string(r.Bytes()); got != "abcd" {
		t.Fatalf("got %q", got)
	}
}

func TestReplayBytesSkipsTruncatedCSI(t *testing.T) {
	r := NewRingBuffer(64)
	_, _ = r.Write([]byte("\x1b[31mred"))
	got := string(r.ReplayBytes())
	if got != "red" {
		t.Fatalf("got %q want red", got)
	}
}

func TestReplayBytesPrefersClearScreen(t *testing.T) {
	r := NewRingBuffer(64)
	_, _ = r.Write([]byte("junk\x1b[2Jhello"))
	got := string(r.ReplayBytes())
	if got != "\x1b[2Jhello" {
		t.Fatalf("got %q", got)
	}
}
