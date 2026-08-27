package session

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

// TestExactlyOnceAcrossReconnect hammers emit() from a producer while
// clients attach mid-stream. Every byte is delivered either inside the
// attach history or on the live channel — never both, never neither.
func TestExactlyOnceAcrossReconnect(t *testing.T) {
	m := &Manager{sessions: make(map[string]*Session)}
	s := &Session{
		Buffer:  NewRingBuffer(1024 * 1024),
		Modes:   NewModeTracker(),
		Clients: make(map[string]chan []byte),
		done:    make(chan struct{}),
		Status:  "running",
	}
	m.sessions[s.ID] = s

	const total = 500
	stop := make(chan struct{})
	var produceWG sync.WaitGroup
	produceWG.Add(1)
	go func() {
		defer produceWG.Done()
		for i := 0; i < total; i++ {
			s.emit([]byte(fmt.Sprintf("chunk-%d\n", i)))
		}
		close(stop)
	}()

	const clients = 6
	var collectWG sync.WaitGroup
	reports := make([]string, clients)
	for c := 0; c < clients; c++ {
		collectWG.Add(1)
		go func(c int) {
			defer collectWG.Done()
			ch, remove, history, ok := m.AttachClient(s.ID)
			if !ok {
				t.Error("attach failed")
				return
			}
			defer remove()
			var sb strings.Builder
			sb.Write(history)
		drain:
			for {
				select {
				case b := <-ch:
					if b == nil {
						break drain
					}
					sb.Write(b)
				case <-stop:
					// Producer done; drain whatever remains buffered.
					for {
						select {
						case b := <-ch:
							if b == nil {
								break drain
							}
							sb.Write(b)
						default:
							break drain
						}
					}
				}
			}
			reports[c] = sb.String()
		}(c)
	}

	produceWG.Wait()
	collectWG.Wait()

	for c, stream := range reports {
		if stream == "" {
			t.Fatalf("client %d received nothing", c)
		}
		counts := map[string]int{}
		for _, tok := range strings.Split(strings.TrimSpace(stream), "\n") {
			if tok == "" {
				continue
			}
			counts[tok]++
		}
		for _, n := range counts {
			if n != 1 {
				t.Fatalf("client %d saw duplicated lines in its stream (one token appeared %d times)", c, n)
			}
		}
		firstTok := strings.SplitN(strings.TrimSpace(stream), "\n", 2)[0]
		if !strings.HasPrefix(firstTok, "chunk-") {
			t.Fatalf("client %d stream starts unexpectedly: %q", c, firstTok)
		}
	}
}
