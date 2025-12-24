package room

import (
	"errors"
	"sync"

	"github.com/google/uuid"
)

var (
	ErrRoomNotFound = errors.New("room not found")
	ErrRoomExists   = errors.New("room already exists")
)

type Manager struct {
	rooms    map[string]*Room
	metadata map[string]*RoomMetadata // Room metadata to show in active rooms list (DEV)
	mu       sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		rooms:    make(map[string]*Room),
		metadata: make(map[string]*RoomMetadata),
	}
}

// CreateRoom creates a new room with a generated UUID
func (m *Manager) CreateRoom(host, description string) (*Room, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	roomID := uuid.New().String()

	room := &Room{
		ID:          roomID,
		Description: description,
		Host:        host,
		Connections: make([]*Client, 0),
	}

	m.rooms[roomID] = room

	// Store metadata for display in active rooms list
	m.metadata[roomID] = &RoomMetadata{
		ID:          roomID,
		Description: description,
	}

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

func (m *Manager) ListActiveRooms() []*RoomMetadata {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*RoomMetadata
	for id := range m.rooms {
		if meta, exists := m.metadata[id]; exists {
			result = append(result, meta)
		}
	}
	return result
}
