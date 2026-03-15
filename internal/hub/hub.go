package hub

import (
	"sync"
)

type InboundMessage struct {
	Client *Client
	Raw    []byte
}

type Hub struct {
	rooms    map[string]*Room
	roomsMu  sync.RWMutex
	register   chan *Client
	unregister chan *Client
	inbound    chan *InboundMessage
}

func NewHub() *Hub {
	return &Hub{
		rooms:      make(map[string]*Room),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		inbound:    make(chan *InboundMessage),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.roomsMu.Lock()
			room, ok := h.rooms[client.RoomID]
			if !ok {
				room = newRoom(client.RoomID)
				h.rooms[client.RoomID] = room
			}
			h.roomsMu.Unlock()
			room.add(client)

		case client := <-h.unregister:
			if onClientLeave != nil {
				onClientLeave(client)
			}
			h.roomsMu.Lock()
			if room, ok := h.rooms[client.RoomID]; ok {
				room.remove(client)
				if room.clientCount() == 0 {
					delete(h.rooms, client.RoomID)
				}
			}
			h.roomsMu.Unlock()
			close(client.send)

		case msg := <-h.inbound:
			if msg != nil && msg.Client != nil {
				h.roomsMu.RLock()
				room, ok := h.rooms[msg.Client.RoomID]
				h.roomsMu.RUnlock()
				if ok {
					h.handleInbound(msg, room)
				}
			}
		}
	}
}

func (h *Hub) handleInbound(msg *InboundMessage, room *Room) {
	if onInbound != nil {
		onInbound(msg, room)
	}
}

var onInbound func(msg *InboundMessage, room *Room)
var onClientLeave func(client *Client)

func SetInboundHandler(fn func(msg *InboundMessage, room *Room)) {
	onInbound = fn
}

func SetClientLeaveHandler(fn func(client *Client)) {
	onClientLeave = fn
}

func (h *Hub) BroadcastToRoom(roomID string, message []byte) {
	h.roomsMu.RLock()
	room, ok := h.rooms[roomID]
	h.roomsMu.RUnlock()
	if ok {
		room.broadcast(message)
	}
}

func (h *Hub) GetRoom(roomID string) *Room {
	h.roomsMu.RLock()
	defer h.roomsMu.RUnlock()
	return h.rooms[roomID]
}

func (h *Hub) Register(c *Client) {
	h.register <- c
}

func (h *Hub) Unregister(c *Client) {
	h.unregister <- c
}
