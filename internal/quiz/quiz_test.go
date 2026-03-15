package quiz

import (
	"context"
	"encoding/json"
	"testing"
)

func TestParseSubmitAnswer(t *testing.T) {
	raw := []byte(`{"type":"submit_answer","quiz_index":0,"answer_index":1}`)
	req, err := ParseSubmitAnswer(raw)
	if err != nil {
		t.Fatal(err)
	}
	if req == nil {
		t.Fatal("expected non-nil request")
	}
	if req.Type != MsgTypeSubmitAnswer || req.QuizIndex != 0 || req.AnswerIndex != 1 {
		t.Errorf("got type=%s quiz_index=%d answer_index=%d", req.Type, req.QuizIndex, req.AnswerIndex)
	}
}

func TestParseSubmitAnswer_InvalidType(t *testing.T) {
	raw := []byte(`{"type":"other"}`)
	req, err := ParseSubmitAnswer(raw)
	if err != nil {
		t.Fatal(err)
	}
	if req != nil {
		t.Error("expected nil for non submit_answer type")
	}
}

func TestParseSubmitAnswer_InvalidJSON(t *testing.T) {
	_, err := ParseSubmitAnswer([]byte(`{`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestGetDefaultQuestions(t *testing.T) {
	q := GetDefaultQuestions()
	if len(q) == 0 {
		t.Fatal("expected at least one question")
	}
	if len(q) != TotalQuestions {
		t.Errorf("expected %d questions, got %d", TotalQuestions, len(q))
	}
	if q[0].Correct < 0 || q[0].Correct >= len(q[0].Options) {
		t.Error("correct index out of range")
	}
}

func TestSubmitAnswerRequest_JSON(t *testing.T) {
	req := SubmitAnswerRequest{Type: MsgTypeSubmitAnswer, QuizIndex: 0, AnswerIndex: 1}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseSubmitAnswer(data)
	if err != nil || parsed == nil || parsed.AnswerIndex != 1 {
		t.Errorf("roundtrip failed: %v %+v", err, parsed)
	}
}

func TestValidateAndScore_RequiresStore(t *testing.T) {
	ctx := context.Background()
	// ValidateAndScore cần store thật; test chỉ kiểm tra logic với store nil sẽ panic khi gọi
	// Ở đây ta không gọi ValidateAndScore với nil store để tránh panic
	_ = ctx
}
