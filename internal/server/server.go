package server

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
	"github.com/jaypopat/duet/internal/pty"
	"github.com/jaypopat/duet/internal/room"
	"github.com/jaypopat/duet/internal/ui"
)

type Server struct {
	addr        string
	hostKeyPath string
	roomManager *room.Manager
	logger      *log.Logger
}

func New(addr, hostKeyPath string) *Server {
	return &Server{
		addr:        addr,
		hostKeyPath: hostKeyPath,
		roomManager: room.NewManager(),
		logger:      log.NewWithOptions(os.Stderr, log.Options{
			Prefix: "duet",
		}),
	}
}

func (s *Server) Start() error {
	srv, err := wish.NewServer(
		wish.WithAddress(s.addr),
		wish.WithHostKeyPath(s.hostKeyPath),
		wish.WithMiddleware(
			bubbletea.Middleware(s.teaHandler),
			logging.Middleware(),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	s.logger.Info("Starting SSH server", "address", s.addr)
	
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			s.logger.Error("Server error", "error", err)
		}
	}()

	<-done
	s.logger.Info("Shutting down server...")
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}
	
	s.logger.Info("Server stopped")
	return nil
}

func (s *Server) teaHandler(sshSession ssh.Session) (tea.Model, []tea.ProgramOption) {
	// Show menu first
	menu := ui.NewMenuModel()
	
	// Run the menu
	renderer := bubbletea.MakeRenderer(sshSession)
	program := tea.NewProgram(menu, tea.WithInput(sshSession), tea.WithOutput(sshSession), bubbletea.WithRenderer(renderer))
	
	finalModel, err := program.Run()
	if err != nil {
		s.logger.Error("Menu error", "error", err)
		return menu, []tea.ProgramOption{}
	}

	menuModel := finalModel.(ui.MenuModel)
	choice := menuModel.GetChoice()

	switch choice {
	case ui.ChoiceCreate:
		return s.handleCreateRoom(sshSession)
	case ui.ChoiceJoin:
		return s.handleJoinRoom(sshSession, menuModel.GetRoomID())
	default:
		return menu, []tea.ProgramOption{}
	}
}

func (s *Server) handleCreateRoom(sshSession ssh.Session) (tea.Model, []tea.ProgramOption) {
	// Get user's public key
	pubKey := sshSession.PublicKey()
	var pubKeyStr string
	if pubKey != nil {
		pubKeyStr = string(ssh.MarshalAuthorizedKey(pubKey))
	} else {
		pubKeyStr = sshSession.User()
	}

	// Create room
	r, err := s.roomManager.CreateRoom(pubKeyStr)
	if err != nil {
		s.logger.Error("Failed to create room", "error", err)
		return ui.NewMenuModel(), []tea.ProgramOption{}
	}

	s.logger.Info("Room created", "roomID", r.ID, "host", pubKeyStr)

	// Create client
	client := &room.Client{
		ID:      sshSession.User() + "-host",
		Session: sshSession,
		IsHost:  true,
	}
	r.AddClient(client)

	// Create PTY handler and start master
	ptyHandler, err := pty.NewHandler(r)
	if err != nil {
		s.logger.Error("Failed to create PTY handler", "error", err)
		return ui.NewMenuModel(), []tea.ProgramOption{}
	}

	if err := ptyHandler.StartMaster(); err != nil {
		s.logger.Error("Failed to start PTY master", "error", err)
		return ui.NewMenuModel(), []tea.ProgramOption{}
	}

	// Show room ID to host
	fmt.Fprintf(sshSession, "\n🎯 Room created!\n\nRoom ID: %s\n\nShare this ID with your pair partner.\nStarting shared terminal...\n\n", r.ID)
	time.Sleep(2 * time.Second)

	// Handle raw PTY session
	go ui.HandleSession(sshSession, r, client, ptyHandler, true)

	// Return a minimal model - actual interaction is in raw mode
	sessionModel := ui.NewSessionModel(r, client, ptyHandler, true)
	return sessionModel, []tea.ProgramOption{}
}

func (s *Server) handleJoinRoom(sshSession ssh.Session, roomID string) (tea.Model, []tea.ProgramOption) {
	if roomID == "" {
		fmt.Fprintf(sshSession, "No room ID provided\n")
		return ui.NewMenuModel(), []tea.ProgramOption{}
	}

	// Get room
	r, err := s.roomManager.GetRoom(roomID)
	if err != nil {
		s.logger.Error("Room not found", "roomID", roomID)
		fmt.Fprintf(sshSession, "❌ Room %s not found\n", roomID)
		time.Sleep(2 * time.Second)
		return ui.NewMenuModel(), []tea.ProgramOption{}
	}

	s.logger.Info("Client joining room", "roomID", roomID, "user", sshSession.User())

	// Create client
	client := &room.Client{
		ID:      sshSession.User() + "-guest",
		Session: sshSession,
		IsHost:  false,
	}
	r.AddClient(client)

	// Get existing PTY handler (host should have created it)
	ptyHandler, _ := pty.NewHandler(r)

	fmt.Fprintf(sshSession, "\n✅ Joined room: %s\n\nConnecting to shared terminal...\n\n", roomID)
	time.Sleep(2 * time.Second)

	// Handle raw PTY session
	go ui.HandleSession(sshSession, r, client, ptyHandler, false)

	sessionModel := ui.NewSessionModel(r, client, ptyHandler, false)
	return sessionModel, []tea.ProgramOption{}
}

func (s *Server) GetRoomManager() *room.Manager {
	return s.roomManager
}
