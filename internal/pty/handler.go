package pty

import (
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
	"github.com/jaypopat/duet/internal/room"
)

// Handler manages PTY sessions and broadcasting
type Handler struct {
	room *room.Room
	ptmx *os.File
	mu   sync.Mutex
}

// NewHandler creates a new PTY handler
func NewHandler(r *room.Room) (*Handler, error) {
	return &Handler{
		room: r,
	}, nil
}

// StartMaster starts the master PTY with a shell
func (h *Handler) StartMaster() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Start a shell
	cmd := exec.Command(os.Getenv("SHELL"))
	if cmd.Path == "" {
		cmd = exec.Command("/bin/bash")
	}

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return err
	}

	h.ptmx = ptmx
	h.room.MasterPath = ptmx.Name()

	return nil
}

// BroadcastToClients broadcasts PTY output to all connected clients
func (h *Handler) BroadcastToClients() {
	if h.ptmx == nil {
		return
	}

	buf := make([]byte, 1024)
	for {
		n, err := h.ptmx.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			continue
		}

		clients := h.room.GetClients()
		for _, client := range clients {
			if client.Session != nil {
				client.Session.Write(buf[:n])
			}
		}
	}
}

// WriteFromClient writes input from a client to the master PTY
func (h *Handler) WriteFromClient(data []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.ptmx == nil {
		return nil
	}

	_, err := h.ptmx.Write(data)
	return err
}

// Close closes the PTY
func (h *Handler) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.ptmx != nil {
		return h.ptmx.Close()
	}
	return nil
}

// Resize resizes the PTY
func (h *Handler) Resize(rows, cols uint) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.ptmx != nil {
		return pty.Setsize(h.ptmx, &pty.Winsize{
			Rows: uint16(rows),
			Cols: uint16(cols),
		})
	}
	return nil
}
