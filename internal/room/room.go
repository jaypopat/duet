package room

import (
	"sync"

	"github.com/charmbracelet/ssh"
	"github.com/creack/pty"
)

// Client represents a connected user
type Client struct {
	ID      string
	Session ssh.Session
	IsHost  bool
}

// Room represents a pairing session
type Room struct {
	ID          string
	Host        string
	Connections []*Client
	PTYSession  *pty.Pty
	MasterPath  string
	mu          sync.RWMutex
}

// AddClient adds a client to the room
func (r *Room) AddClient(client *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Connections = append(r.Connections, client)
}

// RemoveClient removes a client from the room
func (r *Room) RemoveClient(clientID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, c := range r.Connections {
		if c.ID == clientID {
			r.Connections = append(r.Connections[:i], r.Connections[i+1:]...)
			break
		}
	}
}

// GetClients returns all clients in the room
func (r *Room) GetClients() []*Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	clients := make([]*Client, len(r.Connections))
	copy(clients, r.Connections)
	return clients
}

// ClientCount returns the number of clients
func (r *Room) ClientCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.Connections)
}
