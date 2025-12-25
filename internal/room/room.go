package room

import (
	"sync"

	"github.com/jaypopat/duet/internal/terminal"
)

// RoomEvent represents an event that occurred in a room
type RoomEvent struct {
	Type     string // "join", "leave", "typing"
	Username string
	Data     string
}

// represents a connected user
type Client struct {
	ID       string
	Username string
	IsHost   bool
	Events   chan RoomEvent // Channel to receive room events
}

// represents a pairing session
type Room struct {
	ID          string
	Description string
	Host        string
	Connections []*Client
	mu          sync.RWMutex

	// Shared terminal - using v10x for this
	Terminal *terminal.Terminal
}

type RoomMetadata struct {
	ID          string // uuid
	Description string // user provides description on room creation
}

func (r *Room) AddClient(client *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// remove existing client with same ID if present (to handle reconnections)
	for i, c := range r.Connections {
		if c.ID == client.ID {
			if c.Events != nil {
				close(c.Events)
			}
			r.Connections = remove(r.Connections, i)
			break
		}
	}

	r.Connections = append(r.Connections, client)

	// Notify all other clients
	for _, c := range r.Connections {
		if c.ID != client.ID && c.Events != nil {
			select {
			case c.Events <- RoomEvent{Type: "join", Username: client.Username}:
			default:
				// when we push more events than the channel buffer can hold, we drop events to avoid blocking
			}
		}
	}
}

func (r *Room) RemoveClient(clientID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var removedUsername string
	for i, c := range r.Connections {
		if c.ID == clientID {
			removedUsername = c.Username
			// Close the events channel
			if c.Events != nil {
				close(c.Events)
			}
			r.Connections = remove(r.Connections, i)
			break
		}
	}

	// Notify remaining clients
	if removedUsername != "" {
		for _, c := range r.Connections {
			if c.Events != nil {
				select {
				case c.Events <- RoomEvent{Type: "leave", Username: removedUsername}:
				default:
				}
			}
		}
	}
}

// BroadcastEvent sends an event to all clients in the room (Generic implementation)
func (r *Room) BroadcastEvent(event RoomEvent, excludeClientID string) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, c := range r.Connections {
		if c.ID != excludeClientID && c.Events != nil {
			select {
			case c.Events <- event:
			default:
				// when we push more events than the channel buffer can hold, we drop events to avoid blocking
			}
		}
	}
}

// https://stackoverflow.com/questions/37334119/how-to-delete-an-element-from-a-slice-in-golang
func remove(s []*Client, i int) []*Client {
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}

func (r *Room) GetClients() []*Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	clients := make([]*Client, len(r.Connections))
	copy(clients, r.Connections)
	return clients
}

func (r *Room) ClientCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.Connections)
}
