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
	"github.com/jaypopat/duet/internal/ai"
	"github.com/jaypopat/duet/internal/room"
	"github.com/jaypopat/duet/internal/ui"
	"github.com/muesli/termenv"
)

type Server struct {
	addr        string
	hostKeyPath string
	roomManager *room.Manager
	logger      *log.Logger
}

func New(addr, hostKeyPath, workerURL string) *Server {
	logger := log.NewWithOptions(os.Stderr, log.Options{
		Prefix: "duet",
	})

	var aiClient *ai.Client
	if workerURL != "" {
		aiClient = ai.NewClient(workerURL)
	}

	mgr := room.NewManager(workerURL, aiClient, logger)

	return &Server{
		addr:        addr,
		hostKeyPath: hostKeyPath,
		roomManager: mgr,
		logger:      logger,
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		s.logger.Info("Starting SSH server", "address", s.addr)
		if err := srv.ListenAndServe(); err != nil {
			s.logger.Error("Server error", "error", err)
		}
	}()

	<-ctx.Done()

	s.logger.Info("Shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return srv.Shutdown(shutdownCtx)
}

func (s *Server) teaHandler(sess ssh.Session) (tea.Model, []tea.ProgramOption) {
	username := sess.User()
	if username == "" {
		username = "guest"
	}
	renderer := bubbletea.MakeRenderer(sess)

	pty, _, _ := sess.Pty()

	if pty.Term == "xterm-ghostty" {
		renderer.SetColorProfile(termenv.TrueColor)
	}

	s.logger.Info("final renderer",
		"profile", renderer.ColorProfile(),
		"hasDark", renderer.HasDarkBackground(),
	)
	return ui.New(renderer, s.roomManager, username), []tea.ProgramOption{
		tea.WithAltScreen(),
	}
}
