package quiz

import (
	"context"
	"encoding/json"
	"learning/internal/store"
	"time"
)

const (
	MsgTypeSubmitAnswer = "submit_answer"
)

const (
	ReasonRoomNotPlaying    = "room_not_playing"
	ReasonInvalidQuizIndex  = "invalid_quiz_index"
	ReasonQuizNotStarted   = "quiz_not_started"
	ReasonAnswerTimeExpired = "answer_time_expired"
	ReasonDBError          = "db_error"
	ReasonAlreadyAnswered  = "already_answered"
	ReasonScoreUpdateFailed = "score_update_failed"
)

type Question struct {
	ID       int      `json:"id"`
	Question string   `json:"question"`
	Options  []string `json:"options"`
	Correct  int      `json:"correct"`
}

type QuizState struct {
	CurrentIndex int         `json:"current_index"`
	Questions    []Question  `json:"questions"`
	StartedAt    int64       `json:"started_at,omitempty"`
}

const TotalQuestions = 5

var defaultQuestions = []Question{
	{ID: 0, Question: "1 + 1 = ?", Options: []string{"1", "2", "3", "4"}, Correct: 1},
	{ID: 1, Question: "Thủ đô Việt Nam?", Options: []string{"TP.HCM", "Hà Nội", "Đà Nẵng", "Huế"}, Correct: 1},
	{ID: 2, Question: "2 * 3 = ?", Options: []string{"5", "6", "7", "8"}, Correct: 1},
	{ID: 3, Question: "Số nào chia hết cho 3?", Options: []string{"10", "11", "12", "13"}, Correct: 2},
	{ID: 4, Question: "Con gì có cánh nhưng không bay?", Options: []string{"Chim cánh cụt", "Đà điểu", "Gà", "Cả B và C"}, Correct: 3},
}

func GetDefaultQuestions() []Question {
	return defaultQuestions
}

type SubmitAnswerRequest struct {
	Type       string `json:"type"`
	QuizIndex  int    `json:"quiz_index"`
	AnswerIndex int   `json:"answer_index"`
}

func ParseSubmitAnswer(raw []byte) (*SubmitAnswerRequest, error) {
	var req SubmitAnswerRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	if req.Type != MsgTypeSubmitAnswer {
		return nil, nil
	}
	return &req, nil
}

type AnswerResult struct {
	Accepted bool    `json:"accepted"`
	Reason   string  `json:"reason,omitempty"`
	Score    float64 `json:"score,omitempty"`
}

func ValidateAndScore(ctx context.Context, s *store.Store, roomID, userID string, quizIndex, answerIndex int, answerWindowSec int, pointsPerCorrect float64) (*AnswerResult, error) {
	state, err := s.GetRoomState(ctx, roomID)
	if err != nil || state != store.RoomStatePlaying {
		return &AnswerResult{Accepted: false, Reason: ReasonRoomNotPlaying}, nil
	}
	if quizIndex < 0 || quizIndex >= len(defaultQuestions) {
		return &AnswerResult{Accepted: false, Reason: ReasonInvalidQuizIndex}, nil
	}
	startedAt, err := s.GetQuizStartedAt(ctx, roomID, quizIndex)
	if err != nil {
		return &AnswerResult{Accepted: false, Reason: ReasonQuizNotStarted}, nil
	}
	windowEnd := startedAt + int64(answerWindowSec)
	if time.Now().Unix() > windowEnd {
		return &AnswerResult{Accepted: false, Reason: ReasonAnswerTimeExpired}, nil
	}
	added, err := s.MarkAnswered(ctx, roomID, quizIndex, userID)
	if err != nil {
		return &AnswerResult{Accepted: false, Reason: ReasonDBError}, nil
	}
	if added == 0 {
		return &AnswerResult{Accepted: false, Reason: ReasonAlreadyAnswered}, nil
	}
	q := defaultQuestions[quizIndex]
	if answerIndex != q.Correct {
		return &AnswerResult{Accepted: true, Score: 0}, nil
	}
	newScore, err := s.IncrScore(ctx, roomID, userID, pointsPerCorrect)
	if err != nil {
		return &AnswerResult{Accepted: false, Reason: ReasonScoreUpdateFailed}, nil
	}
	_ = newScore
	return &AnswerResult{Accepted: true, Score: pointsPerCorrect}, nil
}
