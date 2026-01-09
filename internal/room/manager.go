package room

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/google/uuid"
	"github.com/jaypopat/duet/internal/ai"
)

var (
	ErrRoomNotFound = errors.New("room not found")
)

var adjectives = []string{"swift", "happy", "clever", "brave", "cosmic", "bright", "mystic", "golden"}
var nouns = []string{"phoenix", "dragon", "tiger", "falcon", "wolf", "eagle", "panda", "orca"}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 30 {
		s = s[:30]
	}
	return s
}

type Manager struct {
	rooms     map[string]*Room
	mu        sync.RWMutex
	workerURL string
	aiClient  *ai.Client // Shared across all sessions
	logger    *log.Logger
}

func NewManager(workerURL string, aiClient *ai.Client, logger *log.Logger) *Manager {
	return &Manager{
		rooms:     make(map[string]*Room),
		workerURL: workerURL,
		aiClient:  aiClient,
		logger:    logger,
	}
}

func (m *Manager) GetAIClient() *ai.Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.aiClient
}

func (m *Manager) CreateRoom(host, description string) (*Room, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	roomID := uuid.New().String()

	// Generate workspace name: slugify description or random readable name
	var workspaceName string
	if description != "" {
		workspaceName = slugify(description)
	}
	if workspaceName == "" {
		workspaceName = fmt.Sprintf("%s-%s", adjectives[rand.Intn(len(adjectives))], nouns[rand.Intn(len(nouns))])
	}

	baseDir := "/app/workspaces"
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		baseDir = filepath.Join(os.TempDir(), "duet-workspaces")
	}

	workspaceDir := filepath.Join(baseDir, workspaceName)

	// Copy workspace template for chroot environment
	cmd := exec.Command("cp", "-r", "/app/workspace-template/.", workspaceDir)
	if err := cmd.Run(); err != nil {
		// local dev
		if err := os.MkdirAll(workspaceDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create workspace directory: %w", err)
		}
	}

	room := &Room{
		ID:           roomID,
		Description:  description,
		Host:         host,
		Connections:  make([]*Client, 0),
		WorkspaceDir: workspaceDir,
	}
	m.rooms[roomID] = room
	return room, nil
}
func (m *Manager) GetRoom(roomID string) (*Room, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	room, exists := m.rooms[roomID]
	if !exists {
		return nil, ErrRoomNotFound
	}

	return room, nil
}

func (m *Manager) RoomCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.rooms)
}

func (m *Manager) LeaveRoom(roomID, clientID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	room, exists := m.rooms[roomID]
	if !exists {
		return false
	}

	room.RemoveClient(clientID)

	if room.ClientCount() == 0 {
		if room.Terminal != nil {
			room.Terminal.Close()
			room.Terminal = nil
		}
		// Clean up workspace directory when room is destroyed
		if room.WorkspaceDir != "" {
			os.RemoveAll(room.WorkspaceDir)
		}
		// Cleanup external resources (sandbox, agent state) if worker configured
		if m.workerURL != "" {
			go m.cleanupRoomResources(roomID)
		}
		delete(m.rooms, roomID)
		return true
	}
	return false
}

func (m *Manager) cleanupRoomResources(roomID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	url := fmt.Sprintf("%s/api/rooms/%s", m.workerURL, roomID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		if m.logger != nil {
			m.logger.Warn("failed to create cleanup request", "roomID", roomID, "error", err)
		}
		return
	}
	
	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		if m.logger != nil {
			m.logger.Warn("failed to cleanup room resources", "roomID", roomID, "error", err)
		}
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 400 {
		if m.logger != nil {
			m.logger.Warn("cleanup request failed", "roomID", roomID, "status", resp.StatusCode)
		}
		return
	}
	
	if m.logger != nil {
		m.logger.Info("cleaned up room resources", "roomID", roomID)
	}
}
