package room

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/google/uuid"
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
	rooms map[string]*Room
	mu    sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		rooms: make(map[string]*Room),
	}
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
		delete(m.rooms, roomID)
		return true
	}
	return false
}
