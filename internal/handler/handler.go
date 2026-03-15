package handler

import (
	"context"
	"encoding/json"
	"learning/internal/config"
	"learning/internal/hub"
	"learning/internal/quiz"
	"learning/internal/store"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Handler struct {
	cfg   *config.Config
	store *store.Store
	hub   *hub.Hub
}

func New(cfg *config.Config, s *store.Store, h *hub.Hub) *Handler {
	return &Handler{cfg: cfg, store: s, hub: h}
}

type CreateRoomResponse struct {
	RoomID string `json:"room_id"`
}

func (h *Handler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	roomID := uuid.New().String()
	ctx := r.Context()
	if err := h.store.CreateRoom(ctx, roomID, h.cfg.MaxPlayersPerRoom); err != nil {
		http.Error(w, "failed to create room", http.StatusInternalServerError)
		return
	}
	writeJSON(w, CreateRoomResponse{RoomID: roomID})
}

type JoinRoomRequest struct {
	UserID string `json:"user_id"`
	RoomID string `json:"room_id"`
}

type JoinRoomResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func (h *Handler) JoinRoom(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	var req JoinRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, JoinRoomResponse{OK: false, Error: ErrInvalidBody})
		return
	}
	if req.UserID == "" || req.RoomID == "" {
		writeJSON(w, JoinRoomResponse{OK: false, Error: ErrUserAndRoomRequired})
		return
	}
	ctx := r.Context()
	existingRoom, err := h.store.GetUserRoom(ctx, req.UserID)
	if err == nil && existingRoom != "" {
		if existingRoom == req.RoomID {
			writeJSON(w, JoinRoomResponse{OK: true})
			return
		}
		writeJSON(w, JoinRoomResponse{OK: false, Error: ErrUserInOtherRoom})
		return
	}
	exists, err := h.store.RoomExists(ctx, req.RoomID)
	if err != nil || !exists {
		writeJSON(w, JoinRoomResponse{OK: false, Error: ErrRoomNotFound})
		return
	}
	state, err := h.store.GetRoomState(ctx, req.RoomID)
	if err != nil || state != store.RoomStateWaiting {
		writeJSON(w, JoinRoomResponse{OK: false, Error: ErrRoomNotAccepting})
		return
	}
	count, err := h.store.RoomMemberCount(ctx, req.RoomID)
	if err != nil {
		writeJSON(w, JoinRoomResponse{OK: false, Error: ErrDB})
		return
	}
	maxPlayers, _ := h.store.GetRoomMaxPlayers(ctx, req.RoomID)
	if maxPlayers <= 0 {
		maxPlayers = h.cfg.MaxPlayersPerRoom
	}
	if count >= int64(maxPlayers) {
		writeJSON(w, JoinRoomResponse{OK: false, Error: ErrRoomFull})
		return
	}
	if err := h.store.AddRoomMember(ctx, req.RoomID, req.UserID); err != nil {
		writeJSON(w, JoinRoomResponse{OK: false, Error: ErrDB})
		return
	}
	ttl := 24 * time.Hour
	if err := h.store.SetUserRoom(ctx, req.UserID, req.RoomID, ttl); err != nil {
		h.store.RemoveRoomMember(ctx, req.RoomID, req.UserID)
		writeJSON(w, JoinRoomResponse{OK: false, Error: ErrDB})
		return
	}
	writeJSON(w, JoinRoomResponse{OK: true})
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:    func(r *http.Request) bool { return true },
}

func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get(QueryUserID)
	roomID := r.URL.Query().Get(QueryRoomID)
	if userID == "" || roomID == "" {
		http.Error(w, ErrUserAndRoomRequired, 400)
		return
	}
	ctx := r.Context()
	currentRoom, err := h.store.GetUserRoom(ctx, userID)
	if err != nil || currentRoom == "" {
		http.Error(w, ErrJoinRoomFirst, 400)
		return
	}
	if currentRoom != roomID {
		http.Error(w, ErrAlreadyInOtherRoom, 400)
		return
	}
	exists, _ := h.store.RoomExists(ctx, roomID)
	if !exists {
		http.Error(w, ErrRoomNotFound, 404)
		return
	}
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("ws upgrade:", err)
		return
	}
	client := hub.NewClient(h.hub, conn, userID, roomID)
	h.hub.Register(client)
	client.RunPumps()
}

func (h *Handler) StartQuiz(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	roomID := getRoomID(r)
	if roomID == "" {
		http.Error(w, ErrRoomIDRequired, 400)
		return
	}
	ctx := r.Context()
	state, err := h.store.GetRoomState(ctx, roomID)
	if err != nil || state != store.RoomStateWaiting {
		http.Error(w, ErrRoomNotWaiting, 400)
		return
	}
	if err := h.store.SetRoomState(ctx, roomID, store.RoomStatePlaying); err != nil {
		http.Error(w, ErrFailedToStart, 500)
		return
	}
	if err := h.store.InitLeaderboardWithMembers(ctx, roomID); err != nil {
		_ = h.store.SetRoomState(ctx, roomID, store.RoomStateWaiting)
		http.Error(w, ErrFailedToStart, 500)
		return
	}
	if err := h.store.SetQuizStartedAt(ctx, roomID, 0); err != nil {
		_ = h.store.SetRoomState(ctx, roomID, store.RoomStateWaiting)
		http.Error(w, ErrFailedToStart, 500)
		return
	}
	_ = h.store.SetRoomCurrentQuizIndex(ctx, roomID, 0)
	questions := quiz.GetDefaultQuestions()
	if len(questions) > 0 {
		data, _ := json.Marshal(map[string]interface{}{
			"type": WSMsgQuestion,
			"quiz_index": 0,
			"question": questions[0],
		})
		h.hub.BroadcastToRoom(roomID, data)
	}
	h.startQuestionTimer(roomID)
	writeJSON(w, map[string]interface{}{"ok": true, "quiz_index": 0})
}

func (h *Handler) NextQuestion(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	roomID := getRoomID(r)
	if roomID == "" {
		http.Error(w, ErrRoomIDRequired, 400)
		return
	}
	var body struct{ QuizIndex int }
	_ = json.NewDecoder(r.Body).Decode(&body)
	quizIndex := body.QuizIndex
	if quizIndex <= 0 {
		quizIndex = 1
	}
	ctx := r.Context()
	_ = h.store.SetQuizStartedAt(ctx, roomID, quizIndex)
	questions := quiz.GetDefaultQuestions()
	if quizIndex < len(questions) {
		data, _ := json.Marshal(map[string]interface{}{
			"type": WSMsgQuestion,
			"quiz_index": quizIndex,
			"question": questions[quizIndex],
		})
		h.hub.BroadcastToRoom(roomID, data)
	}
	writeJSON(w, map[string]interface{}{"ok": true, "quiz_index": quizIndex})
}

func (h *Handler) EndQuiz(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	roomID := getRoomID(r)
	if roomID == "" {
		http.Error(w, ErrRoomIDRequired, 400)
		return
	}
	h.cancelQuestionTimer(roomID)
	ctx := r.Context()
	_ = h.store.SetRoomState(ctx, roomID, store.RoomStateFinished)
	leaderboard, _ := h.store.GetLeaderboard(ctx, roomID, 100)
	result := make([]map[string]interface{}, 0, len(leaderboard))
	for i, z := range leaderboard {
		result = append(result, map[string]interface{}{
			"rank":  i + 1,
			"user":  z.Member,
			"score": z.Score,
		})
	}
	writeJSON(w, map[string]interface{}{"ok": true, WSKeyLeaderboard: result})
	go func() {
		time.Sleep(2 * time.Second)
		_ = h.store.CleanupRoomAfterQuiz(context.Background(), roomID)
	}()
}

func WireInboundHandler(h *Handler) {
	hub.SetClientLeaveHandler(func(c *hub.Client) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = h.store.RemoveRoomMember(ctx, c.RoomID, c.UserID)
		_ = h.store.DeleteUserRoom(ctx, c.UserID)
	})
	hub.SetInboundHandler(func(msg *hub.InboundMessage, room *hub.Room) {
		req, err := quiz.ParseSubmitAnswer(msg.Raw)
		if err != nil || req == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result, err := quiz.ValidateAndScore(ctx, h.store, msg.Client.RoomID, msg.Client.UserID, req.QuizIndex, req.AnswerIndex, h.cfg.AnswerWindowSec, h.cfg.PointsPerCorrect)
		if err != nil {
			return
		}
		msg.Client.SendJSON(map[string]interface{}{
			"type":   WSMsgAnswerResult,
			WSKeyResult: result,
		})
		_ = h.store.PublishLeaderboardUpdate(ctx, msg.Client.RoomID)
		if result.Accepted {
			members, _ := h.store.RoomMemberCount(ctx, msg.Client.RoomID)
			answered, _ := h.store.GetAnsweredCount(ctx, msg.Client.RoomID, req.QuizIndex)
			if members > 0 && answered >= members {
				h.cancelQuestionTimer(msg.Client.RoomID)
				h.Advance(msg.Client.RoomID)
			}
		}
	})
}

func RunLeaderboardSubscriber(ctx context.Context, s *store.Store, h *hub.Hub) {
	pubsub := s.SubscribeLeaderboardGlobal(ctx)
	defer pubsub.Close()
	_, _ = pubsub.Receive(ctx)
	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok || msg == nil {
				return
			}
			roomID := msg.Payload
			lb, err := s.GetLeaderboard(ctx, roomID, 100)
			if err != nil {
				continue
			}
			result := make([]map[string]interface{}, 0, len(lb))
			for i, z := range lb {
				result = append(result, map[string]interface{}{
					"rank":  i + 1,
					"user":  z.Member,
					"score": z.Score,
				})
			}
			data, _ := json.Marshal(map[string]interface{}{
				"type":             WSMsgLeaderboardUpdate,
				WSKeyLeaderboard: result,
			})
			h.BroadcastToRoom(roomID, data)
		}
	}
}
