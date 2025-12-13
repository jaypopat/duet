package room

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
)

var (
	ErrRoomNotFound = errors.New("room not found")
	ErrRoomExists   = errors.New("room already exists")
)

// Manager manages all active rooms
type Manager struct {
	rooms map[string]*Room
	mu    sync.RWMutex
}

// NewManager creates a new room manager
func NewManager() *Manager {
	return &Manager{
		rooms: make(map[string]*Room),
	}
}

// CreateRoom creates a new room with a unique ID
func (m *Manager) CreateRoom(hostPubKey string) (*Room, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	roomID := generateRoomID()
	
	if _, exists := m.rooms[roomID]; exists {
		return nil, ErrRoomExists
	}

	room := &Room{
		ID:          roomID,
		Host:        hostPubKey,
		Connections: make([]*Client, 0),
	}
	
	m.rooms[roomID] = room
	return room, nil
}

// GetRoom retrieves a room by ID
func (m *Manager) GetRoom(roomID string) (*Room, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	room, exists := m.rooms[roomID]
	if !exists {
		return nil, ErrRoomNotFound
	}
	
	return room, nil
}

// DeleteRoom removes a room
func (m *Manager) DeleteRoom(roomID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rooms, roomID)
}

// ListRooms returns all room IDs
func (m *Manager) ListRooms() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	ids := make([]string, 0, len(m.rooms))
	for id := range m.rooms {
		ids = append(ids, id)
	}
	return ids
}

// RoomCount returns the number of active rooms
func (m *Manager) RoomCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.rooms)
}

// generateRoomID generates a random 8-character room ID
func generateRoomID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}
