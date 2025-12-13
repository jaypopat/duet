package ui

import (
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"
	"github.com/jaypopat/duet/internal/pty"
	"github.com/jaypopat/duet/internal/room"
)

type SessionModel struct {
	room      *room.Room
	client    *room.Client
	ptyHandler *pty.Handler
	isHost    bool
	width     int
	height    int
}

type ptyOutputMsg []byte

func NewSessionModel(r *room.Room, c *room.Client, h *pty.Handler, isHost bool) SessionModel {
	return SessionModel{
		room:      r,
		client:    c,
		ptyHandler: h,
		isHost:    isHost,
	}
}

func (m SessionModel) Init() tea.Cmd {
	if m.isHost {
		return tea.Batch(
			listenForPTYOutput(m.ptyHandler),
		)
	}
	return nil
}

func listenForPTYOutput(h *pty.Handler) tea.Cmd {
	return func() tea.Msg {
		// This will be handled by the broadcast goroutine
		return nil
	}
}

func (m SessionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle window resize
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		
		// Forward all key input to PTY
		if m.ptyHandler != nil && m.isHost {
			m.ptyHandler.WriteFromClient([]byte(msg.String()))
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.ptyHandler != nil && m.isHost {
			m.ptyHandler.Resize(uint(msg.Height), uint(msg.Width))
		}

	case ptyOutputMsg:
		// Output already written to session by broadcast goroutine
		return m, nil
	}

	return m, nil
}

func (m SessionModel) View() string {
	if m.isHost {
		return fmt.Sprintf("🎯 Duet Session - Room ID: %s (Host)\nClients: %d\n\nPress Ctrl+C to exit",
			m.room.ID,
			m.room.ClientCount())
	}
	return fmt.Sprintf("🎯 Duet Session - Room ID: %s (Guest)\n\nPress Ctrl+C to exit",
		m.room.ID)
}

// HandleSession manages the raw terminal session for PTY sharing
func HandleSession(s ssh.Session, r *room.Room, c *room.Client, h *pty.Handler, isHost bool) error {
	// Set raw mode
	ptyReq, winCh, isPty := s.Pty()
	if !isPty {
		io.WriteString(s, "PTY not requested\n")
		return fmt.Errorf("pty not requested")
	}

	// Handle window size changes
	go func() {
		for win := range winCh {
			if h != nil && isHost {
				h.Resize(uint(win.Height), uint(win.Width))
			}
		}
	}()

	// Initial resize
	if h != nil && isHost {
		h.Resize(uint(ptyReq.Window.Height), uint(ptyReq.Window.Width))
	}

	// If host, start broadcasting
	if isHost && h != nil {
		go h.BroadcastToClients()

		// Read from session and write to PTY
		buf := make([]byte, 1024)
		for {
			n, err := s.Read(buf)
			if err != nil {
				if err == io.EOF {
					break
				}
				continue
			}
			h.WriteFromClient(buf[:n])
		}
	} else {
		// Guest: just keep session alive and relay input
		buf := make([]byte, 1024)
		for {
			n, err := s.Read(buf)
			if err != nil {
				if err == io.EOF {
					break
				}
				continue
			}
			// Relay to PTY handler
			if h != nil {
				h.WriteFromClient(buf[:n])
			}
		}
	}

	return nil
}
