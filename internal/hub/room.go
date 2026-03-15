package hub

import (
	"sync"
)

type Room struct {
	ID      string
	clients map[*Client]bool
	mu      sync.RWMutex
}

func newRoom(id string) *Room {
	return &Room{
		ID:      id,
		clients: make(map[*Client]bool),
	}
}

func (r *Room) add(client *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[client] = true
}

func (r *Room) remove(client *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.clients[client]; ok {
		delete(r.clients, client)
	}
}

func (r *Room) broadcast(message []byte) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for client := range r.clients {
		select {
		case client.send <- message:
		default:
			close(client.send)
			delete(r.clients, client)
		}
	}
}

func (r *Room) clientCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clients)
}
