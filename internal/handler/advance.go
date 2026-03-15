package handler

import (
	"context"
	"encoding/json"
	"learning/internal/quiz"
	"learning/internal/store"
	"log"
	"sync"
	"time"
)

var (
	advanceMu   sync.Mutex
	roomTimers  = struct {
		mu     sync.Mutex
		cancel map[string]context.CancelFunc
	}{cancel: make(map[string]context.CancelFunc)}
)

func (h *Handler) cancelQuestionTimer(roomID string) {
	roomTimers.mu.Lock()
	defer roomTimers.mu.Unlock()
	if cancel, ok := roomTimers.cancel[roomID]; ok {
		cancel()
		delete(roomTimers.cancel, roomID)
	}
}

func (h *Handler) startQuestionTimer(roomID string) {
	ctx, cancel := context.WithCancel(context.Background())
	roomTimers.mu.Lock()
	if old, ok := roomTimers.cancel[roomID]; ok {
		old()
	}
	roomTimers.cancel[roomID] = cancel
	roomTimers.mu.Unlock()

	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(h.cfg.QuestionTimeoutSec) * time.Second):
			h.Advance(roomID)
		}
	}()
}

func (h *Handler) Advance(roomID string) {
	advanceMu.Lock()
	defer advanceMu.Unlock()

	h.cancelQuestionTimer(roomID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	state, err := h.store.GetRoomState(ctx, roomID)
	if err != nil || state != store.RoomStatePlaying {
		return
	}

	current, err := h.store.GetRoomCurrentQuizIndex(ctx, roomID)
	if err != nil || current < 0 {
		current = -1
	}

	nextIndex := current + 1
	questions := quiz.GetDefaultQuestions()
	total := len(questions)
	if total == 0 {
		total = quiz.TotalQuestions
	}

	if nextIndex >= total {
		h.endQuizAndBroadcast(ctx, roomID)
		return
	}

	_ = h.store.SetRoomCurrentQuizIndex(ctx, roomID, nextIndex)
	_ = h.store.SetQuizStartedAt(ctx, roomID, nextIndex)
	if nextIndex < len(questions) {
		data, _ := json.Marshal(map[string]interface{}{
			"type":        WSMsgQuestion,
			"quiz_index":  nextIndex,
			"question":    questions[nextIndex],
		})
		h.hub.BroadcastToRoom(roomID, data)
	}
	h.startQuestionTimer(roomID)
}

func (h *Handler) endQuizAndBroadcast(ctx context.Context, roomID string) {
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
	data, err := json.Marshal(map[string]interface{}{
		"type":             WSMsgQuizEnd,
		WSKeyLeaderboard: result,
	})
	if err != nil {
		log.Printf("marshal quiz_end: %v", err)
		return
	}
	h.hub.BroadcastToRoom(roomID, data)
	go func() {
		time.Sleep(2 * time.Second)
		_ = h.store.CleanupRoomAfterQuiz(context.Background(), roomID)
	}()
}
