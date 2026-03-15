package config

import (
	"os"
	"strconv"
)

type Config struct {
	HTTPAddr            string
	RedisAddr           string
	RedisPassword       string
	RedisDB             int
	MaxPlayersPerRoom   int
	AnswerWindowSec     int
	PointsPerCorrect    float64
	QuestionTimeoutSec  int
}

func Load() *Config {
	redisDB := 0
	if v := os.Getenv("REDIS_DB"); v != "" {
		if d, err := strconv.Atoi(v); err == nil {
			redisDB = d
		}
	}
	maxPlayers := 10
	if v := os.Getenv("MAX_PLAYERS_PER_ROOM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxPlayers = n
		}
	}
	answerWindow := 30
	if v := os.Getenv("ANSWER_WINDOW_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			answerWindow = n
		}
	}
	points := 10.0
	if v := os.Getenv("POINTS_PER_CORRECT"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			points = f
		}
	}
	questionTimeout := 20
	if v := os.Getenv("QUESTION_TIMEOUT_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			questionTimeout = n
		}
	}
	httpAddr := os.Getenv("HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":8080"
	}
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	return &Config{
		HTTPAddr:           httpAddr,
		RedisAddr:          redisAddr,
		RedisPassword:      os.Getenv("REDIS_PASSWORD"),
		RedisDB:            redisDB,
		MaxPlayersPerRoom:  maxPlayers,
		AnswerWindowSec:    answerWindow,
		PointsPerCorrect:   points,
		QuestionTimeoutSec: questionTimeout,
	}
}
