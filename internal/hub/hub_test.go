package hub

import (
	"sync"
	"testing"
)

func TestNewHub(t *testing.T) {
	h := NewHub()
	if h == nil {
		t.Fatal("NewHub() returned nil")
	}
	if h.rooms == nil {
		t.Error("rooms map not initialized")
	}
}

func TestNewRoom(t *testing.T) {
	r := newRoom("room-1")
	if r.ID != "room-1" {
		t.Errorf("room id = %s", r.ID)
	}
	if r.clientCount() != 0 {
		t.Error("new room should have 0 clients")
	}
}

func TestRoomAddRemove(t *testing.T) {
	r := newRoom("room-1")
	c := &Client{RoomID: "room-1", UserID: "u1"}
	r.add(c)
	if r.clientCount() != 1 {
		t.Errorf("client count = %d", r.clientCount())
	}
	r.remove(c)
	if r.clientCount() != 0 {
		t.Errorf("after remove count = %d", r.clientCount())
	}
}

func TestHubRunRegisterUnregister(t *testing.T) {
	h := NewHub()
	go h.Run()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c := &Client{hub: h, RoomID: "r1", UserID: "u1", send: make(chan []byte, 1)}
		h.Register(c)
		h.Unregister(c)
	}()
	wg.Wait()
}
