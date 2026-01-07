package room

import (
	"sync"

	"github.com/jaypopat/duet/internal/terminal"
)

// RoomEvent represents an event that occurred in a room
type RoomEvent struct {
	Type     string
	Username string
	Data     string
}

type AIMessage struct {
	Role   string `json:"role"`
	UserID string `json:"user_id"`
	Text   string `json:"text"`
	Ts     int64  `json:"ts"`
}

type Client struct {
	ID       string
	Username string
	IsHost   bool
	Events   chan RoomEvent
}

type Room struct {
	ID           string
	Description  string
	Host         string
	Connections  []*Client
	mu           sync.RWMutex
	Terminal     *terminal.Terminal
	AIMessages   []AIMessage
	WorkspaceDir string
}

func (r *Room) AddClient(client *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()

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

	for _, c := range r.Connections {
		if c.ID != client.ID && c.Events != nil {
			select {
			case c.Events <- RoomEvent{Type: "join", Username: client.Username}:
			default:
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
			if c.Events != nil {
				close(c.Events)
			}
			r.Connections = remove(r.Connections, i)
			break
		}
	}

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

func (r *Room) BroadcastEvent(event RoomEvent, excludeClientID string) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, c := range r.Connections {
		if c.ID != excludeClientID && c.Events != nil {
			select {
			case c.Events <- event:
			default:
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

func (r *Room) SetAIMessages(msgs []AIMessage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.AIMessages = msgs
}

func (r *Room) GetAIMessages() []AIMessage {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]AIMessage, len(r.AIMessages))
	copy(result, r.AIMessages)
	return result
}
