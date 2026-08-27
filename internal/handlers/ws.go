package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"grok-build-webui/internal/auth"
	"grok-build-webui/internal/config"
	"grok-build-webui/internal/db"
	"grok-build-webui/internal/middleware"
	"grok-build-webui/internal/session"
)

type WSHandler struct {
	auth    *auth.Service
	manager *session.Manager
	cfg     *config.Config
	db      *db.DB
}

func NewWSHandler(a *auth.Service, m *session.Manager, cfg *config.Config, database *db.DB) *WSHandler {
	return &WSHandler{auth: a, manager: m, cfg: cfg, db: database}
}

func (h *WSHandler) upgrader() websocket.Upgrader {
	return websocket.Upgrader{
		ReadBufferSize:  8192,
		WriteBufferSize: 8192,
		CheckOrigin: func(r *http.Request) bool {
			return middleware.OriginAllowed(r.Header.Get("Origin"), r, h.cfg, h.db)
		},
	}
}

func (h *WSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if _, err := h.auth.VerifyRequest(r); err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		parts := pathSegments(r.URL.Path)
		for i, p := range parts {
			if p == "sessions" && i+1 < len(parts) {
				id = parts[i+1]
				break
			}
		}
	}
	sess, ok := h.manager.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if colsStr := r.URL.Query().Get("cols"); colsStr != "" {
		if rowsStr := r.URL.Query().Get("rows"); rowsStr != "" {
			if c, err1 := strconv.Atoi(colsStr); err1 == nil {
				if rr, err2 := strconv.Atoi(rowsStr); err2 == nil {
					_ = h.manager.Resize(id, c, rr)
				}
			}
		}
	}

	up := h.upgrader()
	conn, err := up.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade: %v", err)
		return
	}
	defer conn.Close()

	ch, remove, history, ok := h.manager.AttachClient(id)
	if !ok {
		_ = conn.WriteJSON(map[string]string{"type": "error", "error": "session not found"})
		return
	}
	defer remove()

	var writeMu sync.Mutex
	writeJSON := func(v any) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteJSON(v)
	}
	writeBinary := func(data []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteMessage(websocket.BinaryMessage, data)
	}

	if len(history) > 0 {
		_ = writeBinary(history)
	}
	// Tell the client the replay is complete so it can refit and snap the
	// viewport back to live output instead of resting inside stale
	// scrollback ("ghost text").
	_ = writeJSON(map[string]string{"type": "sync"})

	done := make(chan struct{})
	var closeDone sync.Once
	stop := func() { closeDone.Do(func() { close(done) }) }

	go func() {
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case data, ok := <-ch:
				if !ok {
					_ = writeJSON(map[string]any{"type": "exit", "code": sess.ExitCode()})
					stop()
					return
				}
				if data == nil {
					_ = writeJSON(map[string]any{"type": "exit", "code": sess.ExitCode()})
					writeMu.Lock()
					_ = conn.WriteControl(
						websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseNormalClosure, "session exited"),
						time.Now().Add(2*time.Second),
					)
					writeMu.Unlock()
					_ = conn.Close()
					stop()
					return
				}
				if err := writeBinary(data); err != nil {
					stop()
					return
				}
			case <-ticker.C:
				writeMu.Lock()
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				err := conn.WriteMessage(websocket.PingMessage, nil)
				writeMu.Unlock()
				if err != nil {
					stop()
					return
				}
			case <-done:
				return
			}
		}
	}()

	conn.SetReadLimit(1 << 20)
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		mt, message, err := conn.ReadMessage()
		if err != nil {
			break
		}
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		if mt == websocket.BinaryMessage {
			_ = h.manager.Write(id, message)
			continue
		}
		var msg map[string]any
		if err := json.Unmarshal(message, &msg); err == nil {
			t, _ := msg["type"].(string)
			switch t {
			case "data":
				if d, ok := msg["data"].(string); ok {
					_ = h.manager.Write(id, []byte(d))
				}
			case "resize":
				cols := toInt(msg["cols"])
				rows := toInt(msg["rows"])
				if cols > 0 && rows > 0 {
					_ = h.manager.Resize(id, cols, rows)
				}
			case "ping":
				_ = writeJSON(map[string]string{"type": "pong"})
			default:
				_ = h.manager.Write(id, message)
			}
		} else {
			_ = h.manager.Write(id, message)
		}
	}
	stop()
}

func toInt(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return int(x)
	case int64:
		return int(x)
	case json.Number:
		i, _ := x.Int64()
		return int(i)
	case string:
		i, _ := strconv.Atoi(x)
		return i
	default:
		return 0
	}
}
